package engine

import (
	"context"
	"errors"
	"sync"
)

// MemoryBlobs is a Blobs for tests and local runs.
type MemoryBlobs struct {
	mu    sync.Mutex
	items map[string][]byte
}

func (m *MemoryBlobs) Put(_ context.Context, path string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.items == nil {
		m.items = map[string][]byte{}
	}
	m.items[path] = append([]byte(nil), data...)
	return nil
}

func (m *MemoryBlobs) Get(_ context.Context, path string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.items[path]
	if !ok {
		return nil, errors.New("blob not found: " + path)
	}
	return append([]byte(nil), b...), nil
}

// Len returns how many blobs are stored.
func (m *MemoryBlobs) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.items)
}
