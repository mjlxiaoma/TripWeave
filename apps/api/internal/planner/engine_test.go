package planner

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mjlxiaoma/TripWeave/apps/api/internal/ai"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/config"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/day"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/trip"
)

// --- fakes ---

type fakeProvider struct {
	// script returns the next ChatResult per call; when exhausted, returns text.
	results []*ai.ChatResult
	calls   int
}

// ChatOnce satisfies ai.Provider (engine only exercises ChatStream).
func (f *fakeProvider) ChatOnce(context.Context, ai.ChatRequest) (*ai.ChatResult, error) {
	return &ai.ChatResult{}, nil
}

func (f *fakeProvider) ChatStream(ctx context.Context, req ai.ChatRequest, onChunk func(ai.StreamChunk)) (*ai.ChatResult, error) {
	f.calls++
	if f.calls-1 < len(f.results) {
		res := f.results[f.calls-1]
		for _, tc := range res.ToolCalls {
			tc := tc
			onChunk(ai.StreamChunk{ToolCall: &tc})
		}
		if res.Text != "" {
			onChunk(ai.StreamChunk{TextDelta: res.Text})
		}
		return res, nil
	}
	return &ai.ChatResult{Text: "完成", FinishReason: "stop"}, nil
}

type fakeTrips struct {
	detail *trip.Detail
	role   string
}

func (f *fakeTrips) Get(ctx context.Context, tripID, userID string) (*trip.Detail, error) {
	return f.detail, nil
}
func (f *fakeTrips) Role(ctx context.Context, tripID, userID string) (string, error) {
	if f.role == "" {
		return "owner", nil
	}
	return f.role, nil
}
func (f *fakeTrips) Update(ctx context.Context, tripID string, p trip.Patch) (*trip.Trip, *trip.Preference, error) {
	return f.detail.Trip, f.detail.Preference, nil
}

type fakeDays struct {
	days     []day.Day
	created  []day.ItineraryDayInput
	activity *day.Activity
}

func (f *fakeDays) ListDays(ctx context.Context, tripID string) ([]day.Day, error) {
	return f.days, nil
}
func (f *fakeDays) CreateItinerary(ctx context.Context, tripID string, in []day.ItineraryDayInput) ([]day.Day, error) {
	f.created = in
	out := make([]day.Day, 0, len(in))
	for i, d := range in {
		nd := day.Day{ID: "d" + itoa(i+1), TripID: tripID, DayNumber: i + 1, Date: d.Date}
		for _, a := range d.Activities {
			nd.Activities = append(nd.Activities, day.Activity{ID: "a", DayID: nd.ID, Type: a.Type, Title: a.Title})
		}
		out = append(out, nd)
	}
	f.days = out
	return out, nil
}
func (f *fakeDays) CreateDay(ctx context.Context, tripID string, in day.DayInput) (*day.Day, error) {
	return &day.Day{ID: "dx", TripID: tripID, DayNumber: len(f.days) + 1}, nil
}
func (f *fakeDays) CreateActivity(ctx context.Context, dayID string, in day.ActivityInput) (*day.Activity, error) {
	return &day.Activity{ID: "ax", DayID: dayID, Type: in.Type, Title: in.Title}, nil
}
func (f *fakeDays) UpdateActivity(ctx context.Context, id string, p day.ActivityPatch) (*day.Activity, error) {
	return f.activity, nil
}
func (f *fakeDays) DeleteActivity(ctx context.Context, id string) error { return nil }
func (f *fakeDays) ReorderActivities(ctx context.Context, dayID string, ids []string) error {
	return nil
}
func (f *fakeDays) TripIDForActivity(ctx context.Context, activityID string) (string, error) {
	return "t1", nil
}
func (f *fakeDays) GetActivity(ctx context.Context, activityID string) (*day.Activity, error) {
	if f.activity != nil {
		return f.activity, nil
	}
	return &day.Activity{ID: activityID, Title: "旧活动"}, nil
}

type fakeStore struct {
	msgs []HistoryMessage
}

func (f *fakeStore) GetOrCreateConversation(ctx context.Context, tripID, userID string) (*Conversation, error) {
	return &Conversation{ID: "c1", TripID: tripID, UserID: userID}, nil
}
func (f *fakeStore) AppendMessage(ctx context.Context, cid, role, content, model string) (string, error) {
	return "m1", nil
}
func (f *fakeStore) RecordToolCall(ctx context.Context, mid, name string, args json.RawMessage) (string, error) {
	return "tc1", nil
}
func (f *fakeStore) FinishToolCall(ctx context.Context, id, status string, result json.RawMessage) error {
	return nil
}
func (f *fakeStore) CreateTask(ctx context.Context, tripID, provider, model, pv string) (string, error) {
	return "task1", nil
}
func (f *fakeStore) FinishTask(ctx context.Context, id, status, errMsg string) error { return nil }
func (f *fakeStore) ListRecentMessages(ctx context.Context, cid string, limit int) ([]HistoryMessage, error) {
	return f.msgs, nil
}

type fakeLock struct{ released *bool }

func (l fakeLock) Release(ctx context.Context) { *l.released = true }

type fakeLocker struct {
	busy     bool
	released *bool
}

func (l *fakeLocker) Acquire(ctx context.Context, tripID string) (Lock, error) {
	if l.busy {
		return nil, ErrBusy
	}
	return fakeLock{released: l.released}, nil
}

// --- helpers ---

func newEngine(t *testing.T, p ai.Provider, tr TripStore, d *fakeDays, lk *fakeLocker) *Engine {
	t.Helper()
	cfg := config.Load()
	cfg.AIMaxToolRounds = 5
	cfg.AITimeout = 5 * time.Second
	return NewEngine(tr, d, &fakeStore{}, p, lk, nil, cfg)
}

func baseDetail() *trip.Detail {
	sd, ed := "2026-10-01", "2026-10-03"
	return &trip.Detail{
		Trip:       &trip.Trip{ID: "t1", Title: "京都", StartDate: &sd, EndDate: &ed, Status: "planning"},
		Preference: &trip.Preference{Constraints: []byte(`{}`), Preferences: []byte(`[]`)},
		Role:       "owner",
	}
}

func runChat(t *testing.T, e *Engine, msg string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/x", nil)
	rec := httptest.NewRecorder()
	e.Chat(rec, req, "11111111-1111-1111-1111-111111111111", "u1", msg)
	return rec
}

func TestChatSingleRoundText(t *testing.T) {
	released := false
	p := &fakeProvider{results: []*ai.ChatResult{{Text: "好的", FinishReason: "stop"}}}
	e := newEngine(t, p, &fakeTrips{detail: baseDetail()}, &fakeDays{}, &fakeLocker{released: &released})

	rec := runChat(t, e, "你好")
	body := rec.Body.String()
	if !strings.Contains(body, "event: token") {
		t.Errorf("expected token event, got:\n%s", body)
	}
	if !strings.Contains(body, "event: done") {
		t.Errorf("expected done event, got:\n%s", body)
	}
	if !released {
		t.Error("lock should be released after chat")
	}
}

func TestChatToolCallCreateItinerary(t *testing.T) {
	released := false
	args := `{"days":[{"date":"2026-10-01","activities":[{"type":"attraction","title":"清水寺","start_time":"09:00","end_time":"11:00"}]}]}`
	p := &fakeProvider{results: []*ai.ChatResult{
		{FinishReason: "tool_calls", ToolCalls: []ai.ToolCall{{ID: "c1", Name: "create_itinerary", Arguments: json.RawMessage(args)}}},
		{Text: "已为你生成行程", FinishReason: "stop"},
	}}
	d := &fakeDays{}
	e := newEngine(t, p, &fakeTrips{detail: baseDetail()}, d, &fakeLocker{released: &released})

	rec := runChat(t, e, "帮我规划3天")
	body := rec.Body.String()
	if !strings.Contains(body, "event: tool_start") {
		t.Errorf("expected tool_start, got:\n%s", body)
	}
	if !strings.Contains(body, `"status":"success"`) {
		t.Errorf("expected success tool_result, got:\n%s", body)
	}
	if !strings.Contains(body, "event: trip_updated") {
		t.Errorf("expected trip_updated, got:\n%s", body)
	}
	if !strings.Contains(body, "event: summary") {
		t.Errorf("expected summary, got:\n%s", body)
	}
	if len(d.created) != 1 || len(d.created[0].Activities) != 1 {
		t.Errorf("itinerary not persisted: %+v", d.created)
	}
}

func TestChatViewerDeniedWrite(t *testing.T) {
	released := false
	args := `{"days":[{"activities":[{"type":"attraction","title":"X"}]}]}`
	p := &fakeProvider{results: []*ai.ChatResult{
		{FinishReason: "tool_calls", ToolCalls: []ai.ToolCall{{ID: "c1", Name: "create_itinerary", Arguments: json.RawMessage(args)}}},
		{Text: "抱歉", FinishReason: "stop"},
	}}
	d := &fakeDays{}
	tr := &fakeTrips{detail: baseDetail(), role: "viewer"}
	e := newEngine(t, p, tr, d, &fakeLocker{released: &released})

	rec := runChat(t, e, "帮我规划")
	body := rec.Body.String()
	if !strings.Contains(body, `"status":"error"`) {
		t.Errorf("viewer write should error, got:\n%s", body)
	}
	if len(d.created) != 0 {
		t.Error("viewer write must not persist")
	}
}

func TestChatInvalidToolArgsFedBack(t *testing.T) {
	released := false
	p := &fakeProvider{results: []*ai.ChatResult{
		{FinishReason: "tool_calls", ToolCalls: []ai.ToolCall{{ID: "c1", Name: "create_itinerary", Arguments: json.RawMessage(`{"days":[]}`)}}},
		{Text: "修正", FinishReason: "stop"},
	}}
	d := &fakeDays{}
	e := newEngine(t, p, &fakeTrips{detail: baseDetail()}, d, &fakeLocker{released: &released})

	rec := runChat(t, e, "空行程")
	body := rec.Body.String()
	if !strings.Contains(body, `"status":"error"`) {
		t.Errorf("invalid args should produce error tool_result, got:\n%s", body)
	}
	if len(d.created) != 0 {
		t.Error("invalid itinerary must not persist")
	}
}

func TestChatBusyLock(t *testing.T) {
	p := &fakeProvider{}
	e := newEngine(t, p, &fakeTrips{detail: baseDetail()}, &fakeDays{}, &fakeLocker{busy: true})
	rec := runChat(t, e, "hi")
	if rec.Code != 409 {
		t.Errorf("busy lock status = %d, want 409", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "AI_BUSY") {
		t.Errorf("expected AI_BUSY, got %s", rec.Body.String())
	}
}

func TestChatProviderError(t *testing.T) {
	released := false
	p := &errProvider{err: ai.ErrTimeout}
	e := newEngine(t, p, &fakeTrips{detail: baseDetail()}, &fakeDays{}, &fakeLocker{released: &released})
	rec := runChat(t, e, "hi")
	if !strings.Contains(rec.Body.String(), "AI_TIMEOUT") {
		t.Errorf("expected AI_TIMEOUT event, got:\n%s", rec.Body.String())
	}
}

type errProvider struct{ err error }

// ChatOnce satisfies ai.Provider (engine only exercises ChatStream).
func (e *errProvider) ChatOnce(context.Context, ai.ChatRequest) (*ai.ChatResult, error) {
	return nil, e.err
}

func (e *errProvider) ChatStream(ctx context.Context, req ai.ChatRequest, onChunk func(ai.StreamChunk)) (*ai.ChatResult, error) {
	return nil, e.err
}

func TestChatNonMember404(t *testing.T) {
	p := &fakeProvider{}
	tr := &notFoundTrips{}
	e := newEngine(t, p, tr, &fakeDays{}, &fakeLocker{released: new(bool)})
	rec := runChat(t, e, "hi")
	if rec.Code != 404 {
		t.Errorf("non-member status = %d, want 404", rec.Code)
	}
}

type notFoundTrips struct{ fakeTrips }

func (n *notFoundTrips) Role(ctx context.Context, tripID, userID string) (string, error) {
	return "", trip.ErrNotFound
}
func (n *notFoundTrips) Get(ctx context.Context, tripID, userID string) (*trip.Detail, error) {
	return nil, trip.ErrNotFound
}

var _ = errors.Is // keep import used when asserts change
