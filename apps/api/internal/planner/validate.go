package planner

import (
	"fmt"
	"regexp"
	"time"

	"github.com/mjlxiaoma/TripWeave/apps/api/internal/shared"
)

// Activity type / status allow-lists, mirroring the DB CHECK constraints.
var activityTypes = map[string]bool{
	"attraction": true, "restaurant": true, "cafe": true, "hotel": true,
	"transport": true, "free_time": true, "other": true,
}

var activityStatuses = map[string]bool{
	"planned": true, "done": true, "skipped": true,
}

const (
	maxDays            = 30
	maxActsPerDay      = 20
	maxTitleLen        = 200
	maxNotesLen        = 2000
	maxConstraintBytes = 4096
)

var hhmmRe = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func validType(t string) bool   { return activityTypes[t] }
func validStatus(s string) bool { return activityStatuses[s] }
func validHHMM(s string) bool   { return hhmmRe.MatchString(s) }
func validDate(s string) bool {
	if !dateRe.MatchString(s) {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// timeOrderOK reports whether start < end when both are set.
func timeOrderOK(start, end *string) bool {
	if start == nil || end == nil {
		return true
	}
	return *start < *end
}

// dateWithinTrip checks a date lies inside the trip's [start,end] range
// (open-ended when the trip has no dates).
func dateWithinTrip(date string, start, end *string) bool {
	if start != nil && date < *start {
		return false
	}
	if end != nil && date > *end {
		return false
	}
	return true
}

// --- structured tool-argument validation ---

// ItineraryDayArg is one day inside create_itinerary arguments.
type ItineraryDayArg struct {
	Date       *string             `json:"date"`
	Title      *string             `json:"title"`
	Activities []ActivityCreateArg `json:"activities"`
}

// ActivityCreateArg is one activity inside create_itinerary / create_activity.
// LocationID optionally binds a real place returned by search_locations.
type ActivityCreateArg struct {
	Type       string  `json:"type"`
	Title      string  `json:"title"`
	StartTime  *string `json:"start_time"`
	EndTime    *string `json:"end_time"`
	Notes      *string `json:"notes"`
	LocationID *string `json:"location_id"`
}

// ItineraryArgs is the full create_itinerary payload.
type ItineraryArgs struct {
	Days []ItineraryDayArg `json:"days"`
}

// validateActivity checks one activity's fields, returning all problems.
func validateActivity(a ActivityCreateArg, dayIdx, actIdx int) []string {
	var errs []string
	where := fmt.Sprintf("第%d天第%d个活动", dayIdx+1, actIdx+1)
	if !validType(a.Type) {
		errs = append(errs, where+"的 type 非法: "+a.Type)
	}
	if a.Title == "" {
		errs = append(errs, where+"缺少 title")
	} else if len([]rune(a.Title)) > maxTitleLen {
		errs = append(errs, where+"的 title 过长")
	}
	if a.StartTime != nil && !validHHMM(*a.StartTime) {
		errs = append(errs, where+"的 start_time 格式应为 HH:MM")
	}
	if a.EndTime != nil && !validHHMM(*a.EndTime) {
		errs = append(errs, where+"的 end_time 格式应为 HH:MM")
	}
	if !timeOrderOK(a.StartTime, a.EndTime) {
		errs = append(errs, where+"的 start_time 必须早于 end_time")
	}
	if a.Notes != nil && len([]rune(*a.Notes)) > maxNotesLen {
		errs = append(errs, where+"的 notes 过长")
	}
	if a.LocationID != nil && !shared.IsUUID(*a.LocationID) {
		errs = append(errs, where+"的 location_id 非法")
	}
	return errs
}

// validateItinerary validates the whole itinerary, aggregating every problem
// so the model can fix them all in one retry.
func validateItinerary(args ItineraryArgs, tripStart, tripEnd *string) []string {
	var errs []string
	if len(args.Days) == 0 {
		return []string{"days 不能为空"}
	}
	if len(args.Days) > maxDays {
		errs = append(errs, fmt.Sprintf("天数 %d 超过上限 %d", len(args.Days), maxDays))
	}
	seenDates := map[string]bool{}
	for i, d := range args.Days {
		if len(d.Activities) == 0 {
			errs = append(errs, fmt.Sprintf("第%d天没有活动", i+1))
		}
		if len(d.Activities) > maxActsPerDay {
			errs = append(errs, fmt.Sprintf("第%d天活动数 %d 超过上限 %d", i+1, len(d.Activities), maxActsPerDay))
		}
		if d.Date != nil {
			if !validDate(*d.Date) {
				errs = append(errs, fmt.Sprintf("第%d天的 date 格式应为 YYYY-MM-DD", i+1))
			} else if !dateWithinTrip(*d.Date, tripStart, tripEnd) {
				errs = append(errs, fmt.Sprintf("第%d天的 date %s 不在旅行日期范围内", i+1, *d.Date))
			} else if seenDates[*d.Date] {
				errs = append(errs, fmt.Sprintf("日期 %s 重复", *d.Date))
			} else {
				seenDates[*d.Date] = true
			}
		}
		if d.Title != nil && len([]rune(*d.Title)) > maxTitleLen {
			errs = append(errs, fmt.Sprintf("第%d天的 title 过长", i+1))
		}
		for j, a := range d.Activities {
			errs = append(errs, validateActivity(a, i, j)...)
		}
	}
	return errs
}
