package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// sseBody builds an SSE payload from raw "data:" lines.
func sseBody(lines ...string) string {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString("data: ")
		b.WriteString(l)
		b.WriteString("\n\n")
	}
	b.WriteString("data: [DONE]\n\n")
	return b.String()
}

func textChunk(text string) string {
	return fmt.Sprintf(`{"choices":[{"delta":{"content":%q},"finish_reason":null}]}`, text)
}

func newTestProvider(t *testing.T, handler http.HandlerFunc) *DeepSeek {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	d, err := NewDeepSeek(srv.URL, "test-key", "test-model")
	if err != nil {
		t.Fatalf("NewDeepSeek: %v", err)
	}
	return d
}

func collect(t *testing.T, d *DeepSeek, req ChatRequest) (*ChatResult, []StreamChunk, error) {
	t.Helper()
	var chunks []StreamChunk
	res, err := d.ChatStream(context.Background(), req, func(c StreamChunk) { chunks = append(chunks, c) })
	return res, chunks, err
}

func TestNewDeepSeekRequiresKey(t *testing.T) {
	if _, err := NewDeepSeek("", "", ""); !errors.Is(err, ErrNoAPIKey) {
		t.Errorf("NewDeepSeek without key = %v, want ErrNoAPIKey", err)
	}
}

func TestNewDeepSeekDefaults(t *testing.T) {
	d, err := NewDeepSeek("", "k", "")
	if err != nil {
		t.Fatal(err)
	}
	if d.BaseURL != "https://api.deepseek.com" || d.Model != "deepseek-chat" {
		t.Errorf("defaults = %q/%q", d.BaseURL, d.Model)
	}
}

func TestChatStreamPlainText(t *testing.T) {
	d := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing auth header")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody(
			textChunk("你好"),
			textChunk("，世界"),
			`{"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`,
		))
	})

	res, chunks, err := collect(t, d, ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "你好，世界" {
		t.Errorf("Text = %q", res.Text)
	}
	if res.FinishReason != "stop" {
		t.Errorf("FinishReason = %q", res.FinishReason)
	}
	if res.Usage == nil || res.Usage.PromptTokens != 10 {
		t.Errorf("Usage = %+v", res.Usage)
	}
	// Two text deltas delivered incrementally.
	texts := 0
	for _, c := range chunks {
		if c.TextDelta != "" {
			texts++
		}
	}
	if texts != 2 {
		t.Errorf("text chunks = %d, want 2", texts)
	}
}

func TestChatStreamToolCallSharded(t *testing.T) {
	// Arguments arrive as arbitrary JSON shards sharing index 0.
	d := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody(
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"create_day","arguments":"{\"da"}}]},"finish_reason":null}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"te\":\"2026-10-01\"}"}}]},"finish_reason":null}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		))
	})

	res, chunks, err := collect(t, d, ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.ToolCalls) != 1 {
		t.Fatalf("ToolCalls = %d, want 1", len(res.ToolCalls))
	}
	tc := res.ToolCalls[0]
	if tc.ID != "call_1" || tc.Name != "create_day" {
		t.Errorf("ToolCall = %+v", tc)
	}
	if string(tc.Arguments) != `{"date":"2026-10-01"}` {
		t.Errorf("Arguments = %q", tc.Arguments)
	}
	// One tool-call chunk emitted on completion.
	toolChunks := 0
	for _, c := range chunks {
		if c.ToolCall != nil {
			toolChunks++
		}
	}
	if toolChunks != 1 {
		t.Errorf("tool chunks = %d, want 1", toolChunks)
	}
}

func TestChatStreamMalformedToolArgs(t *testing.T) {
	d := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody(
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"x","arguments":"{not-json"}}]},"finish_reason":null}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		))
	})
	_, _, err := collect(t, d, ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if !errors.Is(err, ErrInvalidResponse) {
		t.Errorf("err = %v, want ErrInvalidResponse", err)
	}
}

func TestChatStreamRetriesOn429(t *testing.T) {
	var calls int32
	d := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody(textChunk("ok"), `{"choices":[{"delta":{},"finish_reason":"stop"}]}`))
	})
	d.MaxRetries = 2

	res, _, err := collect(t, d, ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "ok" {
		t.Errorf("Text = %q", res.Text)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("calls = %d, want 2 (1 retry)", calls)
	}
}

func TestChatStream429ExhaustsRetries(t *testing.T) {
	d := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})
	d.MaxRetries = 1
	_, _, err := collect(t, d, ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("err = %v, want ErrRateLimited", err)
	}
}

func TestChatStream5xx(t *testing.T) {
	d := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	d.MaxRetries = 0
	_, _, err := collect(t, d, ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
}

func TestChatStream4xxNotRetried(t *testing.T) {
	var calls int32
	d := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"bad request"}}`)
	})
	d.MaxRetries = 3
	_, _, err := collect(t, d, ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if !errors.Is(err, ErrInvalidResponse) {
		t.Errorf("err = %v, want ErrInvalidResponse", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("calls = %d, want 1 (no retry on 4xx)", calls)
	}
}

func TestChatStreamContextCancel(t *testing.T) {
	d := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// Slow: outlasts the client timeout but returns so Server.Close can finish.
		time.Sleep(500 * time.Millisecond)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := d.ChatStream(ctx, ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}}, func(StreamChunk) {})
	if err == nil {
		t.Error("expected error on context timeout")
	}
}

// --- ChatOnce (non-streaming) ---

func TestChatOnceParsesSingleResponse(t *testing.T) {
	var gotStream bool
	d := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Stream bool `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotStream = body.Stream
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"a\":1}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`)
	})
	res, err := d.ChatOnce(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("ChatOnce: %v", err)
	}
	if gotStream {
		t.Error("ChatOnce must send stream:false")
	}
	if res.Text != `{"a":1}` {
		t.Errorf("Text = %q", res.Text)
	}
	if res.FinishReason != "stop" {
		t.Errorf("FinishReason = %q", res.FinishReason)
	}
	if res.Usage == nil || res.Usage.PromptTokens != 10 {
		t.Errorf("Usage = %+v", res.Usage)
	}
}

func TestChatOnceEmptyChoices(t *testing.T) {
	d := newTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"choices":[]}`)
	})
	_, err := d.ChatOnce(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if !errors.Is(err, ErrInvalidResponse) {
		t.Errorf("want ErrInvalidResponse, got %v", err)
	}
}

func TestChatOnceRetriesOn429(t *testing.T) {
	var calls atomic.Int32
	d := newTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`)
	})
	res, err := d.ChatOnce(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("ChatOnce: %v", err)
	}
	if res.Text != "ok" || calls.Load() != 2 {
		t.Errorf("text=%q calls=%d", res.Text, calls.Load())
	}
}
