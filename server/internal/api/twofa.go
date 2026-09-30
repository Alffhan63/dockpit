package api

import (
	"net/http"
	"strconv"
	"time"

	"dockpit/server/internal/auth"
)

// Settings keys for two-factor authentication.
const (
	totpSecretKey  = "totp_secret"    // active secret; enabled when set
	totpPendingKey = "totp_pending"   // generated, not yet confirmed
	totpLastStep   = "totp_last_step" // newest accepted time step (replay guard)
	totpIssuer     = "Docker Cockpit"
	totpAccount    = "admin"
)

func (s *Server) totpSecret(r *http.Request) (string, error) {
	return s.store.GetSetting(r.Context(), totpSecretKey)
}

// checkTOTP verifies a code and rejects one whose time step was used already.
func (s *Server) checkTOTP(r *http.Request, secret, code string) bool {
	step, ok := auth.VerifyTOTP(secret, code, time.Now())
	if !ok {
		return false
	}
	last, _ := s.store.GetSetting(r.Context(), totpLastStep)
	if n, _ := strconv.ParseInt(last, 10, 64); step <= n {
		return false
	}
	if err := s.store.SetSetting(r.Context(), totpLastStep, strconv.FormatInt(step, 10)); err != nil {
		s.logger.Warn("save totp step", "err", err)
	}
	return true
}

func (s *Server) twoFAStatus(w http.ResponseWriter, r *http.Request) {
	secret, err := s.totpSecret(r)
	if err != nil {
		s.internalError(w, "read 2fa state", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": secret != ""})
}

// twoFASetup creates a pending secret. It only takes effect once a valid
// code proves the authenticator app has it.
func (s *Server) twoFASetup(w http.ResponseWriter, r *http.Request) {
	if secret, _ := s.totpSecret(r); secret != "" {
		writeError(w, http.StatusConflict, "two-factor authentication is already on")
		return
	}
	secret := auth.NewTOTPSecret()
	if err := s.store.SetSetting(r.Context(), totpPendingKey, secret); err != nil {
		s.internalError(w, "save 2fa secret", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"secret": secret,
		"uri":    auth.TOTPURI(secret, totpIssuer, totpAccount),
	})
}

func (s *Server) twoFAEnable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	pending, err := s.store.GetSetting(r.Context(), totpPendingKey)
	if err != nil {
		s.internalError(w, "read 2fa secret", err)
		return
	}
	if pending == "" {
		writeError(w, http.StatusConflict, "start the setup first")
		return
	}
	if !s.checkTOTP(r, pending, req.Code) {
		writeError(w, http.StatusUnauthorized, "wrong code")
		return
	}
	if err := s.store.SetSetting(r.Context(), totpSecretKey, pending); err != nil {
		s.internalError(w, "enable 2fa", err)
		return
	}
	_ = s.store.DeleteSetting(r.Context(), totpPendingKey)
	s.audit(r, "auth.2fa.enable", "", "", "")
	w.WriteHeader(http.StatusNoContent)
}

// twoFADisable needs both the password and a current code, so a hijacked
// session alone cannot turn 2FA off.
func (s *Server) twoFADisable(w http.ResponseWriter, r *http.Request) {
	ip := auth.ClientIP(r)
	now := time.Now()
	if !s.logins.Allowed(ip, now) {
		writeError(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
		return
	}
	var req struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	secret, err := s.totpSecret(r)
	if err != nil {
		s.internalError(w, "read 2fa secret", err)
		return
	}
	if secret == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	hash, err := s.store.AdminPasswordHash(r.Context())
	if err != nil {
		s.internalError(w, "read password hash", err)
		return
	}
	if !auth.CheckPassword(hash, req.Password) || !s.checkTOTP(r, secret, req.Code) {
		s.logins.Fail(ip, now)
		writeError(w, http.StatusUnauthorized, "wrong password or code")
		return
	}
	s.logins.Reset(ip)
	_ = s.store.DeleteSetting(r.Context(), totpSecretKey)
	_ = s.store.DeleteSetting(r.Context(), totpPendingKey)
	s.audit(r, "auth.2fa.disable", "", "", "")
	w.WriteHeader(http.StatusNoContent)
}
