package ai

import "errors"

// Sentinel errors returned by providers. Callers (the planner engine) map
// them to SSE error codes; they must stay comparable with errors.Is.
var (
	// ErrRateLimited means the provider rejected the request with HTTP 429.
	ErrRateLimited = errors.New("ai: provider rate limited")

	// ErrTimeout means the request exceeded its context deadline.
	ErrTimeout = errors.New("ai: provider request timed out")

	// ErrInvalidResponse means the provider returned a body that could not
	// be parsed (non-JSON, malformed SSE, truncated tool-call arguments).
	ErrInvalidResponse = errors.New("ai: invalid provider response")

	// ErrUnavailable means the provider is unreachable or returned 5xx
	// after exhausting retries.
	ErrUnavailable = errors.New("ai: provider unavailable")

	// ErrNoAPIKey means the provider was constructed without credentials.
	ErrNoAPIKey = errors.New("ai: API key not configured")
)
