package api

import (
	"context"
	"net/http"

	"dockpit/agent/protocol"
)

func (s *Server) listImages(w http.ResponseWriter, r *http.Request) {
	agent, hostID, ok := s.agentFor(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), agentCallTimeout)
	defer cancel()
	images := []protocol.Image{}
	if err := agent.Call(ctx, protocol.MethodListImages, struct{}{}, &images); err != nil {
		s.writeAgentError(w, hostID, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"host_id": hostID, "images": images})
}

func (s *Server) removeImage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("image")
	if !protocol.ValidImageID(id) {
		writeError(w, http.StatusBadRequest, "invalid image id")
		return
	}
	agent, hostID, ok := s.agentFor(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), agentActionTimeout)
	defer cancel()
	if err := agent.Call(ctx, protocol.MethodRemoveImage, protocol.ImageParams{ID: id}, nil); err != nil {
		s.writeAgentError(w, hostID, err)
		return
	}
	s.logger.Info("image removed", "host", hostID, "image", id)
	s.audit(r, "images.remove", hostID, id, "")
	w.WriteHeader(http.StatusNoContent)
}
