// Package share exposes read-only public views of a trip via an unguessable
// share token. Authenticated endpoints manage the token; the public endpoints
// require no login and never expose owner identity or AI conversation history.
package share

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/mjlxiaoma/TripWeave/apps/api/internal/auth"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/day"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/location"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/shared"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/trip"
	phttp "github.com/mjlxiaoma/TripWeave/apps/api/pkg/http"
)

// Handler wires share endpoints. Authenticated routes manage the token;
// public routes serve the read-only view.
type Handler struct {
	trips *trip.Repository
	days  *day.Repository
	loc   *location.Service
}

// NewHandler creates the share handler. loc may be nil (no map key configured):
// the public route endpoint then returns 503 like its authenticated sibling.
func NewHandler(trips *trip.Repository, days *day.Repository, loc *location.Service) *Handler {
	return &Handler{trips: trips, days: days, loc: loc}
}

// --- authenticated: manage token ---

// Create handles POST /trips/{id}/share — returns the (idempotent) share token.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	tripID := chi.URLParam(r, "id")
	if !shared.IsUUID(tripID) {
		phttp.Fail(w, http.StatusBadRequest, "VALIDATION", "malformed id")
		return
	}
	userID := auth.UserIDFromContext(r.Context())
	if err := h.requireWrite(r.Context(), tripID, userID); err != nil {
		failTrip(w, err)
		return
	}
	token, err := h.trips.SetShareToken(r.Context(), tripID, newToken())
	if err != nil {
		shared.FailDB(w, err, "failed to create share link")
		return
	}
	phttp.OK(w, http.StatusOK, map[string]string{"token": token})
}

// Clear handles DELETE /trips/{id}/share — revokes the link immediately.
func (h *Handler) Clear(w http.ResponseWriter, r *http.Request) {
	tripID := chi.URLParam(r, "id")
	if !shared.IsUUID(tripID) {
		phttp.Fail(w, http.StatusBadRequest, "VALIDATION", "malformed id")
		return
	}
	userID := auth.UserIDFromContext(r.Context())
	// 撤销只允许 owner（editor 可编辑行程但不应能关掉 owner 的分享）。
	role, err := h.trips.Role(r.Context(), tripID, userID)
	if err != nil {
		failTrip(w, err)
		return
	}
	if role != "owner" {
		phttp.Fail(w, http.StatusForbidden, "FORBIDDEN", "owner access required")
		return
	}
	if err := h.trips.ClearShareToken(r.Context(), tripID); err != nil {
		failTrip(w, err)
		return
	}
	phttp.OK(w, http.StatusOK, map[string]bool{"revoked": true})
}

// --- public: read-only view ---

// Trip handles GET /share/{token} — trip + full itinerary, no auth.
func (h *Handler) Trip(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	tr, err := h.trips.GetByShareToken(r.Context(), token)
	if err != nil {
		failTrip(w, err)
		return
	}
	days, err := h.days.ListDays(r.Context(), tr.ID)
	if err != nil {
		shared.FailDB(w, err, "failed to load itinerary")
		return
	}
	// 公开视图不暴露 owner / status 等内部字段，只给展示所需。
	phttp.OK(w, http.StatusOK, map[string]any{
		"trip": map[string]any{
			"id":              tr.ID,
			"title":           tr.Title,
			"destination":     tr.Destination,
			"start_date":      tr.StartDate,
			"end_date":        tr.EndDate,
			"travelers_count": tr.TravelersCount,
		},
		"days": days,
	})
}

// Route handles GET /share/{token}/days/{dayId}/route — public day route.
// dayId 必须属于该 token 的行程，否则 404（防跨行程探测）。
func (h *Handler) Route(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	dayID := chi.URLParam(r, "dayId")
	if !shared.IsUUID(dayID) {
		phttp.Fail(w, http.StatusBadRequest, "VALIDATION", "malformed id")
		return
	}
	mode := strings.TrimSpace(r.URL.Query().Get("mode"))
	if mode == "" {
		mode = "driving"
	}
	if mode != "driving" && mode != "walking" {
		phttp.Fail(w, http.StatusBadRequest, "VALIDATION", "mode must be driving or walking")
		return
	}
	tr, err := h.trips.GetByShareToken(r.Context(), token)
	if err != nil {
		failTrip(w, err)
		return
	}
	tripIDForDay, err := h.days.TripIDForDay(r.Context(), dayID)
	if err != nil {
		failDay(w, err)
		return
	}
	if tripIDForDay != tr.ID {
		phttp.Fail(w, http.StatusNotFound, "DAY_NOT_FOUND", "day not found")
		return
	}
	acts, err := h.days.ActivitiesByDay(r.Context(), dayID)
	if err != nil {
		shared.FailDB(w, err, "failed to load activities")
		return
	}
	pts := make([]location.ActivityPoint, 0, len(acts))
	for _, a := range acts {
		if a.Location == nil {
			continue
		}
		pts = append(pts, location.ActivityPoint{
			ID:        a.ID,
			Title:     a.Title,
			Longitude: a.Location.Longitude,
			Latitude:  a.Location.Latitude,
		})
	}
	route, err := h.loc.RouteForDay(r.Context(), dayID, pts, mode)
	if err != nil {
		if errors.Is(err, location.ErrNoProvider) {
			phttp.Fail(w, http.StatusServiceUnavailable, "MAP_UNAVAILABLE", "map provider not configured")
			return
		}
		phttp.Fail(w, http.StatusBadGateway, "MAP_PROVIDER_ERROR", "day route failed")
		return
	}
	phttp.OK(w, http.StatusOK, route)
}

// --- helpers ---

// requireWrite 复用 trip membership：owner/editor 才能开/关分享。
func (h *Handler) requireWrite(ctx context.Context, tripID, userID string) error {
	role, err := h.trips.Role(ctx, tripID, userID)
	if err != nil {
		return err
	}
	if role != "owner" && role != "editor" {
		return errForbidden
	}
	return nil
}

var errForbidden = errors.New("forbidden")

// newToken 生成 128-bit 随机 token（32 hex chars，不可枚举）。
func newToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func failTrip(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, trip.ErrNotFound):
		phttp.Fail(w, http.StatusNotFound, "TRIP_NOT_FOUND", "trip not found")
	case errors.Is(err, errForbidden):
		phttp.Fail(w, http.StatusForbidden, "FORBIDDEN", "editor access required")
	default:
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to check access")
	}
}

func failDay(w http.ResponseWriter, err error) {
	if errors.Is(err, day.ErrNotFound) {
		phttp.Fail(w, http.StatusNotFound, "DAY_NOT_FOUND", "day not found")
		return
	}
	shared.FailDB(w, err, "failed to resolve day")
}
