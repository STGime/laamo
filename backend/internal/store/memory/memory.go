// Package memory is an in-memory store.Store for unit tests.
//
// It mirrors the database where tests can notice: column defaults, the unique
// constraints the services rely on (ErrConflict), ordering, and rows being
// copies (changing a returned row doesn't change the store).
// Foreign keys and RLS are not checked.
package memory

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/STGime/laamo/backend/internal/store"
)

type Store struct {
	mu         sync.Mutex
	now        func() time.Time
	workspaces map[string]store.Workspace
	members    map[string]store.Member
	brains     map[string]store.Brain // by workspace id
	triggers   map[string]store.Trigger
	places     map[string]store.Place
	helpers    map[string]store.Helper
	versions   map[string]store.HelperVersion
	runs       map[string]store.Run
	steps      map[string]store.Step
	cards      map[string]store.Card
	feed       map[string]store.FeedCard
	lists      map[string]store.List
	listRows   map[string]store.ListRow
	usage      map[string]store.UsageEntry
	outbound   map[string]store.OutboundMessage
	examples   []store.PickExample
}

var _ store.Store = (*Store)(nil)

func New() *Store {
	return &Store{
		now:        time.Now,
		workspaces: map[string]store.Workspace{},
		members:    map[string]store.Member{},
		brains:     map[string]store.Brain{},
		triggers:   map[string]store.Trigger{},
		places:     map[string]store.Place{},
		helpers:    map[string]store.Helper{},
		versions:   map[string]store.HelperVersion{},
		runs:       map[string]store.Run{},
		steps:      map[string]store.Step{},
		cards:      map[string]store.Card{},
		feed:       map[string]store.FeedCard{},
		lists:      map[string]store.List{},
		listRows:   map[string]store.ListRow{},
		usage:      map[string]store.UsageEntry{},
		outbound:   map[string]store.OutboundMessage{},
	}
}

// SetClock replaces time.Now for created_at and other defaults.
func (s *Store) SetClock(now func() time.Time) { s.now = now }

// ---- seeding (the API writes these; tests put them directly) ----

func (s *Store) PutWorkspace(w store.Workspace) store.Workspace {
	s.mu.Lock()
	defer s.mu.Unlock()
	if w.ID == "" {
		w.ID = store.NewID()
	}
	w.Timezone = cmp.Or(w.Timezone, "Europe/Berlin")
	w.Locale = cmp.Or(w.Locale, "en")
	w.Plan = cmp.Or(w.Plan, "free")
	if w.Profile == nil {
		w.Profile = map[string]any{}
	}
	if w.CreatedAt.IsZero() {
		w.CreatedAt = s.now()
		w.UpdatedAt = w.CreatedAt
	}
	s.workspaces[w.ID] = clone(w)
	return w
}

func (s *Store) PutHelper(h store.Helper) store.Helper {
	s.mu.Lock()
	defer s.mu.Unlock()
	if h.ID == "" {
		h.ID = store.NewID()
	}
	h.Status = cmp.Or(h.Status, "draft")
	h.CreatedVia = cmp.Or(h.CreatedVia, "scratch")
	if h.CreatedAt.IsZero() {
		h.CreatedAt = s.now()
		h.UpdatedAt = h.CreatedAt
	}
	s.helpers[h.ID] = clone(h)
	return h
}

func (s *Store) PutHelperVersion(v store.HelperVersion) store.HelperVersion {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.ID == "" {
		v.ID = store.NewID()
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = s.now()
	}
	s.versions[v.ID] = clone(v)
	return v
}

// ---- workspaces, helpers ----

func (s *Store) GetWorkspace(_ context.Context, id string) (store.Workspace, error) {
	return get(s, s.workspaces, id)
}

func (s *Store) GetWorkspaces(_ context.Context, ids []string) ([]store.Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.Workspace
	for _, id := range ids {
		if w, ok := s.workspaces[id]; ok {
			out = append(out, clone(w))
		}
	}
	return out, nil
}

// CreateWorkspace follows migrations/0008 create_workspace.
func (s *Store) CreateWorkspace(_ context.Context, nw store.NewWorkspace) (store.Workspace, error) {
	name := strings.TrimSpace(nw.Name)
	tz := cmp.Or(nw.Timezone, "Europe/Berlin")
	locale := cmp.Or(nw.Locale, "en")
	invalid := func(msg string) (store.Workspace, error) {
		return store.Workspace{}, fmt.Errorf("%w: %s", store.ErrInvalid, msg)
	}
	switch n := utf8.RuneCountInString(name); {
	case n < 1 || n > 80:
		return invalid("name must be 1 to 80 characters")
	case locale != "en" && locale != "de":
		return invalid("locale must be en or de")
	case !validTimezone(tz):
		return invalid("unknown timezone " + tz)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	owned := 0
	for _, w := range s.workspaces {
		if w.UserID == nw.OwnerID {
			owned++
		}
	}
	if owned >= 5 {
		return invalid("you already have 5 workspaces")
	}
	now := s.now()
	w := store.Workspace{ID: store.NewID(), UserID: nw.OwnerID, Name: name, Timezone: tz, Locale: locale,
		Plan: "free", Profile: map[string]any{}, CreatedAt: now, UpdatedAt: now}
	s.workspaces[w.ID] = clone(w)
	m := store.Member{ID: store.NewID(), WorkspaceID: w.ID, UserID: nw.OwnerID, Role: store.RoleOwner, CreatedAt: now}
	s.members[m.ID] = m
	s.brains[w.ID] = newBrain(w.ID, nw.OwnerID, now)
	return w, nil
}

func validTimezone(tz string) bool {
	if tz == "" || tz == "Local" {
		return false
	}
	_, err := time.LoadLocation(tz)
	return err == nil
}

func (s *Store) UpdateWorkspace(_ context.Context, id string, p store.WorkspacePatch) (store.Workspace, error) {
	if p.Locale != nil && *p.Locale != "en" && *p.Locale != "de" {
		return store.Workspace{}, fmt.Errorf("%w: locale must be en or de", store.ErrInvalid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.workspaces[id]
	if !ok {
		return store.Workspace{}, store.ErrNotFound
	}
	w, err := store.ApplyPatch(w, p)
	if err != nil {
		return w, err
	}
	w.UpdatedAt = s.now()
	s.workspaces[id] = clone(w)
	return w, nil
}

func newBrain(workspaceID, userID string, now time.Time) store.Brain {
	return store.Brain{ID: store.NewID(), WorkspaceID: workspaceID, UserID: userID, Mode: "included", Status: "ok",
		AllowanceCredits: 500, PeriodStart: monthStart(now), CreatedAt: now}
}

func monthStart(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// PutBrain sets a workspace's brain (test seeding).
func (s *Store) PutBrain(b store.Brain) store.Brain {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b.ID == "" {
		b.ID = store.NewID()
	}
	b.Mode = cmp.Or(b.Mode, "included")
	b.Status = cmp.Or(b.Status, "ok")
	if b.PeriodStart.IsZero() {
		b.PeriodStart = monthStart(s.now())
	}
	s.brains[b.WorkspaceID] = b
	return b
}

func (s *Store) GetBrain(_ context.Context, workspaceID string) (store.Brain, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.brains[workspaceID]
	if !ok {
		return store.Brain{}, store.ErrNotFound
	}
	return b, nil
}

// AddBrainUsage follows migrations/0009 add_brain_usage.
func (s *Store) AddBrainUsage(_ context.Context, workspaceID string, credits float64) (store.Brain, error) {
	if credits < 0 {
		return store.Brain{}, fmt.Errorf("%w: credits must be zero or more", store.ErrInvalid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.brains[workspaceID]
	if !ok {
		return store.Brain{}, store.ErrNotFound
	}
	if month := monthStart(s.now()); b.PeriodStart.Before(month) {
		b.UsedCreditsThisPeriod, b.PeriodStart = 0, month
	}
	b.UsedCreditsThisPeriod += store.Num(credits)
	switch {
	case b.Mode == "included" && b.UsedCreditsThisPeriod >= b.AllowanceCredits:
		b.Status = "allowance_used"
	case b.Status == "allowance_used":
		b.Status = "ok"
	}
	s.brains[workspaceID] = b
	return b, nil
}

// PutMember adds a membership (test seeding).
func (s *Store) PutMember(m store.Member) store.Member {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m.ID == "" {
		m.ID = store.NewID()
	}
	m.Role = cmp.Or(m.Role, store.RoleMember)
	if m.CreatedAt.IsZero() {
		m.CreatedAt = s.now()
	}
	s.members[m.ID] = m
	return m
}

func (s *Store) Memberships(_ context.Context, userID string) ([]store.Member, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.Member
	for _, m := range s.members {
		if m.UserID == userID {
			out = append(out, clone(m))
		}
	}
	slices.SortFunc(out, func(a, b store.Member) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), cmp.Compare(a.ID, b.ID))
	})
	return out, nil
}

func (s *Store) Membership(_ context.Context, workspaceID, userID string) (store.Member, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.members {
		if m.WorkspaceID == workspaceID && m.UserID == userID {
			return clone(m), nil
		}
	}
	return store.Member{}, store.ErrNotFound
}

func (s *Store) GetHelper(_ context.Context, id string) (store.Helper, error) {
	return get(s, s.helpers, id)
}

func (s *Store) GetHelperVersion(_ context.Context, id string) (store.HelperVersion, error) {
	return get(s, s.versions, id)
}

func (s *Store) CreateHelper(_ context.Context, h *store.Helper, v *store.HelperVersion) error {
	if h.WorkspaceID == "" || h.UserID == "" || h.Name == "" {
		return fmt.Errorf("%w: helper needs a workspace, owner and name", store.ErrInvalid)
	}
	*h = s.PutHelper(*h)
	v.HelperID, v.WorkspaceID, v.UserID = h.ID, h.WorkspaceID, h.UserID
	if v.Version == 0 {
		v.Version = 1
	}
	*v = s.PutHelperVersion(*v)
	h.CurrentVersionID = &v.ID
	*h = s.PutHelper(*h)
	return nil
}

func (s *Store) AddHelperVersion(_ context.Context, v *store.HelperVersion, current bool) error {
	s.mu.Lock()
	h, ok := s.helpers[v.HelperID]
	n := 0
	for _, x := range s.versions {
		if x.HelperID == v.HelperID {
			n = max(n, x.Version)
		}
	}
	s.mu.Unlock()
	if !ok {
		return store.ErrNotFound
	}
	v.ID, v.WorkspaceID, v.UserID, v.Version = "", h.WorkspaceID, h.UserID, n+1
	*v = s.PutHelperVersion(*v)
	if current {
		h.CurrentVersionID = &v.ID
		s.PutHelper(h)
	}
	return nil
}

func (s *Store) UpdateHelper(_ context.Context, id string, p store.HelperPatch) (store.Helper, error) {
	if p.Status != nil && *p.Status != "active" && *p.Status != "paused" && *p.Status != "draft" {
		return store.Helper{}, fmt.Errorf("%w: status %q", store.ErrInvalid, *p.Status)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.helpers[id]
	if !ok {
		return store.Helper{}, store.ErrNotFound
	}
	h, err := store.ApplyPatch(h, p)
	if err != nil {
		return h, err
	}
	h.UpdatedAt = s.now()
	s.helpers[id] = clone(h)
	return h, nil
}

// DeleteHelper cascades like the database does.
func (s *Store) DeleteHelper(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.helpers[id]; !ok {
		return store.ErrNotFound
	}
	delete(s.helpers, id)
	maps.DeleteFunc(s.versions, func(_ string, v store.HelperVersion) bool { return v.HelperID == id })
	maps.DeleteFunc(s.triggers, func(_ string, t store.Trigger) bool { return t.HelperID == id })
	runs := map[string]bool{}
	maps.DeleteFunc(s.runs, func(rid string, r store.Run) bool {
		if r.HelperID == id {
			runs[rid] = true
		}
		return r.HelperID == id
	})
	maps.DeleteFunc(s.steps, func(_ string, st store.Step) bool { return runs[st.RunID] })
	maps.DeleteFunc(s.cards, func(_ string, c store.Card) bool { return runs[c.RunID] })
	maps.DeleteFunc(s.feed, func(_ string, f store.FeedCard) bool { return f.HelperID != nil && *f.HelperID == id })
	s.examples = slices.DeleteFunc(s.examples, func(e store.PickExample) bool { return e.HelperID == id })
	return nil
}

func (s *Store) ListHelpers(_ context.Context, workspaceID string) ([]store.Helper, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.Helper
	for _, h := range s.helpers {
		if h.WorkspaceID == workspaceID {
			out = append(out, clone(h))
		}
	}
	slices.SortFunc(out, func(a, b store.Helper) int {
		return cmp.Or(b.CreatedAt.Compare(a.CreatedAt), cmp.Compare(b.ID, a.ID))
	})
	return out, nil
}

// ---- runs and steps ----

func (s *Store) CreateRun(_ context.Context, r *store.Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.ID == "" {
		r.ID = store.NewID()
	}
	if _, ok := s.runs[r.ID]; ok {
		return dup("runs", r.ID)
	}
	r.Status = cmp.Or(r.Status, store.RunQueued)
	if r.TriggerData == nil {
		r.TriggerData = map[string]any{}
	}
	if r.CreatedAt.IsZero() { // like the database: an explicit value is kept
		r.CreatedAt = s.now()
	}
	s.runs[r.ID] = clone(*r)
	return nil
}

func (s *Store) GetRun(_ context.Context, id string) (store.Run, error) {
	return get(s, s.runs, id)
}

func (s *Store) UpdateRun(_ context.Context, id string, p store.RunPatch) (store.Run, error) {
	return update(s, s.runs, id, p)
}

func (s *Store) ListRuns(_ context.Context, f store.RunFilter) ([]store.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.Run
	for _, r := range s.runs {
		if (f.WorkspaceID == "" || r.WorkspaceID == f.WorkspaceID) &&
			(f.HelperID == "" || r.HelperID == f.HelperID) &&
			(f.Status == "" || r.Status == f.Status) {
			out = append(out, clone(r))
		}
	}
	slices.SortFunc(out, func(a, b store.Run) int {
		return cmp.Or(b.CreatedAt.Compare(a.CreatedAt), cmp.Compare(b.ID, a.ID))
	})
	return limit(out, f.Limit), nil
}

func (s *Store) CreateStep(_ context.Context, st *store.Step) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st.ID == "" {
		st.ID = store.NewID()
	}
	if st.Attempt == 0 {
		st.Attempt = 1
	}
	for _, o := range s.steps {
		if o.ID == st.ID || (o.RunID == st.RunID && o.BrickID == st.BrickID && o.Attempt == st.Attempt) {
			return dup("run_steps", st.ID)
		}
	}
	st.Status = cmp.Or(st.Status, "running")
	if st.StartedAt.IsZero() {
		st.StartedAt = s.now()
	}
	if st.Log == nil {
		st.Log = []map[string]any{}
	}
	s.steps[st.ID] = clone(*st)
	return nil
}

func (s *Store) UpdateStep(_ context.Context, id string, p store.StepPatch) (store.Step, error) {
	return update(s, s.steps, id, p)
}

func (s *Store) ListSteps(_ context.Context, runID string) ([]store.Step, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.Step
	for _, st := range s.steps {
		if st.RunID == runID {
			out = append(out, clone(st))
		}
	}
	slices.SortFunc(out, func(a, b store.Step) int {
		return cmp.Or(cmp.Compare(a.Position, b.Position), cmp.Compare(a.Attempt, b.Attempt))
	})
	return out, nil
}

// ---- cards ----

func (s *Store) InsertCards(_ context.Context, cards []store.Card) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// all-or-nothing, like the insert_cards RPC
	seen := map[[2]string]bool{}
	for i := range cards {
		c := &cards[i]
		if c.ID == "" {
			c.ID = store.NewID()
		}
		key := [2]string{c.StepID, c.CardID}
		if _, ok := s.cards[c.ID]; ok || seen[key] {
			return dup("cards", c.ID)
		}
		for _, o := range s.cards {
			if o.StepID == c.StepID && o.CardID == c.CardID {
				return dup("cards", c.ID)
			}
		}
		if c.WorkspaceID == "" || c.UserID == "" || c.RunID == "" || c.StepID == "" || c.CardID == "" || c.Kind == "" {
			return fmt.Errorf("memory: card %d: workspace_id, user_id, run_id, step_id, card_id and kind are required", i)
		}
		seen[key] = true
	}
	now := s.now()
	for i := range cards {
		c := &cards[i]
		if c.Data == nil {
			c.Data = map[string]any{}
		}
		if c.Trust == nil {
			c.Trust = map[string]string{}
		}
		if c.CreatedAt.IsZero() {
			c.CreatedAt = now
		}
		s.cards[c.ID] = clone(*c)
	}
	return nil
}

func (s *Store) ListCards(_ context.Context, stepID string) ([]store.Card, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.Card
	for _, c := range s.cards {
		if c.StepID == stepID {
			out = append(out, clone(c))
		}
	}
	slices.SortFunc(out, func(a, b store.Card) int {
		return cmp.Or(cmp.Compare(a.Position, b.Position), cmp.Compare(a.ID, b.ID))
	})
	return out, nil
}

// ---- feed ----

func (s *Store) ListFeedCards(_ context.Context, f store.FeedFilter) ([]store.FeedCard, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.FeedCard
	for _, c := range s.feed {
		if c.WorkspaceID != f.WorkspaceID ||
			(len(f.Types) > 0 && !slices.Contains(f.Types, c.Type)) ||
			(len(f.States) > 0 && !slices.Contains(f.States, c.State)) ||
			slices.Contains(f.NotStates, c.State) ||
			(f.Before != nil && !c.CreatedAt.Before(*f.Before)) {
			continue
		}
		out = append(out, clone(c))
	}
	slices.SortFunc(out, func(a, b store.FeedCard) int {
		return cmp.Or(b.CreatedAt.Compare(a.CreatedAt), cmp.Compare(b.ID, a.ID))
	})
	return limit(out, f.Limit), nil
}

func (s *Store) CreateFeedCard(_ context.Context, c *store.FeedCard) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.ID == "" {
		c.ID = store.NewID()
	}
	if _, ok := s.feed[c.ID]; ok {
		return dup("feed_cards", c.ID)
	}
	c.State = cmp.Or(c.State, "open")
	if c.Payload == nil {
		c.Payload = map[string]any{}
	}
	if c.CreatedAt.IsZero() { // like the database: an explicit value is kept
		c.CreatedAt = s.now()
	}
	s.feed[c.ID] = clone(*c)
	return nil
}

// FeedCards returns all Feed cards, oldest first (test helper).
func (s *Store) FeedCards() []store.FeedCard {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.FeedCard
	for _, c := range s.feed {
		out = append(out, clone(c))
	}
	slices.SortFunc(out, func(a, b store.FeedCard) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), cmp.Compare(a.ID, b.ID))
	})
	return out
}

func (s *Store) GetFeedCard(_ context.Context, id string) (store.FeedCard, error) {
	return get(s, s.feed, id)
}

func (s *Store) UpdateFeedCard(_ context.Context, id string, p store.FeedCardPatch) (store.FeedCard, error) {
	return update(s, s.feed, id, p)
}

// ---- lists ----

func (s *Store) CreateList(_ context.Context, l *store.List) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l.ID == "" {
		l.ID = store.NewID()
	}
	for _, o := range s.lists {
		if o.ID == l.ID || (o.WorkspaceID == l.WorkspaceID && o.Name == l.Name) ||
			(l.ShareToken != nil && o.ShareToken != nil && *o.ShareToken == *l.ShareToken) {
			return dup("lists", l.ID)
		}
	}
	if l.Columns == nil {
		l.Columns = []map[string]any{}
	}
	if l.CreatedAt.IsZero() { // like the database: an explicit value is kept
		l.CreatedAt = s.now()
	}
	l.UpdatedAt = s.now()
	s.lists[l.ID] = clone(*l)
	return nil
}

func (s *Store) GetListByName(_ context.Context, workspaceID, name string) (store.List, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, l := range s.lists {
		if l.WorkspaceID == workspaceID && l.Name == name {
			return clone(l), nil
		}
	}
	return store.List{}, store.ErrNotFound
}

func (s *Store) AddListRow(_ context.Context, r *store.ListRow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.ID == "" {
		r.ID = store.NewID()
	}
	for _, o := range s.listRows {
		if o.ID == r.ID || (r.DedupeKey != nil && o.DedupeKey != nil && o.ListID == r.ListID && *o.DedupeKey == *r.DedupeKey) {
			return dup("list_rows", r.ID)
		}
	}
	if r.Data == nil {
		r.Data = map[string]any{}
	}
	if r.CreatedAt.IsZero() { // like the database: an explicit value is kept
		r.CreatedAt = s.now()
	}
	s.listRows[r.ID] = clone(*r)
	return nil
}

func (s *Store) ListRows(_ context.Context, listID string, n int) ([]store.ListRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.ListRow
	for _, r := range s.listRows {
		if r.ListID == listID {
			out = append(out, clone(r))
		}
	}
	slices.SortFunc(out, func(a, b store.ListRow) int {
		return cmp.Or(b.CreatedAt.Compare(a.CreatedAt), cmp.Compare(b.ID, a.ID))
	})
	return limit(out, n), nil
}

// ---- learning from swipes ----

func (s *Store) AddPickExamples(_ context.Context, ex []store.PickExample) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	helpers := map[string]bool{}
	for _, e := range ex {
		if slices.ContainsFunc(s.examples, func(o store.PickExample) bool { return o.StepID == e.StepID && o.CardID == e.CardID }) {
			continue
		}
		if e.ID == "" {
			e.ID = store.NewID()
		}
		if e.CreatedAt.IsZero() {
			e.CreatedAt = s.now()
		}
		s.examples = append(s.examples, e)
		helpers[e.HelperID] = true
	}
	// Each helper keeps its newest 200.
	for h := range helpers {
		var mine []store.PickExample
		for _, e := range s.examples {
			if e.HelperID == h {
				mine = append(mine, e)
			}
		}
		sortNewest(mine)
		if len(mine) > 200 {
			drop := map[string]bool{}
			for _, e := range mine[200:] {
				drop[e.ID] = true
			}
			s.examples = slices.DeleteFunc(s.examples, func(e store.PickExample) bool { return drop[e.ID] })
		}
	}
	return nil
}

func sortNewest(ex []store.PickExample) {
	slices.SortStableFunc(ex, func(a, b store.PickExample) int {
		return cmp.Or(b.CreatedAt.Compare(a.CreatedAt), cmp.Compare(b.ID, a.ID))
	})
}

func (s *Store) PickExamples(_ context.Context, helperID string, kept bool, n int) ([]store.PickExample, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.PickExample
	for _, e := range s.examples {
		if e.HelperID == helperID && e.Kept == kept {
			out = append(out, e)
		}
	}
	sortNewest(out)
	return limit(out, n), nil
}

func (s *Store) CountPickExamples(_ context.Context, helperID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, e := range s.examples {
		if e.HelperID == helperID {
			n++
		}
	}
	return n, nil
}

func (s *Store) ForgetPickExamples(_ context.Context, helperID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.examples = slices.DeleteFunc(s.examples, func(e store.PickExample) bool { return e.HelperID == helperID })
	return nil
}

// ---- usage, outbound ----

func (s *Store) AddUsage(_ context.Context, u *store.UsageEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u.ID == "" {
		u.ID = store.NewID()
	}
	if _, ok := s.usage[u.ID]; ok {
		return dup("usage_ledger", u.ID)
	}
	if u.CreatedAt.IsZero() { // like the database: an explicit value is kept
		u.CreatedAt = s.now()
	}
	s.usage[u.ID] = clone(*u)
	return nil
}

// Usage returns all ledger entries, oldest first (test helper).
func (s *Store) Usage() []store.UsageEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.UsageEntry
	for _, u := range s.usage {
		out = append(out, clone(u))
	}
	slices.SortFunc(out, func(a, b store.UsageEntry) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), cmp.Compare(a.ID, b.ID))
	})
	return out
}

func (s *Store) CreateOutbound(_ context.Context, m *store.OutboundMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m.ID == "" {
		m.ID = store.NewID()
	}
	for _, o := range s.outbound {
		if o.ID == m.ID || o.IdempotencyKey == m.IdempotencyKey {
			return dup("outbound_messages", m.IdempotencyKey)
		}
	}
	m.Status = cmp.Or(m.Status, "pending")
	if m.CreatedAt.IsZero() { // like the database: an explicit value is kept
		m.CreatedAt = s.now()
	}
	s.outbound[m.ID] = clone(*m)
	return nil
}

func (s *Store) GetOutboundByKey(_ context.Context, key string) (store.OutboundMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.outbound {
		if m.IdempotencyKey == key {
			return clone(m), nil
		}
	}
	return store.OutboundMessage{}, store.ErrNotFound
}

func (s *Store) UpdateOutbound(_ context.Context, id string, p store.OutboundPatch) (store.OutboundMessage, error) {
	return update(s, s.outbound, id, p)
}

// ---- places ----

func (s *Store) CreatePlace(_ context.Context, p *store.Place) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.ID == "" {
		p.ID = store.NewID()
	}
	if _, ok := s.places[p.ID]; ok {
		return dup("places", p.ID)
	}
	if p.Meta == nil {
		p.Meta = map[string]any{}
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = s.now()
	}
	s.places[p.ID] = clone(*p)
	return nil
}

func (s *Store) GetPlace(_ context.Context, id string) (store.Place, error) {
	return get(s, s.places, id)
}

func (s *Store) ListPlaces(_ context.Context, workspaceID string) ([]store.Place, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.Place
	for _, p := range s.places {
		if p.WorkspaceID == workspaceID {
			out = append(out, clone(p))
		}
	}
	slices.SortFunc(out, func(a, b store.Place) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), cmp.Compare(a.ID, b.ID))
	})
	return out, nil
}

func (s *Store) UpdatePlace(_ context.Context, id string, p store.PlacePatch) (store.Place, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pl, ok := s.places[id]
	if !ok {
		return store.Place{}, store.ErrNotFound
	}
	pl, err := store.ApplyPatch(pl, p)
	if err != nil {
		return pl, err
	}
	s.places[id] = clone(pl)
	return pl, nil
}

func (s *Store) DeletePlace(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.places[id]; !ok {
		return store.ErrNotFound
	}
	delete(s.places, id)
	return nil
}

// ---- triggers ----

func (s *Store) HelperTriggers(_ context.Context, helperID string) ([]store.Trigger, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.Trigger
	for _, t := range s.triggers {
		if t.HelperID == helperID {
			out = append(out, clone(t))
		}
	}
	slices.SortFunc(out, func(a, b store.Trigger) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), cmp.Compare(a.ID, b.ID))
	})
	return out, nil
}

func (s *Store) CreateTrigger(_ context.Context, t *store.Trigger) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t.ID == "" {
		t.ID = store.NewID()
	}
	for _, o := range s.triggers {
		if o.ID == t.ID || (t.InboundAddress != nil && o.InboundAddress != nil && *o.InboundAddress == *t.InboundAddress) ||
			(t.WebhookToken != nil && o.WebhookToken != nil && *o.WebhookToken == *t.WebhookToken) {
			return dup("triggers", t.ID)
		}
	}
	if t.Config == nil {
		t.Config = map[string]any{}
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = s.now()
	}
	t.UpdatedAt = t.CreatedAt
	s.triggers[t.ID] = clone(*t)
	return nil
}

func (s *Store) UpdateTrigger(_ context.Context, id string, p store.TriggerPatch) (store.Trigger, error) {
	return update(s, s.triggers, id, p)
}

func (s *Store) DeleteTrigger(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.triggers, id)
	return nil
}

// ---- helpers ----

func get[T any](s *Store, table map[string]T, id string) (T, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := table[id]
	if !ok {
		var zero T
		return zero, store.ErrNotFound
	}
	return clone(row), nil
}

func update[T any](s *Store, table map[string]T, id string, patch any) (T, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := table[id]
	if !ok {
		var zero T
		return zero, store.ErrNotFound
	}
	row, err := store.ApplyPatch(row, patch)
	if err != nil {
		return row, err
	}
	table[id] = clone(row)
	return row, nil
}

func dup(table, key string) error {
	return fmt.Errorf("memory: %s %s: %w", table, key, store.ErrConflict)
}

func limit[T any](rows []T, n int) []T {
	if n > 0 && len(rows) > n {
		return rows[:n]
	}
	return rows
}

// clone deep-copies a row so callers can't change stored maps and slices.
func clone[T any](v T) T {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("memory: clone %T: %v", v, err))
	}
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		panic(fmt.Sprintf("memory: clone %T: %v", v, err))
	}
	return out
}
