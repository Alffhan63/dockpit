// Command cockpit-agent connects a Docker host to the Docker Cockpit controller.
//
// The agent opens an outbound WebSocket to the controller and never listens
// on a port itself. It talks to Docker only through the local unix socket.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"dockpit/agent/internal/docker"
	"dockpit/agent/internal/metrics"
	"dockpit/agent/internal/rpc"
	"dockpit/agent/internal/transport"
	"dockpit/agent/protocol"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(logger); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("agent stopped", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	controllerURL := os.Getenv("COCKPIT_CONTROLLER_URL")
	token := os.Getenv("COCKPIT_TOKEN")
	if controllerURL == "" || token == "" {
		return errors.New("COCKPIT_CONTROLLER_URL and COCKPIT_TOKEN must be set")
	}
	connectURL, err := transport.ConnectURL(controllerURL)
	if err != nil {
		return err
	}

	var tlsConfig *tls.Config
	if caFile := os.Getenv("COCKPIT_CA_FILE"); caFile != "" {
		if tlsConfig, err = transport.TLSConfigWithCA(caFile); err != nil {
			return fmt.Errorf("COCKPIT_CA_FILE: %w", err)
		}
	}

	// A misconfigured DOCKER_HOST is fatal; Docker not running yet is not.
	socket, err := docker.ResolveSocket()
	if err != nil && !errors.Is(err, docker.ErrNoSocket) {
		return err
	}
	if err != nil {
		logger.Warn("docker socket not found yet; will retry on each request", "err", err)
	}
	dc := docker.New(docker.ResolveSocket)

	name, _ := os.Hostname()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	diskPath := os.Getenv("COCKPIT_DISK_PATH")
	if diskPath == "" {
		diskPath = "/"
	}
	sampler := &metrics.Sampler{
		Host:     metrics.SystemCollector{DiskPath: diskPath},
		Docker:   dc,
		Interval: 5 * time.Second,
		Logger:   logger,
	}
	go sampler.Run(ctx)

	logger.Info("starting agent", "version", version, "docker_socket", socket, "controller", connectURL)

	return transport.Run(ctx, transport.Config{
		URL:    connectURL,
		Token:  token,
		TLS:    tlsConfig,
		Logger: logger,
		Hello: func(ctx context.Context) protocol.HostInfo {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			dv, err := dc.Version(ctx)
			if err != nil {
				logger.Warn("docker version unavailable", "err", err)
			}
			return protocol.HostInfo{
				Name:          name,
				OS:            runtime.GOOS,
				Arch:          runtime.GOARCH,
				DockerVersion: dv,
				AgentVersion:  version,
			}
		},
		Handle: (&rpc.Handler{Docker: dc, Sampler: sampler}).Handle,
	})
}
