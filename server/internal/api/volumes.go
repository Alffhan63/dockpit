package api

import (
	"context"
	"net/http"

	"dockpit/agent/protocol"
)

func (s *Server) hostHistory(w http.ResponseWriter, r *http.Request) {
	agent, hostID, ok := s.agentFor(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), agentCallTimeout)
	defer cancel()
	var h protocol.HostHistory
	if err := agent.Call(ctx, protocol.MethodHostHistory, struct{}{}, &h); err != nil {
		s.writeAgentError(w, hostID, err)
		return
	}
	if h.Points == nil {
		h.Points = []protocol.HistoryPoint{}
	}
	writeJSON(w, http.StatusOK, h)
}

func (s *Server) listVolumes(w http.ResponseWriter, r *http.Request) {
	agent, hostID, ok := s.agentFor(w, r)
	if !ok {
		return
	}
	// Volume sizes come from system df, which can be slow.
	ctx, cancel := context.WithTimeout(r.Context(), systemTimeout)
	defer cancel()
	volumes := []protocol.Volume{}
	if err := agent.Call(ctx, protocol.MethodListVolumes, struct{}{}, &volumes); err != nil {
		s.writeAgentError(w, hostID, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"host_id": hostID, "volumes": volumes})
}

func (s *Server) removeVolume(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("volume")
	if !protocol.ValidVolumeName(name) {
		writeError(w, http.StatusBadRequest, "invalid volume name")
		return
	}
	agent, hostID, ok := s.agentFor(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), agentActionTimeout)
	defer cancel()
	if err := agent.Call(ctx, protocol.MethodRemoveVolume, protocol.VolumeParams{Name: name}, nil); err != nil {
		s.writeAgentError(w, hostID, err)
		return
	}
	// Volume deletion loses data, so it is always logged.
	s.logger.Warn("volume removed", "host", hostID, "volume", name, "remote", r.RemoteAddr)
	s.audit(r, "volumes.remove", hostID, name, "")
	w.WriteHeader(http.StatusNoContent)
}
