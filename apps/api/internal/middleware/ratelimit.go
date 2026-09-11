package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	phttp "github.com/mjlxiaoma/TripWeave/apps/api/pkg/http"
)

// RateLimit returns a middleware that applies a per-client-IP token bucket:
// burst tokens up front, refilled at rps (requests per second). Exceeding
// clients get 429. Buckets idle for longer than idleTTL are reaped.
//
// In-memory and per-process: correct for the single-node deployment, swap for
// a Redis-backed limiter if the API ever runs more than one replica.
func RateLimit(rps float64, burst int) func(http.Handler) http.Handler {
	l := &ipLimiter{
		rps:     rps,
		burst:   burst,
		idleTTL: 10 * time.Minute,
		buckets: make(map[string]*bucket),
	}
	go l.reapLoop()
	return l.middleware
}

type bucket struct {
	tokens float64
	last   time.Time
}

type ipLimiter struct {
	mu      sync.Mutex
	rps     float64
	burst   int
	idleTTL time.Duration
	buckets map[string]*bucket
}

func (l *ipLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[ip]
	if !ok {
		b = &bucket{tokens: float64(l.burst) - 1, last: time.Now()}
		l.buckets[ip] = b
		return true
	}
	now := time.Now()
	b.tokens += now.Sub(b.last).Seconds() * l.rps
	if b.tokens > float64(l.burst) {
		b.tokens = float64(l.burst)
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (l *ipLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(clientIP(r)) {
			phttp.Fail(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests, please slow down")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (l *ipLimiter) reapLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for now := range ticker.C {
		l.mu.Lock()
		for ip, b := range l.buckets {
			if now.Sub(b.last) > l.idleTTL {
				delete(l.buckets, ip)
			}
		}
		l.mu.Unlock()
	}
}

// clientIP prefers the direct peer address. X-Forwarded-For is trusted only
// when the immediate peer is a loopback (i.e. a local reverse proxy like the
// Nginx in front of this API), so internet clients cannot spoof it.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			if idx := strings.IndexByte(fwd, ','); idx >= 0 {
				fwd = fwd[:idx]
			}
			if first := strings.TrimSpace(fwd); first != "" {
				return first
			}
		}
	}
	return host
}
