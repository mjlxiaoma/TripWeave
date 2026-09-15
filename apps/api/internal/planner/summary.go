package planner

import (
	"fmt"
	"sort"
	"strings"
)

// ChangeItem is one structured modification, aggregated into the summary card.
type ChangeItem struct {
	DayNumber int    `json:"day_number"`
	Kind      string `json:"kind"` // created | updated | deleted | trip_info
	Op        string `json:"op,omitempty"`
	Title     string `json:"title,omitempty"`
	Field     string `json:"field,omitempty"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
}

// Summary aggregates changes per day into a human-readable card, following
// the PRD §17 wording ("Day 2 已重新规划 / 删除：XX / 调整：14:00 → 15:30").
type Summary struct {
	Lines   []string     `json:"lines"`
	Changes []ChangeItem `json:"changes"`
}

// buildSummary groups ChangeItems by day and renders lines. Pure-query rounds
// (no changes) return nil lines so the UI shows no card.
func buildSummary(changes []ChangeItem) *Summary {
	if len(changes) == 0 {
		return &Summary{Lines: []string{}, Changes: []ChangeItem{}}
	}
	byDay := map[int][]ChangeItem{}
	var tripLevel []ChangeItem
	dayKeys := map[int]bool{}
	for _, c := range changes {
		if c.Kind == "trip_info" {
			tripLevel = append(tripLevel, c)
			continue
		}
		byDay[c.DayNumber] = append(byDay[c.DayNumber], c)
		dayKeys[c.DayNumber] = true
	}

	sortedDays := make([]int, 0, len(dayKeys))
	for d := range dayKeys {
		sortedDays = append(sortedDays, d)
	}
	sort.Ints(sortedDays)

	lines := []string{}
	for _, c := range tripLevel {
		lines = append(lines, "旅行信息已更新："+c.Title)
	}
	for _, dn := range sortedDays {
		items := byDay[dn]
		lines = append(lines, fmt.Sprintf("Day %d 已重新规划：", dn))
		for _, c := range items {
			lines = append(lines, "  "+renderChange(c))
		}
	}
	return &Summary{Lines: lines, Changes: changes}
}

func renderChange(c ChangeItem) string {
	switch c.Kind {
	case "created":
		return "新增：" + c.Title
	case "deleted":
		return "删除：" + c.Title
	case "updated":
		if c.Field != "" {
			return fmt.Sprintf("调整 %s：%s → %s", fieldLabel(c.Field), c.From, c.To)
		}
		return "调整：" + c.Title
	default:
		return c.Title
	}
}

func fieldLabel(f string) string {
	switch f {
	case "start_time":
		return "开始时间"
	case "end_time":
		return "结束时间"
	case "title":
		return "名称"
	case "status":
		return "状态"
	case "type":
		return "类型"
	case "time":
		return "时间"
	default:
		return f
	}
}

// joinLabels is a helper for tool labels: "a、b、c".
func joinLabels(parts []string) string {
	return strings.Join(parts, "、")
}
