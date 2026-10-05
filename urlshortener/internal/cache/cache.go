// Package cache defines a read-through cache abstraction for URL lookups and a
// Redis-backed implementation. The interface enables DI so the service and its
// tests depend on an abstraction, not go-redis directly.
//
// Caching is applied on the resolve path (GET /:code): that is the hot path of
// a URL shortener (links are clicked far more often than they are created), so
// avoiding a DB round-trip there gives the biggest win.
package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

// ErrCacheMiss is returned by Get when the key is not present.
var ErrCacheMiss = errors.New("cache miss")

// Cache is the contract used by the URL service.
type Cache interface {
	// Get returns the cached value for key, or ErrCacheMiss if absent.
	Get(ctx context.Context, key string) (string, error)
	// Set stores value under key with the given TTL.
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	// Del removes key. Used to invalidate entries.
	Del(ctx context.Context, key string) error
}

// Noop is a Cache that always misses and never stores. Used when caching is
// disabled in config, so callers don't need a nil-check.
type Noop struct{}

func (Noop) Get(context.Context, string) (string, error)              { return "", ErrCacheMiss }
func (Noop) Set(context.Context, string, string, time.Duration) error { return nil }
func (Noop) Del(context.Context, string) error                        { return nil }

// Redis implements Cache on top of a go-redis client.
type Redis struct {
	client *redis.Client
	log    *logrus.Entry
}

// NewRedis returns a Redis cache backed by the given client.
func NewRedis(client *redis.Client, log *logrus.Entry) *Redis {
	return &Redis{client: client, log: log}
}

// Get fetches a key. redis.Nil is mapped to ErrCacheMiss.
func (r *Redis) Get(ctx context.Context, key string) (string, error) {
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", ErrCacheMiss
	}

	if err != nil {
		r.log.WithError(err).WithField("key", key).Warn("cache get failed")

		return "", err
	}

	return val, nil
}

// Set stores a key with a TTL.
func (r *Redis) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if err := r.client.Set(ctx, key, value, ttl).Err(); err != nil {
		r.log.WithError(err).WithField("key", key).Warn("cache set failed")

		return err
	}

	return nil
}

// Del removes a key.
func (r *Redis) Del(ctx context.Context, key string) error {
	if err := r.client.Del(ctx, key).Err(); err != nil {
		r.log.WithError(err).WithField("key", key).Warn("cache del failed")

		return err
	}

	return nil
}
