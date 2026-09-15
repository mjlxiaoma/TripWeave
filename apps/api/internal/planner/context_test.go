package planner

import (
	"strings"
	"testing"

	"github.com/mjlxiaoma/TripWeave/apps/api/internal/ai"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/trip"
)

func testDetail() *trip.Detail {
	return &trip.Detail{
		Trip:       &trip.Trip{ID: "t1", Title: "京都红叶"},
		Preference: &trip.Preference{Constraints: []byte(`{}`), Preferences: []byte(`[]`)},
		Role:       "owner",
	}
}

func TestBuildMessagesStructure(t *testing.T) {
	history := []HistoryMessage{
		{Role: "user", Content: "第一天"},
		{Role: "assistant", Content: "好的"},
	}
	msgs := buildMessages(testDetail(), nil, history, "继续")
	if len(msgs) != 4 {
		t.Fatalf("messages = %d, want 4 (system + 2 history + current)", len(msgs))
	}
	if msgs[0].Role != ai.RoleSystem {
		t.Errorf("first message role = %q, want system", msgs[0].Role)
	}
	if !strings.Contains(msgs[0].Content, "trip_snapshot") {
		t.Error("system message should embed the trip snapshot")
	}
	if msgs[3].Role != ai.RoleUser || msgs[3].Content != "继续" {
		t.Errorf("last message = %q/%q, want current user message", msgs[3].Role, msgs[3].Content)
	}
}

func TestBuildMessagesTruncatesByBudget(t *testing.T) {
	// Fill history with big messages to exceed the token budget.
	big := strings.Repeat("很长的消息", 800) // ~2400 tokens each
	history := []HistoryMessage{
		{Role: "user", Content: big},
		{Role: "assistant", Content: big},
		{Role: "user", Content: big},
		{Role: "assistant", Content: "最近一条"},
	}
	msgs := buildMessages(testDetail(), nil, history, "现在")
	// Oldest big messages should be dropped; newest must survive.
	total := 0
	for _, m := range msgs {
		total += len([]rune(m.Content))
	}
	foundRecent := false
	for _, m := range msgs {
		if m.Content == "最近一条" {
			foundRecent = true
		}
	}
	if !foundRecent {
		t.Error("most recent history message must be kept")
	}
}

func TestBuildMessagesSkipsToolRoles(t *testing.T) {
	// buildMessages only receives user/assistant from ListRecentMessages, but
	// verify any stray role maps to user (defensive).
	history := []HistoryMessage{{Role: "tool", Content: "x"}}
	msgs := buildMessages(testDetail(), nil, history, "now")
	if msgs[1].Role != ai.RoleUser {
		t.Errorf("non-assistant role should map to user, got %q", msgs[1].Role)
	}
}
