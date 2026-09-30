package auth

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestBearerToken(t *testing.T) {
	cases := map[string]struct {
		want string
		ok   bool
	}{
		"Bearer abc":   {"abc", true},
		"Bearer  abc ": {"abc", true},
		"Bearer ":      {"", false},
		"Basic abc":    {"", false},
		"abc":          {"", false},
		"":             {"", false},
	}
	for header, tc := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		if header != "" {
			r.Header.Set("Authorization", header)
		}
		got, ok := BearerToken(r)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("BearerToken(%q) = %q, %v; want %q, %v", header, got, ok, tc.want, tc.ok)
		}
	}
}

func TestTokenEqual(t *testing.T) {
	if !TokenEqual("secret", "secret") {
		t.Error("equal tokens reported different")
	}
	for _, other := range []string{"secreT", "secret2", "", "s"} {
		if TokenEqual("secret", other) {
			t.Errorf("TokenEqual(secret, %q) = true", other)
		}
	}
}

func TestNewTokenAndHash(t *testing.T) {
	a, b := NewToken(), NewToken()
	if len(a) != 64 || a == b {
		t.Fatalf("tokens %q %q", a, b)
	}
	if HashToken(a) == a || HashToken(a) != HashToken(a) || HashToken(a) == HashToken(b) {
		t.Error("hash not deterministic or not distinct")
	}
}

func TestPassword(t *testing.T) {
	if _, err := HashPassword("short"); err == nil {
		t.Error("short password accepted")
	}
	h, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(h, "correct horse battery") || CheckPassword(h, "correct horse batterY") {
		t.Error("CheckPassword wrong")
	}
}

func TestLimiter(t *testing.T) {
	l := NewLimiter(3, time.Minute)
	now := time.Now()
	for i := 0; i < 3; i++ {
		if !l.Allowed("ip", now) {
			t.Fatalf("blocked after %d failures", i)
		}
		l.Fail("ip", now)
	}
	if l.Allowed("ip", now) {
		t.Error("allowed after max failures")
	}
	if !l.Allowed("other", now) {
		t.Error("other key blocked")
	}
	if !l.Allowed("ip", now.Add(time.Minute+time.Second)) {
		t.Error("still blocked after window")
	}
	l.Fail("ip", now)
	l.Reset("ip")
	if !l.Allowed("ip", now) {
		t.Error("blocked after reset")
	}
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:5555"
	r.Header.Set("X-Real-IP", "203.0.113.7")
	if got := ClientIP(r); got != "203.0.113.7" {
		t.Errorf("behind local proxy: %q", got)
	}
	r.RemoteAddr = "198.51.100.1:5555" // direct client must not spoof
	if got := ClientIP(r); got != "198.51.100.1" {
		t.Errorf("direct client: %q", got)
	}
}
