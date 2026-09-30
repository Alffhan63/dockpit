package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"dockpit/agent/protocol"
)

// systemTimeout covers disk usage and prunes, which walk the whole image and
// build cache store. It is a little longer than the agent's own limit so the
// agent's error, not ours, reaches the user.
const systemTimeout = 150 * time.Second

func (s *Server) diskUsage(w http.ResponseWriter, r *http.Request) {
	agent, hostID, ok := s.agentFor(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), systemTimeout)
	defer cancel()
	var du protocol.DiskUsage
	if err := agent.Call(ctx, protocol.MethodDiskUsage, struct{}{}, &du); err != nil {
		s.writeAgentError(w, hostID, err)
		return
	}
	writeJSON(w, http.StatusOK, du)
}

func (s *Server) prune(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if kind != protocol.PruneBuildCache && kind != protocol.PruneDanglingImages {
		writeError(w, http.StatusNotFound, "unknown prune kind")
		return
	}
	agent, hostID, ok := s.agentFor(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), systemTimeout)
	defer cancel()
	var res protocol.PruneResult
	if err := agent.Call(ctx, protocol.MethodPrune, protocol.PruneParams{Kind: kind}, &res); err != nil {
		s.writeAgentError(w, hostID, err)
		return
	}
	s.logger.Info("pruned", "host", hostID, "kind", kind, "deleted", res.Deleted, "reclaimed", res.SpaceReclaimed)
	s.audit(r, "system.prune", hostID, kind, fmt.Sprintf("deleted %d, reclaimed %d bytes", res.Deleted, res.SpaceReclaimed))
	writeJSON(w, http.StatusOK, res)
}
