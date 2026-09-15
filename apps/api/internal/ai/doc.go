// Package ai provides the LLM provider abstraction for TripWeave.
//
// This package is pure AI infrastructure: it knows how to talk to an
// OpenAI-compatible chat-completions API (streaming, tool calls, retries)
// and how to write Server-Sent Events to an HTTP response. It contains no
// trip domain knowledge — orchestration, prompts and tool execution live in
// internal/planner, which depends on this package one-way (planner → ai).
//
// The Provider interface keeps the vendor pluggable: DeepSeek is the first
// implementation, selected by AI_PROVIDER in config.
package ai
