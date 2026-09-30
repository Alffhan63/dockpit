package metrics

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"dockpit/agent/internal/docker"
	"dockpit/agent/protocol"
)

type fakeHost struct{}

func (fakeHost) Collect(context.Context) (protocol.HostMetrics, error) {
	return protocol.HostMetrics{CPUPercent: 12.5, MemUsed: 1, MemTotal: 2}, nil
}

type fakeDocker struct {
	mu    sync.Mutex
	calls int
	cpu   uint64
}

func (f *fakeDocker) ListContainers(context.Context, bool) ([]protocol.Container, error) {
	return []protocol.Container{{ID: "a"}}, nil
}

func (f *fakeDocker) ContainerStats(context.Context, string) (docker.Stats, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.cpu += 500
	// Each sample: +500ns container CPU over +1000ns system CPU on 2 cores = 100%.
	return docker.Stats{CPUTotal: f.cpu, SystemCPU: uint64(f.calls) * 1000, OnlineCPUs: 2, MemUsage: 64}, nil
}

func (f *fakeDocker) statsCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func newSampler(d *fakeDocker) *Sampler {
	return &Sampler{Host: fakeHost{}, Docker: d, Interval: time.Hour, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestSamplerOnDemand(t *testing.T) {
	d := &fakeDocker{}
	s := newSampler(d)
	ctx := context.Background()
	s.init()

	// No demand yet: host metrics only.
	s.sample(ctx)
	if got := s.HostMetrics(); got.CPUPercent != 12.5 || got.SampledAt == 0 {
		t.Fatalf("host = %+v", got)
	}
	if d.statsCalls() != 0 {
		t.Fatal("container stats sampled without demand")
	}

	// Demand: first sample has memory but no CPU (needs two samples).
	if len(s.ContainerUsage()) != 0 {
		t.Fatal("usage before sampling")
	}
	s.sample(ctx)
	u := s.ContainerUsage()["a"]
	if u.MemUsage != 64 || u.CPUPercent != nil {
		t.Fatalf("first usage = %+v", u)
	}
	s.sample(ctx)
	u = s.ContainerUsage()["a"]
	if u.CPUPercent == nil || *u.CPUPercent != 100 {
		t.Fatalf("second usage cpu = %v", u.CPUPercent)
	}

	// Demand expires: stats stop and old samples are dropped.
	s.mu.Lock()
	s.lastWanted = time.Now().Add(-containerDemandWindow - time.Second)
	s.mu.Unlock()
	calls := d.statsCalls()
	s.sample(ctx)
	if d.statsCalls() != calls {
		t.Error("stats sampled after demand expired")
	}
	s.mu.Lock()
	n := len(s.usage)
	s.mu.Unlock()
	if n != 0 {
		t.Error("stale usage kept")
	}
}

type stepHost struct{ cpu float64 }

func (h *stepHost) Collect(context.Context) (protocol.HostMetrics, error) {
	return protocol.HostMetrics{CPUPercent: h.cpu, MemUsed: 25, MemTotal: 100}, nil
}

func TestHistory(t *testing.T) {
	h := &stepHost{}
	clock := time.Unix(1_700_000_000, 0).Truncate(HistoryStep)
	s := &Sampler{Host: h, Docker: &fakeDocker{}, Interval: time.Hour, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now: func() time.Time { return clock }}
	s.init()
	ctx := context.Background()

	// Two samples in one bucket average; the open bucket is included.
	h.cpu = 10
	s.sample(ctx)
	clock = clock.Add(5 * time.Second)
	h.cpu = 30
	s.sample(ctx)
	got := s.History()
	if got.StepSeconds != 30 || len(got.Points) != 1 || got.Points[0].CPU != 20 || got.Points[0].Mem != 25 {
		t.Fatalf("history = %+v", got)
	}

	// Four hours of samples keep only the last three hours.
	for i := 0; i < 4*60*2; i++ {
		clock = clock.Add(HistoryStep)
		s.sample(ctx)
	}
	got = s.History()
	span := time.Duration(got.Points[len(got.Points)-1].T-got.Points[0].T) * time.Second
	if len(got.Points) > 362 || span > HistoryWindow {
		t.Errorf("kept %d points spanning %v", len(got.Points), span)
	}
	for i := 1; i < len(got.Points); i++ {
		if got.Points[i].T <= got.Points[i-1].T {
			t.Fatal("points not in order")
		}
	}
}
