package planner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrBusy means another generation is already running for the trip.
var ErrBusy = errors.New("an AI generation is already running for this trip")

const lockTTL = 120 * time.Second

// lockToken returns a random token identifying this lock holder.
func lockToken() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// releaseScript deletes the lock only when it is still ours (compare-and-del),
// so an expired-and-reacquired lock is never freed by a stale holder.
var releaseScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("del", KEYS[1])
else
	return 0
end`)

// RedisLocker serializes AI generations per trip via Redis (works across
// replicas; auto-expires if the holder crashes).
type RedisLocker struct {
	rdb *redis.Client
}

// NewRedisLocker creates the Redis-backed locker.
func NewRedisLocker(rdb *redis.Client) *RedisLocker {
	return &RedisLocker{rdb: rdb}
}

// Acquire takes the per-trip lock or returns ErrBusy.
func (l *RedisLocker) Acquire(ctx context.Context, tripID string) (Lock, error) {
	key := "lock:ai:trip:" + tripID
	token := lockToken()
	ok, err := l.rdb.SetNX(ctx, key, token, lockTTL).Result()
	if err != nil {
		return nil, fmt.Errorf("acquire ai lock: %w", err)
	}
	if !ok {
		return nil, ErrBusy
	}
	return &tripLock{rdb: l.rdb, key: key, token: token}, nil
}

// tripLock is a held Redis lock.
type tripLock struct {
	rdb   *redis.Client
	key   string
	token string
}

// Release frees the lock (best-effort; the TTL is the ultimate backstop).
func (l *tripLock) Release(ctx context.Context) {
	_ = releaseScript.Run(ctx, l.rdb, []string{l.key}, l.token).Err()
}
