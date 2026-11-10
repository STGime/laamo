package store

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"maps"
	"strconv"
	"time"
)

// Num is a Postgres numeric. It decodes from a JSON number or a quoted
// number, so it doesn't matter how the API encodes numeric columns.
type Num float64

func (n *Num) UnmarshalJSON(b []byte) error {
	b = bytes.Trim(b, `"`)
	if string(b) == "null" || len(b) == 0 {
		*n = 0
		return nil
	}
	f, err := strconv.ParseFloat(string(b), 64)
	if err != nil {
		return fmt.Errorf("store.Num: %w", err)
	}
	*n = Num(f)
	return nil
}

// Nullable is a patch value that can also clear a column. In a patch, a nil
// *Nullable leaves the column alone, Null() sets it to NULL and Set(v) sets v.
type Nullable[T any] struct {
	Value T
	Valid bool
}

func Set[T any](v T) *Nullable[T] { return &Nullable[T]{Value: v, Valid: true} }
func Null[T any]() *Nullable[T]   { return &Nullable[T]{} }

func (n Nullable[T]) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(n.Value)
}

func (n *Nullable[T]) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*n = Nullable[T]{}
		return nil
	}
	n.Valid = true
	return json.Unmarshal(b, &n.Value)
}

// NewID returns a UUIDv7: time-ordered, so new rows land at the end of the
// primary key index. IDs are made client side so a retried insert can be
// recognised (same id → it already went through).
func NewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never fails (Go 1.24+)
	ms := uint64(time.Now().UnixMilli())
	var ts [8]byte
	binary.BigEndian.PutUint64(ts[:], ms)
	copy(b[0:6], ts[2:8])
	b[6] = (b[6] & 0x0f) | 0x70 // version 7
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 9562 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// ApplyPatch returns row with the patch's set fields applied. Both are
// matched by JSON name, so it works for every row/patch pair in this package.
func ApplyPatch[T any](row T, patch any) (T, error) {
	var out T
	base, err := toMap(row)
	if err != nil {
		return out, err
	}
	changes, err := toMap(patch)
	if err != nil {
		return out, err
	}
	maps.Copy(base, changes)
	b, err := json.Marshal(base)
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(b, &out)
	return out, err
}

func toMap(v any) (map[string]json.RawMessage, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	m := map[string]json.RawMessage{}
	err = json.Unmarshal(b, &m)
	return m, err
}
