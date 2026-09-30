package docker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrNoSocket means no Docker socket was found in the usual locations.
var ErrNoSocket = errors.New("no Docker socket found; is Docker running? (or set DOCKER_HOST=unix:///path/to/docker.sock)")

// ResolveSocket finds the local Docker socket.
//
// DOCKER_HOST is honoured when set, but only unix:// sockets are accepted:
// the agent never talks to Docker over TCP. Otherwise the common locations
// for Docker Desktop, OrbStack, Colima and Linux are probed in order.
func ResolveSocket() (string, error) {
	if h := os.Getenv("DOCKER_HOST"); h != "" {
		path, ok := strings.CutPrefix(h, "unix://")
		if !ok || path == "" {
			return "", fmt.Errorf("DOCKER_HOST=%q is not supported: only unix:// sockets are allowed", h)
		}
		return path, nil
	}

	var candidates []string
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".docker/run/docker.sock"),
			filepath.Join(home, ".orbstack/run/docker.sock"),
			filepath.Join(home, ".colima/default/docker.sock"),
		)
	}
	candidates = append(candidates, "/var/run/docker.sock")

	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && fi.Mode()&os.ModeSocket != 0 {
			return p, nil
		}
	}
	return "", ErrNoSocket
}
