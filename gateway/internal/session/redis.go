package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisKeyPrefix namespaces gateway keys so the instance can be shared.
const redisKeyPrefix = "openussd:session:"

// Redis is a session store backed by Redis, for deployments running more
// than one gateway replica. Expiry is Redis's own TTL, refreshed on every
// Save, so an abandoned dialogue costs nothing to clean up.
type Redis struct {
	client redis.UniversalClient
	ttl    time.Duration
	// owned records whether we created the client and must close it.
	owned bool
}

// RedisOption configures a Redis store.
type RedisOption func(*Redis)

// WithRedisTTL overrides the idle lifetime.
func WithRedisTTL(ttl time.Duration) RedisOption {
	return func(r *Redis) { r.ttl = ttl }
}

// NewRedis connects to the Redis instance at url (redis://host:port/db) and
// verifies the connection before returning, so a bad address fails at
// startup rather than on the first user's first screen.
func NewRedis(ctx context.Context, url string, opts ...RedisOption) (*Redis, error) {
	cfg, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("session: parsing redis url: %w", err)
	}

	store := NewRedisWithClient(redis.NewClient(cfg), opts...)
	store.owned = true

	if err := store.client.Ping(ctx).Err(); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("session: connecting to redis: %w", err)
	}
	return store, nil
}

// NewRedisWithClient wraps an existing client. The caller keeps ownership:
// Close does not shut the client down.
func NewRedisWithClient(client redis.UniversalClient, opts ...RedisOption) *Redis {
	r := &Redis{client: client, ttl: DefaultTTL}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Load implements Store.
func (r *Redis) Load(ctx context.Context, key Key) (Session, error) {
	if err := key.Validate(); err != nil {
		return Session{}, err
	}

	raw, err := r.client.Get(ctx, redisKeyPrefix+key.String()).Bytes()
	if errors.Is(err, redis.Nil) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("session: redis get: %w", err)
	}

	var s Session
	if err := json.Unmarshal(raw, &s); err != nil {
		// Unreadable state is treated as no state: the user gets a fresh
		// dialogue instead of a dead shortcode. The error is returned so
		// the caller can log it, because this should never happen.
		return Session{}, fmt.Errorf("session: decoding stored session: %w", err)
	}
	return s, nil
}

// Save implements Store.
func (r *Redis) Save(ctx context.Context, s Session) error {
	if err := s.Key.Validate(); err != nil {
		return err
	}

	now := time.Now().UTC()
	s.UpdatedAt = now
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}

	raw, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("session: encoding session: %w", err)
	}
	if err := r.client.Set(ctx, redisKeyPrefix+s.Key.String(), raw, r.ttl).Err(); err != nil {
		return fmt.Errorf("session: redis set: %w", err)
	}
	return nil
}

// Delete implements Store.
func (r *Redis) Delete(ctx context.Context, key Key) error {
	if err := r.client.Del(ctx, redisKeyPrefix+key.String()).Err(); err != nil {
		return fmt.Errorf("session: redis del: %w", err)
	}
	return nil
}

// Close shuts down the client if this store created it.
func (r *Redis) Close() error {
	if !r.owned {
		return nil
	}
	return r.client.Close()
}
