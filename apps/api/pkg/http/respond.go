// Package http holds shared HTTP helpers (JSON envelope, error responses).
package http

import (
	"encoding/json"
	"net/http"
)

// Envelope is the unified API response structure: {"data":..., "error":...}.
type Envelope struct {
	Data  any          `json:"data"`
	Error *ErrorDetail `json:"error"`
}

// ErrorDetail describes an API error.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// JSON writes a raw value as a JSON response (no envelope).
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// OK writes a success envelope {"data": v, "error": null}.
func OK(w http.ResponseWriter, status int, data any) {
	JSON(w, status, Envelope{Data: data})
}

// Fail writes an error envelope {"data": null, "error": {...}}.
func Fail(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, Envelope{Error: &ErrorDetail{Code: code, Message: message}})
}
