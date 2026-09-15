package planner

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/mjlxiaoma/TripWeave/apps/api/internal/auth"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/shared"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/trip"
	phttp "github.com/mjlxiaoma/TripWeave/apps/api/pkg/http"
)

// Handler exposes the AI planner HTTP endpoints.
type Handler struct {
	engine *Engine
	repo   *Repository
	trips  *trip.Repository
}

// NewHandler wires the planner handler.
func NewHandler(engine *Engine, repo *Repository, trips *trip.Repository) *Handler {
	return &Handler{engine: engine, repo: repo, trips: trips}
}

const maxMessageLen = 4000

type chatRequest struct {
	Message string `json:"message"`
}

// Chat handles POST /trips/{id}/ai/chat — streams the AI turn over SSE.
func (h *Handler) Chat(w http.ResponseWriter, r *http.Request) {
	tripID := chi.URLParam(r, "id")
	if !shared.IsUUID(tripID) {
		phttp.Fail(w, http.StatusBadRequest, "INVALID_ID", "malformed trip id")
		return
	}
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		phttp.Fail(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing user")
		return
	}
	var req chatRequest
	if err := shared.Decode(r, &req); err != nil {
		phttp.Fail(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		phttp.Fail(w, http.StatusBadRequest, "EMPTY_MESSAGE", "message is required")
		return
	}
	if len([]rune(req.Message)) > maxMessageLen {
		phttp.Fail(w, http.StatusBadRequest, "MESSAGE_TOO_LONG", "message is too long")
		return
	}
	h.engine.Chat(w, r, tripID, userID, req.Message)
}

// Messages handles GET /trips/{id}/ai/messages — returns recent history so a
// refreshed page can restore the conversation.
func (h *Handler) Messages(w http.ResponseWriter, r *http.Request) {
	tripID := chi.URLParam(r, "id")
	if !shared.IsUUID(tripID) {
		phttp.Fail(w, http.StatusBadRequest, "INVALID_ID", "malformed trip id")
		return
	}
	userID := auth.UserIDFromContext(r.Context())
	if userID == "" {
		phttp.Fail(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing user")
		return
	}
	if _, err := h.trips.Role(r.Context(), tripID, userID); err != nil {
		if errors.Is(err, trip.ErrNotFound) {
			phttp.Fail(w, http.StatusNotFound, "TRIP_NOT_FOUND", "trip not found")
			return
		}
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to authorize")
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	conv, err := h.repo.GetOrCreateConversation(r.Context(), tripID, userID)
	if err != nil {
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to load conversation")
		return
	}
	msgs, err := h.repo.ListRecentMessages(r.Context(), conv.ID, limit)
	if err != nil {
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to load messages")
		return
	}
	phttp.OK(w, http.StatusOK, map[string]any{
		"conversation_id": conv.ID,
		"messages":        msgs,
	})
}
