package netinfo

import (
	"net"
	"testing"
)

func TestScope(t *testing.T) {
	cases := map[string]string{
		"10.0.0.5":      "private",
		"172.20.1.1":    "private",
		"192.168.1.20":  "private",
		"100.101.102.3": "private", // Tailscale / CGNAT
		"203.0.113.7":   "public",
		"8.8.8.8":       "public",
		"172.32.0.1":    "public", // just outside 172.16/12
		"127.0.0.1":     "",
		"169.254.10.1":  "",
		"224.0.0.1":     "",
		"0.0.0.0":       "",
		"2001:db8::1":   "", // IPv6 is not reported
	}
	for ip, want := range cases {
		got, ok := Scope(net.ParseIP(ip))
		if (want == "") == ok || got != want {
			t.Errorf("Scope(%s) = %q, %v; want %q", ip, got, ok, want)
		}
	}
}

func TestIsVirtual(t *testing.T) {
	for _, n := range []string{"docker0", "br-1a2b3c", "veth12ab", "bridge100", "cni0", "virbr0"} {
		if !isVirtual(n) {
			t.Errorf("%s should be virtual", n)
		}
	}
	for _, n := range []string{"eth0", "ens3", "en0", "wlan0", "tailscale0", "utun4", "enp0s31f6"} {
		if isVirtual(n) {
			t.Errorf("%s should not be virtual", n)
		}
	}
}

func TestAddressesLocal(t *testing.T) {
	for _, a := range Addresses() {
		if a.Scope != "private" && a.Scope != "public" || net.ParseIP(a.IP).To4() == nil || a.Interface == "" {
			t.Errorf("bad address %+v", a)
		}
	}
}
