package database

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// ConnectRedis opens a Redis client for the given address.
func ConnectRedis(addr string) (*redis.Client, error) {
	if addr == "" {
		return nil, fmt.Errorf("redis address is empty")
	}

	return redis.NewClient(&redis.Options{Addr: addr}), nil
}

// PingRedis verifies connectivity to Redis.
func PingRedis(ctx context.Context, client *redis.Client) error {
	return client.Ping(ctx).Err()
}
