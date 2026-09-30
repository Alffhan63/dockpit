package api

import (
	"context"
	"net/http"
	"time"

	"dockpit/agent/protocol"
	"dockpit/server/internal/hosts"
)

const (
	agentCallTimeout = 15 * time.Second
	// Stop and restart give containers 10s to exit.
	agentActionTimeout = 45 * time.Second
)

var containerActions = map[string]string{
	"start":   protocol.MethodStartContainer,
	"stop":    protocol.MethodStopContainer,
	"restart": protocol.MethodRestartContainer,
}

// agentFor validates the host ID and returns its live agent, writing an error
// response when it cannot.
func (s *Server) agentFor(w http.ResponseWriter, r *http.Request) (*hosts.Agent, string, bool) {
	h, ok := s.hostFromPath(w, r)
	if !ok {
		return nil, "", false
	}
	agent, err := s.registry.Get(h.ID)
	if err != nil {
		s.writeAgentError(w, h.ID, err)
		return nil, "", false
	}
	return agent, h.ID, true
}

func containerRef(w http.ResponseWriter, r *http.Request) (string, bool) {
	ref := r.PathValue("container")
	if !protocol.ValidContainerRef(ref) {
		writeError(w, http.StatusBadRequest, "invalid container id")
		return "", false
	}
	return ref, true
}

func (s *Server) listContainers(w http.ResponseWriter, r *http.Request) {
	agent, hostID, ok := s.agentFor(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), agentCallTimeout)
	defer cancel()
	params := protocol.ListContainersParams{All: r.URL.Query().Get("all") != "false"}
	containers := []protocol.Container{}
	if err := agent.Call(ctx, protocol.MethodListContainers, params, &containers); err != nil {
		s.writeAgentError(w, hostID, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"host_id": hostID, "containers": containers})
}

func (s *Server) containerAction(w http.ResponseWriter, r *http.Request) {
	method, known := containerActions[r.PathValue("action")]
	if !known {
		writeError(w, http.StatusNotFound, "unknown action")
		return
	}
	s.callContainer(w, r, method, false)
}

func (s *Server) removeContainer(w http.ResponseWriter, r *http.Request) {
	s.callContainer(w, r, protocol.MethodRemoveContainer, r.URL.Query().Get("force") == "true")
}

func (s *Server) callContainer(w http.ResponseWriter, r *http.Request, method string, force bool) {
	ref, ok := containerRef(w, r)
	if !ok {
		return
	}
	agent, hostID, ok := s.agentFor(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), agentActionTimeout)
	defer cancel()
	if err := agent.Call(ctx, method, protocol.ContainerParams{ID: ref, Force: force}, nil); err != nil {
		s.writeAgentError(w, hostID, err)
		return
	}
	s.logger.Info("container action", "host", hostID, "container", ref, "method", method, "force", force)
	w.WriteHeader(http.StatusNoContent)
}
