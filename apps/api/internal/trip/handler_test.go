package trip

import (
	"encoding/json"
	"strings"
	"testing"
)

func strp(s string) *string { return &s }

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		req  tripRequest
		want string // "" means valid; otherwise substring of the error
	}{
		{"empty request is valid", tripRequest{}, ""},
		{"normal", tripRequest{Title: strp("日本关东之旅"), Status: strp("draft")}, ""},
		{"blank title", tripRequest{Title: strp("   ")}, "title"},
		{"long title", tripRequest{Title: strp(strings.Repeat("好", 201))}, "title"},
		{"bad status", tripRequest{Status: strp("deleted")}, "status"},
		{"deleted not settable", tripRequest{Status: strp("deleted")}, "status"},
		{"travelers low", tripRequest{TravelersCount: intp(0)}, "travelers_count"},
		{"travelers high", tripRequest{TravelersCount: intp(101)}, "travelers_count"},
		{"negative budget", tripRequest{Budget: floatp(-1)}, "budget"},
		{"bad start date", tripRequest{StartDate: strp("2026/01/01")}, "start_date"},
		{"bad end date", tripRequest{EndDate: strp("13-40-70")}, "end_date"},
		{"end before start", tripRequest{StartDate: strp("2026-05-02"), EndDate: strp("2026-05-01")}, "end_date"},
		{"long natural language", tripRequest{NaturalLanguage: strp(strings.Repeat("x", 2001))}, "natural_language"},
		{"preferences not array", tripRequest{Preferences: rawp(`{"a":1}`)}, "preferences"},
		{"preferences too many", tripRequest{Preferences: rawp(`["` + strings.Repeat(`a","`, 50) + `a"]`)}, "preferences"},
		{"preferences empty tag", tripRequest{Preferences: rawp(`[""]`)}, "preference tag"},
		{"constraints not object", tripRequest{Constraints: rawp(`[1,2]`)}, "constraints"},
		{"constraints ok", tripRequest{Constraints: rawp(`{"max_drive_hours_per_day":4}`)}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validate(&tc.req)
			if tc.want == "" && got != "" {
				t.Errorf("validate() = %q, want valid", got)
			}
			if tc.want != "" && !strings.Contains(got, tc.want) {
				t.Errorf("validate() = %q, want substring %q", got, tc.want)
			}
		})
	}
}

func intp(i int) *int          { return &i }
func floatp(f float64) *float64 { return &f }
func rawp(s string) *json.RawMessage {
	r := json.RawMessage(s)
	return &r
}

func TestStatusOrDraft(t *testing.T) {
	if statusOrDraft("") != "draft" {
		t.Error("empty status should default to draft")
	}
	if statusOrDraft("ready") != "ready" {
		t.Error("explicit status should pass through")
	}
}

func TestParseDatePtr(t *testing.T) {
	if d, err := parseDatePtr(nil); d != nil || err != nil {
		t.Error("nil should parse to nil, nil")
	}
	if d, err := parseDatePtr(strp("")); d != nil || err != nil {
		t.Error("empty should parse to nil, nil")
	}
	if _, err := parseDatePtr(strp("2026-02-30")); err == nil {
		t.Error("invalid calendar date should error")
	}
	if d, err := parseDatePtr(strp("2026-02-28")); err != nil || d == nil {
		t.Error("valid date should parse")
	}
}
