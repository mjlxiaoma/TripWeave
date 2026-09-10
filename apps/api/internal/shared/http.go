// Package shared holds helpers reused across domain packages: JSON body decoding,
// Postgres error mapping and small pointer utilities.
package shared

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	phttp "github.com/mjlxiaoma/TripWeave/apps/api/pkg/http"
)

// Decode reads a JSON request body (max 1 MiB) and rejects unknown fields.
func Decode(r *http.Request, v any) error {
	if r.Body == nil {
		return errors.New("empty body")
	}
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// FailDB logs err and maps common Postgres failures to client errors so bad input
// never surfaces as a 500.
func FailDB(w http.ResponseWriter, err error, msg string) {
	slog.Error(msg, "error", err)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23514": // CHECK violation (ranges, enums)
			phttp.Fail(w, http.StatusBadRequest, "VALIDATION", "field value out of range")
			return
		case "23505": // unique violation
			phttp.Fail(w, http.StatusConflict, "CONFLICT", "duplicate value")
			return
		case "22007", "22P02": // invalid datetime / text representation (bad date, time or uuid input)
			phttp.Fail(w, http.StatusBadRequest, "VALIDATION", "malformed id, date or time value")
			return
		}
	}
	phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", msg)
}

// Deref returns "" for a nil string pointer.
func Deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// TrimPtr trims a non-nil string pointer.
func TrimPtr(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	return &t
}

// RawOr returns raw, or a fallback JSON literal when raw is nil or empty.
func RawOr(raw *json.RawMessage, fallback string) json.RawMessage {
	if raw == nil || len(*raw) == 0 {
		return json.RawMessage(fallback)
	}
	return *raw
}

// IsUUID reports whether s is a canonical 8-4-4-4-12 hex UUID. Path params are
// checked with this before reaching uuid-typed queries: pgx fails to encode
// malformed UUID strings client-side, which would otherwise surface as 500.
func IsUUID(s string) bool {
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return false
	}
	for i := 0; i < 36; i++ {
		c := s[i]
		switch {
		case c == '-':
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}
