package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis stores entries in a Redis server, so the cache survives restarts and is
// shared across instances. TTLs are enforced by Redis itself.
type Redis struct {
	client  *redis.Client
	timeout time.Duration
}

// NewRedis builds a Redis-backed store. url is a standard redis:// URL. The
// timeout bounds every operation so a slow server cannot stall a request.
func NewRedis(url string, timeout time.Duration) (*Redis, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	if timeout > 0 {
		opt.DialTimeout, opt.ReadTimeout, opt.WriteTimeout = timeout, timeout, timeout
	}
	return &Redis{client: redis.NewClient(opt), timeout: timeout}, nil
}

// Get returns the value if present. redis.Nil (a plain miss) is not an error.
func (r *Redis) Get(ctx context.Context, key string) ([]byte, bool, error) {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()
	val, err := r.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("redis get: %w", err)
	}
	return val, true, nil
}

// Set stores a value with a server-side TTL.
func (r *Redis) Set(ctx context.Context, key string, val []byte, ttl time.Duration) error {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()
	if err := r.client.Set(ctx, key, val, ttl).Err(); err != nil {
		return fmt.Errorf("redis set: %w", err)
	}
	return nil
}

// Ping reports whether the server is reachable (used by /healthz).
func (r *Redis) Ping(ctx context.Context) error {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()
	return r.client.Ping(ctx).Err()
}

// Close releases the connection pool.
func (r *Redis) Close() error { return r.client.Close() }

func (r *Redis) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if r.timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, r.timeout)
}
