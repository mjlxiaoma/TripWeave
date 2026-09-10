package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrRefreshNotFound is returned when a refresh token is unknown, expired or revoked.
var ErrRefreshNotFound = errors.New("refresh token not found")

// RefreshStore persists hashed opaque refresh tokens in Redis.
// Layout: "refresh:token:<sha256>" -> userID  (TTL = refresh lifetime)
//         "refresh:user:<userID>" -> set of hashes (for revoke-all)
type RefreshStore struct {
	rdb *redis.Client
	ttl time.Duration
}

// NewRefreshStore creates a refresh token store.
func NewRefreshStore(rdb *redis.Client, ttl time.Duration) *RefreshStore {
	return &RefreshStore{rdb: rdb, ttl: ttl}
}

func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func tokenKey(hash string) string { return "refresh:token:" + hash }
func userKey(userID string) string { return "refresh:user:" + userID }

// Save stores a refresh token mapped to its user.
func (s *RefreshStore) Save(ctx context.Context, userID, token string) error {
	hash := HashToken(token)
	pipe := s.rdb.Pipeline()
	pipe.Set(ctx, tokenKey(hash), userID, s.ttl)
	pipe.SAdd(ctx, userKey(userID), hash)
	pipe.Expire(ctx, userKey(userID), s.ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("save refresh token: %w", err)
	}
	return nil
}

// Consume validates a refresh token, resolves its user and removes it (rotation).
func (s *RefreshStore) Consume(ctx context.Context, token string) (string, error) {
	hash := HashToken(token)
	pipe := s.rdb.Pipeline()
	get := pipe.Get(ctx, tokenKey(hash))
	del := pipe.Del(ctx, tokenKey(hash))
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return "", fmt.Errorf("consume refresh token: %w", err)
	}
	userID, err := get.Result()
	if errors.Is(err, redis.Nil) || del.Val() == 0 {
		return "", ErrRefreshNotFound
	}
	if err != nil {
		return "", fmt.Errorf("consume refresh token: %w", err)
	}
	s.rdb.SRem(ctx, userKey(userID), hash)
	return userID, nil
}

// RevokeAll removes every refresh token for the user (logout).
func (s *RefreshStore) RevokeAll(ctx context.Context, userID string) error {
	hashes, err := s.rdb.SMembers(ctx, userKey(userID)).Result()
	if err != nil {
		return fmt.Errorf("list refresh tokens: %w", err)
	}
	if len(hashes) == 0 {
		return nil
	}
	pipe := s.rdb.Pipeline()
	for _, h := range hashes {
		pipe.Del(ctx, tokenKey(h))
	}
	pipe.Del(ctx, userKey(userID))
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("revoke refresh tokens: %w", err)
	}
	return nil
}
