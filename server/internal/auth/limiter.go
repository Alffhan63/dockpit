package auth

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// Limiter blocks a key after too many failures within a window.
type Limiter struct {
	Max    int
	Window time.Duration

	mu       sync.Mutex
	failures map[string][]time.Time
}

// NewLimiter allows max failures per key within window.
func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{Max: max, Window: window, failures: make(map[string][]time.Time)}
}

// Allowed reports whether key may try again.
func (l *Limiter) Allowed(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recent(key, now)) < l.Max
}

// Fail records a failure for key.
func (l *Limiter) Fail(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures[key] = append(l.recent(key, now), now)
}

// Reset clears failures for key after a success.
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, key)
}

func (l *Limiter) recent(key string, now time.Time) []time.Time {
	kept := l.failures[key][:0]
	for _, t := range l.failures[key] {
		if now.Sub(t) < l.Window {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.failures, key)
		return nil
	}
	l.failures[key] = kept
	return kept
}

// ClientIP returns the client address. X-Real-IP is trusted only when the
// request comes from loopback, i.e. from a local reverse proxy such as nginx.
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		if real := r.Header.Get("X-Real-IP"); net.ParseIP(real) != nil {
			return real
		}
	}
	return host
}
