// Package location's HTTP layer: place search and per-day route aggregation.
package location

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/mjlxiaoma/TripWeave/apps/api/internal/auth"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/day"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/shared"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/trip"
	phttp "github.com/mjlxiaoma/TripWeave/apps/api/pkg/http"
)

// DayStore is the narrow day-repository surface the route endpoint needs
// (consumer-side interface, same pattern as planner/deps.go).
type DayStore interface {
	TripIDForDay(ctx context.Context, dayID string) (string, error)
	ActivitiesByDay(ctx context.Context, dayID string) ([]day.Activity, error)
}

// Handler wires map endpoints (mounted under /api/v1, auth required).
type Handler struct {
	svc   *Service
	days  DayStore
	trips *trip.Repository
}

// NewHandler creates the location handler. trips is used for membership checks.
func NewHandler(svc *Service, days DayStore, trips *trip.Repository) *Handler {
	return &Handler{svc: svc, days: days, trips: trips}
}

// authorize checks trip membership (404 for non-members, leaking nothing).
func (h *Handler) authorize(ctx context.Context, tripID, userID string) error {
	_, err := h.trips.Role(ctx, tripID, userID)
	return err
}

// Search handles GET /locations/search?q=&city= — free-text place lookup used
// by the map panel to locate un-geocoded activities on the fly.
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	city := strings.TrimSpace(r.URL.Query().Get("city"))
	if q == "" || utf8.RuneCountInString(q) > 100 {
		phttp.Fail(w, http.StatusBadRequest, "VALIDATION", "q must be 1-100 characters")
		return
	}
	if utf8.RuneCountInString(city) > 50 {
		phttp.Fail(w, http.StatusBadRequest, "VALIDATION", "city must be at most 50 characters")
		return
	}
	locs, err := h.svc.Search(r.Context(), q, city)
	if err != nil {
		// 上游错误(限流/配额/网络)打日志,否则 502 在前端不可诊断
		slog.Warn("location search failed", "q", q, "city", city, "err", err)
		failMap(w, err, "location search failed")
		return
	}
	phttp.OK(w, http.StatusOK, locs)
}

// DayRoute handles GET /days/{dayId}/route?mode=driving|walking — aggregates
// real routed legs between the day's located activities, in itinerary order.
func (h *Handler) DayRoute(w http.ResponseWriter, r *http.Request) {
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
	tripID, err := h.days.TripIDForDay(r.Context(), dayID)
	if err != nil {
		failDay(w, err)
		return
	}
	userID := auth.UserIDFromContext(r.Context())
	if err := h.authorize(r.Context(), tripID, userID); err != nil {
		failTrip(w, err)
		return
	}
	acts, err := h.days.ActivitiesByDay(r.Context(), dayID)
	if err != nil {
		shared.FailDB(w, err, "failed to load activities")
		return
	}
	pts := make([]ActivityPoint, 0, len(acts))
	for _, a := range acts {
		if a.Location == nil {
			continue
		}
		pts = append(pts, ActivityPoint{
			ID:        a.ID,
			Title:     a.Title,
			Longitude: a.Location.Longitude,
			Latitude:  a.Location.Latitude,
		})
	}
	route, err := h.svc.RouteForDay(r.Context(), dayID, pts, mode)
	if err != nil {
		slog.Warn("day route failed", "day", dayID, "mode", mode, "err", err)
		failMap(w, err, "day route failed")
		return
	}
	phttp.OK(w, http.StatusOK, route)
}

// --- error mapping ---

func failTrip(w http.ResponseWriter, err error) {
	if errors.Is(err, trip.ErrNotFound) {
		phttp.Fail(w, http.StatusNotFound, "TRIP_NOT_FOUND", "trip not found")
		return
	}
	phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to check access")
}

func failDay(w http.ResponseWriter, err error) {
	if errors.Is(err, day.ErrNotFound) {
		phttp.Fail(w, http.StatusNotFound, "DAY_NOT_FOUND", "day not found")
		return
	}
	shared.FailDB(w, err, "failed to resolve day")
}

func failMap(w http.ResponseWriter, err error, msg string) {
	switch {
	case errors.Is(err, ErrNoProvider):
		phttp.Fail(w, http.StatusServiceUnavailable, "MAP_UNAVAILABLE", "map provider not configured")
	case errors.Is(err, ErrProvider):
		phttp.Fail(w, http.StatusBadGateway, "MAP_PROVIDER_ERROR", msg)
	default:
		shared.FailDB(w, err, msg)
	}
}
