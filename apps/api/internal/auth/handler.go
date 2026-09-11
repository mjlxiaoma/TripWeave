// Package auth implements registration, login, token refresh, logout and /me.
package auth

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/mjlxiaoma/TripWeave/apps/api/internal/mail"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/shared"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/user"
	phttp "github.com/mjlxiaoma/TripWeave/apps/api/pkg/http"
)

// Handler wires auth HTTP endpoints.
type Handler struct {
	users      *user.Repository
	refresh    *RefreshStore
	verify     *VerificationStore
	mailer     *mail.Sender
	secret     string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// NewHandler creates the auth handler. mailer may be nil when SMTP is not
// configured; in that case codes are logged instead of emailed (dev only).
func NewHandler(users *user.Repository, refresh *RefreshStore, verify *VerificationStore, mailer *mail.Sender, secret string, accessTTL, refreshTTL time.Duration) *Handler {
	return &Handler{users: users, refresh: refresh, verify: verify, mailer: mailer, secret: secret, accessTTL: accessTTL, refreshTTL: refreshTTL}
}

// Routes returns the auth router mounted at /auth plus /me.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/auth/register", h.register)
	r.Post("/auth/login", h.login)
	r.Post("/auth/refresh", h.refreshTokens)
	r.Post("/auth/verify-email", h.verifyEmail)
	r.Post("/auth/resend-verification", h.resendVerification)
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

type verifyEmailRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

type resendRequest struct {
	Email string `json:"email"`
}

type tokenResponse struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	TokenType    string  `json:"token_type"`
	ExpiresIn    int64   `json:"expires_in"`
	User         userDTO `json:"user"`
}

type registerResponse struct {
	NeedsVerification bool   `json:"needs_verification"`
	Email             string `json:"email"`
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
	if at <= 0 || at != strings.LastIndex(email, "@") {
		return false
	}
	local, domain := email[:at], email[at+1:]
	if local == "" || domain == "" {
		return false
	}
	// 域名至少一个点，且点两侧非空；本地与域名均不含空白/控制字符
	dot := strings.LastIndex(domain, ".")
	if dot <= 0 || dot == len(domain)-1 {
		return false
	}
	return !strings.ContainsAny(email, " \t\r\n\f\v")
}

// sendCode generates a code, stores it and emails it (or logs it when SMTP is
// not configured). Returns false when the resend cooldown is still active.
func (h *Handler) sendCode(r *http.Request, email string) (bool, error) {
	ok, err := h.verify.CanResend(r.Context(), email)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	code, err := NewCode()
	if err != nil {
		return false, err
	}
	if err := h.verify.Save(r.Context(), email, code); err != nil {
		return false, err
	}
	if h.mailer == nil {
		// SMTP 未配置：仅在开发环境把验证码打到日志，绝不返回给客户端
		slog.Warn("SMTP not configured; verification code logged (dev only)", "email", email, "code", code)
		return true, nil
	}
	subject := "TripWeave 邮箱验证码 / Email verification code"
	text := fmt.Sprintf("你的 TripWeave 验证码是 %s，15 分钟内有效。\n\nYour TripWeave verification code is %s. It expires in 15 minutes.\n", code, code)
	html := fmt.Sprintf(`<div style="font-family:system-ui,sans-serif;max-width:480px;margin:auto;padding:32px">
  <h2 style="color:#0e7490;margin:0 0 8px">TripWeave 旅迹编织</h2>
  <p style="color:#334155">你的邮箱验证码 / Your verification code:</p>
  <p style="font-size:32px;letter-spacing:8px;font-weight:700;color:#0f172a;margin:16px 0">%s</p>
  <p style="color:#64748b;font-size:13px">15 分钟内有效 · Expires in 15 minutes</p>
  <p style="color:#94a3b8;font-size:12px;margin-top:24px">如果这不是你的操作，请忽略本邮件。/ If you did not request this, please ignore this email.</p>
</div>`, code)
	if err := h.mailer.Send(email, subject, text, html); err != nil {
		return false, fmt.Errorf("send verification email: %w", err)
	}
	return true, nil
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
	_, err = h.users.Create(r.Context(), req.Email, hash, req.DisplayName)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// 已注册但未验证：覆盖密码与昵称，重新发验证码
			existing, loadErr := h.users.ByEmail(r.Context(), req.Email)
			if loadErr != nil || existing.Verified() {
				phttp.Fail(w, http.StatusConflict, "EMAIL_TAKEN", "email already registered")
				return
			}
			if _, loadErr = h.users.UpdateCredentials(r.Context(), existing.ID, hash, req.DisplayName); loadErr != nil {
				phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to update user")
				return
			}
		} else {
			phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to create user")
			return
		}
	}

	sent, err := h.sendCode(r, req.Email)
	if err != nil {
		slog.Error("failed to send verification code", "email", req.Email, "error", err)
		phttp.Fail(w, http.StatusBadGateway, "MAIL_SEND_FAILED", "failed to send verification email")
		return
	}
	if !sent {
		phttp.Fail(w, http.StatusTooManyRequests, "RESEND_TOO_SOON", "verification email already sent, please wait")
		return
	}
	phttp.OK(w, http.StatusCreated, registerResponse{NeedsVerification: true, Email: req.Email})
}

func (h *Handler) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var req verifyEmailRequest
	if err := decodeJSON(r, &req); err != nil {
		phttp.Fail(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Code = strings.TrimSpace(req.Code)
	if !validateEmail(req.Email) || len(req.Code) != 6 {
		phttp.Fail(w, http.StatusBadRequest, "INVALID_BODY", "email and 6-digit code are required")
		return
	}
	u, err := h.users.ByEmail(r.Context(), req.Email)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			phttp.Fail(w, http.StatusNotFound, "USER_NOT_FOUND", "no account for this email")
			return
		}
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to load user")
		return
	}
	if u.Verified() {
		phttp.Fail(w, http.StatusConflict, "ALREADY_VERIFIED", "email already verified, please log in")
		return
	}
	if err := h.verify.Consume(r.Context(), req.Email, req.Code); err != nil {
		if errors.Is(err, ErrBadCode) {
			phttp.Fail(w, http.StatusBadRequest, "CODE_INVALID", "verification code is invalid or expired")
			return
		}
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to verify code")
		return
	}
	u, err = h.users.MarkVerified(r.Context(), u.ID)
	if err != nil {
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to mark verified")
		return
	}
	h.issueTokens(w, r, u, http.StatusOK)
}

func (h *Handler) resendVerification(w http.ResponseWriter, r *http.Request) {
	var req resendRequest
	if err := decodeJSON(r, &req); err != nil {
		phttp.Fail(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if !validateEmail(req.Email) {
		phttp.Fail(w, http.StatusBadRequest, "INVALID_EMAIL", "invalid email format")
		return
	}
	u, err := h.users.ByEmail(r.Context(), req.Email)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			phttp.Fail(w, http.StatusNotFound, "USER_NOT_FOUND", "no account for this email")
			return
		}
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to load user")
		return
	}
	if u.Verified() {
		phttp.Fail(w, http.StatusConflict, "ALREADY_VERIFIED", "email already verified, please log in")
		return
	}
	sent, err := h.sendCode(r, req.Email)
	if err != nil {
		slog.Error("failed to resend verification code", "email", req.Email, "error", err)
		phttp.Fail(w, http.StatusBadGateway, "MAIL_SEND_FAILED", "failed to send verification email")
		return
	}
	if !sent {
		phttp.Fail(w, http.StatusTooManyRequests, "RESEND_TOO_SOON", "please wait before requesting a new code")
		return
	}
	phttp.OK(w, http.StatusOK, map[string]bool{"sent": true})
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
			// 对不存在的账号也执行一次 Argon2 校验，抹平响应时间差，防邮箱枚举
			_ = VerifyPassword(req.Password, dummyHash)
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
	if !u.Verified() {
		phttp.Fail(w, http.StatusForbidden, "EMAIL_NOT_VERIFIED", "email is not verified yet")
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

// decodeJSON reads a JSON request body (max 1 MiB, unknown fields rejected).
// It is a thin alias of shared.Decode kept for call-site brevity.
func decodeJSON(r *http.Request, v any) error {
	return shared.Decode(r, v)
}

