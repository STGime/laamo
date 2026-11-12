package api

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/csv"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/STGime/laamo/backend/internal/bricks/do"
	"github.com/STGime/laamo/backend/internal/store"
)

// Lists (#47): rows that Save to a list put there. Members can read, search,
// export and delete rows; the owner deletes lists and shares them.

const (
	maxListRowsShown  = 500  // getList
	maxSharedRows     = 2000 // getSharedList
	maxListsPerSpace  = 50
	shareTokenBytes   = 24 // 32 characters
	maxListNameRunes  = 80
	listNameRuleShort = "a list name is 1 to 80 characters"
)

var listNotFound = apiErr(ErrorCodeNotFound, "list not found")

// list loads a list the signed-in user may see, with their membership.
func (s *Server) list(ctx context.Context, id string) (store.List, store.Member, bool, error) {
	l, err := s.store.GetList(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return l, store.Member{}, false, nil
	}
	if err != nil {
		return l, store.Member{}, false, err
	}
	_, m, ok, err := s.member(ctx, l.WorkspaceID)
	return l, m, ok, err
}

func (s *Server) listOut(ctx context.Context, l store.List) (ListSummary, error) {
	n, err := s.store.CountListRows(ctx, l.ID)
	return ListSummary{Id: uuid(l.ID), Name: l.Name, Rows: n, ShareToken: l.ShareToken,
		CreatedAt: l.CreatedAt, UpdatedAt: l.UpdatedAt}, err
}

func (s *Server) ListLists(ctx context.Context, req ListListsRequestObject) (ListListsResponseObject, error) {
	_, m, ok, err := s.member(ctx, req.WorkspaceId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return ListLists404JSONResponse{NotFoundJSONResponse(notFound)}, nil
	}
	ls, err := s.store.ListLists(ctx, m.WorkspaceID)
	if err != nil {
		return nil, err
	}
	out := ListLists200JSONResponse{}
	for _, l := range ls {
		lo, err := s.listOut(ctx, l)
		if err != nil {
			return nil, err
		}
		out = append(out, lo)
	}
	return out, nil
}

func listName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	n := utf8.RuneCountInString(name)
	return name, n >= 1 && n <= maxListNameRunes
}

func (s *Server) CreateList(ctx context.Context, req CreateListRequestObject) (CreateListResponseObject, error) {
	_, m, ok, err := s.member(ctx, req.WorkspaceId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return CreateList404JSONResponse{NotFoundJSONResponse(notFound)}, nil
	}
	name, ok := listName(req.Body.Name)
	if !ok {
		return CreateList400JSONResponse{BadRequestJSONResponse(apiErr(ErrorCodeBadRequest, listNameRuleShort))}, nil
	}
	if ls, err := s.store.ListLists(ctx, m.WorkspaceID); err != nil {
		return nil, err
	} else if len(ls) >= maxListsPerSpace {
		return CreateList400JSONResponse{BadRequestJSONResponse(apiErr(ErrorCodeBadRequest,
			fmt.Sprintf("a workspace has at most %d lists", maxListsPerSpace)))}, nil
	}
	w, err := s.store.GetWorkspace(ctx, m.WorkspaceID)
	if err != nil {
		return nil, err
	}
	l := store.List{WorkspaceID: m.WorkspaceID, UserID: w.UserID, Name: name}
	if err := s.store.CreateList(ctx, &l); errors.Is(err, store.ErrConflict) {
		return CreateList409JSONResponse{ConflictJSONResponse(apiErr(ErrorCodeConflict, "you have a list with that name"))}, nil
	} else if err != nil {
		return nil, err
	}
	lo, err := s.listOut(ctx, l)
	if err != nil {
		return nil, err
	}
	return CreateList201JSONResponse(lo), nil
}

func (s *Server) GetList(ctx context.Context, req GetListRequestObject) (GetListResponseObject, error) {
	l, _, ok, err := s.list(ctx, req.ListId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return GetList404JSONResponse{NotFoundJSONResponse(listNotFound)}, nil
	}
	rows, err := s.store.ListRows(ctx, l.ID, 0)
	if err != nil {
		return nil, err
	}
	if q := strings.ToLower(strings.TrimSpace(deref(req.Params.Q))); q != "" {
		rows = slices.DeleteFunc(rows, func(r store.ListRow) bool { return !rowHas(r, q) })
	}
	sortRows(rows, deref(req.Params.Sort), req.Params.Order == nil || *req.Params.Order == Desc)
	lo, err := s.listOut(ctx, l)
	if err != nil {
		return nil, err
	}
	out := ListDetail{List: lo, Columns: columnsOut(l), Matching: len(rows)}
	out.Rows = rowsOut(rows[:min(len(rows), maxListRowsShown)])
	return GetList200JSONResponse(out), nil
}

func (s *Server) UpdateList(ctx context.Context, req UpdateListRequestObject) (UpdateListResponseObject, error) {
	l, _, ok, err := s.list(ctx, req.ListId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return UpdateList404JSONResponse{NotFoundJSONResponse(listNotFound)}, nil
	}
	name, ok := listName(req.Body.Name)
	if !ok {
		return UpdateList400JSONResponse{BadRequestJSONResponse(apiErr(ErrorCodeBadRequest, listNameRuleShort))}, nil
	}
	l, err = s.store.UpdateList(ctx, l.ID, store.ListPatch{Name: &name})
	if errors.Is(err, store.ErrConflict) {
		return UpdateList409JSONResponse{ConflictJSONResponse(apiErr(ErrorCodeConflict, "you have a list with that name"))}, nil
	}
	if err != nil {
		return nil, err
	}
	lo, err := s.listOut(ctx, l)
	if err != nil {
		return nil, err
	}
	return UpdateList200JSONResponse(lo), nil
}

func (s *Server) DeleteList(ctx context.Context, req DeleteListRequestObject) (DeleteListResponseObject, error) {
	l, m, ok, err := s.list(ctx, req.ListId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return DeleteList404JSONResponse{NotFoundJSONResponse(listNotFound)}, nil
	}
	if m.Role != store.RoleOwner {
		return DeleteList403JSONResponse{ForbiddenJSONResponse(apiErr(ErrorCodeForbidden, "only the owner can delete lists"))}, nil
	}
	if err := s.store.DeleteList(ctx, l.ID); err != nil {
		return nil, err
	}
	s.log.Info("list deleted", "list", l.ID, "workspace", l.WorkspaceID)
	return DeleteList204Response{}, nil
}

// pickForList: the rows "Add to list" would add, from what the run stored:
// chosen cards, chosen source links, or everything. false when something
// asked for isn't on the Feed card.
func (s *Server) pickForList(ctx context.Context, fc store.FeedCard, body *AddListRowsJSONRequestBody) ([]map[string]any, bool, error) {
	shown, err := s.shownCards(ctx, fc)
	if err != nil {
		return nil, false, err
	}
	links := payloadLinks(fc)
	if body.Links != nil || len(shown) == 0 {
		want := map[string]bool{}
		for _, u := range deref(body.Links) {
			want[u] = true
		}
		var out []map[string]any
		found := map[string]bool{}
		for _, l := range links {
			if u := l["url"].(string); len(want) == 0 || (want[u] && !found[u]) {
				out = append(out, l)
				found[u] = true
			}
		}
		return out, len(out) > 0 && (len(want) == 0 || len(found) == len(want)), nil
	}
	want := map[string]bool{}
	for _, id := range deref(body.CardIds) {
		want[id.String()] = true
	}
	var out []map[string]any
	matched := 0
	for _, c := range shown {
		if len(want) > 0 && !want[c.CardID] {
			continue
		}
		// A summary (Sum it all up) is worth keeping for its sources:
		// one row per link, not one row of text.
		if src := sourceLinks(c.Data["sources"]); c.Kind == "summary" && len(src) > 0 {
			out = append(out, src...)
			matched++
			continue
		}
		out = append(out, do.RowData(c.Data, deref(c.SourceURL)))
		matched++
	}
	return out, len(out) > 0 && (len(want) == 0 || matched == len(want)), nil
}

// payloadLinks: a result's source links (title and url), as the engine put
// them on the Feed card.
func payloadLinks(fc store.FeedCard) []map[string]any {
	return sourceLinks(fc.Payload["links"])
}

// sourceLinks turns [{title, url}] into rows, http(s) links only.
func sourceLinks(v any) []map[string]any {
	raw, _ := v.([]any)
	var out []map[string]any
	for _, x := range raw {
		m, _ := x.(map[string]any)
		u, _ := m["url"].(string)
		if !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
			continue
		}
		row := map[string]any{"url": u}
		if t, _ := m["title"].(string); strings.TrimSpace(t) != "" {
			row["title"] = strings.TrimSpace(t)
		}
		out = append(out, row)
	}
	return out
}

// AddListRows: "Add to list" from the Feed (#47). The cards come from the
// run as stored, never from the request, and duplicates are skipped like
// Save to a list does: same website, else email, else link, else name.
func (s *Server) AddListRows(ctx context.Context, req AddListRowsRequestObject) (AddListRowsResponseObject, error) {
	l, _, ok, err := s.list(ctx, req.ListId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return AddListRows404JSONResponse{NotFoundJSONResponse(listNotFound)}, nil
	}
	fc, _, ok, err := s.feedCard(ctx, req.Body.FeedCardId.String())
	if err != nil {
		return nil, err
	}
	if !ok || fc.WorkspaceID != l.WorkspaceID {
		return AddListRows404JSONResponse{NotFoundJSONResponse(feedNotFound)}, nil
	}
	picked, ok, err := s.pickForList(ctx, fc, req.Body)
	if err != nil {
		return nil, err
	}
	if !ok {
		return AddListRows400JSONResponse{BadRequestJSONResponse(apiErr(ErrorCodeBadRequest, "those cards or links aren't on this Feed card"))}, nil
	}
	count, err := s.store.CountListRows(ctx, l.ID)
	if err != nil {
		return nil, err
	}
	if count+len(picked) > do.MaxListRows {
		return AddListRows400JSONResponse{BadRequestJSONResponse(apiErr(ErrorCodeBadRequest,
			fmt.Sprintf("the list is full (%d rows); delete some rows first", do.MaxListRows)))}, nil
	}
	added, dups := 0, 0
	var saved []map[string]any
	for _, data := range picked {
		row := store.ListRow{WorkspaceID: l.WorkspaceID, UserID: l.UserID, ListID: l.ID, Data: data, SourceRunID: fc.RunID}
		for _, f := range []string{"website", "email", "url", "name"} {
			if v, _ := data[f].(string); v != "" {
				if key := do.DedupeKey(f, v); key != "" {
					row.DedupeKey = &key
					break
				}
			}
		}
		if row.DedupeKey != nil {
			if _, err := s.store.GetListRowByKey(ctx, l.ID, *row.DedupeKey); err == nil {
				dups++
				continue
			} else if !errors.Is(err, store.ErrNotFound) {
				return nil, err
			}
		}
		if err := s.store.AddListRow(ctx, &row); errors.Is(err, store.ErrConflict) {
			dups++
			continue
		} else if err != nil {
			return nil, err
		}
		added++
		saved = append(saved, data)
	}
	if err := do.AddColumns(ctx, s.store, l, saved...); err != nil {
		return nil, err
	}
	if l, err = s.store.GetList(ctx, l.ID); err != nil {
		return nil, err
	}
	lo, err := s.listOut(ctx, l)
	if err != nil {
		return nil, err
	}
	return AddListRows200JSONResponse{Added: added, Duplicates: dups, List: lo}, nil
}

func (s *Server) DeleteListRow(ctx context.Context, req DeleteListRowRequestObject) (DeleteListRowResponseObject, error) {
	l, _, ok, err := s.list(ctx, req.ListId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return DeleteListRow404JSONResponse{NotFoundJSONResponse(listNotFound)}, nil
	}
	r, err := s.store.GetListRow(ctx, req.RowId.String())
	if errors.Is(err, store.ErrNotFound) || (err == nil && r.ListID != l.ID) {
		return DeleteListRow404JSONResponse{NotFoundJSONResponse(apiErr(ErrorCodeNotFound, "row not found"))}, nil
	}
	if err != nil {
		return nil, err
	}
	if err := s.store.DeleteListRow(ctx, r.ID); err != nil {
		return nil, err
	}
	return DeleteListRow204Response{}, nil
}

func (s *Server) ExportList(ctx context.Context, req ExportListRequestObject) (ExportListResponseObject, error) {
	l, _, ok, err := s.list(ctx, req.ListId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return ExportList404JSONResponse{NotFoundJSONResponse(listNotFound)}, nil
	}
	rows, err := s.store.ListRows(ctx, l.ID, 0)
	if err != nil {
		return nil, err
	}
	b, err := listCSV(columnKeys(l), rows)
	if err != nil {
		return nil, err
	}
	return ExportList200TextcsvResponse{Body: bytes.NewReader(b), ContentLength: int64(len(b))}, nil
}

func (s *Server) ShareList(ctx context.Context, req ShareListRequestObject) (ShareListResponseObject, error) {
	l, m, ok, err := s.list(ctx, req.ListId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return ShareList404JSONResponse{NotFoundJSONResponse(listNotFound)}, nil
	}
	if m.Role != store.RoleOwner {
		return ShareList403JSONResponse{ForbiddenJSONResponse(apiErr(ErrorCodeForbidden, "only the owner can share lists"))}, nil
	}
	if l.ShareToken == nil {
		b := make([]byte, shareTokenBytes)
		_, _ = rand.Read(b)
		if l, err = s.store.UpdateList(ctx, l.ID, store.ListPatch{ShareToken: store.Set(base64.RawURLEncoding.EncodeToString(b))}); err != nil {
			return nil, err
		}
		s.log.Info("list shared", "list", l.ID)
	}
	lo, err := s.listOut(ctx, l)
	if err != nil {
		return nil, err
	}
	return ShareList200JSONResponse(lo), nil
}

func (s *Server) UnshareList(ctx context.Context, req UnshareListRequestObject) (UnshareListResponseObject, error) {
	l, m, ok, err := s.list(ctx, req.ListId.String())
	if err != nil {
		return nil, err
	}
	if !ok {
		return UnshareList404JSONResponse{NotFoundJSONResponse(listNotFound)}, nil
	}
	if m.Role != store.RoleOwner {
		return UnshareList403JSONResponse{ForbiddenJSONResponse(apiErr(ErrorCodeForbidden, "only the owner can share lists"))}, nil
	}
	if _, err := s.store.UpdateList(ctx, l.ID, store.ListPatch{ShareToken: store.Null[string]()}); err != nil {
		return nil, err
	}
	return UnshareList204Response{}, nil
}

// GetSharedList is public: the token is the only key.
func (s *Server) GetSharedList(ctx context.Context, req GetSharedListRequestObject) (GetSharedListResponseObject, error) {
	l, err := s.store.GetListByShareToken(ctx, req.Token)
	if errors.Is(err, store.ErrNotFound) {
		return GetSharedList404JSONResponse{NotFoundJSONResponse(listNotFound)}, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.store.ListRows(ctx, l.ID, maxSharedRows)
	if err != nil {
		return nil, err
	}
	return GetSharedList200JSONResponse{Name: l.Name, Columns: columnsOut(l), Rows: rowsOut(rows)}, nil
}

func columnKeys(l store.List) []string {
	var out []string
	for _, c := range l.Columns {
		if k, ok := c["key"].(string); ok {
			out = append(out, k)
		}
	}
	return out
}

func columnsOut(l store.List) []ListColumn {
	keys := columnKeys(l)
	out := make([]ListColumn, len(keys))
	for i, k := range keys {
		out[i] = ListColumn{Key: k}
	}
	return out
}

func rowsOut(rows []store.ListRow) []ListRow {
	out := make([]ListRow, len(rows))
	for i, r := range rows {
		data := r.Data
		if data == nil {
			data = map[string]any{}
		}
		out[i] = ListRow{Id: uuid(r.ID), Data: data, CreatedAt: r.CreatedAt}
	}
	return out
}

func cellText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	default:
		return fmt.Sprint(x)
	}
}

func rowHas(r store.ListRow, q string) bool {
	for _, v := range r.Data {
		if strings.Contains(strings.ToLower(cellText(v)), q) {
			return true
		}
	}
	return false
}

// sortRows sorts by a column (numbers as numbers, text ignoring case;
// empty cells last) or by when the row was saved.
func sortRows(rows []store.ListRow, col string, desc bool) {
	slices.SortStableFunc(rows, func(a, b store.ListRow) int {
		var c int
		if col == "" || col == "created_at" {
			c = a.CreatedAt.Compare(b.CreatedAt)
		} else {
			av, bv := a.Data[col], b.Data[col]
			if (av == nil) != (bv == nil) { // empty last, either way
				if av == nil {
					return 1
				}
				return -1
			}
			an, aNum := av.(float64)
			bn, bNum := bv.(float64)
			if aNum && bNum {
				c = cmp.Compare(an, bn)
			} else {
				c = strings.Compare(strings.ToLower(cellText(av)), strings.ToLower(cellText(bv)))
			}
		}
		if desc {
			c = -c
		}
		return cmp.Or(c, strings.Compare(a.ID, b.ID))
	})
}

// listCSV writes rows as CSV for spreadsheets: a byte-order mark so Excel
// reads umlauts, and cells that start like a formula are quoted with '
// so opening the file can't run anything.
func listCSV(cols []string, rows []store.ListRow) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("\ufeff")
	w := csv.NewWriter(&buf)
	header := append(slices.Clone(cols), "saved_at")
	if err := w.Write(header); err != nil {
		return nil, err
	}
	for _, r := range rows {
		rec := make([]string, 0, len(header))
		for _, c := range cols {
			rec = append(rec, safeCell(cellText(r.Data[c])))
		}
		rec = append(rec, r.CreatedAt.UTC().Format("2006-01-02 15:04"))
		if err := w.Write(rec); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

func safeCell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
