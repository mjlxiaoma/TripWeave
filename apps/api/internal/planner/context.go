package planner

import (
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/ai"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/day"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/trip"
)

// History is capped by count and by a rough token budget so long threads stay
// within the model's context window.
const (
	historyMaxMessages = 20
	historyTokenBudget = 6000
)

// estimateTokens approximates token count as ~1 token per 1.5 runes (safe
// over-estimate for mixed zh/en content).
func estimateTokens(s string) int {
	return (len([]rune(s)) + 1) / 2
}

// buildMessages assembles the request messages: stable system prompt +
// snapshot, recent history (truncated), and the current user message.
// Tool messages are never replayed — replaying them without their exact
// assistant tool_calls pairing violates the OpenAI protocol.
func buildMessages(detail *trip.Detail, days []day.Day, history []HistoryMessage, current string) []ai.Message {
	msgs := []ai.Message{
		{Role: ai.RoleSystem, Content: buildSystemMessage(detail, days)},
	}

	budget := historyTokenBudget
	kept := make([]HistoryMessage, 0, len(history))
	for i := len(history) - 1; i >= 0; i-- {
		m := history[i]
		cost := estimateTokens(m.Content)
		if budget-cost < 0 {
			break
		}
		budget -= cost
		kept = append(kept, m)
	}
	// kept is newest-first; reverse to chronological.
	for i := len(kept) - 1; i >= 0; i-- {
		m := kept[i]
		role := ai.RoleUser
		if m.Role == "assistant" {
			role = ai.RoleAssistant
		}
		msgs = append(msgs, ai.Message{Role: role, Content: m.Content})
	}

	msgs = append(msgs, ai.Message{Role: ai.RoleUser, Content: current})
	return msgs
}
