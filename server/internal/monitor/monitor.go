// Package monitor watches the connected agents in the background: it keeps
// the latest snapshot per host, stores host metrics, raises alerts and runs
// the scheduled maintenance (prune, backup, cleanup).
package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"dockpit/agent/protocol"
	"dockpit/server/internal/hosts"
	"dockpit/server/internal/notify"
	"dockpit/server/internal/storage"
)

const (
	tickEvery      = 30 * time.Second
	callTimeout    = 15 * time.Second
	offlineGrace   = 2 * time.Minute // avoids alerts for agent restarts and upgrades
	storeMetricsIn = 55 * time.Second
	sparkLen       = 60 // 30 minutes of container CPU
	restartWindow  = 10 * time.Minute
	restartAlertAt = 3
	sustainedTicks = 3 // disk/memory must stay high this many checks
	expectedTTL    = 2 * time.Minute

	// NotificationsKey is the settings key holding notify.Config as JSON.
	NotificationsKey = "notifications"
)

// Problem is something on a host that needs attention.
type Problem struct {
	Kind      string `json:"kind"` // unhealthy, restarting, crashed, oom
	Container string `json:"container"`
	ID        string `json:"id"`
	Detail    string `json:"detail,omitempty"`
}

// HostSummary is the latest known state of one host.
type HostSummary struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Online     bool      `json:"online"`
	CPU        float64   `json:"cpu"`
	Mem        float64   `json:"mem"`
	Disk       float64   `json:"disk"`
	Running    int       `json:"running"`
	Containers int       `json:"containers"`
	Problems   []Problem `json:"problems"`
	UpdatedAt  int64     `json:"updated_at"`
}

// Monitor is the background watcher.
type Monitor struct {
	store    *storage.Store
	registry *hosts.Registry
	logger   *slog.Logger
	backups  string // directory for database backups; "" disables

	mu       sync.Mutex
	state    map[string]*hostState
	expected map[string]time.Time
	// sender is replaceable in tests.
	sender func(context.Context, notify.Channels, notify.Message) error
	now    func() time.Time
}

type prevContainer struct {
	state        string
	restartCount int
	seen         bool
}

type hostState struct {
	summary     HostSummary
	everOnline  bool
	offlineAt   time.Time
	active      map[string]bool // alert keys that are currently raised
	prev        map[string]prevContainer
	unhealthy   map[string]int
	restarting  map[string]int
	restarts    map[string][]time.Time
	spark       map[string][]float64
	diskHigh    int
	memHigh     int
	lastMetrics time.Time
}

// New returns a Monitor. backupDir may be empty to disable backups.
func New(store *storage.Store, registry *hosts.Registry, logger *slog.Logger, backupDir string) *Monitor {
	return &Monitor{
		store: store, registry: registry, logger: logger, backups: backupDir,
		state: map[string]*hostState{}, expected: map[string]time.Time{},
		sender: func(ctx context.Context, c notify.Channels, m notify.Message) error { return c.Send(ctx, m) },
		now:    time.Now,
	}
}

// Run blocks until ctx ends.
func (m *Monitor) Run(ctx context.Context) {
	go m.maintenanceLoop(ctx)
	t := time.NewTicker(tickEvery)
	defer t.Stop()
	m.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.tick(ctx)
		}
	}
}

// Expect marks a container action started from the dashboard, so the stop or
// restart it causes is not reported as a crash.
func (m *Monitor) Expect(hostID, container string) {
	m.mu.Lock()
	m.expected[hostID+"/"+container] = m.now().Add(expectedTTL)
	m.mu.Unlock()
}

func (m *Monitor) isExpected(hostID string, names ...string) bool {
	now := m.now()
	for _, n := range names {
		if until, ok := m.expected[hostID+"/"+n]; ok && now.Before(until) {
			return true
		}
	}
	return false
}

// Overview returns the latest summary of every registered host, sorted by
// name. Offline hosts keep their last known values.
func (m *Monitor) Overview() []HostSummary {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]HostSummary, 0, len(m.state))
	for _, st := range m.state {
		s := st.summary
		s.Problems = append([]Problem{}, s.Problems...)
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// Sparks returns recent CPU percentages per container ID of a host.
func (m *Monitor) Sparks(hostID string) map[string][]float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string][]float64{}
	if st := m.state[hostID]; st != nil {
		for id, v := range st.spark {
			out[id] = append([]float64{}, v...)
		}
	}
	return out
}

func (m *Monitor) config(ctx context.Context) notify.Config {
	cfg := notify.DefaultConfig()
	raw, err := m.store.GetSetting(ctx, NotificationsKey)
	if err != nil {
		m.logger.Warn("read notification settings", "err", err)
	} else if raw != "" {
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			m.logger.Warn("parse notification settings", "err", err)
		}
	}
	return cfg
}

func (m *Monitor) tick(ctx context.Context) {
	list, err := m.store.ListHosts(ctx)
	if err != nil {
		m.logger.Warn("monitor: list hosts", "err", err)
		return
	}
	cfg := m.config(ctx)

	m.mu.Lock()
	known := map[string]bool{}
	for _, h := range list {
		known[h.ID] = true
		if m.state[h.ID] == nil {
			m.state[h.ID] = newHostState()
		}
		m.state[h.ID].summary.ID, m.state[h.ID].summary.Name = h.ID, h.Name
	}
	for id := range m.state {
		if !known[id] {
			delete(m.state, id)
		}
	}
	for k, until := range m.expected {
		if m.now().After(until) {
			delete(m.expected, k)
		}
	}
	m.mu.Unlock()

	var wg sync.WaitGroup
	for _, h := range list {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.checkHost(ctx, h.ID, h.Name, cfg)
		}()
	}
	wg.Wait()
}

func newHostState() *hostState {
	return &hostState{
		active: map[string]bool{}, prev: map[string]prevContainer{}, unhealthy: map[string]int{},
		restarting: map[string]int{}, restarts: map[string][]time.Time{}, spark: map[string][]float64{},
	}
}

func (m *Monitor) checkHost(ctx context.Context, id, name string, cfg notify.Config) {
	agent, err := m.registry.Get(id)
	if err != nil {
		m.markOffline(ctx, id, name, cfg)
		return
	}
	cctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	var status protocol.HostStatus
	if err := agent.Call(cctx, protocol.MethodHostStatus, nil, &status); err != nil {
		m.logger.Debug("monitor: host status", "host", id, "err", err)
		m.markOffline(ctx, id, name, cfg)
		return
	}
	containers := []protocol.Container{}
	if err := agent.Call(cctx, protocol.MethodListContainers, protocol.ListContainersParams{All: true}, &containers); err != nil {
		m.logger.Debug("monitor: list containers", "host", id, "err", err)
		containers = nil
	}
	m.observe(ctx, id, name, cfg, status, containers)
}

func (m *Monitor) markOffline(ctx context.Context, id, name string, cfg notify.Config) {
	m.mu.Lock()
	st := m.state[id]
	if st == nil {
		m.mu.Unlock()
		return
	}
	st.summary.Online = false
	if st.offlineAt.IsZero() {
		st.offlineAt = m.now()
	}
	var msgs []func()
	if cfg.Rules.HostOffline && st.everOnline && !st.active["offline"] && m.now().Sub(st.offlineAt) >= offlineGrace {
		st.active["offline"] = true
		msgs = append(msgs, m.alert(ctx, cfg, notify.Message{
			Level: notify.Critical, Title: name + " is offline",
			Body: fmt.Sprintf("The agent on %s has been disconnected since %s.", name, st.offlineAt.Format("15:04:05")),
		}))
	}
	m.mu.Unlock()
	for _, f := range msgs {
		f()
	}
}

// alert returns a function that sends msg; callers run it after unlocking.
func (m *Monitor) alert(ctx context.Context, cfg notify.Config, msg notify.Message) func() {
	m.logger.Info("alert", "level", msg.Level, "title", msg.Title)
	if !cfg.Enabled || !cfg.Channels.Configured() {
		return func() {}
	}
	return func() {
		sctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if err := m.sender(sctx, cfg.Channels, msg); err != nil {
			m.logger.Warn("send notification", "err", err)
		}
	}
}

func pct(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(used) / float64(total) * 100
}

func (m *Monitor) observe(ctx context.Context, id, name string, cfg notify.Config, status protocol.HostStatus, containers []protocol.Container) {
	now := m.now()
	hm := status.Metrics
	cpu, mem, disk := hm.CPUPercent, pct(hm.MemUsed, hm.MemTotal), pct(hm.DiskUsed, hm.DiskTotal)

	m.mu.Lock()
	st := m.state[id]
	if st == nil {
		m.mu.Unlock()
		return
	}
	var send []func()
	add := func(f func()) { send = append(send, f) }
	raise := func(key string, msg notify.Message) {
		if !st.active[key] {
			st.active[key] = true
			add(m.alert(ctx, cfg, msg))
		}
	}
	clear := func(key string, msg notify.Message) {
		if st.active[key] {
			delete(st.active, key)
			add(m.alert(ctx, cfg, msg))
		}
	}

	// Back online.
	wasOffline := st.active["offline"]
	st.everOnline, st.offlineAt = true, time.Time{}
	if wasOffline {
		clear("offline", notify.Message{Level: notify.Recovery, Title: name + " is back online", Body: "The agent reconnected."})
	}
	delete(st.active, "offline")

	st.summary.Online = true
	st.summary.CPU, st.summary.Mem, st.summary.Disk = cpu, mem, disk
	st.summary.UpdatedAt = now.Unix()

	// Stored metrics (only once the agent has sampled).
	if hm.SampledAt > 0 && now.Sub(st.lastMetrics) >= storeMetricsIn {
		st.lastMetrics = now
		if err := m.store.AddMetric(ctx, id, storage.MetricPoint{T: now.Unix(), CPU: cpu, Mem: mem, Disk: disk}); err != nil {
			m.logger.Warn("store metric", "host", id, "err", err)
		}
	}

	// Disk / memory thresholds, sustained and with 5 points of hysteresis.
	threshold := func(key string, value float64, limit int, counter *int, label string) {
		if limit <= 0 || hm.SampledAt == 0 {
			*counter = 0
			clear(key, notify.Message{Level: notify.Recovery, Title: fmt.Sprintf("%s: %s back to normal", name, label), Body: "The alert was turned off."})
			return
		}
		switch {
		case value >= float64(limit):
			*counter++
			if *counter >= sustainedTicks {
				raise(key, notify.Message{Level: notify.Warning, Title: fmt.Sprintf("%s: %s at %.0f%%", name, label, value),
					Body: fmt.Sprintf("%s on %s has been at or above %d%%.", label, name, limit)})
			}
		case value < float64(limit-5):
			*counter = 0
			clear(key, notify.Message{Level: notify.Recovery, Title: fmt.Sprintf("%s: %s back to %.0f%%", name, label, value),
				Body: fmt.Sprintf("%s on %s dropped below %d%%.", label, name, limit-5)})
		}
	}
	if hm.DiskTotal > 0 {
		threshold("disk", disk, cfg.Rules.DiskPercent, &st.diskHigh, "disk")
	}
	threshold("mem", mem, cfg.Rules.MemPercent, &st.memHigh, "memory")

	if containers != nil {
		fire := func(msg notify.Message) { add(m.alert(ctx, cfg, msg)) }
		m.observeContainers(id, name, cfg, st, containers, now, raise, clear, fire)
	}
	m.mu.Unlock()
	for _, f := range send {
		f()
	}
}

func (m *Monitor) observeContainers(hostID, host string, cfg notify.Config, st *hostState, containers []protocol.Container, now time.Time,
	raise func(string, notify.Message), clear func(string, notify.Message), fire func(notify.Message)) {

	running := 0
	problems := []Problem{}
	seen := map[string]bool{}
	for _, c := range containers {
		seen[c.ID] = true
		if c.State == "running" {
			running++
		}
		prev, hadPrev := st.prev[c.ID]
		label := fmt.Sprintf("%s on %s", c.Name, host)

		// CPU spark.
		if c.State == "running" {
			v := 0.0
			if c.CPUPercent != nil {
				v = *c.CPUPercent
			}
			sp := append(st.spark[c.ID], v)
			if len(sp) > sparkLen {
				sp = sp[len(sp)-sparkLen:]
			}
			st.spark[c.ID] = sp
		} else {
			delete(st.spark, c.ID)
		}

		// Health.
		hkey := "unhealthy:" + c.ID
		if c.Health == "unhealthy" {
			st.unhealthy[c.ID]++
			problems = append(problems, Problem{Kind: "unhealthy", Container: c.Name, ID: c.ID, Detail: c.Status})
			if cfg.Rules.ContainerHealth && st.unhealthy[c.ID] >= 2 {
				raise(hkey, notify.Message{Level: notify.Critical, Title: c.Name + " is unhealthy",
					Body: label + " failed its healthcheck.\nStatus: " + c.Status})
			}
		} else {
			st.unhealthy[c.ID] = 0
			clear(hkey, notify.Message{Level: notify.Recovery, Title: c.Name + " is healthy again", Body: label})
		}

		// Restart loops: repeated restart-count increases, or stuck restarting.
		rkey := "restartloop:" + c.ID
		if hadPrev && c.RestartCount > prev.restartCount {
			for i := prev.restartCount; i < c.RestartCount; i++ {
				st.restarts[c.ID] = append(st.restarts[c.ID], now)
			}
		}
		recent := st.restarts[c.ID][:0]
		for _, t := range st.restarts[c.ID] {
			if now.Sub(t) <= restartWindow {
				recent = append(recent, t)
			}
		}
		st.restarts[c.ID] = recent
		if c.State == "restarting" {
			st.restarting[c.ID]++
		} else {
			st.restarting[c.ID] = 0
		}
		looping := len(recent) >= restartAlertAt || st.restarting[c.ID] >= 2
		if looping {
			problems = append(problems, Problem{Kind: "restarting", Container: c.Name, ID: c.ID,
				Detail: fmt.Sprintf("%d restarts in the last %d min", len(recent), int(restartWindow.Minutes()))})
			if cfg.Rules.RestartLoop {
				raise(rkey, notify.Message{Level: notify.Critical, Title: c.Name + " keeps restarting",
					Body: fmt.Sprintf("%s restarted %d times in %d minutes.", label, len(recent), int(restartWindow.Minutes()))})
			}
		} else if len(recent) == 0 {
			clear(rkey, notify.Message{Level: notify.Recovery, Title: c.Name + " stopped restarting", Body: label})
		}

		// Exits: a running container that is now stopped. Unexpected only if
		// it was not stopped from the dashboard.
		if hadPrev && prev.state == "running" && (c.State == "exited" || c.State == "dead") {
			code := 0
			if c.ExitCode != nil {
				code = *c.ExitCode
			}
			if !m.isExpected(hostID, c.ID, c.Name, c.ID[:min(12, len(c.ID))]) {
				kind := "crashed"
				if c.OOMKilled {
					kind = "oom"
				}
				if code != 0 || c.OOMKilled {
					problems = append(problems, Problem{Kind: kind, Container: c.Name, ID: c.ID, Detail: fmt.Sprintf("exit code %d", code)})
				}
				if cfg.Rules.ContainerExit && (code != 0 || c.OOMKilled) {
					body := fmt.Sprintf("%s exited with code %d.", label, code)
					if c.OOMKilled {
						body += " It was killed for running out of memory."
					}
					fire(notify.Message{Level: notify.Critical, Title: c.Name + " stopped unexpectedly", Body: body})
				}
			}
		} else if c.State == "exited" && c.ExitCode != nil && (*c.ExitCode != 0 || c.OOMKilled) && hadPrev && prev.state == "exited" {
			// Still down from an earlier crash: keep it visible in the overview.
			kind := "crashed"
			if c.OOMKilled {
				kind = "oom"
			}
			problems = append(problems, Problem{Kind: kind, Container: c.Name, ID: c.ID, Detail: fmt.Sprintf("exit code %d", *c.ExitCode)})
		}
		st.prev[c.ID] = prevContainer{state: c.State, restartCount: c.RestartCount, seen: true}
	}
	for id := range st.prev {
		if !seen[id] {
			delete(st.prev, id)
			delete(st.unhealthy, id)
			delete(st.restarting, id)
			delete(st.restarts, id)
			delete(st.spark, id)
			delete(st.active, "unhealthy:"+id)
			delete(st.active, "restartloop:"+id)
		}
	}
	st.summary.Running, st.summary.Containers = running, len(containers)
	st.summary.Problems = problems
}

// TestNotification sends a message through cfg's channels.
func (m *Monitor) TestNotification(ctx context.Context, ch notify.Channels) error {
	return ch.Send(ctx, notify.Message{Level: notify.Info, Title: "Docker Cockpit test", Body: "Notifications are working."})
}
