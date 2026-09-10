// Package auth implements registration, login, token refresh, logout and /me.
package auth

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/mjlxiaoma/TripWeave/apps/api/internal/user"
	phttp "github.com/mjlxiaoma/TripWeave/apps/api/pkg/http"
)

// Handler wires auth HTTP endpoints.
type Handler struct {
	users      *user.Repository
	refresh    *RefreshStore
	secret     string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// NewHandler creates the auth handler.
func NewHandler(users *user.Repository, refresh *RefreshStore, secret string, accessTTL, refreshTTL time.Duration) *Handler {
	return &Handler{users: users, refresh: refresh, secret: secret, accessTTL: accessTTL, refreshTTL: refreshTTL}
}

// Routes returns the auth router mounted at /auth plus /me.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/auth/register", h.register)
	r.Post("/auth/login", h.login)
	r.Post("/auth/refresh", h.refreshTokens)
	return r
}

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type tokenResponse struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	TokenType    string  `json:"token_type"`
	ExpiresIn    int64   `json:"expires_in"`
	User         userDTO `json:"user"`
}

type userDTO struct {
	ID          string  `json:"id"`
	Email       string  `json:"email"`
	DisplayName string  `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
}

func toUserDTO(u *user.User) userDTO {
	return userDTO{ID: u.ID, Email: u.Email, DisplayName: u.DisplayName, AvatarURL: u.AvatarURL}
}

func validateEmail(email string) bool {
	email = strings.TrimSpace(email)
	if len(email) < 3 || len(email) > 254 {
		return false
	}
	at := strings.Index(email, "@")
	return at > 0 && at < len(email)-1 && strings.Contains(email[at:], ".")
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeJSON(r, &req); err != nil {
		phttp.Fail(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if !validateEmail(req.Email) {
		phttp.Fail(w, http.StatusBadRequest, "INVALID_EMAIL", "invalid email format")
		return
	}
	if len(req.Password) < 8 || len(req.Password) > 72 {
		phttp.Fail(w, http.StatusBadRequest, "WEAK_PASSWORD", "password must be 8-72 characters")
		return
	}
	if req.DisplayName == "" || len([]rune(req.DisplayName)) > 50 {
		phttp.Fail(w, http.StatusBadRequest, "INVALID_DISPLAY_NAME", "display name must be 1-50 characters")
		return
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to hash password")
		return
	}
	u, err := h.users.Create(r.Context(), req.Email, hash, req.DisplayName)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			phttp.Fail(w, http.StatusConflict, "EMAIL_TAKEN", "email already registered")
			return
		}
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to create user")
		return
	}
	h.issueTokens(w, r, u, http.StatusCreated)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		phttp.Fail(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}
	u, err := h.users.ByEmail(r.Context(), strings.TrimSpace(req.Email))
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			phttp.Fail(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "email or password is incorrect")
			return
		}
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to load user")
		return
	}
	if u.Status != "active" {
		phttp.Fail(w, http.StatusForbidden, "ACCOUNT_DISABLED", "account is not active")
		return
	}
	if !VerifyPassword(req.Password, u.PasswordHash) {
		phttp.Fail(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "email or password is incorrect")
		return
	}
	h.issueTokens(w, r, u, http.StatusOK)
}

func (h *Handler) refreshTokens(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := decodeJSON(r, &req); err != nil || req.RefreshToken == "" {
		phttp.Fail(w, http.StatusBadRequest, "INVALID_BODY", "refresh_token is required")
		return
	}
	userID, err := h.refresh.Consume(r.Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, ErrRefreshNotFound) {
			phttp.Fail(w, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", "refresh token is invalid or expired")
			return
		}
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to validate refresh token")
		return
	}
	u, err := h.users.ByID(r.Context(), userID)
	if err != nil {
		phttp.Fail(w, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", "user no longer exists")
		return
	}
	h.issueTokens(w, r, u, http.StatusOK)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromContext(r.Context())
	if userID == "" {
		// Allow logout without a valid access token: revoke by refresh token if given.
		var req refreshRequest
		if err := decodeJSON(r, &req); err == nil && req.RefreshToken != "" {
			if uid, err := h.refresh.Consume(r.Context(), req.RefreshToken); err == nil {
				userID = uid
			}
		}
	}
	if userID != "" {
		if err := h.refresh.RevokeAll(r.Context(), userID); err != nil {
			phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to revoke tokens")
			return
		}
	}
	phttp.OK(w, http.StatusOK, map[string]bool{"logged_out": true})
}

func (h *Handler) issueTokens(w http.ResponseWriter, r *http.Request, u *user.User, status int) {
	access, err := IssueAccessToken(h.secret, u.ID, h.accessTTL)
	if err != nil {
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to issue token")
		return
	}
	refresh, err := NewRefreshToken()
	if err != nil {
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to issue token")
		return
	}
	if err := h.refresh.Save(r.Context(), u.ID, refresh); err != nil {
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to store refresh token")
		return
	}
	phttp.OK(w, status, tokenResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    int64(h.accessTTL.Seconds()),
		User:         toUserDTO(u),
	})
}

func decodeJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return errors.New("empty body")
	}
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	dec := jsonDecoder(r.Body)
	return dec.Decode(v)
}
