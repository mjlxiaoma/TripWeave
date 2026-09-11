package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRateLimitAllowsBurstThenDenies(t *testing.T) {
	// burst 3, ~no refill: first 3 pass, 4th is rejected
	rl := RateLimit(0.0001, 3)
	handler := rl(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("POST", "/auth/login", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200", i+1, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("POST", "/auth/login", nil))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("request 4: got %d, want 429", rec.Code)
	}
}

func TestRateLimitIsPerIP(t *testing.T) {
	rl := RateLimit(0.0001, 1)
	handler := rl(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	req := func(remote string) *http.Request {
		r := httptest.NewRequest("POST", "/", nil)
		r.RemoteAddr = remote
		return r
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req("1.1.1.1:1000"))
	if rec.Code == 429 {
		t.Fatal("first request from 1.1.1.1 should pass")
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req("2.2.2.2:1000"))
	if rec.Code == 429 {
		t.Fatal("first request from 2.2.2.2 should pass (per-IP buckets)")
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req("1.1.1.1:2000"))
	if rec.Code != 429 {
		t.Fatal("second request from 1.1.1.1 should be limited")
	}
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "203.0.113.9:5555"
	r.Header.Set("X-Forwarded-For", "198.51.100.1")
	if got := clientIP(r); got != "203.0.113.9" {
		t.Errorf("non-loopback peer must not trust XFF, got %q", got)
	}

	r.RemoteAddr = "127.0.0.1:8080"
	if got := clientIP(r); got != "198.51.100.1" {
		t.Errorf("loopback proxy: got %q, want XFF value", got)
	}

	r.Header.Set("X-Forwarded-For", "198.51.100.1, 10.0.0.1")
	if got := clientIP(r); got != "198.51.100.1" {
		t.Errorf("XFF chain: got %q, want first hop", got)
	}
}
