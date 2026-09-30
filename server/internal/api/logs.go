package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"dockpit/agent/protocol"
	"dockpit/server/internal/hosts"
)

const browserWriteTimeout = 10 * time.Second

// logEvent is what the browser receives on the logs WebSocket.
type logEvent struct {
	Type  string          `json:"type"` // "lines", "end" or "error"
	Lines json.RawMessage `json:"lines,omitempty"`
	Error string          `json:"error,omitempty"`
}

// containerLogs streams a container's logs to the browser over WebSocket.
//
// The upgrade happens before the host is checked so that problems reach the
// browser as readable "error" events instead of a failed handshake.
func (s *Server) containerLogs(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("container")
	hostID := r.PathValue("id")
	tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))

	// Accept rejects cross-origin requests by default, which stops other
	// sites from opening this socket with the user's session cookie.
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		s.logger.Warn("logs websocket upgrade failed", "err", err)
		return
	}
	defer conn.CloseNow()
	// We never read from the browser; this also notices when it goes away.
	ctx := conn.CloseRead(r.Context())

	send := func(ev logEvent) error {
		wctx, cancel := context.WithTimeout(ctx, browserWriteTimeout)
		defer cancel()
		return wsjson.Write(wctx, conn, ev)
	}
	fail := func(msg string) {
		send(logEvent{Type: "error", Error: msg})
		conn.Close(websocket.StatusNormalClosure, "")
	}

	if !hosts.ValidID(hostID) || !protocol.ValidContainerRef(ref) {
		fail("invalid host or container id")
		return
	}
	agent, err := s.registry.Get(hostID)
	if err != nil {
		fail("host is offline")
		return
	}

	s.logger.Info("log stream opened", "host", hostID, "container", ref)
	err = agent.Stream(ctx, protocol.MethodContainerLogs, protocol.LogsParams{ID: ref, Tail: tail}, func(chunk json.RawMessage) error {
		return send(logEvent{Type: "lines", Lines: chunk})
	})
	s.logger.Info("log stream closed", "host", hostID, "container", ref, "err", err)

	var agentErr *hosts.AgentError
	switch {
	case err == nil:
		send(logEvent{Type: "end"})
		conn.Close(websocket.StatusNormalClosure, "")
	case ctx.Err() != nil:
		// Browser went away.
	case errors.Is(err, hosts.ErrOffline):
		fail("host went offline")
	case errors.Is(err, hosts.ErrSlowConsumer):
		fail("log stream too fast for this connection; reconnect to resume")
	case errors.As(err, &agentErr):
		fail(agentErr.Msg)
	default:
		fail("log stream failed")
	}
}
