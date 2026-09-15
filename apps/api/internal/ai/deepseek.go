package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DeepSeek talks to an OpenAI-compatible streaming chat-completions API
// (https://api.deepseek.com by default). It also works with any other vendor
// exposing the same protocol — only BaseURL/APIKey/Model differ.
type DeepSeek struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client // must NOT set Timeout; deadlines come from ctx
	MaxRetries int          // retries on 429/5xx before any chunk was emitted; default 2
}

// NewDeepSeek validates configuration and returns the provider.
func NewDeepSeek(baseURL, apiKey, model string) (*DeepSeek, error) {
	if apiKey == "" {
		return nil, ErrNoAPIKey
	}
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}
	if model == "" {
		model = "deepseek-chat"
	}
	return &DeepSeek{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		APIKey:     apiKey,
		Model:      model,
		HTTPClient: &http.Client{},
		MaxRetries: 2,
	}, nil
}

// --- wire format (OpenAI-compatible) ---

type wireMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type wireToolCall struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type wireTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type streamOpts struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []wireMessage `json:"messages"`
	Tools       []wireTool    `json:"tools,omitempty"`
	ToolChoice  string        `json:"tool_choice,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream"`
	StreamOpts  *streamOpts   `json:"stream_options,omitempty"`
}

type streamResponse struct {
	Choices []struct {
		Delta struct {
			Content   string              `json:"content"`
			ToolCalls []wireToolCallDelta `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *Usage `json:"usage"`
}

type wireToolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// toolCallBuilder accumulates streamed tool-call fragments. OpenAI streams
// arguments as arbitrary JSON shards; fragments for one call share an index
// and must be concatenated before parsing.
type toolCallBuilder struct {
	id   string
	name string
	args strings.Builder
}

func toWireMessages(msgs []Message) []wireMessage {
	out := make([]wireMessage, 0, len(msgs))
	for _, m := range msgs {
		wm := wireMessage{Role: string(m.Role), Content: m.Content, ToolCallID: m.ToolCallID}
		for _, tc := range m.ToolCalls {
			w := wireToolCall{ID: tc.ID, Type: "function"}
			w.Function.Name = tc.Name
			w.Function.Arguments = string(tc.Arguments)
			wm.ToolCalls = append(wm.ToolCalls, w)
		}
		out = append(out, wm)
	}
	return out
}

func toWireTools(tools []ToolDef) []wireTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]wireTool, 0, len(tools))
	for _, t := range tools {
		w := wireTool{Type: "function"}
		w.Function.Name = t.Name
		w.Function.Description = t.Description
		w.Function.Parameters = t.Parameters
		out = append(out, w)
	}
	return out
}

// ChatStream implements Provider. onChunk fires for every text delta and for
// each tool call the moment its arguments are complete.
func (d *DeepSeek) ChatStream(ctx context.Context, req ChatRequest, onChunk func(StreamChunk)) (*ChatResult, error) {
	attempts := 1 + d.MaxRetries
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second): // 1s, 2s backoff
			}
		}
		res, emitted, err := d.streamOnce(ctx, req, onChunk)
		if err == nil {
			return res, nil
		}
		lastErr = err
		// Only retry when nothing was emitted yet — retrying after partial
		// output would duplicate text the client already received.
		if emitted || !isRetryable(err) {
			return nil, err
		}
	}
	return nil, lastErr
}

func isRetryable(err error) bool {
	return errors.Is(err, ErrRateLimited) || errors.Is(err, ErrUnavailable)
}

func (d *DeepSeek) streamOnce(ctx context.Context, req ChatRequest, onChunk func(StreamChunk)) (*ChatResult, bool, error) {
	body := chatRequest{
		Model:      d.Model,
		Messages:   toWireMessages(req.Messages),
		Tools:      toWireTools(req.Tools),
		ToolChoice: req.ToolChoice,
		Stream:     true,
		StreamOpts: &streamOpts{IncludeUsage: true},
	}
	if req.Temperature > 0 {
		body.Temperature = &req.Temperature
	}
	if req.MaxTokens > 0 {
		body.MaxTokens = req.MaxTokens
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, false, fmt.Errorf("marshal chat request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, d.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, false, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+d.APIKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := d.HTTPClient.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, false, ErrTimeout
		}
		if errors.Is(err, context.Canceled) {
			return nil, false, err
		}
		return nil, false, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, false, ErrRateLimited
	}
	if resp.StatusCode >= 500 {
		return nil, false, fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, false, fmt.Errorf("%w: status %d: %s", ErrInvalidResponse, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	return d.consumeStream(ctx, resp.Body, onChunk)
}

// consumeStream reads the SSE body. The second return value reports whether
// any chunk was delivered to onChunk (used to gate retries).
func (d *DeepSeek) consumeStream(ctx context.Context, body io.Reader, onChunk func(StreamChunk)) (*ChatResult, bool, error) {
	result := &ChatResult{}
	builders := map[int]*toolCallBuilder{}
	order := []int{}
	emitted := false

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk streamResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return result, emitted, fmt.Errorf("%w: malformed stream chunk", ErrInvalidResponse)
		}
		if chunk.Usage != nil {
			result.Usage = chunk.Usage
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		if text := choice.Delta.Content; text != "" {
			result.Text += text
			emitted = true
			onChunk(StreamChunk{TextDelta: text})
		}
		for _, dtc := range choice.Delta.ToolCalls {
			b := builders[dtc.Index]
			if b == nil {
				b = &toolCallBuilder{}
				builders[dtc.Index] = b
				order = append(order, dtc.Index)
			}
			if dtc.ID != "" {
				b.id = dtc.ID
			}
			if dtc.Function.Name != "" {
				b.name = dtc.Function.Name
			}
			b.args.WriteString(dtc.Function.Arguments)
		}
		if choice.FinishReason != nil {
			result.FinishReason = *choice.FinishReason
			if *choice.FinishReason == "tool_calls" {
				for _, idx := range order {
					if builders[idx] != nil {
						b := builders[idx]
						args := b.args.String()
						if args == "" {
							args = "{}"
						}
						tc := &ToolCall{ID: b.id, Name: b.name, Arguments: json.RawMessage(args)}
						result.ToolCalls = append(result.ToolCalls, *tc)
						emitted = true
						onChunk(StreamChunk{ToolCall: tc})
						delete(builders, idx)
					}
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return result, emitted, ctx.Err()
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return result, emitted, ErrTimeout
		}
		return result, emitted, fmt.Errorf("%w: stream read: %v", ErrUnavailable, err)
	}
	// Validate accumulated tool-call arguments.
	for i := range result.ToolCalls {
		var probe any
		if err := json.Unmarshal(result.ToolCalls[i].Arguments, &probe); err != nil {
			return result, emitted, fmt.Errorf("%w: tool %q arguments: %v", ErrInvalidResponse, result.ToolCalls[i].Name, err)
		}
	}
	return result, emitted, nil
}
