package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/redis/go-redis/v9"
)

// Verification code policy.
const (
	verifyCodeTTL      = 15 * time.Minute
	verifyMaxAttempts  = 5
	verifyResendCool   = 60 * time.Second
	verifyCodeKeyPfx   = "verify:code:"
	verifyCoolKeyPfx   = "verify:cooldown:"
	verifyAttKeyPfx    = "verify:attempts:"
)

// ErrBadCode is returned when the code is wrong, expired or attempts are exhausted.
var ErrBadCode = errors.New("invalid or expired verification code")

// ErrResendTooSoon is returned when a resend is requested inside the cooldown window.
var ErrResendTooSoon = errors.New("resend requested too soon")

// VerificationStore keeps hashed 6-digit email codes in Redis.
type VerificationStore struct {
	rdb *redis.Client
}

// NewVerificationStore creates the store.
func NewVerificationStore(rdb *redis.Client) *VerificationStore {
	return &VerificationStore{rdb: rdb}
}

// NewCode generates a random 6-digit code.
func NewCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", fmt.Errorf("generate code: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// Save stores the hashed code for an email, resetting attempts and starting the resend cooldown.
func (s *VerificationStore) Save(ctx context.Context, email, code string) error {
	pipe := s.rdb.Pipeline()
	pipe.Set(ctx, verifyCodeKeyPfx+email, HashToken(code), verifyCodeTTL)
	pipe.Del(ctx, verifyAttKeyPfx+email)
	pipe.Set(ctx, verifyCoolKeyPfx+email, 1, verifyResendCool)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("save verification code: %w", err)
	}
	return nil
}

// CanResend reports whether a new code may be sent for the email (cooldown over).
func (s *VerificationStore) CanResend(ctx context.Context, email string) (bool, error) {
	n, err := s.rdb.Exists(ctx, verifyCoolKeyPfx+email).Result()
	if err != nil {
		return false, fmt.Errorf("check resend cooldown: %w", err)
	}
	return n == 0, nil
}

// Consume validates the code for an email and deletes it on success.
// Failed attempts are counted; after verifyMaxAttempts the code is invalidated.
func (s *VerificationStore) Consume(ctx context.Context, email, code string) error {
	stored, err := s.rdb.Get(ctx, verifyCodeKeyPfx+email).Result()
	if errors.Is(err, redis.Nil) {
		return ErrBadCode
	}
	if err != nil {
		return fmt.Errorf("load verification code: %w", err)
	}
	if stored != HashToken(code) {
		attempts, err := s.rdb.Incr(ctx, verifyAttKeyPfx+email).Result()
		if err != nil {
			return fmt.Errorf("count attempts: %w", err)
		}
		if attempts == 1 {
			s.rdb.Expire(ctx, verifyAttKeyPfx+email, verifyCodeTTL)
		}
		if attempts >= verifyMaxAttempts {
			pipe := s.rdb.Pipeline()
			pipe.Del(ctx, verifyCodeKeyPfx+email)
			pipe.Del(ctx, verifyAttKeyPfx+email)
			_, _ = pipe.Exec(ctx)
		}
		return ErrBadCode
	}
	pipe := s.rdb.Pipeline()
	pipe.Del(ctx, verifyCodeKeyPfx+email)
	pipe.Del(ctx, verifyAttKeyPfx+email)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("consume verification code: %w", err)
	}
	return nil
}
