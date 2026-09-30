package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIPTrustedProxies(t *testing.T) {
	req := func(remote, realIP string) *http.Request {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = remote
		if realIP != "" {
			r.Header.Set("X-Real-IP", realIP)
		}
		return r
	}
	t.Cleanup(func() { SetTrustedProxies(nil) })

	SetTrustedProxies(nil)
	if got := ClientIP(req("172.24.0.1:5555", "203.0.113.9")); got != "172.24.0.1" {
		t.Errorf("untrusted peer: got %s, want the peer address", got)
	}
	if got := ClientIP(req("127.0.0.1:5555", "203.0.113.9")); got != "203.0.113.9" {
		t.Errorf("loopback peer: got %s", got)
	}

	if err := SetTrustedProxies([]string{"172.16.0.0/12"}); err != nil {
		t.Fatal(err)
	}
	if got := ClientIP(req("172.24.0.1:5555", "203.0.113.9")); got != "203.0.113.9" {
		t.Errorf("docker gateway peer: got %s", got)
	}
	if got := ClientIP(req("198.51.100.7:5555", "203.0.113.9")); got != "198.51.100.7" {
		t.Errorf("outside trusted range must not be believed: got %s", got)
	}
	if got := ClientIP(req("172.24.0.1:5555", "not-an-ip")); got != "172.24.0.1" {
		t.Errorf("garbage header: got %s", got)
	}
	if err := SetTrustedProxies([]string{"nonsense"}); err == nil {
		t.Error("invalid CIDR accepted")
	}
}
