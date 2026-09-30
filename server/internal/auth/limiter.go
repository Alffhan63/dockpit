package auth

import (
	"fmt"
	"net"
	"net/http"
	"strings"
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

var (
	trustedMu      sync.RWMutex
	trustedProxies []*net.IPNet
)

// SetTrustedProxies lists the networks (CIDR) whose X-Real-IP header is
// believed, besides loopback. Behind Docker's port publishing, nginx reaches
// the controller from the bridge gateway (e.g. 172.16.0.0/12), not loopback.
func SetTrustedProxies(cidrs []string) error {
	var nets []*net.IPNet
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			return fmt.Errorf("trusted proxy %q: %w", c, err)
		}
		nets = append(nets, n)
	}
	trustedMu.Lock()
	trustedProxies = nets
	trustedMu.Unlock()
	return nil
}

func trusted(ip net.IP) bool {
	if ip.IsLoopback() {
		return true
	}
	trustedMu.RLock()
	defer trustedMu.RUnlock()
	for _, n := range trustedProxies {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// ClientIP returns the client address. X-Real-IP is trusted only when the
// request comes from loopback or a configured trusted proxy network, i.e.
// from the reverse proxy in front of the controller.
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil && trusted(ip) {
		if real := r.Header.Get("X-Real-IP"); net.ParseIP(real) != nil {
			return real
		}
	}
	return host
}
