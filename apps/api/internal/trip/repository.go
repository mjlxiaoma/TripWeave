// Package trip holds the trip domain: trips, membership authorization and preferences.
package trip

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a trip does not exist or the user is not a member
// (callers must map both to 404 to avoid leaking trip existence / IDOR probing).
var ErrNotFound = errors.New("trip not found")

// Trip is the core trip entity. Dates are "2006-01-02" strings (nullable).
type Trip struct {
	ID             string    `json:"id"`
	OwnerID        string    `json:"owner_id"`
	Title          string    `json:"title"`
	Destination    *string   `json:"destination"`
	StartDate      *string   `json:"start_date"`
	EndDate        *string   `json:"end_date"`
	TravelersCount *int      `json:"travelers_count"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	// Role is the querying member's role; populated only by ListByUser/Get.
	Role string `json:"-"`

	// HasCover 是否上传了自定义封面(ListByUser 走 EXISTS 子查询;Get/Update 由 coverExists 回填)。
	HasCover bool `json:"has_cover"`
}

// Preference holds per-trip planning preferences (1:1 with trip).
type Preference struct {
	Budget          *float64        `json:"budget"`
	TransportMode   *string         `json:"transport_mode"`
	TravelStyle     *string         `json:"travel_style"`
	Constraints     json.RawMessage `json:"constraints"`
	Preferences     json.RawMessage `json:"preferences"`
	NaturalLanguage *string         `json:"natural_language"`
}

// Repository provides trip persistence.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a trip repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// tripColsBare is for INSERT ... RETURNING (no table alias allowed there).
const tripColsBare = `id, owner_id, title, destination, start_date::text, end_date::text,
	travelers_count, status, created_at, updated_at`

const tripCols = `t.id, t.owner_id, t.title, t.destination, t.start_date::text, t.end_date::text,
	t.travelers_count, t.status, t.created_at, t.updated_at`

func scanTrip(row pgx.Row) (*Trip, error) {
	var t Trip
	err := row.Scan(&t.ID, &t.OwnerID, &t.Title, &t.Destination, &t.StartDate, &t.EndDate,
		&t.TravelersCount, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// CreateParams carries everything the creation wizard submits.
type CreateParams struct {
	Trip Trip
	Pref Preference
}

// Create inserts trip + owner membership + preferences in one transaction.
func (r *Repository) Create(ctx context.Context, p CreateParams) (*Trip, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	t, err := scanTrip(tx.QueryRow(ctx, `
		INSERT INTO trips (owner_id, title, destination, start_date, end_date, travelers_count, status)
		VALUES ($1, $2, $3, $4::date, $5::date, $6, $7)
		RETURNING `+tripColsBare,
		p.Trip.OwnerID, p.Trip.Title, p.Trip.Destination, p.Trip.StartDate, p.Trip.EndDate,
		p.Trip.TravelersCount, statusOrDraft(p.Trip.Status)))
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO trip_members (trip_id, user_id, role) VALUES ($1, $2, 'owner')`,
		t.ID, p.Trip.OwnerID); err != nil {
		return nil, err
	}

	if err := r.upsertPref(ctx, tx, t.ID, &p.Pref); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return t, nil
}

func statusOrDraft(s string) string {
	if s == "" {
		return "draft"
	}
	return s
}

// ListByUser returns all trips the user is a member of, newest first.
// SELECT 中带 has_cover 布尔(EXISTS 子查询),不拖图片本体。
func (r *Repository) ListByUser(ctx context.Context, userID string) ([]Trip, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+tripCols+`, m.role,
			EXISTS(SELECT 1 FROM trip_covers c WHERE c.trip_id = t.id) AS has_cover
		FROM trip_members m
		JOIN trips t ON t.id = m.trip_id
		WHERE m.user_id = $1 AND t.status <> 'deleted'
		ORDER BY t.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	trips := []Trip{}
	for rows.Next() {
		var t Trip
		if err := rows.Scan(&t.ID, &t.OwnerID, &t.Title, &t.Destination, &t.StartDate, &t.EndDate,
			&t.TravelersCount, &t.Status, &t.CreatedAt, &t.UpdatedAt, &t.Role, &t.HasCover); err != nil {
			return nil, err
		}
		trips = append(trips, t)
	}
	return trips, rows.Err()
}

// Detail bundles a trip with its preferences and the requesting user's role.
type Detail struct {
	Trip       *Trip       `json:"-"`
	Preference *Preference `json:"-"`
	Role       string      `json:"-"`
}

// Get loads trip + preferences + caller role. Returns ErrNotFound for non-members.
func (r *Repository) Get(ctx context.Context, tripID, userID string) (*Detail, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT `+tripCols+`, m.role
		FROM trips t
		JOIN trip_members m ON m.trip_id = t.id AND m.user_id = $2
		WHERE t.id = $1`, tripID, userID)

	var t Trip
	var role string
	err := row.Scan(&t.ID, &t.OwnerID, &t.Title, &t.Destination, &t.StartDate, &t.EndDate,
		&t.TravelersCount, &t.Status, &t.CreatedAt, &t.UpdatedAt, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := r.coverExists(ctx, tripID, &t.HasCover); err != nil {
		return nil, err
	}

	pref, err := r.getPref(ctx, tripID)
	if err != nil {
		return nil, err
	}
	return &Detail{Trip: &t, Preference: pref, Role: role}, nil
}

// Role returns the user's membership role, or ErrNotFound when not a member.
func (r *Repository) Role(ctx context.Context, tripID, userID string) (string, error) {
	var role string
	err := r.pool.QueryRow(ctx,
		`SELECT role FROM trip_members WHERE trip_id = $1 AND user_id = $2`, tripID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return role, err
}

// Patch carries optional trip updates; nil = leave unchanged.
type Patch struct {
	Title           *string
	Destination     *string
	StartDate       *string
	EndDate         *string
	TravelersCount  *int
	Status          *string
	Budget          *float64
	TransportMode   *string
	TravelStyle     *string
	Constraints     *json.RawMessage
	Preferences     *json.RawMessage
	NaturalLanguage *string
}

// Update applies a patch to trips + trip_preferences in one transaction.
func (r *Repository) Update(ctx context.Context, tripID string, p Patch) (*Trip, *Preference, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)

	t, err := scanTrip(tx.QueryRow(ctx, `
		UPDATE trips AS t SET
			title           = COALESCE($2, title),
			destination     = COALESCE($3, destination),
			start_date      = COALESCE($4::date, start_date),
			end_date        = COALESCE($5::date, end_date),
			travelers_count = COALESCE($6, travelers_count),
			status          = COALESCE($7, status),
			updated_at      = now()
		WHERE id = $1
		RETURNING `+tripCols,
		tripID, p.Title, p.Destination, p.StartDate, p.EndDate, p.TravelersCount, p.Status))
	if err != nil {
		return nil, nil, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE trip_preferences SET
			budget           = COALESCE($2, budget),
			transport_mode   = COALESCE($3, transport_mode),
			travel_style     = COALESCE($4, travel_style),
			constraints      = COALESCE($5::jsonb, constraints),
			preferences      = COALESCE($6::jsonb, preferences),
			natural_language = COALESCE($7, natural_language),
			updated_at       = now()
		WHERE trip_id = $1`,
		tripID, p.Budget, p.TransportMode, p.TravelStyle,
		rawOrNull(p.Constraints), rawOrNull(p.Preferences), p.NaturalLanguage); err != nil {
		return nil, nil, err
	}

	pref, err := getPrefTx(ctx, tx, tripID)
	if err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	// has_cover 必须回填:调用方(列表卡片)会把响应整体 merge 进本地 state,
	// 零值 false 会把已上传封面"冲掉"(UI 消失,数据仍在)。
	if err := r.coverExists(ctx, tripID, &t.HasCover); err != nil {
		return nil, nil, err
	}
	return t, pref, nil
}

// Delete hard-deletes a trip; FK cascades remove days/activities/members/preferences.
func (r *Repository) Delete(ctx context.Context, tripID, ownerID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM trips WHERE id = $1 AND owner_id = $2`, tripID, ownerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) getPref(ctx context.Context, tripID string) (*Preference, error) {
	return getPrefTx(ctx, r.pool, tripID)
}

type prefQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func getPrefTx(ctx context.Context, q prefQuerier, tripID string) (*Preference, error) {
	var p Preference
	var constraints, prefs []byte
	err := q.QueryRow(ctx, `
		SELECT budget, transport_mode, travel_style, constraints, preferences, natural_language
		FROM trip_preferences WHERE trip_id = $1`, tripID).
		Scan(&p.Budget, &p.TransportMode, &p.TravelStyle, &constraints, &prefs, &p.NaturalLanguage)
	if errors.Is(err, pgx.ErrNoRows) {
		return &Preference{Constraints: json.RawMessage(`{}`), Preferences: json.RawMessage(`[]`)}, nil
	}
	if err != nil {
		return nil, err
	}
	p.Constraints, p.Preferences = json.RawMessage(constraints), json.RawMessage(prefs)
	return &p, nil
}

func (r *Repository) upsertPref(ctx context.Context, tx pgx.Tx, tripID string, p *Preference) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO trip_preferences (trip_id, budget, transport_mode, travel_style, constraints, preferences, natural_language)
		VALUES ($1, $2, $3, $4, COALESCE($5::jsonb, '{}'::jsonb), COALESCE($6::jsonb, '[]'::jsonb), $7)
		ON CONFLICT (trip_id) DO UPDATE SET
			budget = EXCLUDED.budget, transport_mode = EXCLUDED.transport_mode,
			travel_style = EXCLUDED.travel_style, constraints = EXCLUDED.constraints,
			preferences = EXCLUDED.preferences, natural_language = EXCLUDED.natural_language,
			updated_at = now()`,
		tripID, p.Budget, p.TransportMode, p.TravelStyle,
		rawOrNull(&p.Constraints), rawOrNull(&p.Preferences), p.NaturalLanguage)
	return err
}

func rawOrNull(raw *json.RawMessage) any {
	if raw == nil || len(*raw) == 0 {
		return nil
	}
	return []byte(*raw)
}

// --- share token（公开只读链接） ---

// SetShareToken 生成（或复用已有）分享 token，返回当前值。
// 幂等：已存在 token 时直接返回，避免分享链接每次刷新都变。
func (r *Repository) SetShareToken(ctx context.Context, tripID, token string) (string, error) {
	var out string
	err := r.pool.QueryRow(ctx, `
		UPDATE trips
		SET share_token = COALESCE(share_token, $2),
		    updated_at  = now()
		WHERE id = $1
		RETURNING share_token`, tripID, token).Scan(&out)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return out, err
}

// ClearShareToken 撤销分享（token 置空，旧链接立即失效）。
func (r *Repository) ClearShareToken(ctx context.Context, tripID string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE trips SET share_token = NULL, updated_at = now() WHERE id = $1`, tripID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetByShareToken 用公开 token 找行程（无需登录）。找不到即 ErrNotFound。
func (r *Repository) GetByShareToken(ctx context.Context, token string) (*Trip, error) {
	var t Trip
	err := r.pool.QueryRow(ctx, `
		SELECT id, owner_id, title, destination, start_date::text, end_date::text,
		       travelers_count, status, created_at, updated_at
		FROM trips
		WHERE share_token = $1`, token).Scan(
		&t.ID, &t.OwnerID, &t.Title, &t.Destination, &t.StartDate, &t.EndDate,
		&t.TravelersCount, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// --- 自定义封面(trip_covers 表) ---

// coverExists 查询封面存在性并写入 *dst;单条响应(Get/Update)的 HasCover 回填用。
func (r *Repository) coverExists(ctx context.Context, tripID string, dst *bool) error {
	return r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM trip_covers WHERE trip_id = $1)`, tripID).Scan(dst)
}

// SaveCover upserts the trip's custom cover image (content type已由魔数嗅探校验)。
func (r *Repository) SaveCover(ctx context.Context, tripID string, image []byte, contentType string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO trip_covers (trip_id, image, content_type)
		VALUES ($1, $2, $3)
		ON CONFLICT (trip_id) DO UPDATE SET
			image = EXCLUDED.image, content_type = EXCLUDED.content_type, updated_at = now()`,
		tripID, image, contentType)
	return err
}

// GetCover returns the cover image bytes + content type + 更新时间(ETag 用)。无封面即 ErrNotFound。
func (r *Repository) GetCover(ctx context.Context, tripID string) ([]byte, string, time.Time, error) {
	var img []byte
	var ct string
	var updatedAt time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT image, content_type, updated_at FROM trip_covers WHERE trip_id = $1`, tripID).
		Scan(&img, &ct, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", time.Time{}, ErrNotFound
	}
	return img, ct, updatedAt, err
}

// DeleteCover removes the custom cover(幂等:无封面也返回成功)。
func (r *Repository) DeleteCover(ctx context.Context, tripID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM trip_covers WHERE trip_id = $1`, tripID)
	return err
}
