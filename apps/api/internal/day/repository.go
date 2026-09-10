// Package day holds the planner domain: trip days and their activities.
package day

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a day or activity does not exist, or is not visible
// to the caller. Handlers map it to 404 so cross-trip probing leaks nothing.
var ErrNotFound = errors.New("not found")

// ErrReorderMismatch is returned when a reorder payload does not cover exactly the
// activities of the target day.
var ErrReorderMismatch = errors.New("reorder list does not match the day")

// Day is one day of an itinerary. DayNumber is a stable ordering key assigned on
// creation; it is not renumbered after deletions, so the UI labels days by their
// position in the (day_number-sorted) list rather than by the number itself.
type Day struct {
	ID        string    `json:"id"`
	TripID    string    `json:"trip_id"`
	DayNumber int       `json:"day_number"`
	Date      *string   `json:"date"`
	Title     *string   `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Activities []Activity `json:"activities"`
}

// Activity is one scheduled item inside a day. location_id is intentionally not
// exposed yet: the map (P9) and AI (P10) phases attach real location records.
type Activity struct {
	ID        string    `json:"id"`
	DayID     string    `json:"day_id"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	StartTime *string   `json:"start_time"`
	EndTime   *string   `json:"end_time"`
	SortOrder int       `json:"sort_order"`
	Notes     *string   `json:"notes"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Repository provides day/activity persistence.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a day repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Bare column lists used for INSERT/UPDATE RETURNING (a single-table context
// cannot reference a table alias).
const dayCols = `id, trip_id, day_number, date::text, title, created_at, updated_at`

const actColsBare = `id, trip_day_id, type, title, start_time::text, end_time::text,
	sort_order, notes, status, created_at, updated_at`

const actColsA = `a.id, a.trip_day_id, a.type, a.title, a.start_time::text, a.end_time::text,
	a.sort_order, a.notes, a.status, a.created_at, a.updated_at`

func scanDay(row pgx.Row) (*Day, error) {
	var d Day
	err := row.Scan(&d.ID, &d.TripID, &d.DayNumber, &d.Date, &d.Title, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	d.Activities = []Activity{}
	return &d, nil
}

func scanActivity(row pgx.Row) (*Activity, error) {
	var a Activity
	err := row.Scan(&a.ID, &a.DayID, &a.Type, &a.Title, &a.StartTime, &a.EndTime,
		&a.SortOrder, &a.Notes, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// TripIDForDay resolves the trip owning a day (ErrNotFound when the day is gone).
func (r *Repository) TripIDForDay(ctx context.Context, dayID string) (string, error) {
	var tripID string
	err := r.pool.QueryRow(ctx, `SELECT trip_id FROM trip_days WHERE id = $1`, dayID).Scan(&tripID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return tripID, err
}

// TripIDForActivity resolves the trip owning an activity.
func (r *Repository) TripIDForActivity(ctx context.Context, activityID string) (string, error) {
	var tripID string
	err := r.pool.QueryRow(ctx, `
		SELECT d.trip_id
		FROM activities a
		JOIN trip_days d ON d.id = a.trip_day_id
		WHERE a.id = $1`, activityID).Scan(&tripID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return tripID, err
}

// ListDays returns every day of a trip with nested activities, in itinerary order.
func (r *Repository) ListDays(ctx context.Context, tripID string) ([]Day, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+dayCols+` FROM trip_days WHERE trip_id = $1 ORDER BY day_number`, tripID)
	if err != nil {
		return nil, err
	}
	days := []Day{}
	for rows.Next() {
		var d Day
		if err := rows.Scan(&d.ID, &d.TripID, &d.DayNumber, &d.Date, &d.Title, &d.CreatedAt, &d.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		d.Activities = []Activity{}
		days = append(days, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	acts, err := r.activitiesByTrip(ctx, tripID)
	if err != nil {
		return nil, err
	}
	byDay := make(map[string][]Activity, len(acts))
	for _, a := range acts {
		byDay[a.DayID] = append(byDay[a.DayID], a)
	}
	for i := range days {
		if list, ok := byDay[days[i].ID]; ok {
			days[i].Activities = list
		}
	}
	return days, nil
}

func (r *Repository) activitiesByTrip(ctx context.Context, tripID string) ([]Activity, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+actColsA+`
		FROM activities a
		JOIN trip_days d ON d.id = a.trip_day_id
		WHERE d.trip_id = $1
		ORDER BY a.sort_order, a.created_at`, tripID)
	if err != nil {
		return nil, err
	}
	acts := []Activity{}
	for rows.Next() {
		var a Activity
		if err := rows.Scan(&a.ID, &a.DayID, &a.Type, &a.Title, &a.StartTime, &a.EndTime,
			&a.SortOrder, &a.Notes, &a.Status, &a.CreatedAt, &a.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		acts = append(acts, a)
	}
	rows.Close()
	return acts, rows.Err()
}

// ActivitiesByDay returns the activities of a single day in display order.
func (r *Repository) ActivitiesByDay(ctx context.Context, dayID string) ([]Activity, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+actColsBare+` FROM activities WHERE trip_day_id = $1 ORDER BY sort_order, created_at`, dayID)
	if err != nil {
		return nil, err
	}
	acts := []Activity{}
	for rows.Next() {
		var a Activity
		if err := rows.Scan(&a.ID, &a.DayID, &a.Type, &a.Title, &a.StartTime, &a.EndTime,
			&a.SortOrder, &a.Notes, &a.Status, &a.CreatedAt, &a.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		acts = append(acts, a)
	}
	rows.Close()
	return acts, rows.Err()
}

// DayInput carries the fields a caller may set when creating a day.
type DayInput struct {
	Date  *string
	Title *string
}

// CreateDay appends a day at the end of the itinerary. When no date is given and
// the trip has a start date, the day inherits start_date + (day_number - 1).
func (r *Repository) CreateDay(ctx context.Context, tripID string, in DayInput) (*Day, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var next int
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(MAX(day_number), 0) + 1 FROM trip_days WHERE trip_id = $1`, tripID).Scan(&next); err != nil {
		return nil, err
	}

	date := in.Date
	if date == nil {
		date, err = scheduledDate(ctx, tx, tripID, next)
		if err != nil {
			return nil, err
		}
	}

	d, err := scanDay(tx.QueryRow(ctx, `
		INSERT INTO trip_days (trip_id, day_number, date, title)
		VALUES ($1, $2, $3::date, $4)
		RETURNING `+dayCols, tripID, next, date, in.Title))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return d, nil
}

// scheduledDate derives a day's date from the trip's start date, or nil when the
// trip is undated.
func scheduledDate(ctx context.Context, tx pgx.Tx, tripID string, dayNumber int) (*string, error) {
	var start *string
	if err := tx.QueryRow(ctx, `SELECT start_date::text FROM trips WHERE id = $1`, tripID).Scan(&start); err != nil {
		return nil, err
	}
	if start == nil {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", *start)
	if err != nil {
		return nil, nil
	}
	s := t.AddDate(0, 0, dayNumber-1).Format("2006-01-02")
	return &s, nil
}

// DayPatch carries optional day updates; nil = leave unchanged.
type DayPatch struct {
	Date  *string
	Title *string
}

// UpdateDay patches a day that belongs to the trip.
func (r *Repository) UpdateDay(ctx context.Context, tripID, dayID string, p DayPatch) (*Day, error) {
	d, err := scanDay(r.pool.QueryRow(ctx, `
		UPDATE trip_days SET
			date       = COALESCE($3::date, date),
			title      = COALESCE($4, title),
			updated_at = now()
		WHERE id = $2 AND trip_id = $1
		RETURNING `+dayCols, tripID, dayID, p.Date, p.Title))
	if err != nil {
		return nil, err
	}
	acts, err := r.ActivitiesByDay(ctx, dayID)
	if err != nil {
		return nil, err
	}
	d.Activities = acts
	return d, nil
}

// DeleteDay removes a day; its activities cascade via the FK.
func (r *Repository) DeleteDay(ctx context.Context, tripID, dayID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM trip_days WHERE id = $2 AND trip_id = $1`, tripID, dayID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ActivityInput carries creation fields; empty Type/Status fall back to defaults.
type ActivityInput struct {
	Type      string
	Title     string
	StartTime *string
	EndTime   *string
	Notes     *string
	Status    string
}

// CreateActivity appends an activity to the end of a day.
func (r *Repository) CreateActivity(ctx context.Context, dayID string, in ActivityInput) (*Activity, error) {
	var next int
	if err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(sort_order), 0) + 1 FROM activities WHERE trip_day_id = $1`, dayID).Scan(&next); err != nil {
		return nil, err
	}
	return scanActivity(r.pool.QueryRow(ctx, `
		INSERT INTO activities (trip_day_id, type, title, start_time, end_time, sort_order, notes, status)
		VALUES ($1, $2, $3, $4::time, $5::time, $6, $7, $8)
		RETURNING `+actColsBare,
		dayID, orDefault(in.Type, "other"), in.Title, in.StartTime, in.EndTime, next, in.Notes,
		orDefault(in.Status, "planned")))
}

// ActivityPatch carries optional activity updates; nil = leave unchanged.
type ActivityPatch struct {
	Type      *string
	Title     *string
	StartTime *string
	EndTime   *string
	Notes     *string
	Status    *string
}

// UpdateActivity patches an activity.
func (r *Repository) UpdateActivity(ctx context.Context, activityID string, p ActivityPatch) (*Activity, error) {
	return scanActivity(r.pool.QueryRow(ctx, `
		UPDATE activities SET
			type       = COALESCE($2, type),
			title      = COALESCE($3, title),
			start_time = COALESCE($4::time, start_time),
			end_time   = COALESCE($5::time, end_time),
			notes      = COALESCE($6, notes),
			status     = COALESCE($7, status),
			updated_at = now()
		WHERE id = $1
		RETURNING `+actColsBare,
		activityID, p.Type, p.Title, p.StartTime, p.EndTime, p.Notes, p.Status))
}

// DeleteActivity removes one activity.
func (r *Repository) DeleteActivity(ctx context.Context, activityID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM activities WHERE id = $1`, activityID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ReorderActivities assigns sort_order from each id's position in ids. ids must
// list every activity of the day exactly once (the drag-and-drop UI always sends
// the full list), which keeps the order total and tie-free.
func (r *Repository) ReorderActivities(ctx context.Context, dayID string, ids []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM activities WHERE trip_day_id = $1`, dayID).Scan(&count); err != nil {
		return err
	}
	if count != len(ids) {
		return ErrReorderMismatch
	}

	tag, err := tx.Exec(ctx, `
		UPDATE activities SET sort_order = u.ord::int, updated_at = now()
		FROM unnest($1::uuid[]) WITH ORDINALITY AS u(id, ord)
		WHERE activities.id = u.id AND activities.trip_day_id = $2`, ids, dayID)
	if err != nil {
		return err
	}
	if int(tag.RowsAffected()) != len(ids) {
		return ErrReorderMismatch
	}
	return tx.Commit(ctx)
}

func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
