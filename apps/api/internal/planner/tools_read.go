package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

// toolGetWeather returns the multi-day weather forecast for a city (Amap).
func (e *Engine) toolGetWeather() Tool {
	return Tool{
		Name:        "get_weather",
		Description: "查询指定城市的天气预报（今天起未来几天）。当用户关心目的地天气、或需要按天气调整户外/室内安排时调用。",
		Write:       false,
		Schema:      schemaGetWeather,
		Run: func(ctx context.Context, ec *ExecContext, args json.RawMessage) *ToolResult {
			var in struct {
				City string `json:"city"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return toolError("参数格式错误: " + err.Error())
			}
			city := strings.TrimSpace(in.City)
			if city == "" {
				city = cityOf(ec)
			}
			if city == "" {
				return toolError("缺少城市：请提供 city 参数")
			}
			if e.locator == nil {
				return toolError("天气服务不可用（未配置地图服务）")
			}
			casts, err := e.locator.Weather(ctx, city)
			if err != nil {
				return toolError("天气查询失败: " + err.Error())
			}
			if len(casts) == 0 {
				return toolError(fmt.Sprintf("没有 %s 的天气预报数据", city))
			}
			return toolOK(fmt.Sprintf("已查询 %s 天气", city), map[string]any{
				"city":     city,
				"forecast": casts,
			})
		},
	}
}

// toolSearchLocations searches real places so the model can bind exact POIs.
func (e *Engine) toolSearchLocations() Tool {
	return Tool{
		Name:        "search_locations",
		Description: "搜索真实地点（景点/餐厅/酒店等），返回名称、地址、经纬度和 location_id。需要把活动绑定到精确地点时先搜索，再在 create_activity / create_itinerary 里带上返回的 location_id。",
		Write:       false,
		Schema:      schemaSearchLocations,
		Run: func(ctx context.Context, ec *ExecContext, args json.RawMessage) *ToolResult {
			var in struct {
				Query string `json:"query"`
				City  string `json:"city"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return toolError("参数格式错误: " + err.Error())
			}
			q := strings.TrimSpace(in.Query)
			if q == "" {
				return toolError("缺少搜索词：请提供 query 参数")
			}
			city := strings.TrimSpace(in.City)
			if city == "" {
				city = cityOf(ec)
			}
			if e.locator == nil {
				return toolError("地点服务不可用（未配置地图服务）")
			}
			locs, err := e.locator.Search(ctx, q, city)
			if err != nil {
				return toolError("地点搜索失败: " + err.Error())
			}
			type item struct {
				LocationID string  `json:"location_id"`
				Name       string  `json:"name"`
				Address    *string `json:"address,omitempty"`
				City       *string `json:"city,omitempty"`
				Latitude   float64 `json:"latitude"`
				Longitude  float64 `json:"longitude"`
			}
			items := make([]item, 0, len(locs))
			for _, l := range locs {
				items = append(items, item{
					LocationID: l.ID,
					Name:       l.Name,
					Address:    l.Address,
					City:       l.City,
					Latitude:   l.Latitude,
					Longitude:  l.Longitude,
				})
			}
			if len(items) == 0 {
				return toolError(fmt.Sprintf("没有找到「%s」相关地点", q))
			}
			return toolOK(fmt.Sprintf("找到 %d 个地点", len(items)), map[string]any{"locations": items})
		},
	}
}
