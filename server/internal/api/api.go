// Package api is the controller's HTTP API.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"

	"dockpit/agent/protocol"
	"dockpit/server/internal/auth"
	"dockpit/server/internal/hosts"
	"dockpit/server/internal/monitor"
	"dockpit/server/internal/storage"
)

const agentReadLimit = 8 << 20 // container lists can be large

// Server serves the controller API.
type Server struct {
	store    *storage.Store
	registry *hosts.Registry
	logger   *slog.Logger
	web      http.Handler
	logins   *auth.Limiter
	mon      *monitor.Monitor // optional
	version  string
}

// New returns a Server. web serves the built UI and may be nil.
func New(store *storage.Store, registry *hosts.Registry, logger *slog.Logger, web http.Handler) *Server {
	return &Server{
		store:    store,
		registry: registry,
		logger:   logger,
		web:      web,
		logins:   auth.NewLimiter(5, 15*time.Minute),
	}
}

// SetVersion sets the controller version shown next to agent versions.
func (s *Server) SetVersion(v string) { s.version = v }

// SetMonitor attaches the background monitor that feeds the overview,
// sparklines and alerts.
func (s *Server) SetMonitor(m *monitor.Monitor) { s.mon = m }

// audit records an action taken from the dashboard.
func (s *Server) audit(r *http.Request, action, hostID, target, detail string) {
	e := storage.AuditEntry{Actor: auth.ClientIP(r), Action: action, HostID: hostID, Target: target, Detail: detail}
	if err := s.store.AddAudit(context.WithoutCancel(r.Context()), e); err != nil {
		s.logger.Warn("write audit log", "err", err)
	}
}

// Handler returns the HTTP handler for the controller.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Public.
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET "+protocol.ConnectPath, s.agentConnect) // agent token auth
	mux.HandleFunc("GET /api/v1/auth/session", s.session)
	mux.Handle("POST /api/v1/auth/login", s.csrf(http.HandlerFunc(s.login)))

	// Dashboard session required.
	private := func(pattern string, h http.HandlerFunc) {
		mux.Handle(pattern, s.requireSession(s.csrf(h)))
	}
	private("POST /api/v1/auth/logout", s.logout)
	private("GET /api/v1/hosts", s.listHosts)
	private("POST /api/v1/hosts", s.createHost)
	private("GET /api/v1/hosts/{id}", s.getHost)
	private("DELETE /api/v1/hosts/{id}", s.deleteHost)
	private("POST /api/v1/hosts/{id}/token", s.rotateToken)
	private("GET /api/v1/hosts/{id}/status", s.hostStatus)
	private("GET /api/v1/hosts/{id}/containers", s.listContainers)
	private("POST /api/v1/hosts/{id}/containers/{container}/{action}", s.containerAction)
	private("DELETE /api/v1/hosts/{id}/containers/{container}", s.removeContainer)
	private("GET /api/v1/hosts/{id}/containers/{container}/logs", s.containerLogs) // WebSocket
	private("GET /api/v1/hosts/{id}/images", s.listImages)
	private("DELETE /api/v1/hosts/{id}/images/{image}", s.removeImage)
	private("GET /api/v1/hosts/{id}/disk", s.diskUsage)
	private("GET /api/v1/hosts/{id}/history", s.hostHistory)
	private("GET /api/v1/hosts/{id}/volumes", s.listVolumes)
	private("DELETE /api/v1/hosts/{id}/volumes/{volume}", s.removeVolume)
	private("POST /api/v1/hosts/{id}/prune/{kind}", s.prune)
	private("GET /api/v1/hosts/{id}/containers/{container}/inspect", s.inspectContainer)
	private("GET /api/v1/hosts/{id}/sparks", s.sparks)
	private("GET /api/v1/hosts/{id}/metrics", s.hostMetrics)
	private("GET /api/v1/overview", s.overview)
	private("GET /api/v1/audit", s.listAudit)
	private("GET /api/v1/settings/notifications", s.getNotifications)
	private("PUT /api/v1/settings/notifications", s.putNotifications)
	private("POST /api/v1/settings/notifications/test", s.testNotifications)
	private("GET /api/v1/settings/prune", s.getPrune)
	private("PUT /api/v1/settings/prune", s.putPrune)
	private("GET /api/v1/auth/2fa", s.twoFAStatus)
	private("POST /api/v1/auth/2fa/setup", s.twoFASetup)
	private("POST /api/v1/auth/2fa/enable", s.twoFAEnable)
	private("POST /api/v1/auth/2fa/disable", s.twoFADisable)

	if s.web != nil {
		mux.Handle("GET /", securityHeaders(s.web))
	}
	return s.logRequests(mux)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) agentConnect(w http.ResponseWriter, r *http.Request) {
	token, ok := auth.BearerToken(r)
	var hostID string
	var err error
	if ok {
		hostID, err = s.store.HostIDByTokenHash(r.Context(), auth.HashToken(token))
	}
	if !ok || err != nil {
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			s.logger.Error("agent token lookup", "err", err)
		}
		s.logger.Warn("agent authentication failed", "remote", auth.ClientIP(r))
		writeError(w, http.StatusUnauthorized, "invalid agent token")
		return
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		s.logger.Warn("agent websocket upgrade failed", "host", hostID, "err", err)
		return
	}
	conn.SetReadLimit(agentReadLimit)

	remote := auth.ClientIP(r)
	s.logger.Info("agent connected", "host", hostID, "remote", remote)
	err = s.registry.Serve(r.Context(), hostID, conn, func(info protocol.HostInfo) {
		if err := s.store.RecordHello(context.Background(), hostID, info, remote, time.Now()); err != nil {
			s.logger.Warn("record hello", "host", hostID, "err", err)
		}
	})
	if err := s.store.TouchHost(context.Background(), hostID, time.Now()); err != nil && !errors.Is(err, storage.ErrNotFound) {
		s.logger.Warn("touch host", "host", hostID, "err", err)
	}
	s.logger.Info("agent disconnected", "host", hostID, "err", err)
}

func (s *Server) writeAgentError(w http.ResponseWriter, hostID string, err error) {
	var agentErr *hosts.AgentError
	switch {
	case errors.Is(err, context.Canceled):
		// The browser went away mid-request; nobody reads this response.
		s.logger.Debug("agent call cancelled by client", "host", hostID)
		w.WriteHeader(499) // nginx's "client closed request"
	case errors.Is(err, hosts.ErrOffline):
		writeError(w, http.StatusServiceUnavailable, "host is offline")
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, "agent did not respond in time")
	case errors.As(err, &agentErr) && agentErr.Code >= 400 && agentErr.Code < 500:
		writeError(w, agentErr.Code, agentErr.Msg)
	case errors.As(err, &agentErr):
		writeError(w, http.StatusBadGateway, agentErr.Msg)
	default:
		s.logger.Error("agent call failed", "host", hostID, "err", err)
		writeError(w, http.StatusBadGateway, "agent call failed")
	}
}

func (s *Server) internalError(w http.ResponseWriter, msg string, err error) {
	s.logger.Error(msg, "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// readJSON decodes a small JSON request body.
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		// Content-hashed build output can be cached forever. Everything else
		// (index.html, the service worker, the manifest) must be revalidated
		// so an installed app picks up new versions.
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			h.Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			h.Set("Cache-Control", "no-cache")
		}
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Unwrap lets the websocket library reach the underlying http.Hijacker.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.logger.Info("http request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "duration", time.Since(start).String())
	})
}
