// Package user holds the user domain model and repository.
package user

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a user does not exist.
var ErrNotFound = errors.New("user not found")

// User is the core user entity.
type User struct {
	ID              string     `json:"id"`
	Email           string     `json:"email"`
	PasswordHash    string     `json:"-"`
	DisplayName     string     `json:"display_name"`
	AvatarURL       *string    `json:"avatar_url"`
	Status          string     `json:"status"`
	EmailVerifiedAt *time.Time `json:"email_verified_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Verified reports whether the user's email has been verified.
func (u *User) Verified() bool { return u.EmailVerifiedAt != nil }

// Repository provides user persistence.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a user repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const userCols = `id, email, password_hash, display_name, avatar_url, status, email_verified_at, created_at, updated_at`

func scanUser(row pgx.Row) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.AvatarURL, &u.Status, &u.EmailVerifiedAt, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// Create inserts a new user.
func (r *Repository) Create(ctx context.Context, email, passwordHash, displayName string) (*User, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, display_name) VALUES ($1, $2, $3) RETURNING `+userCols,
		email, passwordHash, displayName)
	u, err := scanUser(row)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return u, nil
}

// ByEmail finds a user by email (case-insensitive).
func (r *Repository) ByEmail(ctx context.Context, email string) (*User, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE lower(email) = lower($1)`, email)
	u, err := scanUser(row)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// ByID finds a user by ID.
func (r *Repository) ByID(ctx context.Context, id string) (*User, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, id)
	u, err := scanUser(row)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// MarkVerified sets email_verified_at to now for the user.
func (r *Repository) MarkVerified(ctx context.Context, id string) (*User, error) {
	row := r.pool.QueryRow(ctx,
		`UPDATE users SET email_verified_at = now(), updated_at = now() WHERE id = $1 RETURNING `+userCols, id)
	u, err := scanUser(row)
	if err != nil {
		return nil, fmt.Errorf("mark verified: %w", err)
	}
	return u, nil
}

// UpdateCredentials rewrites password hash and display name (used when an
// unverified account registers again).
func (r *Repository) UpdateCredentials(ctx context.Context, id, passwordHash, displayName string) (*User, error) {
	row := r.pool.QueryRow(ctx,
		`UPDATE users SET password_hash = $2, display_name = $3, updated_at = now() WHERE id = $1 RETURNING `+userCols,
		id, passwordHash, displayName)
	u, err := scanUser(row)
	if err != nil {
		return nil, fmt.Errorf("update credentials: %w", err)
	}
	return u, nil
}
