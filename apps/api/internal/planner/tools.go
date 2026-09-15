package planner

import (
	"context"
	"encoding/json"

	"github.com/mjlxiaoma/TripWeave/apps/api/internal/ai"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/day"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/trip"
)

// ExecContext carries everything a tool needs to run: identity, role and the
// trip snapshot taken at the start of the round.
type ExecContext struct {
	TripID string
	UserID string
	Role   string // owner | editor | viewer
	Trip   *trip.Detail
	Days   []day.Day
}

// ToolResult is what a tool returns: data fed back to the model, a human
// label for the SSE progress line, and structured changes for the summary.
type ToolResult struct {
	OK       bool         `json:"ok"`
	Data     any          `json:"data,omitempty"`
	Error    string       `json:"error,omitempty"`
	Label    string       `json:"-"`
	Changes  []ChangeItem `json:"-"`
	Warnings []string     `json:"warnings,omitempty"`
}

// Tool is one function the model may call.
type Tool struct {
	Name        string
	Description string
	Write       bool // write tools are rejected for viewer role
	Schema      json.RawMessage
	Run         func(ctx context.Context, ec *ExecContext, args json.RawMessage) *ToolResult
}

// toolError builds a failed ToolResult whose message is fed back to the model
// so it can self-correct (rather than aborting the round).
func toolError(msg string) *ToolResult {
	return &ToolResult{OK: false, Error: msg, Label: msg}
}

// toolOK builds a successful ToolResult.
func toolOK(label string, data any) *ToolResult {
	return &ToolResult{OK: true, Data: data, Label: label}
}

// registry returns all tools. Order is stable for prompt determinism.
func (e *Engine) registry() []Tool {
	return []Tool{
		e.toolGetTripContext(),
		e.toolCreateItinerary(),
		e.toolCreateDay(),
		e.toolCreateActivity(),
		e.toolUpdateActivity(),
		e.toolDeleteActivity(),
		e.toolReorderActivities(),
		e.toolUpdateTripInfo(),
	}
}

// toolDefs converts the registry to provider tool definitions.
func toolDefs(tools []Tool) []ai.ToolDef {
	out := make([]ai.ToolDef, 0, len(tools))
	for _, t := range tools {
		out = append(out, ai.ToolDef{Name: t.Name, Description: t.Description, Parameters: t.Schema})
	}
	return out
}

// findTool looks up a tool by name.
func findTool(tools []Tool, name string) *Tool {
	for i := range tools {
		if tools[i].Name == name {
			return &tools[i]
		}
	}
	return nil
}

// --- JSON Schemas (OpenAI function.parameters) ---

var schemaGetTripContext = json.RawMessage(`{
  "type": "object", "properties": {}, "additionalProperties": false
}`)

var activitySchema = `{
  "type": "object",
  "properties": {
    "type":       {"type": "string", "enum": ["attraction","restaurant","cafe","hotel","transport","free_time","other"]},
    "title":      {"type": "string", "description": "具体名称，如「清水寺」，不要用占位符"},
    "start_time": {"type": "string", "description": "HH:MM，可选"},
    "end_time":   {"type": "string", "description": "HH:MM，可选"},
    "notes":      {"type": "string", "description": "备注，可选"}
  },
  "required": ["type", "title"],
  "additionalProperties": false
}`

var schemaCreateItinerary = json.RawMessage(`{
  "type": "object",
  "properties": {
    "days": {
      "type": "array",
      "description": "按顺序的多日行程",
      "items": {
        "type": "object",
        "properties": {
          "date":  {"type": "string", "description": "YYYY-MM-DD，可选，须在旅行日期范围内"},
          "title": {"type": "string", "description": "当日主题，可选"},
          "activities": {"type": "array", "items": ` + activitySchema + `}
        },
        "required": ["activities"],
        "additionalProperties": false
      }
    }
  },
  "required": ["days"],
  "additionalProperties": false
}`)

var schemaCreateDay = json.RawMessage(`{
  "type": "object",
  "properties": {
    "date":  {"type": "string", "description": "YYYY-MM-DD，可选"},
    "title": {"type": "string", "description": "当日主题，可选"}
  },
  "additionalProperties": false
}`)

var schemaCreateActivity = json.RawMessage(`{
  "type": "object",
  "properties": {
    "day_number": {"type": "integer", "description": "目标天的 day_number"},
    "type":       {"type": "string", "enum": ["attraction","restaurant","cafe","hotel","transport","free_time","other"]},
    "title":      {"type": "string"},
    "start_time": {"type": "string", "description": "HH:MM，可选"},
    "end_time":   {"type": "string", "description": "HH:MM，可选"},
    "notes":      {"type": "string", "description": "可选"}
  },
  "required": ["day_number", "type", "title"],
  "additionalProperties": false
}`)

var schemaUpdateActivity = json.RawMessage(`{
  "type": "object",
  "properties": {
    "activity_id": {"type": "string", "description": "来自行程快照的活动 id"},
    "type":       {"type": "string", "enum": ["attraction","restaurant","cafe","hotel","transport","free_time","other"]},
    "title":      {"type": "string"},
    "start_time": {"type": "string", "description": "HH:MM"},
    "end_time":   {"type": "string", "description": "HH:MM"},
    "notes":      {"type": "string"},
    "status":     {"type": "string", "enum": ["planned","done","skipped"]}
  },
  "required": ["activity_id"],
  "additionalProperties": false
}`)

var schemaDeleteActivity = json.RawMessage(`{
  "type": "object",
  "properties": {
    "activity_id": {"type": "string", "description": "来自行程快照的活动 id"}
  },
  "required": ["activity_id"],
  "additionalProperties": false
}`)

var schemaReorderActivities = json.RawMessage(`{
  "type": "object",
  "properties": {
    "day_number":    {"type": "integer"},
    "activity_ids":  {"type": "array", "items": {"type": "string"}, "description": "该天全部活动 id 的新顺序，必须一个不漏"}
  },
  "required": ["day_number", "activity_ids"],
  "additionalProperties": false
}`)

var schemaUpdateTripInfo = json.RawMessage(`{
  "type": "object",
  "properties": {
    "title":           {"type": "string"},
    "destination":     {"type": "string"},
    "start_date":      {"type": "string", "description": "YYYY-MM-DD"},
    "end_date":        {"type": "string", "description": "YYYY-MM-DD"},
    "travelers_count": {"type": "integer"},
    "budget":          {"type": "number"},
    "transport_mode":  {"type": "string"},
    "travel_style":    {"type": "string"}
  },
  "additionalProperties": false
}`)
