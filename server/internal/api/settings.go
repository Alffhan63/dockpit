package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"dockpit/server/internal/monitor"
	"dockpit/server/internal/notify"
)

// maskPrefix marks a secret the browser was shown in hidden form. Sending it
// back unchanged means "keep the stored value".
const maskPrefix = "••••"

func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 6 {
		return maskPrefix
	}
	return maskPrefix + s[len(s)-4:]
}

func keepIfMasked(incoming, stored string) string {
	if strings.HasPrefix(incoming, maskPrefix) {
		return stored
	}
	return incoming
}

func (s *Server) loadNotifications(ctx context.Context) (notify.Config, error) {
	cfg := notify.DefaultConfig()
	raw, err := s.store.GetSetting(ctx, monitor.NotificationsKey)
	if err != nil || raw == "" {
		return cfg, err
	}
	return cfg, json.Unmarshal([]byte(raw), &cfg)
}

func masked(cfg notify.Config) notify.Config {
	cfg.Channels.Telegram.BotToken = maskSecret(cfg.Channels.Telegram.BotToken)
	cfg.Channels.Ntfy.Token = maskSecret(cfg.Channels.Ntfy.Token)
	if cfg.Channels.Webhook.URL != "" {
		// Webhook URLs often embed a secret (Slack, Discord).
		cfg.Channels.Webhook.URL = maskSecret(cfg.Channels.Webhook.URL)
	}
	return cfg
}

func (s *Server) getNotifications(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadNotifications(r.Context())
	if err != nil {
		s.internalError(w, "read notification settings", err)
		return
	}
	writeJSON(w, http.StatusOK, masked(cfg))
}

func (s *Server) putNotifications(w http.ResponseWriter, r *http.Request) {
	var in notify.Config
	if !readJSON(w, r, &in) {
		return
	}
	old, err := s.loadNotifications(r.Context())
	if err != nil {
		s.internalError(w, "read notification settings", err)
		return
	}
	in.Channels.Telegram.BotToken = keepIfMasked(in.Channels.Telegram.BotToken, old.Channels.Telegram.BotToken)
	in.Channels.Ntfy.Token = keepIfMasked(in.Channels.Ntfy.Token, old.Channels.Ntfy.Token)
	in.Channels.Webhook.URL = keepIfMasked(in.Channels.Webhook.URL, old.Channels.Webhook.URL)
	if err := in.Channels.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if in.Rules.DiskPercent < 0 || in.Rules.DiskPercent > 100 || in.Rules.MemPercent < 0 || in.Rules.MemPercent > 100 {
		writeError(w, http.StatusBadRequest, "percentages must be between 0 and 100")
		return
	}
	if in.Enabled && !in.Channels.Configured() {
		writeError(w, http.StatusBadRequest, "set up at least one channel before turning notifications on")
		return
	}
	raw, _ := json.Marshal(in)
	if err := s.store.SetSetting(r.Context(), monitor.NotificationsKey, string(raw)); err != nil {
		s.internalError(w, "save notification settings", err)
		return
	}
	s.audit(r, "settings.notifications", "", "", "")
	writeJSON(w, http.StatusOK, masked(in))
}

// testNotifications sends a message through the channels in the request body
// (so unsaved changes can be tested), falling back to the saved secrets for
// values the browser only saw masked.
func (s *Server) testNotifications(w http.ResponseWriter, r *http.Request) {
	var in notify.Channels
	if !readJSON(w, r, &in) {
		return
	}
	old, err := s.loadNotifications(r.Context())
	if err != nil {
		s.internalError(w, "read notification settings", err)
		return
	}
	in.Telegram.BotToken = keepIfMasked(in.Telegram.BotToken, old.Channels.Telegram.BotToken)
	in.Ntfy.Token = keepIfMasked(in.Ntfy.Token, old.Channels.Ntfy.Token)
	in.Webhook.URL = keepIfMasked(in.Webhook.URL, old.Channels.Webhook.URL)
	if err := in.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !in.Configured() {
		writeError(w, http.StatusBadRequest, "no channel configured")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	if err := in.Send(ctx, notify.Message{Level: notify.Info, Title: "Docker Cockpit test", Body: "Notifications are working."}); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.audit(r, "settings.notifications.test", "", "", "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getPrune(w http.ResponseWriter, r *http.Request) {
	var sched monitor.PruneSchedule
	raw, err := s.store.GetSetting(r.Context(), monitor.PruneKey)
	if err != nil {
		s.internalError(w, "read prune schedule", err)
		return
	}
	if raw == "" {
		sched = monitor.PruneSchedule{Weekday: 0, Hour: 4, DanglingImage: true, BuildCache: true}
	} else {
		_ = json.Unmarshal([]byte(raw), &sched)
	}
	writeJSON(w, http.StatusOK, sched)
}

func (s *Server) putPrune(w http.ResponseWriter, r *http.Request) {
	var in monitor.PruneSchedule
	if !readJSON(w, r, &in) {
		return
	}
	if in.Weekday < 0 || in.Weekday > 6 || in.Hour < 0 || in.Hour > 23 {
		writeError(w, http.StatusBadRequest, "invalid weekday or hour")
		return
	}
	if in.Enabled && !in.DanglingImage && !in.BuildCache {
		writeError(w, http.StatusBadRequest, "choose what to clean")
		return
	}
	raw, _ := json.Marshal(in)
	if err := s.store.SetSetting(r.Context(), monitor.PruneKey, string(raw)); err != nil {
		s.internalError(w, "save prune schedule", err)
		return
	}
	s.audit(r, "settings.prune", "", "", "")
	writeJSON(w, http.StatusOK, in)
}
