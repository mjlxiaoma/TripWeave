package shared

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsUUID(t *testing.T) {
	valid := "123e4567-e89b-42d3-a456-426614174000"
	if !IsUUID(valid) {
		t.Errorf("IsUUID(%q) = false, want true", valid)
	}
	if !IsUUID(strings.ToUpper(valid)) {
		t.Error("uppercase hex UUID should be accepted")
	}
	invalid := []string{
		"",
		"not-a-uuid",
		"123e4567e89b42d3a456426614174000",     // no dashes
		"123e4567-e89b-42d3-a456-42661417400g",  // non-hex char
		"123e4567-e89b-42d3-a456-42661417400",   // too short
		"123e4567-e89b-42d3-a456-4266141740000", // too long
		"123e4567-e89b-42d3-a456-426614174000/extra",
	}
	for _, s := range invalid {
		if IsUUID(s) {
			t.Errorf("IsUUID(%q) = true, want false", s)
		}
	}
}

func TestDecodeRejectsUnknownFields(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"known":1,"bogus":2}`))
	var v struct {
		Known int `json:"known"`
	}
	if err := Decode(r, &v); err == nil {
		t.Error("Decode should reject unknown fields")
	}
}

func TestDecodeRejectsOversizedBody(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"v":"`+strings.Repeat("x", 2<<20)+`"}`))
	var v struct {
		V string `json:"v"`
	}
	if err := Decode(r, &v); err == nil {
		t.Error("Decode should reject bodies over 1 MiB")
	}
}

func TestDecodeEmptyBody(t *testing.T) {
	r := httptest.NewRequest("POST", "/", nil)
	r.Body = nil
	var v struct{}
	if err := Decode(r, &v); err == nil {
		t.Error("Decode should reject nil body")
	}
}

func TestPtrHelpers(t *testing.T) {
	if Deref(nil) != "" {
		t.Error("Deref(nil) should be empty")
	}
	s := "  hi  "
	if got := TrimPtr(&s); got == nil || *got != "hi" {
		t.Errorf("TrimPtr = %v, want \"hi\"", got)
	}
	if TrimPtr(nil) != nil {
		t.Error("TrimPtr(nil) should stay nil")
	}
	if string(RawOr(nil, `{}`)) != `{}` {
		t.Error("RawOr(nil) should return fallback")
	}
	empty := json.RawMessage{}
	if string(RawOr(&empty, `[]`)) != `[]` {
		t.Error("RawOr(empty) should return fallback")
	}
	raw := json.RawMessage(`{"a":1}`)
	if string(RawOr(&raw, `{}`)) != `{"a":1}` {
		t.Error("RawOr should pass through non-empty raw")
	}
}
