// Package eurobase implements store.Store over the Eurobase data API.
//
// Single rows (runs, steps, feed cards, …) are written one per REST call, so
// the app gets realtime events for them. Card piles go through the
// insert_cards RPC in chunks under the 1 MB request limit.
package eurobase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"

	eb "github.com/STGime/laamo/backend/internal/eurobase"
	"github.com/STGime/laamo/backend/internal/store"
)

type Store struct {
	c *eb.Client
	// chunk limits for insert_cards (tests lower them)
	maxChunkBytes int
	maxChunkRows  int
}

var _ store.Store = (*Store)(nil)

func New(c *eb.Client) *Store {
	return &Store{c: c, maxChunkBytes: eb.MaxBody - 1024, maxChunkRows: 500}
}

// SetChunkLimits overrides the insert_cards chunk size (tests).
func (s *Store) SetChunkLimits(maxBytes, maxRows int) {
	s.maxChunkBytes, s.maxChunkRows = maxBytes, maxRows
}

// ---- workspaces, helpers ----

func (s *Store) GetWorkspace(ctx context.Context, id string) (store.Workspace, error) {
	return getRow[store.Workspace](ctx, s, "workspaces", id)
}

func (s *Store) GetWorkspaces(ctx context.Context, ids []string) ([]store.Workspace, error) {
	var out []store.Workspace
	for chunk := range slices.Chunk(ids, 100) {
		rows, err := listRows[store.Workspace](ctx, s, "workspaces", eb.NewQuery().In("id", chunk...), 0)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	return out, nil
}

// CreateWorkspace calls create_workspace (migrations/0008): workspace,
// owner membership and included brain in one transaction.
func (s *Store) CreateWorkspace(ctx context.Context, w store.NewWorkspace) (store.Workspace, error) {
	args := map[string]any{"owner": w.OwnerID, "name": w.Name}
	if w.Timezone != "" {
		args["timezone"] = w.Timezone
	}
	if w.Locale != "" {
		args["locale"] = w.Locale
	}
	var out store.Workspace
	if err := s.c.RPC(ctx, "create_workspace", args, &out); err != nil {
		return store.Workspace{}, mapErr(err)
	}
	return out, nil
}

func (s *Store) UpdateWorkspace(ctx context.Context, id string, p store.WorkspacePatch) (store.Workspace, error) {
	return updateRow[store.Workspace](ctx, s, "workspaces", id, p)
}

func (s *Store) Memberships(ctx context.Context, userID string) ([]store.Member, error) {
	q := eb.NewQuery().Eq("user_id", userID).Order("created_at", false).Order("id", false)
	return listRows[store.Member](ctx, s, "members", q, 0)
}

func (s *Store) Membership(ctx context.Context, workspaceID, userID string) (store.Member, error) {
	q := eb.NewQuery().Eq("workspace_id", workspaceID).Eq("user_id", userID)
	rows, err := listRows[store.Member](ctx, s, "members", q, 1)
	if err != nil {
		return store.Member{}, err
	}
	if len(rows) == 0 {
		return store.Member{}, store.ErrNotFound
	}
	return rows[0], nil
}

func (s *Store) GetHelper(ctx context.Context, id string) (store.Helper, error) {
	return getRow[store.Helper](ctx, s, "helpers", id)
}

func (s *Store) GetHelperVersion(ctx context.Context, id string) (store.HelperVersion, error) {
	return getRow[store.HelperVersion](ctx, s, "helper_versions", id)
}

// ---- runs and steps ----

func (s *Store) CreateRun(ctx context.Context, r *store.Run) error {
	return insertRow(ctx, s, "runs", &r.ID, r)
}

func (s *Store) GetRun(ctx context.Context, id string) (store.Run, error) {
	return getRow[store.Run](ctx, s, "runs", id)
}

func (s *Store) UpdateRun(ctx context.Context, id string, p store.RunPatch) (store.Run, error) {
	return updateRow[store.Run](ctx, s, "runs", id, p)
}

func (s *Store) ListRuns(ctx context.Context, f store.RunFilter) ([]store.Run, error) {
	q := eb.NewQuery().Order("created_at", true).Order("id", true)
	if f.WorkspaceID != "" {
		q.Eq("workspace_id", f.WorkspaceID)
	}
	if f.HelperID != "" {
		q.Eq("helper_id", f.HelperID)
	}
	if f.Status != "" {
		q.Eq("status", f.Status)
	}
	return listRows[store.Run](ctx, s, "runs", q, f.Limit)
}

func (s *Store) CreateStep(ctx context.Context, st *store.Step) error {
	return insertRow(ctx, s, "run_steps", &st.ID, st)
}

func (s *Store) UpdateStep(ctx context.Context, id string, p store.StepPatch) (store.Step, error) {
	return updateRow[store.Step](ctx, s, "run_steps", id, p)
}

func (s *Store) ListSteps(ctx context.Context, runID string) ([]store.Step, error) {
	q := eb.NewQuery().Eq("run_id", runID).Order("position", false).Order("attempt", false)
	return listRows[store.Step](ctx, s, "run_steps", q, 0)
}

// ---- cards ----

func (s *Store) InsertCards(ctx context.Context, cards []store.Card) error {
	if len(cards) == 0 {
		return nil
	}
	for i := range cards {
		if cards[i].ID == "" {
			cards[i].ID = store.NewID()
		}
	}
	// Encode each card once, then pack them into chunks.
	encoded := make([]json.RawMessage, len(cards))
	for i := range cards {
		b, err := json.Marshal(cards[i])
		if err != nil {
			return fmt.Errorf("store: encode card %d: %w", i, err)
		}
		if len(b) > s.maxChunkBytes {
			return fmt.Errorf("store: card %d is %d bytes: %w", i, len(b), store.ErrTooLarge)
		}
		encoded[i] = b
	}
	start, size := 0, 0
	for i, b := range encoded {
		if i > start && (size+len(b)+1 > s.maxChunkBytes || i-start >= s.maxChunkRows) {
			if err := s.insertChunk(ctx, cards[start].ID, encoded[start:i]); err != nil {
				return err
			}
			start, size = i, 0
		}
		size += len(b) + 1
	}
	return s.insertChunk(ctx, cards[start].ID, encoded[start:])
}

// insertChunk calls insert_cards for one chunk. If the outcome is unknown
// (timeout, 502), the chunk's first card tells: insert_cards is one
// transaction, so either all cards of the chunk exist or none do.
func (s *Store) insertChunk(ctx context.Context, firstID string, rows []json.RawMessage) error {
	const tries = 3
	var err error
	for range tries {
		err = s.c.RPC(ctx, "insert_cards", map[string]any{"rows": rows}, nil)
		if err == nil || !errors.Is(err, eb.ErrUncertain) {
			return mapErr(err)
		}
		var probe struct {
			ID string `json:"id"`
		}
		switch getErr := s.c.Get(ctx, "cards", firstID, &probe); {
		case getErr == nil:
			return nil // the uncertain call went through
		case errors.Is(getErr, eb.ErrNotFound):
			continue // it didn't; try again
		default:
			return fmt.Errorf("store: insert_cards outcome unknown: %w (check failed: %w)", err, getErr)
		}
	}
	return mapErr(err)
}

func (s *Store) ListCards(ctx context.Context, stepID string) ([]store.Card, error) {
	q := eb.NewQuery().Eq("step_id", stepID).Order("position", false).Order("id", false)
	return listRows[store.Card](ctx, s, "cards", q, 0)
}

// ---- feed ----

func (s *Store) CreateFeedCard(ctx context.Context, c *store.FeedCard) error {
	return insertRow(ctx, s, "feed_cards", &c.ID, c)
}

func (s *Store) GetFeedCard(ctx context.Context, id string) (store.FeedCard, error) {
	return getRow[store.FeedCard](ctx, s, "feed_cards", id)
}

func (s *Store) UpdateFeedCard(ctx context.Context, id string, p store.FeedCardPatch) (store.FeedCard, error) {
	return updateRow[store.FeedCard](ctx, s, "feed_cards", id, p)
}

// ---- lists ----

func (s *Store) CreateList(ctx context.Context, l *store.List) error {
	return insertRow(ctx, s, "lists", &l.ID, l)
}

func (s *Store) GetListByName(ctx context.Context, workspaceID, name string) (store.List, error) {
	q := eb.NewQuery().Eq("workspace_id", workspaceID).Eq("name", name)
	rows, err := listRows[store.List](ctx, s, "lists", q, 1)
	if err != nil {
		return store.List{}, err
	}
	if len(rows) == 0 {
		return store.List{}, store.ErrNotFound
	}
	return rows[0], nil
}

func (s *Store) AddListRow(ctx context.Context, r *store.ListRow) error {
	return insertRow(ctx, s, "list_rows", &r.ID, r)
}

func (s *Store) ListRows(ctx context.Context, listID string, limit int) ([]store.ListRow, error) {
	q := eb.NewQuery().Eq("list_id", listID).Order("created_at", true).Order("id", true)
	return listRows[store.ListRow](ctx, s, "list_rows", q, limit)
}

// ---- brains ----

func (s *Store) GetBrain(ctx context.Context, workspaceID string) (store.Brain, error) {
	rows, err := listRows[store.Brain](ctx, s, "brains", eb.NewQuery().Eq("workspace_id", workspaceID), 1)
	if err != nil {
		return store.Brain{}, err
	}
	if len(rows) == 0 {
		return store.Brain{}, store.ErrNotFound
	}
	return rows[0], nil
}

// AddBrainUsage calls add_brain_usage (migrations/0009). It is not retried
// after an uncertain failure (it would count twice); the ledger row is the
// record of truth.
func (s *Store) AddBrainUsage(ctx context.Context, workspaceID string, credits float64) (store.Brain, error) {
	var b store.Brain
	err := s.c.RPC(ctx, "add_brain_usage", map[string]any{"ws": workspaceID, "credits": credits}, &b)
	var apiErr *eb.APIError
	if errors.As(err, &apiErr) && apiErr.Code == "P0002" { // no_data_found: no brain
		return store.Brain{}, fmt.Errorf("%w: %s", store.ErrNotFound, apiErr.Message)
	}
	return b, mapErr(err)
}

// ---- usage, outbound ----

func (s *Store) AddUsage(ctx context.Context, u *store.UsageEntry) error {
	return insertRow(ctx, s, "usage_ledger", &u.ID, u)
}

func (s *Store) CreateOutbound(ctx context.Context, m *store.OutboundMessage) error {
	return insertRow(ctx, s, "outbound_messages", &m.ID, m)
}

func (s *Store) GetOutboundByKey(ctx context.Context, key string) (store.OutboundMessage, error) {
	q := eb.NewQuery().Eq("idempotency_key", key)
	rows, err := listRows[store.OutboundMessage](ctx, s, "outbound_messages", q, 1)
	if err != nil {
		return store.OutboundMessage{}, err
	}
	if len(rows) == 0 {
		return store.OutboundMessage{}, store.ErrNotFound
	}
	return rows[0], nil
}

func (s *Store) UpdateOutbound(ctx context.Context, id string, p store.OutboundPatch) (store.OutboundMessage, error) {
	return updateRow[store.OutboundMessage](ctx, s, "outbound_messages", id, p)
}

// ---- helpers ----

func getRow[T any](ctx context.Context, s *Store, table, id string) (T, error) {
	var row T
	err := s.c.Get(ctx, table, id, &row)
	return row, mapErr(err)
}

// insertRow fills a missing id, inserts row and copies the stored row
// (with database defaults) back into it.
func insertRow[T any](ctx context.Context, s *Store, table string, id *string, row *T) error {
	if *id == "" {
		*id = store.NewID()
	}
	var out T
	if err := s.c.Insert(ctx, table, *id, row, &out); err != nil {
		return mapErr(err)
	}
	*row = out
	return nil
}

func updateRow[T any](ctx context.Context, s *Store, table, id string, patch any) (T, error) {
	var row T
	err := s.c.Update(ctx, table, id, patch, &row)
	return row, mapErr(err)
}

func listRows[T any](ctx context.Context, s *Store, table string, q *eb.Query, limit int) ([]T, error) {
	rows, err := eb.ListAll[T](ctx, s.c, table, q, limit)
	return rows, mapErr(err)
}

// mapErr turns client errors into store errors, keeping the details.
func mapErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, eb.ErrNotFound):
		return fmt.Errorf("%w: %w", store.ErrNotFound, err)
	case errors.Is(err, eb.ErrConflict):
		return fmt.Errorf("%w: %w", store.ErrConflict, err)
	case errors.Is(err, eb.ErrTooLarge):
		return fmt.Errorf("%w: %w", store.ErrTooLarge, err)
	}
	var apiErr *eb.APIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusBadRequest {
		// The database refused a value; its message says why.
		return fmt.Errorf("%w: %s", store.ErrInvalid, apiErr.Message)
	}
	return err
}
