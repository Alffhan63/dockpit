package monitor

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"dockpit/agent/protocol"
	"dockpit/server/internal/hosts"
	"dockpit/server/internal/notify"
	"dockpit/server/internal/storage"
)

type harness struct {
	m    *Monitor
	cfg  notify.Config
	mu   sync.Mutex
	sent []notify.Message
	now  time.Time
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	h := &harness{now: time.Unix(1_800_000_000, 0), cfg: notify.DefaultConfig()}
	h.cfg.Enabled = true
	h.cfg.Channels.Webhook.URL = "http://example.invalid"
	m := New(store, hosts.NewRegistry(), slog.New(slog.NewTextHandler(io.Discard, nil)), "")
	m.now = func() time.Time { return h.now }
	m.sender = func(_ context.Context, _ notify.Channels, msg notify.Message) error {
		h.mu.Lock()
		h.sent = append(h.sent, msg)
		h.mu.Unlock()
		return nil
	}
	if _, err := store.CreateHost(context.Background(), "h1", "Host One", "hash"); err != nil {
		t.Fatal(err)
	}
	m.state["h1"] = newHostState()
	m.state["h1"].summary.ID, m.state["h1"].summary.Name = "h1", "Host One"
	h.m = m
	return h
}

func (h *harness) step(status protocol.HostStatus, cs ...protocol.Container) {
	h.now = h.now.Add(tickEvery)
	h.m.observe(context.Background(), "h1", "Host One", h.cfg, status, cs)
}

func (h *harness) titles() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var t []string
	for _, m := range h.sent {
		t = append(t, m.Title)
	}
	return strings.Join(t, " | ")
}

func status(disk, mem uint64) protocol.HostStatus {
	return protocol.HostStatus{Metrics: protocol.HostMetrics{
		SampledAt: 1, DiskTotal: 100, DiskUsed: disk, MemTotal: 100, MemUsed: mem,
	}}
}

func ctr(name, state, health string) protocol.Container {
	return protocol.Container{ID: name + "0123456789abcdef", Name: name, State: state, Health: health, Status: state}
}

func TestUnhealthyNeedsTwoChecksAndRecovers(t *testing.T) {
	h := newHarness(t)
	h.step(status(10, 10), ctr("web", "running", "healthy"))
	h.step(status(10, 10), ctr("web", "running", "unhealthy"))
	if h.titles() != "" {
		t.Fatalf("alerted after one failed check: %s", h.titles())
	}
	h.step(status(10, 10), ctr("web", "running", "unhealthy"))
	h.step(status(10, 10), ctr("web", "running", "unhealthy"))
	if h.titles() != "web is unhealthy" {
		t.Fatalf("got %q, want exactly one unhealthy alert", h.titles())
	}
	if p := h.m.Overview()[0].Problems; len(p) != 1 || p[0].Kind != "unhealthy" {
		t.Fatalf("overview problems = %+v", p)
	}
	h.step(status(10, 10), ctr("web", "running", "healthy"))
	if h.titles() != "web is unhealthy | web is healthy again" {
		t.Fatalf("got %q", h.titles())
	}
}

func TestCrashAlertsButDashboardStopDoesNot(t *testing.T) {
	h := newHarness(t)
	code := 137
	exited := func(name string) protocol.Container {
		c := ctr(name, "exited", "")
		c.ExitCode = &code
		return c
	}
	h.step(status(10, 10), ctr("api", "running", ""), ctr("worker", "running", ""))
	h.m.Expect("h1", "worker")
	h.step(status(10, 10), exited("api"), exited("worker"))
	if h.titles() != "api stopped unexpectedly" {
		t.Fatalf("got %q", h.titles())
	}
	// Staying down does not alert again but stays visible.
	h.step(status(10, 10), exited("api"), exited("worker"))
	if h.titles() != "api stopped unexpectedly" {
		t.Fatalf("repeat alert: %q", h.titles())
	}
	if p := h.m.Overview()[0].Problems; len(p) < 1 {
		t.Fatalf("crashed container not listed: %+v", p)
	}
}

func TestCleanExitIsNotAnAlert(t *testing.T) {
	h := newHarness(t)
	zero := 0
	h.step(status(10, 10), ctr("job", "running", ""))
	c := ctr("job", "exited", "")
	c.ExitCode = &zero
	h.step(status(10, 10), c)
	if h.titles() != "" {
		t.Fatalf("alerted for exit 0: %q", h.titles())
	}
}

func TestRestartLoop(t *testing.T) {
	h := newHarness(t)
	c := ctr("db", "running", "")
	h.step(status(10, 10), c)
	for i := 1; i <= 3; i++ {
		c.RestartCount = i
		h.step(status(10, 10), c)
	}
	if h.titles() != "db keeps restarting" {
		t.Fatalf("got %q", h.titles())
	}
}

func TestDiskSustainedThenRecovery(t *testing.T) {
	h := newHarness(t)
	h.step(status(95, 10))
	h.step(status(95, 10))
	if h.titles() != "" {
		t.Fatalf("alerted before sustained: %q", h.titles())
	}
	h.step(status(95, 10))
	h.step(status(95, 10))
	if h.titles() != "Host One: disk at 95%" {
		t.Fatalf("got %q", h.titles())
	}
	h.step(status(88, 10)) // inside the hysteresis band: still raised
	if strings.Contains(h.titles(), "back") {
		t.Fatalf("recovered too early: %q", h.titles())
	}
	h.step(status(50, 10))
	if !strings.Contains(h.titles(), "disk back to 50%") {
		t.Fatalf("no recovery: %q", h.titles())
	}
}

func TestOfflineNeedsGraceAndEverOnline(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.m.markOffline(ctx, "h1", "Host One", h.cfg) // never seen online: no alert
	h.now = h.now.Add(10 * time.Minute)
	h.m.markOffline(ctx, "h1", "Host One", h.cfg)
	if h.titles() != "" {
		t.Fatalf("alerted for a host that never connected: %q", h.titles())
	}
	h.step(status(10, 10))
	h.m.markOffline(ctx, "h1", "Host One", h.cfg)
	h.now = h.now.Add(time.Minute)
	h.m.markOffline(ctx, "h1", "Host One", h.cfg)
	if h.titles() != "" {
		t.Fatalf("alerted inside grace period: %q", h.titles())
	}
	h.now = h.now.Add(2 * time.Minute)
	h.m.markOffline(ctx, "h1", "Host One", h.cfg)
	h.m.markOffline(ctx, "h1", "Host One", h.cfg)
	if h.titles() != "Host One is offline" {
		t.Fatalf("got %q", h.titles())
	}
	h.step(status(10, 10))
	if h.titles() != "Host One is offline | Host One is back online" {
		t.Fatalf("got %q", h.titles())
	}
}

func TestDisabledSendsNothing(t *testing.T) {
	h := newHarness(t)
	h.cfg.Enabled = false
	c := ctr("x", "running", "unhealthy")
	for range 4 {
		h.step(status(10, 10), c)
	}
	if h.titles() != "" {
		t.Fatalf("sent while disabled: %q", h.titles())
	}
	if len(h.m.Overview()[0].Problems) != 1 {
		t.Fatal("overview must still show the problem")
	}
}
