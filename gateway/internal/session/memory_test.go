package session

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestMemoryRoundTrip(t *testing.T) {
	store := NewMemory()
	defer store.Close()

	ctx := context.Background()
	key := Key{MNO: "africastalking", SessionID: "ATUid_1"}
	want := Session{
		Key:       key,
		Tenant:    "fediverse",
		MSISDN:    "+254711223344",
		Shortcode: "*384*1234#",
		Turn:      1,
		State:     json.RawMessage(`{"screen":"menu"}`),
	}

	if err := store.Save(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Load(ctx, key)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Tenant != want.Tenant || string(got.State) != string(want.State) {
		t.Errorf("round trip lost data: got %+v", got)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Error("Save did not stamp CreatedAt/UpdatedAt")
	}
}

func TestMemoryLoadMissingIsNotFound(t *testing.T) {
	store := NewMemory()
	defer store.Close()

	_, err := store.Load(context.Background(), Key{MNO: "mno", SessionID: "nope"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestMemoryExpiresOnRead is the important one: expiry must hold even when
// the background sweeper has not run, or a user could resume a dialogue the
// network has already abandoned.
func TestMemoryExpiresOnRead(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	store := NewMemory(WithTTL(180*time.Second), WithClock(func() time.Time { return clock() }))
	defer store.Close()

	ctx := context.Background()
	key := Key{MNO: "africastalking", SessionID: "ATUid_expiring"}
	if err := store.Save(ctx, Session{Key: key}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	now = now.Add(179 * time.Second)
	if _, err := store.Load(ctx, key); err != nil {
		t.Fatalf("session expired early: %v", err)
	}

	now = now.Add(2 * time.Second)
	if _, err := store.Load(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound after TTL", err)
	}
}

// TestMemorySaveRefreshesExpiry checks the TTL is idle-based: a user
// working through a long menu must not be cut off at a fixed deadline.
func TestMemorySaveRefreshesExpiry(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	store := NewMemory(WithTTL(180*time.Second), WithClock(func() time.Time { return now }))
	defer store.Close()

	ctx := context.Background()
	key := Key{MNO: "africastalking", SessionID: "ATUid_busy"}

	for turn := 1; turn <= 5; turn++ {
		if err := store.Save(ctx, Session{Key: key, Turn: turn}); err != nil {
			t.Fatalf("Save turn %d: %v", turn, err)
		}
		now = now.Add(100 * time.Second)
	}

	got, err := store.Load(ctx, key)
	if err != nil {
		t.Fatalf("Load after 500s of activity: %v", err)
	}
	if got.Turn != 5 {
		t.Errorf("Turn = %d, want 5", got.Turn)
	}
}

func TestMemoryEvictExpiredReleasesMemory(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	store := NewMemory(WithTTL(time.Minute), WithClock(func() time.Time { return now }))
	defer store.Close()

	ctx := context.Background()
	for _, id := range []string{"a", "b", "c"} {
		if err := store.Save(ctx, Session{Key: Key{MNO: "m", SessionID: id}}); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}
	if store.Len() != 3 {
		t.Fatalf("Len = %d, want 3", store.Len())
	}

	now = now.Add(2 * time.Minute)
	store.evictExpired()
	if store.Len() != 0 {
		t.Errorf("Len = %d after eviction, want 0", store.Len())
	}
}

func TestMemoryRejectsIncompleteKeys(t *testing.T) {
	store := NewMemory()
	defer store.Close()
	ctx := context.Background()

	if err := store.Save(ctx, Session{Key: Key{SessionID: "x"}}); err == nil {
		t.Error("Save accepted a key with no MNO")
	}
	if _, err := store.Load(ctx, Key{MNO: "m"}); err == nil {
		t.Error("Load accepted a key with no session id")
	}
}

// TestMemoryConcurrentAccess exists for the race detector: one gateway
// process serves many dialogues at once.
func TestMemoryConcurrentAccess(t *testing.T) {
	store := NewMemory()
	defer store.Close()

	ctx := context.Background()
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := Key{MNO: "m", SessionID: string(rune('a' + i%8))}
			_ = store.Save(ctx, Session{Key: key, Turn: i})
			_, _ = store.Load(ctx, key)
			if i%4 == 0 {
				_ = store.Delete(ctx, key)
			}
		}(i)
	}
	wg.Wait()
}

func TestKeyString(t *testing.T) {
	if got := (Key{MNO: "africastalking", SessionID: "ATUid_1"}).String(); got != "africastalking:ATUid_1" {
		t.Errorf("String() = %q", got)
	}
}
