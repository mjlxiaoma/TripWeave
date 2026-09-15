// Package planner orchestrates AI-driven trip planning: it owns the prompt,
// the tool registry and the tool-calling loop, and persists conversations to
// the ai_* tables. It depends on internal/ai for the LLM transport and on
// trip/day repositories for reads and writes — the LLM never touches the
// database directly.
package planner

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository persists AI conversations, messages, tool calls and generation
// tasks.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates the planner repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Conversation is one AI chat thread bound to a trip.
type Conversation struct {
	ID     string
	TripID string
	UserID string
}

// GetOrCreateConversation returns the user's single conversation for a trip,
// creating it on first use. The unique-per-(trip,user) invariant is enforced
// by taking the most recent row under a transaction.
func (r *Repository) GetOrCreateConversation(ctx context.Context, tripID, userID string) (*Conversation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var c Conversation
	err = tx.QueryRow(ctx, `
		SELECT id, trip_id, user_id FROM ai_conversations
		WHERE trip_id = $1 AND user_id = $2
		ORDER BY created_at DESC LIMIT 1`, tripID, userID).
		Scan(&c.ID, &c.TripID, &c.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `
			INSERT INTO ai_conversations (user_id, trip_id) VALUES ($1, $2)
			RETURNING id, trip_id, user_id`, userID, tripID).
			Scan(&c.ID, &c.TripID, &c.UserID)
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &c, nil
}

// AppendMessage stores one message and returns its id.
func (r *Repository) AppendMessage(ctx context.Context, conversationID, role, content, model string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO ai_messages (conversation_id, role, content, model)
		VALUES ($1, $2, $3, NULLIF($4, ''))
		RETURNING id`, conversationID, role, content, model).Scan(&id)
	return id, err
}

// RecordToolCall stores a tool invocation against its assistant message and
// returns its id (status left NULL until FinishToolCall).
func (r *Repository) RecordToolCall(ctx context.Context, messageID, name string, args json.RawMessage) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO ai_tool_calls (message_id, tool_name, arguments)
		VALUES ($1, $2, $3::jsonb)
		RETURNING id`, messageID, name, rawOrNull(args)).Scan(&id)
	return id, err
}

// FinishToolCall sets the outcome of a recorded tool call.
func (r *Repository) FinishToolCall(ctx context.Context, id, status string, result json.RawMessage) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE ai_tool_calls SET status = $2, result = $3::jsonb WHERE id = $1`,
		id, status, rawOrNull(result))
	return err
}

// CreateTask starts a generation task row and returns its id.
func (r *Repository) CreateTask(ctx context.Context, tripID, provider, model, promptVersion string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO trip_generation_tasks (trip_id, status, provider, model, prompt_version, started_at)
		VALUES ($1, 'running', $2, $3, $4, now())
		RETURNING id`, tripID, provider, model, promptVersion).Scan(&id)
	return id, err
}

// FinishTask marks a generation task completed/failed with an optional error.
func (r *Repository) FinishTask(ctx context.Context, id, status, errMsg string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE trip_generation_tasks SET status = $2, error = NULLIF($3, ''), completed_at = now()
		WHERE id = $1`, id, status, errMsg)
	return err
}

// HistoryMessage is a past message replayed into the model context.
type HistoryMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ListRecentMessages returns the most recent user/assistant text messages of
// a conversation, oldest-first, capped at limit. Tool messages are excluded —
// replaying them without their exact tool_call pairing breaks the OpenAI
// protocol (400).
func (r *Repository) ListRecentMessages(ctx context.Context, conversationID string, limit int) ([]HistoryMessage, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT role, content FROM (
			SELECT role, content, created_at
			FROM ai_messages
			WHERE conversation_id = $1 AND role IN ('user','assistant') AND content <> ''
			ORDER BY created_at DESC LIMIT $2
		) recent ORDER BY created_at`, conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HistoryMessage{}
	for rows.Next() {
		var m HistoryMessage
		if err := rows.Scan(&m.Role, &m.Content); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func rawOrNull(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return []byte(raw)
}
