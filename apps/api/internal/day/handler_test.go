package day

import (
	"strings"
	"testing"
)

func sp(s string) *string { return &s }

func TestValidateDay(t *testing.T) {
	if msg := validateDay(&dayRequest{}); msg != "" {
		t.Errorf("empty day request should be valid, got %q", msg)
	}
	if msg := validateDay(&dayRequest{Date: sp("2026-09-11")}); msg != "" {
		t.Errorf("valid date rejected: %q", msg)
	}
	if msg := validateDay(&dayRequest{Date: sp("11/09/2026")}); msg == "" {
		t.Error("bad date format should be rejected")
	}
	if msg := validateDay(&dayRequest{Title: sp(" ")}); msg == "" {
		t.Error("blank title should be rejected")
	}
}

func TestValidateActivity(t *testing.T) {
	cases := []struct {
		name    string
		req     activityRequest
		isCreate bool
		wantErr bool
	}{
		{"create minimal", activityRequest{Title: sp("浅草寺")}, true, false},
		{"create missing title", activityRequest{}, true, true},
		{"create blank title", activityRequest{Title: sp("  ")}, true, true},
		{"update without title ok", activityRequest{}, false, false},
		{"bad type", activityRequest{Title: sp("x"), Type: sp("flight")}, true, true},
		{"good type", activityRequest{Title: sp("x"), Type: sp("attraction")}, true, false},
		{"bad status", activityRequest{Status: sp("maybe")}, false, true},
		{"bad time", activityRequest{StartTime: sp("25:00")}, false, true},
		{"time with seconds ok", activityRequest{StartTime: sp("09:30:00")}, false, false},
		{"end before start", activityRequest{StartTime: sp("18:00"), EndTime: sp("09:00")}, false, true},
		{"long notes", activityRequest{Notes: sp(strings.Repeat("n", 2001))}, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := validateActivity(&tc.req, tc.isCreate)
			if tc.wantErr && msg == "" {
				t.Error("want validation error, got none")
			}
			if !tc.wantErr && msg != "" {
				t.Errorf("want valid, got %q", msg)
			}
		})
	}
}

func TestValidateReorder(t *testing.T) {
	id1 := "123e4567-e89b-42d3-a456-426614174000"
	id2 := "123e4567-e89b-42d3-a456-426614174001"
	if msg := validateReorder(nil); msg == "" {
		t.Error("empty list should be rejected")
	}
	if msg := validateReorder([]string{id1, id2}); msg != "" {
		t.Errorf("valid ids rejected: %q", msg)
	}
	if msg := validateReorder([]string{id1, id1}); msg == "" {
		t.Error("duplicates should be rejected")
	}
	if msg := validateReorder([]string{id1, "bogus"}); msg == "" {
		t.Error("non-uuid should be rejected")
	}
}
