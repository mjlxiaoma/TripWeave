// Package redis provides the Redis client.
package redis

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// NewClient creates a Redis client and verifies connectivity. useTLS is for
// managed providers (Upstash etc.) that require TLS on the same host:port.
func NewClient(ctx context.Context, addr, password string, useTLS bool) (*redis.Client, error) {
	opts := &redis.Options{Addr: addr, Password: password, DB: 0}
	if useTLS {
		opts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	rdb := redis.NewClient(opts)
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return rdb, nil
}
