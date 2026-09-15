package planner

import (
	"strings"
	"testing"
)

func strp(s string) *string { return &s }

func TestValidateActivity(t *testing.T) {
	tests := []struct {
		name    string
		arg     ActivityCreateArg
		wantErr bool
	}{
		{"valid minimal", ActivityCreateArg{Type: "attraction", Title: "清水寺"}, false},
		{"valid with times", ActivityCreateArg{Type: "restaurant", Title: "一兰拉面", StartTime: strp("12:00"), EndTime: strp("13:30")}, false},
		{"bad type", ActivityCreateArg{Type: "casino", Title: "X"}, true},
		{"empty title", ActivityCreateArg{Type: "cafe", Title: ""}, true},
		{"bad start format", ActivityCreateArg{Type: "cafe", Title: "X", StartTime: strp("25:00")}, true},
		{"bad end format", ActivityCreateArg{Type: "cafe", Title: "X", EndTime: strp("9:99")}, true},
		{"start after end", ActivityCreateArg{Type: "cafe", Title: "X", StartTime: strp("18:00"), EndTime: strp("09:00")}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateActivity(tt.arg, 0, 0)
			if (len(errs) > 0) != tt.wantErr {
				t.Errorf("validateActivity() errs = %v, wantErr %v", errs, tt.wantErr)
			}
		})
	}
}

func TestValidateItinerary(t *testing.T) {
	start := strp("2026-10-01")
	end := strp("2026-10-05")

	t.Run("empty days", func(t *testing.T) {
		if errs := validateItinerary(ItineraryArgs{Days: nil}, start, end); len(errs) == 0 {
			t.Error("empty days should error")
		}
	})

	t.Run("date out of range", func(t *testing.T) {
		args := ItineraryArgs{Days: []ItineraryDayArg{{
			Date:       strp("2026-12-01"),
			Activities: []ActivityCreateArg{{Type: "attraction", Title: "X"}},
		}}}
		errs := validateItinerary(args, start, end)
		if len(errs) == 0 || !strings.Contains(strings.Join(errs, ""), "不在旅行日期范围") {
			t.Errorf("expected range error, got %v", errs)
		}
	})

	t.Run("duplicate dates", func(t *testing.T) {
		args := ItineraryArgs{Days: []ItineraryDayArg{
			{Date: strp("2026-10-02"), Activities: []ActivityCreateArg{{Type: "attraction", Title: "A"}}},
			{Date: strp("2026-10-02"), Activities: []ActivityCreateArg{{Type: "attraction", Title: "B"}}},
		}}
		errs := validateItinerary(args, start, end)
		if len(errs) == 0 || !strings.Contains(strings.Join(errs, ""), "重复") {
			t.Errorf("expected duplicate error, got %v", errs)
		}
	})

	t.Run("aggregates activity errors", func(t *testing.T) {
		args := ItineraryArgs{Days: []ItineraryDayArg{{
			Activities: []ActivityCreateArg{
				{Type: "bad", Title: "A"},
				{Type: "cafe", Title: ""},
			},
		}}}
		errs := validateItinerary(args, start, end)
		if len(errs) < 2 {
			t.Errorf("expected >=2 aggregated errors, got %v", errs)
		}
	})

	t.Run("day with no activities", func(t *testing.T) {
		args := ItineraryArgs{Days: []ItineraryDayArg{{Date: strp("2026-10-02")}}}
		if errs := validateItinerary(args, start, end); len(errs) == 0 {
			t.Error("day without activities should error")
		}
	})
}

func TestValidHelpers(t *testing.T) {
	if !validHHMM("09:30") || !validHHMM("23:59") {
		t.Error("valid HH:MM rejected")
	}
	if validHHMM("24:00") || validHHMM("9:30") || validHHMM("12:60") {
		t.Error("invalid HH:MM accepted")
	}
	if !validDate("2026-02-28") {
		t.Error("valid date rejected")
	}
	if validDate("2026-02-30") || validDate("2026-13-01") {
		t.Error("invalid date accepted")
	}
}
