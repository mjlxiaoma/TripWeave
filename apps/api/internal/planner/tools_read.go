package planner

import (
	"context"
	"encoding/json"
)

// toolGetTripContext returns the current trip + full itinerary to the model.
func (e *Engine) toolGetTripContext() Tool {
	return Tool{
		Name:        "get_trip_context",
		Description: "读取当前旅行的完整信息：基本信息、偏好约束、以及按天排列的全部活动。修改行程前先调用它了解现状。",
		Write:       false,
		Schema:      schemaGetTripContext,
		Run: func(ctx context.Context, ec *ExecContext, _ json.RawMessage) *ToolResult {
			detail, err := e.trips.Get(ctx, ec.TripID, ec.UserID)
			if err != nil {
				return toolError("读取旅行信息失败")
			}
			days, err := e.days.ListDays(ctx, ec.TripID)
			if err != nil {
				return toolError("读取行程失败")
			}
			return toolOK("已读取当前行程", map[string]any{
				"trip": detail.Trip,
				"preference": map[string]any{
					"budget":         detail.Preference.Budget,
					"transport_mode": detail.Preference.TransportMode,
					"travel_style":   detail.Preference.TravelStyle,
					"constraints":    json.RawMessage(detail.Preference.Constraints),
					"preferences":    json.RawMessage(detail.Preference.Preferences),
				},
				"days": days,
			})
		},
	}
}
