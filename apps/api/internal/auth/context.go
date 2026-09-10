package auth

import (
	"context"
	"net/http"
	"strings"

	phttp "github.com/mjlxiaoma/TripWeave/apps/api/pkg/http"
)

type ctxKey string

const userIDKey ctxKey = "user_id"

// UserIDFromContext returns the authenticated user ID from ctx, or "".
func UserIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(userIDKey).(string); ok {
		return v
	}
	return ""
}

// RequireAuth is middleware that validates the Bearer access token.
func RequireAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				phttp.Fail(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing bearer token")
				return
			}
			userID, err := ParseAccessToken(secret, strings.TrimPrefix(header, "Bearer "))
			if err != nil {
				phttp.Fail(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or expired token")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIDKey, userID)))
		})
	}
}
