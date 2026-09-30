package api

import (
	"net/http"
	"time"

	"dockpit/server/internal/auth"
)

const (
	sessionCookie = "cockpit_session"
	sessionTTL    = 30 * 24 * time.Hour
	// csrfHeader must be sent with every state-changing request. Browsers
	// cannot add custom headers cross-origin without a CORS preflight, which
	// this server never grants.
	csrfHeader = "X-Requested-With"
	csrfValue  = "cockpit"
)

func (s *Server) csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if r.Header.Get(csrfHeader) != csrfValue {
				writeError(w, http.StatusForbidden, "missing "+csrfHeader+" header")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.authenticated(r) {
			writeError(w, http.StatusUnauthorized, "login required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authenticated(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return false
	}
	ok, err := s.store.SessionValid(r.Context(), auth.HashToken(c.Value), time.Now())
	if err != nil {
		s.logger.Error("session lookup", "err", err)
	}
	return ok
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	hash, err := s.store.AdminPasswordHash(r.Context())
	if err != nil {
		s.internalError(w, "read password hash", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{
		"authenticated": s.authenticated(r),
		"password_set":  hash != "",
	})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	ip := auth.ClientIP(r)
	now := time.Now()
	if !s.logins.Allowed(ip, now) {
		s.logger.Warn("login rate limited", "remote", ip)
		writeError(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	hash, err := s.store.AdminPasswordHash(r.Context())
	if err != nil {
		s.internalError(w, "read password hash", err)
		return
	}
	if hash == "" {
		writeError(w, http.StatusServiceUnavailable, "no admin password set; run: cockpit-server passwd")
		return
	}
	if !auth.CheckPassword(hash, req.Password) {
		s.logins.Fail(ip, now)
		s.logger.Warn("login failed", "remote", ip)
		writeError(w, http.StatusUnauthorized, "wrong password")
		return
	}
	s.logins.Reset(ip)

	token := auth.NewToken()
	expires := now.Add(sessionTTL)
	if err := s.store.CreateSession(r.Context(), auth.HashToken(token), expires); err != nil {
		s.internalError(w, "create session", err)
		return
	}
	if err := s.store.PurgeExpiredSessions(r.Context(), now); err != nil {
		s.logger.Warn("purge sessions", "err", err)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteStrictMode,
	})
	s.logger.Info("login", "remote", ip)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := s.store.DeleteSession(r.Context(), auth.HashToken(c.Value)); err != nil {
			s.logger.Warn("delete session", "err", err)
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteStrictMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

// isHTTPS reports whether the browser reached us over TLS, directly or via
// a local reverse proxy that sets X-Forwarded-Proto.
func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}
