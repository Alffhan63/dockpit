package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"dockpit/agent/protocol"
	"dockpit/server/internal/hosts"
	"dockpit/server/internal/storage"
)

// hostView is a registered host as the dashboard sees it.
type hostView struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Online      bool              `json:"online"`
	CreatedAt   time.Time         `json:"created_at"`
	LastSeenAt  *time.Time        `json:"last_seen_at,omitempty"`
	ConnectedAt *time.Time        `json:"connected_at,omitempty"`
	Info        protocol.HostInfo `json:"info"`
}

func (s *Server) view(h storage.Host, live map[string]hosts.Host) hostView {
	v := hostView{ID: h.ID, Name: h.Name, CreatedAt: h.CreatedAt, LastSeenAt: h.LastSeenAt, Info: h.Info}
	v.Info.Name = ""
	if l, ok := live[h.ID]; ok {
		v.Online = true
		v.ConnectedAt = &l.ConnectedAt
		v.Info = l.Info
		v.Info.Name = ""
	}
	return v
}

func (s *Server) liveHosts() map[string]hosts.Host {
	live := make(map[string]hosts.Host)
	for _, h := range s.registry.List() {
		live[h.ID] = h
	}
	return live
}

func (s *Server) listHosts(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListHosts(r.Context())
	if err != nil {
		s.internalError(w, "list hosts", err)
		return
	}
	live := s.liveHosts()
	views := make([]hostView, 0, len(list))
	for _, h := range list {
		views = append(views, s.view(h, live))
	}
	writeJSON(w, http.StatusOK, map[string]any{"hosts": views})
}

func (s *Server) hostFromPath(w http.ResponseWriter, r *http.Request) (storage.Host, bool) {
	id := r.PathValue("id")
	if !hosts.ValidID(id) {
		writeError(w, http.StatusBadRequest, "invalid host id")
		return storage.Host{}, false
	}
	h, err := s.store.GetHost(r.Context(), id)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "host not found")
		return h, false
	}
	if err != nil {
		s.internalError(w, "get host", err)
		return h, false
	}
	return h, true
}

func (s *Server) getHost(w http.ResponseWriter, r *http.Request) {
	h, ok := s.hostFromPath(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.view(h, s.liveHosts()))
}

func (s *Server) createHost(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	h, token, err := hosts.Register(r.Context(), s.store, req.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.logger.Info("host registered", "host", h.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"host": s.view(h, nil), "token": token})
}

func (s *Server) deleteHost(w http.ResponseWriter, r *http.Request) {
	h, ok := s.hostFromPath(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteHost(r.Context(), h.ID); err != nil {
		s.internalError(w, "delete host", err)
		return
	}
	s.registry.Disconnect(h.ID)
	s.logger.Info("host removed", "host", h.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) rotateToken(w http.ResponseWriter, r *http.Request) {
	h, ok := s.hostFromPath(w, r)
	if !ok {
		return
	}
	token, err := hosts.RotateToken(r.Context(), s.store, h.ID)
	if err != nil {
		s.internalError(w, "rotate token", err)
		return
	}
	// The connected agent used the old token; make it reconnect.
	s.registry.Disconnect(h.ID)
	s.logger.Info("host token rotated", "host", h.ID)
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

// hostStatus returns the host with live metrics from its agent.
func (s *Server) hostStatus(w http.ResponseWriter, r *http.Request) {
	agent, hostID, ok := s.agentFor(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), agentCallTimeout)
	defer cancel()
	var status protocol.HostStatus
	if err := agent.Call(ctx, protocol.MethodHostStatus, struct{}{}, &status); err != nil {
		s.writeAgentError(w, hostID, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}
