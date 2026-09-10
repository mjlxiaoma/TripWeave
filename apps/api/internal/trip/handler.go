package trip

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/mjlxiaoma/TripWeave/apps/api/internal/auth"
	phttp "github.com/mjlxiaoma/TripWeave/apps/api/pkg/http"
)

// Handler wires trip HTTP endpoints (mounted under /api/v1/trips, auth required).
type Handler struct {
	repo *Repository
}

// NewHandler creates the trip handler.
func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}


// --- request/response DTOs ---

type tripRequest struct {
	Title           *string          `json:"title"`
	Destination     *string          `json:"destination"`
	StartDate       *string          `json:"start_date"`
	EndDate         *string          `json:"end_date"`
	TravelersCount  *int             `json:"travelers_count"`
	Status          *string          `json:"status"`
	Budget          *float64         `json:"budget"`
	TransportMode   *string          `json:"transport_mode"`
	TravelStyle     *string          `json:"travel_style"`
	Constraints     *json.RawMessage `json:"constraints"`
	Preferences     *json.RawMessage `json:"preferences"`
	NaturalLanguage *string          `json:"natural_language"`
}

type preferenceDTO struct {
	Budget          *float64        `json:"budget"`
	TransportMode   *string         `json:"transport_mode"`
	TravelStyle     *string         `json:"travel_style"`
	Constraints     json.RawMessage `json:"constraints"`
	Preferences     json.RawMessage `json:"preferences"`
	NaturalLanguage *string         `json:"natural_language"`
}

type tripDTO struct {
	ID             string     `json:"id"`
	Title          string     `json:"title"`
	Destination    *string    `json:"destination"`
	StartDate      *string    `json:"start_date"`
	EndDate        *string    `json:"end_date"`
	TravelersCount *int       `json:"travelers_count"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	Role           string     `json:"role,omitempty"`
	Preference     *preferenceDTO `json:"preference,omitempty"`
}

func toTripDTO(t *Trip) tripDTO {
	return tripDTO{ID: t.ID, Title: t.Title, Destination: t.Destination,
		StartDate: t.StartDate, EndDate: t.EndDate, TravelersCount: t.TravelersCount,
		Status: t.Status, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt}
}

func toPrefDTO(p *Preference) *preferenceDTO {
	if p == nil {
		return nil
	}
	return &preferenceDTO{Budget: p.Budget, TransportMode: p.TransportMode, TravelStyle: p.TravelStyle,
		Constraints: p.Constraints, Preferences: p.Preferences, NaturalLanguage: p.NaturalLanguage}
}

// --- handlers ---

// List returns all trips the caller belongs to.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	trips, err := h.repo.ListByUser(r.Context(), userID)
	if err != nil {
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to load trips")
		return
	}
	out := make([]tripDTO, 0, len(trips))
	for i := range trips {
		out = append(out, toTripDTO(&trips[i]))
	}
	phttp.OK(w, http.StatusOK, out)
}

// Create builds a trip from the wizard payload (basic info + preferences + constraints + NL).
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req tripRequest
	if err := decode(r, &req); err != nil {
		phttp.Fail(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}
	if msg := validate(&req, true); msg != "" {
		phttp.Fail(w, http.StatusBadRequest, "VALIDATION", msg)
		return
	}

	userID := auth.UserIDFromContext(r.Context())
	title := strings.TrimSpace(deref(req.Title))
	if title == "" {
		title = strings.TrimSpace(deref(req.Destination))
	}
	if title == "" {
		title = "Untitled Trip"
	}

	p := CreateParams{
		Trip: Trip{
			OwnerID: userID, Title: title,
			Destination: trimPtr(req.Destination), StartDate: req.StartDate, EndDate: req.EndDate,
			TravelersCount: req.TravelersCount, Status: deref(req.Status),
		},
		Pref: Preference{
			Budget: req.Budget, TransportMode: trimPtr(req.TransportMode), TravelStyle: trimPtr(req.TravelStyle),
			Constraints: rawOr(req.Constraints, `{}`), Preferences: rawOr(req.Preferences, `[]`),
			NaturalLanguage: trimPtr(req.NaturalLanguage),
		},
	}
	t, err := h.repo.Create(r.Context(), p)
	if err != nil {
		failDB(w, err, "failed to create trip")
		return
	}
	dto := toTripDTO(t)
	dto.Role = "owner"
	dto.Preference = toPrefDTO(&p.Pref)
	phttp.OK(w, http.StatusCreated, dto)
}

// Get returns one trip with preferences and the caller's role (404 for non-members).
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	d, err := h.repo.Get(r.Context(), chi.URLParam(r, "id"), userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			phttp.Fail(w, http.StatusNotFound, "TRIP_NOT_FOUND", "trip not found")
			return
		}
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to load trip")
		return
	}
	dto := toTripDTO(d.Trip)
	dto.Role = d.Role
	dto.Preference = toPrefDTO(d.Preference)
	phttp.OK(w, http.StatusOK, dto)
}

// Update patches trip + preferences. Requires owner or editor role.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	tripID := chi.URLParam(r, "id")
	userID := auth.UserIDFromContext(r.Context())

	role, err := h.repo.Role(r.Context(), tripID, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			phttp.Fail(w, http.StatusNotFound, "TRIP_NOT_FOUND", "trip not found")
			return
		}
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to check access")
		return
	}
	if role != "owner" && role != "editor" {
		phttp.Fail(w, http.StatusForbidden, "FORBIDDEN", "editor access required")
		return
	}

	var req tripRequest
	if err := decode(r, &req); err != nil {
		phttp.Fail(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}
	if msg := validate(&req, false); msg != "" {
		phttp.Fail(w, http.StatusBadRequest, "VALIDATION", msg)
		return
	}

	patch := Patch{
		Title: trimPtr(req.Title), Destination: trimPtr(req.Destination),
		StartDate: req.StartDate, EndDate: req.EndDate,
		TravelersCount: req.TravelersCount, Status: req.Status,
		Budget: req.Budget, TransportMode: trimPtr(req.TransportMode), TravelStyle: trimPtr(req.TravelStyle),
		Constraints: req.Constraints, Preferences: req.Preferences, NaturalLanguage: trimPtr(req.NaturalLanguage),
	}
	t, pref, err := h.repo.Update(r.Context(), tripID, patch)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			phttp.Fail(w, http.StatusNotFound, "TRIP_NOT_FOUND", "trip not found")
			return
		}
		failDB(w, err, "failed to update trip")
		return
	}
	dto := toTripDTO(t)
	dto.Role = role
	dto.Preference = toPrefDTO(pref)
	phttp.OK(w, http.StatusOK, dto)
}

// Delete removes a trip entirely. Owner only.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	tripID := chi.URLParam(r, "id")
	userID := auth.UserIDFromContext(r.Context())

	role, err := h.repo.Role(r.Context(), tripID, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			phttp.Fail(w, http.StatusNotFound, "TRIP_NOT_FOUND", "trip not found")
			return
		}
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to check access")
		return
	}
	if role != "owner" {
		phttp.Fail(w, http.StatusForbidden, "FORBIDDEN", "only the owner can delete a trip")
		return
	}

	if err := h.repo.Delete(r.Context(), tripID, userID); err != nil {
		if errors.Is(err, ErrNotFound) {
			phttp.Fail(w, http.StatusNotFound, "TRIP_NOT_FOUND", "trip not found")
			return
		}
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to delete trip")
		return
	}
	phttp.OK(w, http.StatusOK, map[string]bool{"deleted": true})
}

// --- validation & helpers ---

var allowedStatus = map[string]bool{"draft": true, "planning": true, "ready": true, "archived": true}

func validate(req *tripRequest, isCreate bool) string {
	if req.Title != nil {
		t := strings.TrimSpace(*req.Title)
		if t == "" || utf8.RuneCountInString(t) > 200 {
			return "title must be 1-200 characters"
		}
	}
	if isCreate && (req.Title == nil || strings.TrimSpace(*req.Title) == "") &&
		(req.Destination == nil || strings.TrimSpace(*req.Destination) == "") {
		// allowed: falls back to "Untitled Trip"
		_ = req
	}
	if req.Status != nil && !allowedStatus[*req.Status] {
		return "status must be one of draft/planning/ready/archived"
	}
	if req.TravelersCount != nil && (*req.TravelersCount < 1 || *req.TravelersCount > 100) {
		return "travelers_count must be 1-100"
	}
	if req.Budget != nil && *req.Budget < 0 {
		return "budget must be >= 0"
	}
	start, errS := parseDatePtr(req.StartDate)
	end, errE := parseDatePtr(req.EndDate)
	if errS != nil {
		return "start_date must be YYYY-MM-DD"
	}
	if errE != nil {
		return "end_date must be YYYY-MM-DD"
	}
	if start != nil && end != nil && end.Before(*start) {
		return "end_date must not be before start_date"
	}
	for _, s := range []*string{req.Destination, req.TransportMode, req.TravelStyle} {
		if s != nil && utf8.RuneCountInString(*s) > 100 {
			return "text fields must be <= 100 characters"
		}
	}
	if req.NaturalLanguage != nil && utf8.RuneCountInString(*req.NaturalLanguage) > 2000 {
		return "natural_language must be <= 2000 characters"
	}
	if req.Preferences != nil {
		var tags []string
		if err := json.Unmarshal(*req.Preferences, &tags); err != nil {
			return "preferences must be an array of strings"
		}
		if len(tags) > 50 {
			return "preferences must have <= 50 tags"
		}
		for _, tag := range tags {
			if tag == "" || utf8.RuneCountInString(tag) > 30 {
				return "each preference tag must be 1-30 characters"
			}
		}
	}
	if req.Constraints != nil {
		var obj map[string]any
		if err := json.Unmarshal(*req.Constraints, &obj); err != nil {
			return "constraints must be a JSON object"
		}
	}
	return ""
}

func parseDatePtr(s *string) (*time.Time, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", *s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func decode(r *http.Request, v any) error {
	if r.Body == nil {
		return errors.New("empty body")
	}
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func trimPtr(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	return &t
}

func rawOr(raw *json.RawMessage, fallback string) json.RawMessage {
	if raw == nil || len(*raw) == 0 {
		return json.RawMessage(fallback)
	}
	return *raw
}

func failDB(w http.ResponseWriter, err error, msg string) {
	slog.Error(msg, "error", err)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23514": // CHECK violation (date range, budget, count, enums)
			phttp.Fail(w, http.StatusBadRequest, "VALIDATION", "field value out of range")
			return
		case "23505":
			phttp.Fail(w, http.StatusConflict, "CONFLICT", "duplicate value")
			return
		}
	}
	phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", msg)
}
