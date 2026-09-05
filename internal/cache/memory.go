package cache

import (
	"context"
	"sync"
	"time"
)

type entry struct {
	val       []byte
	expiresAt time.Time
}

// Memory is an in-process TTL cache with a bounded entry count. It needs no
// external service, and is lost on restart.
type Memory struct {
	mu      sync.Mutex
	items   map[string]entry
	maxSize int
}

// NewMemory builds a memory store holding at most maxSize entries (<=0 means 1000).
func NewMemory(maxSize int) *Memory {
	if maxSize <= 0 {
		maxSize = 1000
	}
	return &Memory{items: make(map[string]entry), maxSize: maxSize}
}

// Get returns the value if present and unexpired.
func (m *Memory) Get(ctx context.Context, key string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.items[key]
	if !ok {
		return nil, false, nil
	}
	if time.Now().After(e.expiresAt) {
		delete(m.items, key)
		return nil, false, nil
	}
	return e.val, true, nil
}

// Set stores a value, evicting when the store is at capacity.
func (m *Memory) Set(ctx context.Context, key string, val []byte, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.items[key]; !exists && len(m.items) >= m.maxSize {
		m.evictLocked()
	}
	m.items[key] = entry{val: val, expiresAt: time.Now().Add(ttl)}
	return nil
}

// evictLocked drops expired entries first, then the entry closest to expiring.
// Callers must hold m.mu.
func (m *Memory) evictLocked() {
	now := time.Now()
	for k, e := range m.items {
		if now.After(e.expiresAt) {
			delete(m.items, k)
		}
	}
	if len(m.items) < m.maxSize {
		return
	}
	var oldestKey string
	var oldest time.Time
	for k, e := range m.items {
		if oldestKey == "" || e.expiresAt.Before(oldest) {
			oldestKey, oldest = k, e.expiresAt
		}
	}
	delete(m.items, oldestKey)
}

// Len reports the current entry count (used by tests and diagnostics).
func (m *Memory) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.items)
}

// Close releases the store's entries.
func (m *Memory) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = make(map[string]entry)
	return nil
}
