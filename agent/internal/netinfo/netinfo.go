// Package netinfo lists the host's IPv4 addresses.
package netinfo

import (
	"net"
	"sort"
	"strings"

	"dockpit/agent/protocol"
)

// Virtual interfaces created by Docker, Kubernetes CNIs and VM software.
// Their addresses are internal plumbing, not how anyone reaches the host.
var virtualPrefixes = []string{
	"docker", "br-", "veth", "cni", "flannel", "cali", "cilium", "weave", "kube",
	"virbr", "vboxnet", "vmnet", "bridge", "awdl", "llw", "anpi", "ap1",
}

var cgnat = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

// Addresses returns the IPv4 addresses of interfaces that are up, skipping
// loopback, link-local and virtual interfaces.
func Addresses() []protocol.Address {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []protocol.Address
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 || isVirtual(ifc.Name) {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if scope, ok := Scope(ipnet.IP); ok {
				out = append(out, protocol.Address{Interface: ifc.Name, IP: ipnet.IP.To4().String(), Scope: scope})
			}
		}
	}
	// Public first, then by interface name, for a stable display order.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Scope != out[j].Scope {
			return out[i].Scope == "public"
		}
		return out[i].Interface < out[j].Interface
	})
	return out
}

// Scope classifies an IPv4 address as "private" or "public". ok is false for
// addresses that are neither useful to show (IPv6, loopback, link-local,
// multicast, unspecified).
func Scope(ip net.IP) (scope string, ok bool) {
	v4 := ip.To4()
	if v4 == nil || v4.IsLoopback() || v4.IsLinkLocalUnicast() || v4.IsMulticast() || v4.IsUnspecified() {
		return "", false
	}
	if v4.IsPrivate() || cgnat.Contains(v4) {
		return "private", true
	}
	return "public", true
}

func isVirtual(name string) bool {
	for _, p := range virtualPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}
