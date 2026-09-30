package metrics

import (
	"context"
	"log/slog"
	"math"
	"sync"
	"time"

	"dockpit/agent/internal/docker"
	"dockpit/agent/protocol"
)

const (
	// Container stats are only sampled while someone is looking.
	containerDemandWindow = 2 * time.Minute
	statsConcurrency      = 8

	// Host metrics history: 3 hours in 30s buckets (360 points), in memory.
	HistoryWindow = 3 * time.Hour
	HistoryStep   = 30 * time.Second
)

// Docker is the part of the Docker client the sampler needs.
type Docker interface {
	ListContainers(ctx context.Context, all bool) ([]protocol.Container, error)
	ContainerStats(ctx context.Context, id string) (docker.Stats, error)
}

// ContainerUsage is the latest usage of one container.
type ContainerUsage struct {
	CPUPercent *float64
	MemUsage   uint64
	MemLimit   uint64
}

// Sampler periodically samples host metrics, and container stats while
// they are in demand.
type Sampler struct {
	Host     HostCollector
	Docker   Docker
	Interval time.Duration
	Logger   *slog.Logger
	// Now is the clock; nil means time.Now. Tests set it.
	Now func() time.Time

	mu         sync.Mutex
	host       protocol.HostMetrics
	usage      map[string]ContainerUsage
	prev       map[string]docker.Stats
	lastWanted time.Time
	wake       chan struct{}
	once       sync.Once

	history []protocol.HistoryPoint // completed buckets, oldest first
	bucket  historyBucket           // bucket being filled
}

type historyBucket struct {
	start  time.Time
	cpuSum float64
	n      int
	mem    float64
}

func (s *Sampler) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// record adds a host sample to the history. Callers hold s.mu.
func (s *Sampler) record(m protocol.HostMetrics, at time.Time) {
	start := at.Truncate(HistoryStep)
	if !s.bucket.start.Equal(start) {
		if s.bucket.n > 0 {
			s.history = append(s.history, s.bucket.point())
		}
		s.bucket = historyBucket{start: start}
		cutoff := at.Add(-HistoryWindow).Unix()
		drop := 0
		for drop < len(s.history) && s.history[drop].T < cutoff {
			drop++
		}
		s.history = s.history[drop:]
	}
	s.bucket.cpuSum += m.CPUPercent
	s.bucket.n++
	if m.MemTotal > 0 {
		s.bucket.mem = float64(m.MemUsed) / float64(m.MemTotal) * 100
	}
}

func (b historyBucket) point() protocol.HistoryPoint {
	round := func(v float64) float64 { return math.Round(v*10) / 10 }
	return protocol.HistoryPoint{T: b.start.Unix(), CPU: round(b.cpuSum / float64(b.n)), Mem: round(b.mem)}
}

// History returns the last HistoryWindow of host metrics, including the
// bucket still being filled.
func (s *Sampler) History() protocol.HostHistory {
	s.init()
	s.mu.Lock()
	defer s.mu.Unlock()
	points := make([]protocol.HistoryPoint, len(s.history), len(s.history)+1)
	copy(points, s.history)
	if s.bucket.n > 0 {
		points = append(points, s.bucket.point())
	}
	return protocol.HostHistory{StepSeconds: int(HistoryStep / time.Second), Points: points}
}

func (s *Sampler) init() {
	s.once.Do(func() {
		s.usage = make(map[string]ContainerUsage)
		s.prev = make(map[string]docker.Stats)
		s.wake = make(chan struct{}, 1)
	})
}

// Run samples until ctx is done.
func (s *Sampler) Run(ctx context.Context) {
	s.init()
	t := time.NewTicker(s.Interval)
	defer t.Stop()
	for {
		s.sample(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-s.wake:
		}
	}
}

// HostMetrics returns the latest host sample.
func (s *Sampler) HostMetrics() protocol.HostMetrics {
	s.init()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.host
}

// ContainerUsage returns the latest container usage by container ID and
// marks container stats as wanted.
func (s *Sampler) ContainerUsage() map[string]ContainerUsage {
	s.init()
	s.mu.Lock()
	idle := time.Since(s.lastWanted) > containerDemandWindow
	s.lastWanted = time.Now()
	out := make(map[string]ContainerUsage, len(s.usage))
	for id, u := range s.usage {
		out[id] = u
	}
	s.mu.Unlock()

	if idle {
		// Start sampling now rather than at the next tick.
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
	return out
}

func (s *Sampler) sample(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, s.Interval)
	defer cancel()

	if m, err := s.Host.Collect(ctx); err != nil {
		s.Logger.Debug("host metrics", "err", err)
	} else {
		now := s.now()
		m.SampledAt = now.Unix()
		s.mu.Lock()
		s.host = m
		s.record(m, now)
		s.mu.Unlock()
	}

	s.mu.Lock()
	wanted := time.Since(s.lastWanted) <= containerDemandWindow
	s.mu.Unlock()
	if !wanted {
		s.mu.Lock()
		// Stale samples would give wrong CPU deltas when demand resumes.
		clear(s.prev)
		clear(s.usage)
		s.mu.Unlock()
		return
	}
	s.sampleContainers(ctx)
}

func (s *Sampler) sampleContainers(ctx context.Context) {
	running, err := s.Docker.ListContainers(ctx, false)
	if err != nil {
		s.Logger.Debug("container stats: list", "err", err)
		return
	}

	type result struct {
		id    string
		stats docker.Stats
		err   error
	}
	results := make(chan result, len(running))
	sem := make(chan struct{}, statsConcurrency)
	for _, c := range running {
		go func(id string) {
			sem <- struct{}{}
			defer func() { <-sem }()
			st, err := s.Docker.ContainerStats(ctx, id)
			results <- result{id, st, err}
		}(c.ID)
	}

	usage := make(map[string]ContainerUsage, len(running))
	prev := make(map[string]docker.Stats, len(running))
	s.mu.Lock()
	oldPrev := s.prev
	s.mu.Unlock()
	for range running {
		r := <-results
		if r.err != nil {
			continue
		}
		u := ContainerUsage{MemUsage: r.stats.MemUsage, MemLimit: r.stats.MemLimit}
		if p, ok := oldPrev[r.id]; ok {
			if pct, ok := docker.CPUPercent(p, r.stats); ok {
				u.CPUPercent = &pct
			}
		}
		usage[r.id] = u
		prev[r.id] = r.stats
	}

	s.mu.Lock()
	s.usage, s.prev = usage, prev
	s.mu.Unlock()
}
