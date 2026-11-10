package eurobase

import (
	"net/url"
	"strconv"
	"strings"
)

// Query builds Eurobase list parameters (PostgREST style):
// ?status=eq.waiting&order=created_at.desc&limit=20.
// Filters on the same or different columns are ANDed.
type Query struct {
	v     url.Values
	order []string
}

func NewQuery() *Query { return &Query{v: url.Values{}} }

func (q *Query) filter(col, op, val string) *Query {
	q.v.Add(col, op+"."+val)
	return q
}

func (q *Query) Eq(col, val string) *Query  { return q.filter(col, "eq", val) }
func (q *Query) Neq(col, val string) *Query { return q.filter(col, "neq", val) }
func (q *Query) Lt(col, val string) *Query  { return q.filter(col, "lt", val) }
func (q *Query) Lte(col, val string) *Query { return q.filter(col, "lte", val) }
func (q *Query) Gt(col, val string) *Query  { return q.filter(col, "gt", val) }
func (q *Query) Gte(col, val string) *Query { return q.filter(col, "gte", val) }

// In matches any of vals. Values must not contain commas or parentheses
// (fine for ids and enum values).
func (q *Query) In(col string, vals ...string) *Query {
	return q.filter(col, "in", "("+strings.Join(vals, ",")+")")
}

// IsNull matches NULL values.
func (q *Query) IsNull(col string) *Query { return q.filter(col, "is", "null") }

// Order adds a sort column; call again for tie-breakers.
func (q *Query) Order(col string, desc bool) *Query {
	if desc {
		col += ".desc"
	} else {
		col += ".asc"
	}
	q.order = append(q.order, col)
	return q
}

func (q *Query) Select(cols ...string) *Query {
	q.v.Set("select", strings.Join(cols, ","))
	return q
}

// Limit caps one page (the API allows at most 1000).
func (q *Query) Limit(n int) *Query {
	q.v.Set("limit", strconv.Itoa(n))
	return q
}

func (q *Query) Offset(n int) *Query {
	q.v.Set("offset", strconv.Itoa(n))
	return q
}

func (q *Query) clone() *Query {
	c := &Query{v: url.Values{}, order: append([]string(nil), q.order...)}
	for k, vs := range q.v {
		c.v[k] = append([]string(nil), vs...)
	}
	return c
}

func (q *Query) values() url.Values {
	if q == nil {
		return nil
	}
	v := q.clone().v
	if len(q.order) > 0 {
		v.Set("order", strings.Join(q.order, ","))
	}
	return v
}
