package ai

import (
	"context"
	"encoding/json"
)

// Role identifies the author of a chat message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is a single chat-completion message. ToolCalls is only set on
// assistant messages; ToolCallID is only set on tool messages.
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolDef describes a function the model may call (OpenAI function-calling
// schema).
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ToolCall is a single function invocation requested by the model.
// Arguments is the raw JSON object produced by the model.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// StreamChunk is one incremental piece of a streaming completion: either a
// text delta or a completed tool call (arguments fully accumulated).
type StreamChunk struct {
	TextDelta string
	ToolCall  *ToolCall // non-nil exactly when a tool call just completed
}

// Usage reports token consumption for a completion, when the provider
// includes it in the stream.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// ChatRequest is a single streaming completion request.
type ChatRequest struct {
	Messages    []Message
	Tools       []ToolDef
	ToolChoice  string  // "auto" or "none"
	Temperature float64 // 0 keeps the provider default
	MaxTokens   int     // 0 keeps the provider default
}

// ChatResult is the final outcome of a streaming completion.
type ChatResult struct {
	Text         string
	ToolCalls    []ToolCall
	FinishReason string
	Usage        *Usage
}

// Provider streams chat completions from an LLM vendor. onChunk is invoked
// synchronously for every incremental piece; implementations must not call
// it after returning.
type Provider interface {
	ChatStream(ctx context.Context, req ChatRequest, onChunk func(StreamChunk)) (*ChatResult, error)
}
