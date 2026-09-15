package planner

import (
	"strings"
	"testing"
)

func TestBuildSummaryEmpty(t *testing.T) {
	s := buildSummary(nil)
	if len(s.Lines) != 0 {
		t.Errorf("empty changes should yield no lines, got %v", s.Lines)
	}
}

func TestBuildSummaryCreated(t *testing.T) {
	s := buildSummary([]ChangeItem{
		{DayNumber: 1, Kind: "created", Title: "清水寺"},
		{DayNumber: 2, Kind: "created", Title: "金阁寺"},
	})
	joined := strings.Join(s.Lines, "\n")
	if !strings.Contains(joined, "Day 1 已重新规划") || !strings.Contains(joined, "Day 2 已重新规划") {
		t.Errorf("missing day headers: %v", s.Lines)
	}
	if !strings.Contains(joined, "新增：清水寺") {
		t.Errorf("missing created line: %v", s.Lines)
	}
}

func TestBuildSummaryUpdated(t *testing.T) {
	s := buildSummary([]ChangeItem{
		{DayNumber: 2, Kind: "updated", Title: "博物馆", Field: "start_time", From: "14:00", To: "15:30"},
		{DayNumber: 2, Kind: "deleted", Title: "某景点"},
	})
	joined := strings.Join(s.Lines, "\n")
	if !strings.Contains(joined, "调整 开始时间：14:00 → 15:30") {
		t.Errorf("missing update diff: %v", s.Lines)
	}
	if !strings.Contains(joined, "删除：某景点") {
		t.Errorf("missing delete line: %v", s.Lines)
	}
}

func TestBuildSummaryTripInfo(t *testing.T) {
	s := buildSummary([]ChangeItem{{Kind: "trip_info", Title: "目的地、预算"}})
	if !strings.Contains(s.Lines[0], "旅行信息已更新：目的地、预算") {
		t.Errorf("missing trip info line: %v", s.Lines)
	}
}

func TestBuildSummaryDayOrder(t *testing.T) {
	s := buildSummary([]ChangeItem{
		{DayNumber: 3, Kind: "created", Title: "C"},
		{DayNumber: 1, Kind: "created", Title: "A"},
	})
	i1 := indexOf(s.Lines, "Day 1")
	i3 := indexOf(s.Lines, "Day 3")
	if i1 < 0 || i3 < 0 || i1 > i3 {
		t.Errorf("days out of order: %v", s.Lines)
	}
}

func indexOf(lines []string, sub string) int {
	for i, l := range lines {
		if strings.Contains(l, sub) {
			return i
		}
	}
	return -1
}
