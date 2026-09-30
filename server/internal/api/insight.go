package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"dockpit/agent/protocol"
	"dockpit/server/internal/monitor"
	"dockpit/server/internal/storage"
)

func (s *Server) inspectContainer(w http.ResponseWriter, r *http.Request) {
	ref, ok := containerRef(w, r)
	if !ok {
		return
	}
	agent, hostID, ok := s.agentFor(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), agentCallTimeout)
	defer cancel()
	var d protocol.ContainerDetail
	if err := agent.Call(ctx, protocol.MethodInspectContainer, protocol.ContainerParams{ID: ref}, &d); err != nil {
		s.writeAgentError(w, hostID, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) sparks(w http.ResponseWriter, r *http.Request) {
	h, ok := s.hostFromPath(w, r)
	if !ok {
		return
	}
	out := map[string][]float64{}
	if s.mon != nil {
		out = s.mon.Sparks(h.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"sparks": out})
}

// metricRanges maps ?range= to a window and a bucket size that keeps every
// chart around 200-400 points.
var metricRanges = map[string]struct{ window, bucket time.Duration }{
	"1h":  {time.Hour, time.Minute},
	"6h":  {6 * time.Hour, time.Minute},
	"24h": {24 * time.Hour, 5 * time.Minute},
	"7d":  {7 * 24 * time.Hour, 30 * time.Minute},
	"30d": {30 * 24 * time.Hour, 2 * time.Hour},
}

func (s *Server) hostMetrics(w http.ResponseWriter, r *http.Request) {
	h, ok := s.hostFromPath(w, r)
	if !ok {
		return
	}
	rg, ok := metricRanges[r.URL.Query().Get("range")]
	if !ok {
		rg = metricRanges["24h"]
	}
	points, err := s.store.Metrics(r.Context(), h.ID, time.Now().Add(-rg.window), int64(rg.bucket.Seconds()))
	if err != nil {
		s.internalError(w, "read metrics", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"bucket_seconds": int64(rg.bucket.Seconds()), "points": points})
}

type overviewHost struct {
	monitor.HostSummary
	Spark []float64 `json:"spark"` // CPU %, last hour, one point per minute
}

// overview summarises every host and what needs attention.
func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListHosts(r.Context())
	if err != nil {
		s.internalError(w, "list hosts", err)
		return
	}
	summaries := map[string]monitor.HostSummary{}
	if s.mon != nil {
		for _, sm := range s.mon.Overview() {
			summaries[sm.ID] = sm
		}
	}
	live := s.liveHosts()
	since := time.Now().Add(-time.Hour)
	out := make([]overviewHost, 0, len(list))
	for _, h := range list {
		sm := summaries[h.ID]
		sm.ID, sm.Name = h.ID, h.Name
		_, sm.Online = live[h.ID]
		if sm.Problems == nil {
			sm.Problems = []monitor.Problem{}
		}
		spark := []float64{}
		if pts, err := s.store.Metrics(r.Context(), h.ID, since, 60); err == nil {
			for _, p := range pts {
				spark = append(spark, p.CPU)
			}
		}
		out = append(out, overviewHost{HostSummary: sm, Spark: spark})
	}
	writeJSON(w, http.StatusOK, map[string]any{"hosts": out})
}

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	entries, err := s.store.ListAudit(r.Context(), limit, before)
	if err != nil {
		s.internalError(w, "read audit log", err)
		return
	}
	if entries == nil {
		entries = []storage.AuditEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}
