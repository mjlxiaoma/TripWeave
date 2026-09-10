// Package day's HTTP layer: itinerary days and the activities inside them.
package day

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/mjlxiaoma/TripWeave/apps/api/internal/auth"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/shared"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/trip"
	phttp "github.com/mjlxiaoma/TripWeave/apps/api/pkg/http"
)

var errForbidden = errors.New("forbidden")

// Handler wires day/activity endpoints (mounted under /api/v1, auth required).
type Handler struct {
	repo  *Repository
	trips *trip.Repository
}

// NewHandler creates the day handler. trips is used for membership checks.
func NewHandler(repo *Repository, trips *trip.Repository) *Handler {
	return &Handler{repo: repo, trips: trips}
}

// authorize checks trip membership (404 for non-members) and, for writes, the
// owner/editor role (403 otherwise).
func (h *Handler) authorize(ctx context.Context, tripID, userID string, write bool) error {
	role, err := h.trips.Role(ctx, tripID, userID)
	if err != nil {
		return err
	}
	if write && role != "owner" && role != "editor" {
		return errForbidden
	}
	return nil
}

// --- request DTOs ---

type dayRequest struct {
	Date  *string `json:"date"`
	Title *string `json:"title"`
}

type activityRequest struct {
	Type      *string `json:"type"`
	Title     *string `json:"title"`
	StartTime *string `json:"start_time"`
	EndTime   *string `json:"end_time"`
	Notes     *string `json:"notes"`
	Status    *string `json:"status"`
}

type reorderRequest struct {
	ActivityIDs []string `json:"activity_ids"`
}

// --- day handlers ---

// ListDays returns every day of a trip with its activities.
func (h *Handler) ListDays(w http.ResponseWriter, r *http.Request) {
	tripID := chi.URLParam(r, "id")
	if !checkIDs(w, tripID) {
		return
	}
	userID := auth.UserIDFromContext(r.Context())
	if err := h.authorize(r.Context(), tripID, userID, false); err != nil {
		failTrip(w, err)
		return
	}
	days, err := h.repo.ListDays(r.Context(), tripID)
	if err != nil {
		shared.FailDB(w, err, "failed to load days")
		return
	}
	phttp.OK(w, http.StatusOK, days)
}

// CreateDay appends a day to the itinerary.
func (h *Handler) CreateDay(w http.ResponseWriter, r *http.Request) {
	tripID := chi.URLParam(r, "id")
	if !checkIDs(w, tripID) {
		return
	}
	userID := auth.UserIDFromContext(r.Context())
	if err := h.authorize(r.Context(), tripID, userID, true); err != nil {
		failTrip(w, err)
		return
	}
	var req dayRequest
	if err := shared.Decode(r, &req); err != nil {
		phttp.Fail(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}
	if msg := validateDay(&req); msg != "" {
		phttp.Fail(w, http.StatusBadRequest, "VALIDATION", msg)
		return
	}
	d, err := h.repo.CreateDay(r.Context(), tripID, DayInput{
		Date:  shared.TrimPtr(req.Date),
		Title: shared.TrimPtr(req.Title),
	})
	if err != nil {
		shared.FailDB(w, err, "failed to create day")
		return
	}
	phttp.OK(w, http.StatusCreated, d)
}

// UpdateDay patches one day of a trip.
func (h *Handler) UpdateDay(w http.ResponseWriter, r *http.Request) {
	tripID := chi.URLParam(r, "id")
	dayID := chi.URLParam(r, "dayId")
	if !checkIDs(w, tripID, dayID) {
		return
	}
	userID := auth.UserIDFromContext(r.Context())
	if err := h.authorize(r.Context(), tripID, userID, true); err != nil {
		failTrip(w, err)
		return
	}
	var req dayRequest
	if err := shared.Decode(r, &req); err != nil {
		phttp.Fail(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}
	if msg := validateDay(&req); msg != "" {
		phttp.Fail(w, http.StatusBadRequest, "VALIDATION", msg)
		return
	}
	d, err := h.repo.UpdateDay(r.Context(), tripID, dayID, DayPatch{
		Date:  shared.TrimPtr(req.Date),
		Title: shared.TrimPtr(req.Title),
	})
	if err != nil {
		failDay(w, err, "failed to update day")
		return
	}
	phttp.OK(w, http.StatusOK, d)
}

// DeleteDay removes a day and its activities (cascade). Days are not renumbered.
func (h *Handler) DeleteDay(w http.ResponseWriter, r *http.Request) {
	tripID := chi.URLParam(r, "id")
	dayID := chi.URLParam(r, "dayId")
	if !checkIDs(w, tripID, dayID) {
		return
	}
	userID := auth.UserIDFromContext(r.Context())
	if err := h.authorize(r.Context(), tripID, userID, true); err != nil {
		failTrip(w, err)
		return
	}
	if err := h.repo.DeleteDay(r.Context(), tripID, dayID); err != nil {
		failDay(w, err, "failed to delete day")
		return
	}
	phttp.OK(w, http.StatusOK, map[string]bool{"deleted": true})
}

// --- activity handlers ---

// CreateActivity appends an activity to a day.
func (h *Handler) CreateActivity(w http.ResponseWriter, r *http.Request) {
	dayID := chi.URLParam(r, "dayId")
	if !checkIDs(w, dayID) {
		return
	}
	tripID, err := h.repo.TripIDForDay(r.Context(), dayID)
	if err != nil {
		failDay(w, err, "failed to resolve day")
		return
	}
	userID := auth.UserIDFromContext(r.Context())
	if err := h.authorize(r.Context(), tripID, userID, true); err != nil {
		failTrip(w, err)
		return
	}
	var req activityRequest
	if err := shared.Decode(r, &req); err != nil {
		phttp.Fail(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}
	if msg := validateActivity(&req, true); msg != "" {
		phttp.Fail(w, http.StatusBadRequest, "VALIDATION", msg)
		return
	}
	a, err := h.repo.CreateActivity(r.Context(), dayID, ActivityInput{
		Type:      shared.Deref(req.Type),
		Title:     strings.TrimSpace(shared.Deref(req.Title)),
		StartTime: shared.TrimPtr(req.StartTime),
		EndTime:   shared.TrimPtr(req.EndTime),
		Notes:     shared.TrimPtr(req.Notes),
		Status:    shared.Deref(req.Status),
	})
	if err != nil {
		shared.FailDB(w, err, "failed to create activity")
		return
	}
	phttp.OK(w, http.StatusCreated, a)
}

// UpdateActivity patches one activity.
func (h *Handler) UpdateActivity(w http.ResponseWriter, r *http.Request) {
	activityID := chi.URLParam(r, "id")
	if !checkIDs(w, activityID) {
		return
	}
	tripID, err := h.repo.TripIDForActivity(r.Context(), activityID)
	if err != nil {
		failActivity(w, err, "failed to resolve activity")
		return
	}
	userID := auth.UserIDFromContext(r.Context())
	if err := h.authorize(r.Context(), tripID, userID, true); err != nil {
		failTrip(w, err)
		return
	}
	var req activityRequest
	if err := shared.Decode(r, &req); err != nil {
		phttp.Fail(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}
	if msg := validateActivity(&req, false); msg != "" {
		phttp.Fail(w, http.StatusBadRequest, "VALIDATION", msg)
		return
	}
	a, err := h.repo.UpdateActivity(r.Context(), activityID, ActivityPatch{
		Type:      req.Type,
		Title:     shared.TrimPtr(req.Title),
		StartTime: shared.TrimPtr(req.StartTime),
		EndTime:   shared.TrimPtr(req.EndTime),
		Notes:     shared.TrimPtr(req.Notes),
		Status:    req.Status,
	})
	if err != nil {
		failActivity(w, err, "failed to update activity")
		return
	}
	phttp.OK(w, http.StatusOK, a)
}

// DeleteActivity removes one activity.
func (h *Handler) DeleteActivity(w http.ResponseWriter, r *http.Request) {
	activityID := chi.URLParam(r, "id")
	if !checkIDs(w, activityID) {
		return
	}
	tripID, err := h.repo.TripIDForActivity(r.Context(), activityID)
	if err != nil {
		failActivity(w, err, "failed to resolve activity")
		return
	}
	userID := auth.UserIDFromContext(r.Context())
	if err := h.authorize(r.Context(), tripID, userID, true); err != nil {
		failTrip(w, err)
		return
	}
	if err := h.repo.DeleteActivity(r.Context(), activityID); err != nil {
		failActivity(w, err, "failed to delete activity")
		return
	}
	phttp.OK(w, http.StatusOK, map[string]bool{"deleted": true})
}

// ReorderActivities replaces the sort order of a day's activities. The payload
// must list every activity of the day exactly once.
func (h *Handler) ReorderActivities(w http.ResponseWriter, r *http.Request) {
	dayID := chi.URLParam(r, "dayId")
	if !checkIDs(w, dayID) {
		return
	}
	tripID, err := h.repo.TripIDForDay(r.Context(), dayID)
	if err != nil {
		failDay(w, err, "failed to resolve day")
		return
	}
	userID := auth.UserIDFromContext(r.Context())
	if err := h.authorize(r.Context(), tripID, userID, true); err != nil {
		failTrip(w, err)
		return
	}
	var req reorderRequest
	if err := shared.Decode(r, &req); err != nil {
		phttp.Fail(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}
	if msg := validateReorder(req.ActivityIDs); msg != "" {
		phttp.Fail(w, http.StatusBadRequest, "VALIDATION", msg)
		return
	}
	if err := h.repo.ReorderActivities(r.Context(), dayID, req.ActivityIDs); err != nil {
		if errors.Is(err, ErrReorderMismatch) {
			phttp.Fail(w, http.StatusBadRequest, "VALIDATION",
				"activity_ids must list every activity of the day exactly once")
			return
		}
		shared.FailDB(w, err, "failed to reorder activities")
		return
	}
	acts, err := h.repo.ActivitiesByDay(r.Context(), dayID)
	if err != nil {
		shared.FailDB(w, err, "failed to load activities")
		return
	}
	phttp.OK(w, http.StatusOK, acts)
}

// --- error mapping ---

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

func failDay(w http.ResponseWriter, err error, msg string) {
	if errors.Is(err, ErrNotFound) {
		phttp.Fail(w, http.StatusNotFound, "DAY_NOT_FOUND", "day not found")
		return
	}
	shared.FailDB(w, err, msg)
}

func failActivity(w http.ResponseWriter, err error, msg string) {
	if errors.Is(err, ErrNotFound) {
		phttp.Fail(w, http.StatusNotFound, "ACTIVITY_NOT_FOUND", "activity not found")
		return
	}
	shared.FailDB(w, err, msg)
}

// --- validation ---

// checkIDs rejects malformed path params with 400 before they reach uuid queries.
func checkIDs(w http.ResponseWriter, ids ...string) bool {
	for _, id := range ids {
		if !shared.IsUUID(id) {
			phttp.Fail(w, http.StatusBadRequest, "VALIDATION", "malformed id")
			return false
		}
	}
	return true
}

func validateDay(req *dayRequest) string {
	if req.Title != nil {
		t := strings.TrimSpace(*req.Title)
		if t == "" || utf8.RuneCountInString(t) > 200 {
			return "title must be 1-200 characters"
		}
	}
	if req.Date != nil {
		if _, err := time.Parse("2006-01-02", strings.TrimSpace(*req.Date)); err != nil {
			return "date must be YYYY-MM-DD"
		}
	}
	return ""
}

var allowedTypes = map[string]bool{
	"attraction": true, "restaurant": true, "cafe": true, "hotel": true,
	"transport": true, "free_time": true, "other": true,
}

var allowedActivityStatus = map[string]bool{"planned": true, "done": true, "skipped": true}

func parseTimePtr(s *string) (*time.Time, error) {
	if s == nil {
		return nil, nil
	}
	v := strings.TrimSpace(*s)
	t, err := time.Parse("15:04", v)
	if err != nil {
		t, err = time.Parse("15:04:05", v)
		if err != nil {
			return nil, err
		}
	}
	return &t, nil
}

func validateActivity(req *activityRequest, isCreate bool) string {
	if req.Type != nil && !allowedTypes[*req.Type] {
		return "type must be one of attraction/restaurant/cafe/hotel/transport/free_time/other"
	}
	if req.Status != nil && !allowedActivityStatus[*req.Status] {
		return "status must be one of planned/done/skipped"
	}
	if req.Title != nil {
		t := strings.TrimSpace(*req.Title)
		if t == "" || utf8.RuneCountInString(t) > 200 {
			return "title must be 1-200 characters"
		}
	} else if isCreate {
		return "title is required"
	}
	start, errS := parseTimePtr(req.StartTime)
	end, errE := parseTimePtr(req.EndTime)
	if errS != nil || errE != nil {
		return "start_time/end_time must be HH:MM or HH:MM:SS"
	}
	if start != nil && end != nil && end.Before(*start) {
		return "end_time must not be before start_time"
	}
	if req.Notes != nil && utf8.RuneCountInString(*req.Notes) > 2000 {
		return "notes must be <= 2000 characters"
	}
	return ""
}

func validateReorder(ids []string) string {
	if len(ids) == 0 {
		return "activity_ids must not be empty"
	}
	if len(ids) > 500 {
		return "activity_ids must have <= 500 entries"
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if !shared.IsUUID(id) {
			return "activity_ids must contain valid ids"
		}
		if seen[id] {
			return "activity_ids must not contain duplicates"
		}
		seen[id] = true
	}
	return ""
}
