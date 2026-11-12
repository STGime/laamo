package do

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/STGime/laamo/backend/internal/engine"
	"github.com/STGime/laamo/backend/internal/queue"
	"github.com/STGime/laamo/backend/internal/store"
)

// MaxListRows: a list holds at most this many rows.
const MaxListRows = 5000

// maxListValue caps one value in a row.
const maxListValue = 1000

// Fields that don't go into a row: whole pages and internal ones.
var skipFields = map[string]bool{"page_text": true, "sources": true, "list_row_id": true}

// columnOrder: the usual columns come first, in this order.
var columnOrder = []string{"name", "title", "email", "phone", "website", "address", "url", "date", "time", "place", "price"}

// List is the Save to a list brick (do.list): every card becomes a row in
// one of the user's Lists, unless a row with the same email, website or
// name is there already. Every card goes on with list_row_id: its new row,
// or the one it duplicates.
type List struct {
	Store store.ListStore
}

func (l List) Run(ctx context.Context, rc *engine.RunContext, in []engine.Card, cfg engine.Config) ([]engine.Card, error) {
	if len(in) == 0 {
		return nil, nil
	}
	list, err := l.Store.GetList(ctx, cfg.String("list"))
	if errors.Is(err, store.ErrNotFound) || (err == nil && list.WorkspaceID != rc.Run.WorkspaceID) {
		return nil, queue.Permanent(errors.New("this list doesn't exist anymore; pick another in the brick"))
	}
	if err != nil {
		return nil, err
	}
	count, err := l.Store.CountListRows(ctx, list.ID)
	if err != nil {
		return nil, err
	}
	dedupe := cfg.String("dedupe")
	columns := columnKeys(list.Columns)
	added := 0
	for i := range in {
		c := &in[i]
		data := rowData(*c)
		row := store.ListRow{
			// The same card in the same run always gets the same row id, so
			// a retried job finds its own rows instead of seeing duplicates.
			ID:          uuid.NewSHA1(uuid.NameSpaceOID, []byte(rc.Run.ID+"/"+rc.BrickID+"/"+c.ID)).String(),
			WorkspaceID: rc.Run.WorkspaceID, UserID: rc.Run.UserID, ListID: list.ID, Data: data,
			SourceRunID: &rc.Run.ID,
		}
		if key := dedupeKey(dedupe, *c); key != "" {
			row.DedupeKey = &key
		}
		if existing, ok, err := l.known(ctx, row); err != nil {
			return nil, err
		} else if ok {
			c.Set("list_row_id", existing, engine.TrustUser)
			continue
		}
		if count >= MaxListRows {
			return nil, queue.Permanent(fmt.Errorf("the list “%s” is full (%d rows); delete some rows or use another list", list.Name, MaxListRows))
		}
		err := l.Store.AddListRow(ctx, &row)
		if errors.Is(err, store.ErrConflict) {
			// Saved meanwhile (by an earlier try or another run): look again.
			if existing, ok, err := l.known(ctx, row); err != nil || !ok {
				return nil, cmpErr(err, fmt.Errorf("row %s: conflict, but no row found", row.ID))
			} else {
				c.Set("list_row_id", existing, engine.TrustUser)
				continue
			}
		}
		if err != nil {
			return nil, err
		}
		count++
		added++
		c.Set("list_row_id", row.ID, engine.TrustUser)
		for k := range data {
			if !slices.Contains(columns, k) {
				columns = append(columns, k)
			}
		}
	}
	if cols := sortColumns(columns); !slices.Equal(cols, columnKeys(list.Columns)) {
		out := make([]map[string]any, len(cols))
		for i, k := range cols {
			out[i] = map[string]any{"key": k}
		}
		if _, err := l.Store.UpdateList(ctx, list.ID, store.ListPatch{Columns: &out}); err != nil {
			return nil, err
		}
	}
	rc.Log.Info("saved to list", "list", list.ID, "added", added, "duplicates", len(in)-added)
	return in, nil
}

// known finds the row a card already has: this run's own (a retry), or
// another one with the same dedupe key.
func (l List) known(ctx context.Context, row store.ListRow) (string, bool, error) {
	if r, err := l.Store.GetListRow(ctx, row.ID); err == nil {
		return r.ID, true, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return "", false, err
	}
	if row.DedupeKey == nil {
		return "", false, nil
	}
	r, err := l.Store.GetListRowByKey(ctx, row.ListID, *row.DedupeKey)
	if errors.Is(err, store.ErrNotFound) {
		return "", false, nil
	}
	return r.ID, err == nil, err
}

func cmpErr(err, fallback error) error {
	if err != nil {
		return err
	}
	return fallback
}

// rowData is what a card puts in its row: its text, numbers and yes/no
// fields, shortened.
func rowData(c engine.Card) map[string]any {
	data := map[string]any{}
	for k, v := range c.Fields {
		if skipFields[k] {
			continue
		}
		switch x := v.(type) {
		case string:
			if x = strings.TrimSpace(x); x != "" {
				data[k] = cut(x, maxListValue)
			}
		case float64, bool, int:
			data[k] = x
		}
	}
	if _, ok := data["url"]; !ok && c.Source != "" {
		data["url"] = c.Source
	}
	return data
}

// dedupeKey is what makes two cards the same row, e.g. "website:baeckerei-klein.de".
// Empty when the card doesn't have that detail (it's saved anyway).
func dedupeKey(field string, c engine.Card) string {
	v := strings.ToLower(strings.TrimSpace(c.Text(field)))
	switch field {
	case "email":
		v = strings.TrimPrefix(v, "mailto:")
	case "website":
		u := v
		if !strings.Contains(u, "://") {
			u = "https://" + u
		}
		if p, err := url.Parse(u); err == nil && p.Hostname() != "" {
			v = strings.TrimPrefix(p.Hostname(), "www.")
		}
	case "name":
		v = strings.Join(strings.Fields(v), " ")
	default:
		return ""
	}
	if v == "" {
		return ""
	}
	return field + ":" + cut(v, 300)
}

func columnKeys(cols []map[string]any) []string {
	var out []string
	for _, c := range cols {
		if k, ok := c["key"].(string); ok {
			out = append(out, k)
		}
	}
	return out
}

// sortColumns puts the usual columns first; the rest stay in the order they came.
func sortColumns(keys []string) []string {
	var out []string
	for _, k := range columnOrder {
		if slices.Contains(keys, k) {
			out = append(out, k)
		}
	}
	for _, k := range keys {
		if !slices.Contains(columnOrder, k) {
			out = append(out, k)
		}
	}
	return out
}
