package planner

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/mjlxiaoma/TripWeave/apps/api/internal/ai"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/config"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/trip"
	phttp "github.com/mjlxiaoma/TripWeave/apps/api/pkg/http"
)

// Engine orchestrates one AI chat turn: it assembles context, drives the
// provider's tool-calling loop, executes tools against the repositories and
// streams every step to the client over SSE.
type Engine struct {
	trips    TripStore
	days     DayStore
	repo     Store
	provider ai.Provider
	locker   Locker
	locator  Locator
	cfg      *config.Config
}

// NewEngine wires the planner engine. trips/days/repo are satisfied by the
// concrete repositories; locker by the Redis-backed RedisLocker. locator may be
// nil: without a map provider, activities are simply created unlocated.
func NewEngine(trips TripStore, days DayStore, repo Store, provider ai.Provider, locker Locker, locator Locator, cfg *config.Config) *Engine {
	return &Engine{trips: trips, days: days, repo: repo, provider: provider, locker: locker, locator: locator, cfg: cfg}
}

// --- SSE event payloads ---

type eventMeta struct {
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	TaskID         string `json:"task_id"`
}
type eventToken struct {
	Text string `json:"text"`
}
type eventTool struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Label  string `json:"label"`
	Status string `json:"status,omitempty"`
}
type eventTripUpdated struct {
	Revision    int      `json:"revision"`
	ChangedDays []string `json:"changed_days"`
}
type eventSummary struct {
	Lines   []string     `json:"lines"`
	Changes []ChangeItem `json:"changes"`
}
type eventDone struct {
	MessageID    string    `json:"message_id"`
	Usage        *ai.Usage `json:"usage,omitempty"`
	FinishReason string    `json:"finish_reason"`
}
type eventError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Chat runs one turn and streams the result. It writes SSE directly to w.
// Returns the HTTP status to report when it bails before streaming starts.
func (e *Engine) Chat(w http.ResponseWriter, r *http.Request, tripID, userID, message string) {
	ctx := r.Context()

	if e.provider == nil {
		phttp.Fail(w, http.StatusServiceUnavailable, "AI_UNAVAILABLE", "AI is not configured on this server")
		return
	}

	role, err := e.trips.Role(ctx, tripID, userID)
	if err != nil {
		if errors.Is(err, trip.ErrNotFound) {
			phttp.Fail(w, http.StatusNotFound, "TRIP_NOT_FOUND", "trip not found")
			return
		}
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to authorize")
		return
	}

	lock, err := e.locker.Acquire(ctx, tripID)
	if err != nil {
		if errors.Is(err, ErrBusy) {
			phttp.Fail(w, http.StatusConflict, "AI_BUSY", "an AI generation is already running for this trip")
			return
		}
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to acquire lock")
		return
	}
	defer lock.Release(context.WithoutCancel(ctx))

	detail, err := e.trips.Get(ctx, tripID, userID)
	if err != nil {
		phttp.Fail(w, http.StatusNotFound, "TRIP_NOT_FOUND", "trip not found")
		return
	}
	days, err := e.days.ListDays(ctx, tripID)
	if err != nil {
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to load itinerary")
		return
	}

	conv, err := e.repo.GetOrCreateConversation(ctx, tripID, userID)
	if err != nil {
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to open conversation")
		return
	}
	userMsgID, err := e.repo.AppendMessage(ctx, conv.ID, "user", message, "")
	if err != nil {
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to store message")
		return
	}
	taskID, err := e.repo.CreateTask(ctx, tripID, e.cfg.AIProvider, e.cfg.AIModel, PromptVersion)
	if err != nil {
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "failed to start task")
		return
	}

	sse, ok := ai.NewSSEWriter(w)
	if !ok {
		phttp.Fail(w, http.StatusInternalServerError, "INTERNAL", "streaming unsupported")
		return
	}
	_ = sse.WriteEvent("meta", eventMeta{ConversationID: conv.ID, MessageID: userMsgID, TaskID: taskID})

	// Keep-alive ping so proxies don't time out long generations.
	stopPing := make(chan struct{})
	defer close(stopPing)
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopPing:
				return
			case <-ticker.C:
				sse.Comment("ping")
			}
		}
	}()

	ec := &ExecContext{TripID: tripID, UserID: userID, Role: role, Trip: detail, Days: days}
	tools := e.registry()
	defs := toolDefs(tools)

	history, _ := e.repo.ListRecentMessages(ctx, conv.ID, historyMaxMessages)
	// Exclude the message we just appended (it is the current turn).
	if n := len(history); n > 0 && history[n-1].Role == "user" && history[n-1].Content == message {
		history = history[:n-1]
	}

	msgs := buildMessages(detail, days, history, message)
	allChanges := []ChangeItem{}
	var lastUsage *ai.Usage
	var finishReason string
	var assistantText string

	roundCtx, cancel := context.WithTimeout(ctx, e.cfg.AITimeout)
	defer cancel()

	for round := 0; round < e.cfg.AIMaxToolRounds; round++ {
		toolChoice := "auto"
		if round == e.cfg.AIMaxToolRounds-1 {
			// Last round: force a text answer so we always terminate.
			toolChoice = "none"
		}
		res, err := e.provider.ChatStream(roundCtx, ai.ChatRequest{
			Messages:    msgs,
			Tools:       defs,
			ToolChoice:  toolChoice,
			Temperature: 0.3,
		}, func(c ai.StreamChunk) {
			if c.TextDelta != "" {
				assistantText += c.TextDelta
				_ = sse.WriteEvent("token", eventToken{Text: c.TextDelta})
			}
		})
		if err != nil {
			e.fail(sse, taskID, err)
			return
		}
		lastUsage = res.Usage
		finishReason = res.FinishReason
		if len(res.ToolCalls) == 0 {
			break
		}

		// Persist the assistant turn carrying the tool calls, then feed the
		// assistant message (with tool_calls) back before the tool results —
		// the OpenAI protocol requires tool messages to follow their call.
		asstID, _ := e.repo.AppendMessage(ctx, conv.ID, "assistant", res.Text, e.cfg.AIModel)
		msgs = append(msgs, ai.Message{Role: ai.RoleAssistant, Content: res.Text, ToolCalls: res.ToolCalls})

		for _, tc := range res.ToolCalls {
			tcID, _ := e.repo.RecordToolCall(ctx, asstID, tc.Name, tc.Arguments)
			tool := findTool(tools, tc.Name)
			var result *ToolResult
			if tool == nil {
				result = toolError("未知工具: " + tc.Name)
			} else {
				_ = sse.WriteEvent("tool_start", eventTool{ID: tc.ID, Name: tc.Name})
				result = tool.Run(roundCtx, ec, tc.Arguments)
			}
			status := "success"
			if !result.OK {
				status = "error"
			}
			resultJSON, _ := json.Marshal(result)
			_ = e.repo.FinishToolCall(ctx, tcID, status, resultJSON)
			_ = sse.WriteEvent("tool_result", eventTool{ID: tc.ID, Name: tc.Name, Label: result.Label, Status: status})

			if result.OK && tool != nil && tool.Write {
				// Refresh the snapshot so subsequent tools see fresh state.
				if fresh, err := e.days.ListDays(ctx, tripID); err == nil {
					ec.Days = fresh
				}
				_ = sse.WriteEvent("trip_updated", eventTripUpdated{Revision: round + 1, ChangedDays: changedDays(result.Changes)})
			}
			allChanges = append(allChanges, result.Changes...)
			msgs = append(msgs, ai.Message{Role: ai.RoleTool, ToolCallID: tc.ID, Content: string(resultJSON)})
		}
	}

	// Persist the final assistant text.
	finalID := userMsgID
	if assistantText != "" {
		if id, err := e.repo.AppendMessage(ctx, conv.ID, "assistant", assistantText, e.cfg.AIModel); err == nil {
			finalID = id
		}
	}

	summary := buildSummary(allChanges)
	if len(summary.Lines) > 0 {
		_ = sse.WriteEvent("summary", eventSummary{Lines: summary.Lines, Changes: summary.Changes})
	}

	// Promote the trip to ready once it has an itinerary.
	if fresh, err := e.days.ListDays(ctx, tripID); err == nil && len(fresh) > 0 && detail.Trip.Status == "planning" {
		_, _, _ = e.trips.Update(ctx, tripID, trip.Patch{Status: strPtr("ready")})
	}

	_ = e.repo.FinishTask(ctx, taskID, "completed", "")
	_ = sse.WriteEvent("done", eventDone{MessageID: finalID, Usage: lastUsage, FinishReason: finishReason})
}

// fail emits an SSE error event and marks the task failed.
func (e *Engine) fail(sse *ai.SSEWriter, taskID string, err error) {
	code, msg := "INTERNAL", err.Error()
	switch {
	case errors.Is(err, ai.ErrTimeout):
		code, msg = "AI_TIMEOUT", "AI request timed out"
	case errors.Is(err, ai.ErrRateLimited):
		code, msg = "AI_RATE_LIMITED", "AI provider rate limited"
	case errors.Is(err, ai.ErrInvalidResponse):
		code, msg = "AI_INVALID", "AI returned an invalid response"
	case errors.Is(err, ai.ErrUnavailable):
		code, msg = "AI_UNAVAILABLE", "AI provider unavailable"
	case errors.Is(err, context.Canceled):
		code, msg = "CANCELLED", "request cancelled"
	}
	slog.Error("planner chat failed", "task", taskID, "error", err)
	_ = e.repo.FinishTask(context.Background(), taskID, "failed", msg)
	_ = sse.WriteEvent("error", eventError{Code: code, Message: msg})
}

func changedDays(changes []ChangeItem) []string {
	seen := map[int]bool{}
	out := []string{}
	for _, c := range changes {
		if c.DayNumber > 0 && !seen[c.DayNumber] {
			seen[c.DayNumber] = true
			out = append(out, itoa(c.DayNumber))
		}
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func strPtr(s string) *string { return &s }
