package session

import (
	"context"
	"sync"
	"time"
)

// Memory is an in-process session store. It is the default: a single
// gateway replica needs nothing else, and the demo runs with no Redis.
//
// Sessions do not survive a restart. That is acceptable for USSD - a
// dialogue outlives a deploy only in the seconds sense - but it is the
// reason multi-replica deployments need Redis, since two replicas would
// otherwise each hold half a conversation.
type Memory struct {
	ttl   time.Duration
	now   func() time.Time
	mu    sync.RWMutex
	items map[string]memoryItem

	stop     chan struct{}
	stopOnce sync.Once
}

type memoryItem struct {
	session   Session
	expiresAt time.Time
}

// MemoryOption configures a Memory store.
type MemoryOption func(*Memory)

// WithTTL overrides the idle lifetime.
func WithTTL(ttl time.Duration) MemoryOption {
	return func(m *Memory) { m.ttl = ttl }
}

// WithClock overrides the clock, for tests.
func WithClock(now func() time.Time) MemoryOption {
	return func(m *Memory) { m.now = now }
}

// NewMemory returns a memory store and starts its sweeper.
//
// Expiry is enforced on read as well as by the sweeper, so a session is
// never served after its deadline even if the sweep has not run yet. The
// sweeper exists to release memory, not to define correctness.
func NewMemory(opts ...MemoryOption) *Memory {
	m := &Memory{
		ttl:   DefaultTTL,
		now:   func() time.Time { return time.Now().UTC() },
		items: make(map[string]memoryItem),
		stop:  make(chan struct{}),
	}
	for _, opt := range opts {
		opt(m)
	}

	go m.sweep(m.ttl)
	return m
}

// Load implements Store.
func (m *Memory) Load(_ context.Context, key Key) (Session, error) {
	if err := key.Validate(); err != nil {
		return Session{}, err
	}

	m.mu.RLock()
	item, ok := m.items[key.String()]
	m.mu.RUnlock()

	if !ok || !m.now().Before(item.expiresAt) {
		return Session{}, ErrNotFound
	}
	return item.session, nil
}

// Save implements Store.
func (m *Memory) Save(_ context.Context, s Session) error {
	if err := s.Key.Validate(); err != nil {
		return err
	}

	now := m.now()
	s.UpdatedAt = now
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[s.Key.String()] = memoryItem{session: s, expiresAt: now.Add(m.ttl)}
	return nil
}

// Delete implements Store.
func (m *Memory) Delete(_ context.Context, key Key) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, key.String())
	return nil
}

// Close stops the sweeper. Safe to call more than once.
func (m *Memory) Close() error {
	m.stopOnce.Do(func() { close(m.stop) })
	return nil
}

// Len reports the number of entries, expired ones included. Exposed for
// metrics and tests, not for routing decisions.
func (m *Memory) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.items)
}

func (m *Memory) sweep(ttl time.Duration) {
	interval := ttl / 4
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stop:
			return
		case <-ticker.C:
			m.evictExpired()
		}
	}
}

func (m *Memory) evictExpired() {
	now := m.now()

	m.mu.Lock()
	defer m.mu.Unlock()
	for k, item := range m.items {
		if !now.Before(item.expiresAt) {
			delete(m.items, k)
		}
	}
}
