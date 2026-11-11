// Package eurobasetest is a fake Eurobase data API for tests.
//
// It follows the real API (checked against the Eurobase gateway source):
//   - GET /v1/db/{t}?col=op.val&order=&limit=&offset= → {"data": […], "count": n};
//     default limit 20, max 1000
//   - GET/PATCH/DELETE /v1/db/{t}/{id}; POST /v1/db/{t} inserts one row (201)
//   - unique violation on REST → 409, missing row → 404, errors as {"error": msg}
//   - POST /v1/db/rpc/{fn} → {"result": …}; errors as {"error", "code"} with
//     the status Eurobase maps the SQLSTATE to (since Eurobase #829)
//
// Tables get the column defaults and unique constraints from migrations/
// that the Go code depends on. RLS, foreign keys and types are not checked.
package eurobasetest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TimeFormat is how the fake writes timestamps: fixed width, so string
// comparison matches time order.
const TimeFormat = "2006-01-02T15:04:05.000000Z07:00"

type Row = map[string]any

// RPCFunc implements a database function. Return a *PGError for a database
// error; any other error is a 500 "internal server error".
type RPCFunc func(f *Fake, args map[string]json.RawMessage) (any, error)

// PGError is a Postgres error raised inside an RPC function.
type PGError struct {
	Code    string // SQLSTATE, e.g. 23505
	Message string
}

func (e *PGError) Error() string { return e.Message }

// rpcStatus mirrors Eurobase's rpcErrorFor (internal/query/rpc_error.go).
func rpcStatus(code string) int {
	switch {
	case code == "42501":
		return http.StatusForbidden
	case code == "25006":
		return http.StatusConflict
	case strings.HasPrefix(code, "40"):
		return http.StatusConflict
	case strings.HasPrefix(code, "53"), strings.HasPrefix(code, "57"):
		return http.StatusServiceUnavailable
	case strings.HasPrefix(code, "08"), strings.HasPrefix(code, "58"), strings.HasPrefix(code, "F0"), strings.HasPrefix(code, "XX"):
		return http.StatusInternalServerError
	}
	return http.StatusBadRequest
}

// Fault is injected into one request. With After, the request is applied
// first and the fault replaces the response (a lost answer).
type Fault struct {
	Status     int
	RetryAfter string
	After      bool
}

type table struct {
	defaults func(now string) Row
	uniques  [][]string // column sets; a set with a NULL value doesn't conflict
}

type Fake struct {
	Server *httptest.Server
	Key    string

	mu     sync.Mutex
	clock  time.Time
	tables map[string]table
	rows   map[string][]Row
	rpcs   map[string]RPCFunc
	// Objects is project storage, by key.
	Objects map[string][]byte
	faults  []func(*http.Request) *Fault
	// Requests records "METHOD path" for every request received.
	Requests []string
}

// New starts a fake with Laamo's tables and the insert_cards function.
// The server is closed when the test ends.
func New(t interface{ Cleanup(func()) }) *Fake {
	f := &Fake{
		Key:     "eb_sk_test",
		clock:   time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC),
		tables:  laamoTables(),
		rows:    map[string][]Row{},
		Objects: map[string][]byte{},
		rpcs:    map[string]RPCFunc{"insert_cards": insertCards, "claim_jobs": claimJobs, "extend_lease": extendLease, "finish_job": finishJob, "create_workspace": createWorkspace, "add_brain_usage": addBrainUsage},
	}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Server.Close)
	return f
}

func (f *Fake) URL() string { return f.Server.URL }

// Now returns the fake's clock and advances it by a millisecond, so every
// created_at is unique and increasing.
func (f *Fake) Now() string {
	f.clock = f.clock.Add(time.Millisecond)
	return f.clock.Format(TimeFormat)
}

// Advance moves the fake's clock forward (lease expiry, backoff).
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clock = f.clock.Add(d)
}

// Put inserts a seed row (with defaults) and returns it. It panics on a
// unique conflict: seeds are test setup.
func (f *Fake) Put(tbl string, row Row) Row {
	f.mu.Lock()
	defer f.mu.Unlock()
	stored, err := f.insert(tbl, row)
	if err != nil {
		panic(fmt.Sprintf("eurobasetest: seed %s: %v", tbl, err))
	}
	return stored
}

// Rows returns a copy of a table's rows.
func (f *Fake) Rows(tbl string) []Row {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Row, len(f.rows[tbl]))
	for i, r := range f.rows[tbl] {
		out[i] = copyRow(r)
	}
	return out
}

// HandleRPC adds or replaces a database function.
func (f *Fake) HandleRPC(name string, fn RPCFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rpcs[name] = fn
}

// Inject adds a fault rule. It is asked for every request until it returns
// a fault; then it is removed (one fault per rule).
func (f *Fake) Inject(rule func(*http.Request) *Fault) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.faults = append(f.faults, rule)
}

// FailNext makes the next request matching method and path prefix fail.
func (f *Fake) FailNext(method, pathPrefix string, fault Fault) {
	f.Inject(func(r *http.Request) *Fault {
		if r.Method == method && strings.HasPrefix(r.URL.Path, pathPrefix) {
			return &fault
		}
		return nil
	})
}

// ---- HTTP ----

type response struct {
	status int
	body   any
}

func (f *Fake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Requests = append(f.Requests, r.Method+" "+r.URL.Path)

	var fault *Fault
	for i, rule := range f.faults {
		if fault = rule(r); fault != nil {
			f.faults = slices.Delete(f.faults, i, i+1)
			break
		}
	}
	if fault != nil && !fault.After {
		writeFault(w, fault)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/storage/") && r.Header.Get("apikey") == f.Key {
		f.storage(w, r)
		return
	}
	res := f.route(r)
	if fault != nil {
		writeFault(w, fault)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(res.status)
	if res.body != nil {
		_ = json.NewEncoder(w).Encode(res.body)
	}
}

func writeFault(w http.ResponseWriter, fault *Fault) {
	if fault.RetryAfter != "" {
		w.Header().Set("Retry-After", fault.RetryAfter)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(fault.Status)
	_ = json.NewEncoder(w).Encode(Row{"error": http.StatusText(fault.Status)})
}

func errBody(status int, msg string) response {
	return response{status: status, body: Row{"error": msg}}
}

func (f *Fake) route(r *http.Request) response {
	if r.Header.Get("apikey") != f.Key {
		return errBody(http.StatusUnauthorized, "invalid API key")
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/db/"), "/")
	if !strings.HasPrefix(r.URL.Path, "/v1/db/") || len(parts) > 2 || parts[0] == "" {
		return errBody(http.StatusNotFound, "no route")
	}
	if parts[0] == "rpc" && len(parts) == 2 && r.Method == http.MethodPost {
		return f.rpc(parts[1], r)
	}
	tbl := parts[0]
	if _, ok := f.tables[tbl]; !ok {
		return errBody(http.StatusNotFound, fmt.Sprintf("table %q does not exist", tbl))
	}
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		return f.list(tbl, r)
	case len(parts) == 1 && r.Method == http.MethodPost:
		var row Row
		if err := json.NewDecoder(r.Body).Decode(&row); err != nil {
			return errBody(http.StatusBadRequest, "invalid request body")
		}
		stored, err := f.insert(tbl, row)
		if err != nil {
			return errBody(http.StatusConflict, err.Error())
		}
		return response{http.StatusCreated, stored}
	case len(parts) == 2 && r.Method == http.MethodGet:
		if i := f.find(tbl, parts[1]); i >= 0 {
			return response{http.StatusOK, f.rows[tbl][i]}
		}
		return errBody(http.StatusNotFound, "row not found")
	case len(parts) == 2 && r.Method == http.MethodPatch:
		var patch Row
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			return errBody(http.StatusBadRequest, "invalid request body")
		}
		i := f.find(tbl, parts[1])
		if i < 0 {
			return errBody(http.StatusNotFound, "row not found")
		}
		updated := copyRow(f.rows[tbl][i])
		maps.Copy(updated, patch)
		if err := f.checkUnique(tbl, updated, i); err != nil {
			return errBody(http.StatusConflict, err.Error())
		}
		if _, ok := updated["updated_at"]; ok {
			updated["updated_at"] = f.Now()
		}
		f.rows[tbl][i] = updated
		return response{http.StatusOK, updated}
	case len(parts) == 2 && r.Method == http.MethodDelete:
		i := f.find(tbl, parts[1])
		if i < 0 {
			return errBody(http.StatusNotFound, "row not found")
		}
		f.rows[tbl] = slices.Delete(f.rows[tbl], i, i+1)
		if tbl == "helpers" { // ON DELETE CASCADE, like the migrations
			for _, child := range []string{"helper_versions", "triggers", "runs", "feed_cards"} {
				f.rows[child] = slices.DeleteFunc(f.rows[child], func(r Row) bool { return r["helper_id"] == parts[1] })
			}
		}
		return response{status: http.StatusNoContent}
	}
	return errBody(http.StatusMethodNotAllowed, "method not allowed")
}

func (f *Fake) rpc(name string, r *http.Request) response {
	fn, ok := f.rpcs[name]
	if !ok {
		return errBody(http.StatusNotFound, fmt.Sprintf("function %s does not exist", name))
	}
	args := map[string]json.RawMessage{}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
			return errBody(http.StatusBadRequest, "invalid request body")
		}
	}
	res, err := fn(f, args)
	var pgErr *PGError
	switch {
	case errors.As(err, &pgErr):
		status := rpcStatus(pgErr.Code)
		if status == http.StatusInternalServerError {
			return errBody(status, "internal server error")
		}
		return response{status, Row{"error": pgErr.Message, "code": pgErr.Code}}
	case err != nil:
		return errBody(http.StatusInternalServerError, "internal server error")
	}
	return response{http.StatusOK, Row{"result": res}}
}

// ---- table operations (caller holds f.mu) ----

func (f *Fake) find(tbl, id string) int {
	return slices.IndexFunc(f.rows[tbl], func(r Row) bool { return r["id"] == id })
}

// Insert adds a row with defaults and unique checks. For RPCFuncs, which
// run with the fake's lock held.
func (f *Fake) Insert(tbl string, row Row) (Row, error) { return f.insert(tbl, row) }

// Table returns a table's live rows. For RPCFuncs only (lock held).
func (f *Fake) Table(tbl string) []Row { return f.rows[tbl] }

func (f *Fake) insert(tbl string, row Row) (Row, error) {
	t := f.tables[tbl]
	stored := Row{}
	if t.defaults != nil {
		stored = t.defaults(f.Now())
	}
	maps.Copy(stored, row)
	if _, ok := stored["id"]; !ok {
		stored["id"] = fmt.Sprintf("00000000-0000-4000-8000-%012d", len(f.rows[tbl])+1)
	}
	if err := f.checkUnique(tbl, stored, -1); err != nil {
		return nil, err
	}
	f.rows[tbl] = append(f.rows[tbl], stored)
	return copyRow(stored), nil
}

func (f *Fake) checkUnique(tbl string, row Row, skip int) error {
	sets := append([][]string{{"id"}}, f.tables[tbl].uniques...)
	for _, cols := range sets {
		for i, o := range f.rows[tbl] {
			if i == skip {
				continue
			}
			same := true
			for _, c := range cols {
				if row[c] == nil || fmt.Sprint(row[c]) != fmt.Sprint(o[c]) {
					same = false
					break
				}
			}
			if same {
				return fmt.Errorf("a record with this %s already exists", strings.Join(cols, ", "))
			}
		}
	}
	return nil
}

func (f *Fake) list(tbl string, r *http.Request) response {
	q := r.URL.Query()
	var out []Row
	for _, row := range f.rows[tbl] {
		if matches(row, q) {
			out = append(out, row)
		}
	}
	if order := q.Get("order"); order != "" {
		keys := strings.Split(order, ",")
		slices.SortStableFunc(out, func(a, b Row) int {
			for _, k := range keys {
				col, dir, _ := strings.Cut(k, ".")
				c := compare(a[col], b[col])
				if dir == "desc" {
					c = -c
				}
				if c != 0 {
					return c
				}
			}
			return 0
		})
	}
	count := len(out)
	limit, offset := 20, 0
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 {
		limit = min(n, 1000)
	}
	if n, err := strconv.Atoi(q.Get("offset")); err == nil && n >= 0 {
		offset = n
	}
	out = out[min(offset, len(out)):]
	out = out[:min(limit, len(out))]
	if out == nil {
		out = []Row{}
	}
	return response{http.StatusOK, Row{"data": out, "count": count}}
}

func matches(row Row, q map[string][]string) bool {
	for col, vals := range q {
		switch col {
		case "select", "order", "limit", "offset", "aggregate":
			continue
		}
		for _, v := range vals {
			op, val, ok := strings.Cut(v, ".")
			if !ok {
				op, val = "eq", v
			}
			cell := row[col]
			switch op {
			case "eq":
				if cell == nil || fmt.Sprint(cell) != val {
					return false
				}
			case "neq":
				if cell == nil || fmt.Sprint(cell) == val {
					return false
				}
			case "in":
				set := strings.Split(strings.Trim(val, "()"), ",")
				if cell == nil || !slices.Contains(set, fmt.Sprint(cell)) {
					return false
				}
			case "is":
				if val == "null" && cell != nil {
					return false
				}
			case "lt", "lte", "gt", "gte":
				if cell == nil {
					return false
				}
				c := compare(cell, parseScalar(val))
				if (op == "lt" && c >= 0) || (op == "lte" && c > 0) || (op == "gt" && c <= 0) || (op == "gte" && c < 0) {
					return false
				}
			default:
				return false
			}
		}
	}
	return true
}

func parseScalar(s string) any {
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		return n
	}
	return s
}

func compare(a, b any) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1 // NULLS LAST for asc, like Postgres
	case b == nil:
		return -1
	}
	if x, ok := a.(float64); ok {
		if y, ok := b.(float64); ok {
			switch {
			case x < y:
				return -1
			case x > y:
				return 1
			}
			return 0
		}
	}
	// timestamps: normalise to the fixed format before comparing
	as, bs := fmt.Sprint(a), fmt.Sprint(b)
	if ta, err := time.Parse(time.RFC3339Nano, as); err == nil {
		if tb, err := time.Parse(time.RFC3339Nano, bs); err == nil {
			return ta.Compare(tb)
		}
	}
	return strings.Compare(as, bs)
}

func copyRow(r Row) Row {
	b, _ := json.Marshal(r)
	var out Row
	_ = json.Unmarshal(b, &out)
	return out
}

// ---- Laamo schema ----

func laamoTables() map[string]table {
	ts := func(cols ...string) func(string) Row {
		return func(now string) Row {
			r := Row{}
			for _, c := range cols {
				r[c] = now
			}
			return r
		}
	}
	with := func(base func(string) Row, extra Row) func(string) Row {
		return func(now string) Row {
			r := base(now)
			maps.Copy(r, extra)
			return copyRow(r)
		}
	}
	return map[string]table{
		"workspaces": {defaults: with(ts("created_at", "updated_at"), Row{"timezone": "Europe/Berlin", "locale": "en", "plan": "free", "profile": Row{}, "needs": []any{}})},
		"members":    {defaults: ts("created_at"), uniques: [][]string{{"workspace_id", "user_id"}}},
		"places":     {defaults: with(ts("created_at"), Row{"meta": Row{}})},
		"brains": {defaults: func(now string) Row {
			r := Row{"created_at": now, "updated_at": now, "mode": "included", "status": "ok",
				"allowance_credits": 500.0, "used_credits_this_period": 0.0}
			t, _ := time.Parse(TimeFormat, now)
			r["period_start"] = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC).Format(TimeFormat)
			return r
		}, uniques: [][]string{{"workspace_id"}}},
		"helpers":         {defaults: with(ts("created_at", "updated_at"), Row{"status": "draft", "created_via": "scratch"})},
		"helper_versions": {defaults: ts("created_at"), uniques: [][]string{{"helper_id", "version"}}},
		"triggers":        {defaults: with(ts("created_at", "updated_at"), Row{"config": Row{}}), uniques: [][]string{{"inbound_address"}, {"webhook_token"}}},
		"runs": {defaults: with(ts("created_at"), Row{"trigger_data": Row{}, "test_mode": false, "status": "queued",
			"next_brick_index": 0.0, "cost_eur": 0.0, "tokens_in": 0.0, "tokens_out": 0.0})},
		"run_steps": {defaults: with(ts("started_at"), Row{"attempt": 1.0, "status": "running", "cards_in": 0.0, "cards_out": 0.0,
			"cost_eur": 0.0, "log": []any{}}), uniques: [][]string{{"run_id", "brick_id", "attempt"}}},
		"cards":             {defaults: with(ts("created_at"), Row{"position": 0.0, "data": Row{}, "trust": Row{}}), uniques: [][]string{{"step_id", "card_id"}}},
		"feed_cards":        {defaults: with(ts("created_at"), Row{"state": "open", "payload": Row{}})},
		"lists":             {defaults: with(ts("created_at", "updated_at"), Row{"columns": []any{}}), uniques: [][]string{{"workspace_id", "name"}, {"share_token"}}},
		"list_rows":         {defaults: with(ts("created_at"), Row{"data": Row{}}), uniques: [][]string{{"list_id", "dedupe_key"}}},
		"usage_ledger":      {defaults: with(ts("created_at"), Row{"units": 0.0, "brain_credits": 0.0, "est_cost_eur": 0.0})},
		"outbound_messages": {defaults: with(ts("created_at"), Row{"status": "pending", "media_file_ids": []any{}}), uniques: [][]string{{"idempotency_key"}}},
		"jobs": {defaults: with(ts("created_at", "run_after"), Row{"user_id": "00000000-0000-0000-0000-000000000000",
			"payload": Row{}, "status": "queued", "attempts": 0.0, "max_attempts": 5.0})},
	}
}

// insertCards mirrors migrations/0005 insert_cards: all-or-nothing, position
// defaults to input order, returns [{id, card_id}] by position.
func insertCards(f *Fake, args map[string]json.RawMessage) (any, error) {
	var rows []Row
	if err := json.Unmarshal(args["rows"], &rows); err != nil {
		return nil, &PGError{Code: "22023", Message: "rows must be a jsonb array"}
	}
	before := len(f.rows["cards"])
	var out []Row
	for i, row := range rows {
		for _, c := range []string{"workspace_id", "user_id", "run_id", "step_id", "card_id", "kind"} {
			if row[c] == nil {
				f.rows["cards"] = f.rows["cards"][:before]
				return nil, &PGError{Code: "23502", Message: fmt.Sprintf("null value in column %q violates not-null constraint", c)}
			}
		}
		if row["position"] == nil {
			row["position"] = float64(i)
		}
		stored, err := f.insert("cards", row)
		if err != nil {
			f.rows["cards"] = f.rows["cards"][:before] // roll back the whole call
			return nil, &PGError{Code: "23505", Message: "duplicate key value violates unique constraint: " + err.Error()}
		}
		out = append(out, Row{"id": stored["id"], "card_id": stored["card_id"], "position": stored["position"]})
	}
	slices.SortStableFunc(out, func(a, b Row) int { return compare(a["position"], b["position"]) })
	for _, r := range out {
		delete(r, "position")
	}
	if out == nil {
		out = []Row{}
	}
	return out, nil
}

// storage serves POST /v1/storage/upload (multipart: key, file) and
// GET/DELETE /v1/storage/{key}.
func (f *Fake) storage(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/v1/storage/")
	switch {
	case key == "upload" && r.Method == http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 50<<20)
		if err := r.ParseMultipartForm(1 << 20); err != nil { //nolint:gosec // G120: body capped above; test server
			http.Error(w, `{"error":"request must be multipart/form-data"}`, http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, `{"error":"file field is required"}`, http.StatusBadRequest)
			return
		}
		defer func() { _ = file.Close() }()
		data, _ := io.ReadAll(file)
		k := r.FormValue("key")
		f.Objects[k] = data
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(Row{"key": k, "size": len(data)})
	case r.Method == http.MethodGet:
		data, ok := f.Objects[key]
		if !ok {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		_, _ = w.Write(data)
	case r.Method == http.MethodDelete:
		if _, ok := f.Objects[key]; !ok {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		delete(f.Objects, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, `{"error":"no route"}`, http.StatusNotFound)
	}
}
