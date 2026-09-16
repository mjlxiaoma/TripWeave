package planner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mjlxiaoma/TripWeave/apps/api/internal/day"
	"github.com/mjlxiaoma/TripWeave/apps/api/internal/trip"
)

// writeDenied is returned when a viewer tries a write tool.
func writeDenied() *ToolResult {
	return toolError("你当前是只读成员，无法修改行程")
}

// toolCreateItinerary bulk-creates the whole itinerary (cold start only).
func (e *Engine) toolCreateItinerary() Tool {
	return Tool{
		Name:        "create_itinerary",
		Description: "当行程为空时，一次性创建完整的多日行程方案。仅在当前没有任何日程（days 为空）时可用；已有内容时请改用细粒度工具做增量修改。",
		Write:       true,
		Schema:      schemaCreateItinerary,
		Run: func(ctx context.Context, ec *ExecContext, args json.RawMessage) *ToolResult {
			if ec.Role == "viewer" {
				return writeDenied()
			}
			var in ItineraryArgs
			if err := json.Unmarshal(args, &in); err != nil {
				return toolError("参数格式错误: " + err.Error())
			}
			// Only allowed on an empty itinerary to avoid clobbering manual edits.
			if len(ec.Days) > 0 {
				return toolError("当前行程已有内容，不能用 create_itinerary 整体覆盖；请改用 create_day / create_activity 等细粒度工具")
			}
			if errs := validateItinerary(in, ec.Trip.Trip.StartDate, ec.Trip.Trip.EndDate); len(errs) > 0 {
				return toolError("行程校验未通过，请修正后重试：\n" + strings.Join(errs, "\n"))
			}

			input := make([]day.ItineraryDayInput, 0, len(in.Days))
			for _, d := range in.Days {
				acts := make([]day.ActivityInput, 0, len(d.Activities))
				for _, a := range d.Activities {
					acts = append(acts, day.ActivityInput{
						Type: a.Type, Title: a.Title, StartTime: a.StartTime, EndTime: a.EndTime, Notes: a.Notes,
					})
				}
				input = append(input, day.ItineraryDayInput{Date: d.Date, Title: d.Title, Activities: acts})
			}
			created, err := e.days.CreateItinerary(ctx, ec.TripID, input)
			if err != nil {
				return toolError("创建行程失败: " + err.Error())
			}

			// Best-effort geocoding in the background so the map panel has
			// coordinates without slowing down the tool response.
			flat := make([]day.Activity, 0, len(created)*4)
			for _, d := range created {
				flat = append(flat, d.Activities...)
			}
			e.locateActivities(flat, cityOf(ec))

			changes := []ChangeItem{}
			for _, d := range created {
				for _, a := range d.Activities {
					changes = append(changes, ChangeItem{DayNumber: d.DayNumber, Kind: "created", Title: a.Title})
				}
			}
			res := toolOK(fmt.Sprintf("已创建 %d 天行程", len(created)), map[string]any{"days": created})
			res.Changes = changes
			res.Warnings = overlapWarnings(created)
			return res
		},
	}
}

// toolCreateDay appends one day.
func (e *Engine) toolCreateDay() Tool {
	return Tool{
		Name:        "create_day",
		Description: "在行程末尾追加一天。",
		Write:       true,
		Schema:      schemaCreateDay,
		Run: func(ctx context.Context, ec *ExecContext, args json.RawMessage) *ToolResult {
			if ec.Role == "viewer" {
				return writeDenied()
			}
			var in struct {
				Date  *string `json:"date"`
				Title *string `json:"title"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return toolError("参数格式错误: " + err.Error())
			}
			if len(ec.Days) >= maxDays {
				return toolError(fmt.Sprintf("天数已达上限 %d", maxDays))
			}
			if in.Date != nil {
				if !validDate(*in.Date) {
					return toolError("date 格式应为 YYYY-MM-DD")
				}
				if !dateWithinTrip(*in.Date, ec.Trip.Trip.StartDate, ec.Trip.Trip.EndDate) {
					return toolError("date 不在旅行日期范围内")
				}
			}
			d, err := e.days.CreateDay(ctx, ec.TripID, day.DayInput{Date: in.Date, Title: in.Title})
			if err != nil {
				return toolError("创建失败: " + err.Error())
			}
			res := toolOK(fmt.Sprintf("已创建 Day %d", d.DayNumber), d)
			res.Changes = []ChangeItem{{DayNumber: d.DayNumber, Kind: "created", Title: fmt.Sprintf("Day %d", d.DayNumber)}}
			return res
		},
	}
}

// toolCreateActivity adds one activity to a day (by day_number, not uuid).
func (e *Engine) toolCreateActivity() Tool {
	return Tool{
		Name:        "create_activity",
		Description: "在指定某一天（day_number）的末尾添加一个活动。",
		Write:       true,
		Schema:      schemaCreateActivity,
		Run: func(ctx context.Context, ec *ExecContext, args json.RawMessage) *ToolResult {
			if ec.Role == "viewer" {
				return writeDenied()
			}
			var in struct {
				DayNumber int     `json:"day_number"`
				Type      string  `json:"type"`
				Title     string  `json:"title"`
				StartTime *string `json:"start_time"`
				EndTime   *string `json:"end_time"`
				Notes     *string `json:"notes"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return toolError("参数格式错误: " + err.Error())
			}
			target := findDayByNumber(ec.Days, in.DayNumber)
			if target == nil {
				return toolError(fmt.Sprintf("day_number %d 不属于当前行程", in.DayNumber))
			}
			if len(target.Activities) >= maxActsPerDay {
				return toolError(fmt.Sprintf("该天活动数已达上限 %d", maxActsPerDay))
			}
			if errs := validateActivity(ActivityCreateArg{
				Type: in.Type, Title: in.Title, StartTime: in.StartTime, EndTime: in.EndTime, Notes: in.Notes,
			}, in.DayNumber-1, len(target.Activities)); len(errs) > 0 {
				return toolError(strings.Join(errs, "\n"))
			}
			a, err := e.days.CreateActivity(ctx, target.ID, day.ActivityInput{
				Type: in.Type, Title: in.Title, StartTime: in.StartTime, EndTime: in.EndTime, Notes: in.Notes,
			})
			if err != nil {
				return toolError("创建失败: " + err.Error())
			}
			e.locateActivities([]day.Activity{*a}, cityOf(ec))
			res := toolOK(fmt.Sprintf("已在 Day %d 添加「%s」", in.DayNumber, a.Title), a)
			res.Changes = []ChangeItem{{DayNumber: in.DayNumber, Kind: "created", Title: a.Title}}
			return res
		},
	}
}

// toolUpdateActivity patches an activity, capturing before/after for the summary.
func (e *Engine) toolUpdateActivity() Tool {
	return Tool{
		Name:        "update_activity",
		Description: "修改一个活动的字段（时间、名称、类型、状态等）。activity_id 必须来自行程快照。",
		Write:       true,
		Schema:      schemaUpdateActivity,
		Run: func(ctx context.Context, ec *ExecContext, args json.RawMessage) *ToolResult {
			if ec.Role == "viewer" {
				return writeDenied()
			}
			var in struct {
				ActivityID string  `json:"activity_id"`
				Type       *string `json:"type"`
				Title      *string `json:"title"`
				StartTime  *string `json:"start_time"`
				EndTime    *string `json:"end_time"`
				Notes      *string `json:"notes"`
				Status     *string `json:"status"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return toolError("参数格式错误: " + err.Error())
			}
			if in.ActivityID == "" {
				return toolError("缺少 activity_id")
			}
			// Ownership check: the activity must belong to this trip.
			ownerTrip, err := e.days.TripIDForActivity(ctx, in.ActivityID)
			if err != nil || ownerTrip != ec.TripID {
				return toolError("该活动不属于当前行程")
			}
			if in.Type != nil && !validType(*in.Type) {
				return toolError("type 非法: " + *in.Type)
			}
			if in.Status != nil && !validStatus(*in.Status) {
				return toolError("status 非法: " + *in.Status)
			}
			if in.StartTime != nil && !validHHMM(*in.StartTime) {
				return toolError("start_time 格式应为 HH:MM")
			}
			if in.EndTime != nil && !validHHMM(*in.EndTime) {
				return toolError("end_time 格式应为 HH:MM")
			}

			before, err := e.days.GetActivity(ctx, in.ActivityID)
			if err != nil {
				return toolError("读取活动失败")
			}
			// Time-order check against the merged result.
			mergedStart := pick(in.StartTime, before.StartTime)
			mergedEnd := pick(in.EndTime, before.EndTime)
			if !timeOrderOK(mergedStart, mergedEnd) {
				return toolError("start_time 必须早于 end_time")
			}

			updated, err := e.days.UpdateActivity(ctx, in.ActivityID, day.ActivityPatch{
				Type: in.Type, Title: in.Title, StartTime: in.StartTime,
				EndTime: in.EndTime, Notes: in.Notes, Status: in.Status,
			})
			if err != nil {
				return toolError("更新失败: " + err.Error())
			}

			dn := dayNumberOf(ec.Days, in.ActivityID)
			changes := diffActivity(dn, before, updated)
			res := toolOK(fmt.Sprintf("已更新「%s」", updated.Title), updated)
			res.Changes = changes
			return res
		},
	}
}

// toolDeleteActivity removes one activity.
func (e *Engine) toolDeleteActivity() Tool {
	return Tool{
		Name:        "delete_activity",
		Description: "删除一个活动。activity_id 必须来自行程快照。",
		Write:       true,
		Schema:      schemaDeleteActivity,
		Run: func(ctx context.Context, ec *ExecContext, args json.RawMessage) *ToolResult {
			if ec.Role == "viewer" {
				return writeDenied()
			}
			var in struct {
				ActivityID string `json:"activity_id"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return toolError("参数格式错误: " + err.Error())
			}
			ownerTrip, err := e.days.TripIDForActivity(ctx, in.ActivityID)
			if err != nil || ownerTrip != ec.TripID {
				return toolError("该活动不属于当前行程")
			}
			before, err := e.days.GetActivity(ctx, in.ActivityID)
			if err != nil {
				return toolError("读取活动失败")
			}
			if err := e.days.DeleteActivity(ctx, in.ActivityID); err != nil {
				return toolError("删除失败: " + err.Error())
			}
			dn := dayNumberOf(ec.Days, in.ActivityID)
			res := toolOK(fmt.Sprintf("已删除「%s」", before.Title), map[string]any{"deleted": in.ActivityID})
			res.Changes = []ChangeItem{{DayNumber: dn, Kind: "deleted", Title: before.Title}}
			return res
		},
	}
}

// toolReorderActivities sets a day's full activity order.
func (e *Engine) toolReorderActivities() Tool {
	return Tool{
		Name:        "reorder_activities",
		Description: "调整某一天内活动的顺序。activity_ids 必须包含该天全部活动 id，一个不漏。",
		Write:       true,
		Schema:      schemaReorderActivities,
		Run: func(ctx context.Context, ec *ExecContext, args json.RawMessage) *ToolResult {
			if ec.Role == "viewer" {
				return writeDenied()
			}
			var in struct {
				DayNumber   int      `json:"day_number"`
				ActivityIDs []string `json:"activity_ids"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return toolError("参数格式错误: " + err.Error())
			}
			target := findDayByNumber(ec.Days, in.DayNumber)
			if target == nil {
				return toolError(fmt.Sprintf("day_number %d 不属于当前行程", in.DayNumber))
			}
			if err := e.days.ReorderActivities(ctx, target.ID, in.ActivityIDs); err != nil {
				if errors.Is(err, day.ErrReorderMismatch) {
					return toolError("activity_ids 必须恰好包含该天全部活动，一个不漏")
				}
				return toolError("重排失败: " + err.Error())
			}
			res := toolOK(fmt.Sprintf("已调整 Day %d 的活动顺序", in.DayNumber), map[string]any{"day_number": in.DayNumber})
			res.Changes = []ChangeItem{{DayNumber: in.DayNumber, Kind: "updated", Op: "reorder", Title: "活动顺序"}}
			return res
		},
	}
}

// toolUpdateTripInfo patches trip-level fields (never status).
func (e *Engine) toolUpdateTripInfo() Tool {
	return Tool{
		Name:        "update_trip_info",
		Description: "修改旅行的基本信息：标题、目的地、起止日期、人数、预算、交通方式、旅行风格。",
		Write:       true,
		Schema:      schemaUpdateTripInfo,
		Run: func(ctx context.Context, ec *ExecContext, args json.RawMessage) *ToolResult {
			if ec.Role == "viewer" {
				return writeDenied()
			}
			var in struct {
				Title          *string  `json:"title"`
				Destination    *string  `json:"destination"`
				StartDate      *string  `json:"start_date"`
				EndDate        *string  `json:"end_date"`
				TravelersCount *int     `json:"travelers_count"`
				Budget         *float64 `json:"budget"`
				TransportMode  *string  `json:"transport_mode"`
				TravelStyle    *string  `json:"travel_style"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return toolError("参数格式错误: " + err.Error())
			}
			if in.StartDate != nil && !validDate(*in.StartDate) {
				return toolError("start_date 格式应为 YYYY-MM-DD")
			}
			if in.EndDate != nil && !validDate(*in.EndDate) {
				return toolError("end_date 格式应为 YYYY-MM-DD")
			}
			// Date coherence against the merged result.
			mergedStart := pick(in.StartDate, ec.Trip.Trip.StartDate)
			mergedEnd := pick(in.EndDate, ec.Trip.Trip.EndDate)
			if mergedStart != nil && mergedEnd != nil && *mergedEnd < *mergedStart {
				return toolError("end_date 不能早于 start_date")
			}
			if in.TravelersCount != nil && (*in.TravelersCount < 1 || *in.TravelersCount > 100) {
				return toolError("travelers_count 须在 1-100 之间")
			}
			if in.Budget != nil && *in.Budget < 0 {
				return toolError("budget 不能为负")
			}

			_, _, err := e.trips.Update(ctx, ec.TripID, trip.Patch{
				Title: in.Title, Destination: in.Destination, StartDate: in.StartDate,
				EndDate: in.EndDate, TravelersCount: in.TravelersCount, Budget: in.Budget,
				TransportMode: in.TransportMode, TravelStyle: in.TravelStyle,
			})
			if err != nil {
				return toolError("更新失败: " + err.Error())
			}

			changed := []string{}
			for _, f := range []struct {
				set   bool
				label string
			}{
				{in.Title != nil, "标题"}, {in.Destination != nil, "目的地"},
				{in.StartDate != nil, "开始日期"}, {in.EndDate != nil, "结束日期"},
				{in.TravelersCount != nil, "人数"}, {in.Budget != nil, "预算"},
				{in.TransportMode != nil, "交通方式"}, {in.TravelStyle != nil, "旅行风格"},
			} {
				if f.set {
					changed = append(changed, f.label)
				}
			}
			res := toolOK("已更新旅行信息："+joinLabels(changed), map[string]any{"updated": changed})
			res.Changes = []ChangeItem{{Kind: "trip_info", Title: joinLabels(changed)}}
			return res
		},
	}
}

// --- helpers ---

func findDayByNumber(days []day.Day, n int) *day.Day {
	for i := range days {
		if days[i].DayNumber == n {
			return &days[i]
		}
	}
	return nil
}

func dayNumberOf(days []day.Day, activityID string) int {
	for _, d := range days {
		for _, a := range d.Activities {
			if a.ID == activityID {
				return d.DayNumber
			}
		}
	}
	return 0
}

func pick(newVal, old *string) *string {
	if newVal != nil {
		return newVal
	}
	return old
}

// diffActivity builds change items describing what actually changed.
func diffActivity(dayNumber int, before, after *day.Activity) []ChangeItem {
	var out []ChangeItem
	if before.Title != after.Title {
		out = append(out, ChangeItem{DayNumber: dayNumber, Kind: "updated", Title: after.Title, Field: "title", From: before.Title, To: after.Title})
	}
	if strVal(before.StartTime) != strVal(after.StartTime) {
		out = append(out, ChangeItem{DayNumber: dayNumber, Kind: "updated", Title: after.Title, Field: "start_time", From: strVal(before.StartTime), To: strVal(after.StartTime)})
	}
	if strVal(before.EndTime) != strVal(after.EndTime) {
		out = append(out, ChangeItem{DayNumber: dayNumber, Kind: "updated", Title: after.Title, Field: "end_time", From: strVal(before.EndTime), To: strVal(after.EndTime)})
	}
	if before.Status != after.Status {
		out = append(out, ChangeItem{DayNumber: dayNumber, Kind: "updated", Title: after.Title, Field: "status", From: before.Status, To: after.Status})
	}
	if len(out) == 0 {
		out = append(out, ChangeItem{DayNumber: dayNumber, Kind: "updated", Title: after.Title})
	}
	return out
}

func strVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// overlapWarnings flags same-day time overlaps as a soft warning (never blocks).
func overlapWarnings(days []day.Day) []string {
	var warns []string
	for _, d := range days {
		for i := 0; i < len(d.Activities); i++ {
			for j := i + 1; j < len(d.Activities); j++ {
				a, b := d.Activities[i], d.Activities[j]
				if a.StartTime != nil && a.EndTime != nil && b.StartTime != nil && b.EndTime != nil {
					if *a.StartTime < *b.EndTime && *b.StartTime < *a.EndTime {
						warns = append(warns, fmt.Sprintf("Day %d 的「%s」与「%s」时间重叠", d.DayNumber, a.Title, b.Title))
					}
				}
			}
		}
	}
	return warns
}
