package monitor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"dockpit/agent/protocol"
	"dockpit/server/internal/storage"
)

const (
	// PruneKey is the settings key holding PruneSchedule as JSON.
	PruneKey        = "prune_schedule"
	keepBackups     = 7
	keepMetricsFor  = 30 * 24 * time.Hour
	keepAuditFor    = 180 * 24 * time.Hour
	maintenanceEach = time.Minute
	backupHour      = 3
)

// PruneSchedule runs the safe prunes (dangling images, build cache) weekly.
// Volumes and containers are never pruned.
type PruneSchedule struct {
	Enabled       bool `json:"enabled"`
	Weekday       int  `json:"weekday"` // 0 = Sunday
	Hour          int  `json:"hour"`    // 0-23, server time zone
	DanglingImage bool `json:"dangling_images"`
	BuildCache    bool `json:"build_cache"`
}

func (m *Monitor) maintenanceLoop(ctx context.Context) {
	var lastBackup, lastPrune, lastPurge string
	t := time.NewTicker(maintenanceEach)
	defer t.Stop()
	for {
		now := m.now()
		day := now.Format("2006-01-02")

		if m.backups != "" && lastBackup != day && (now.Hour() >= backupHour) {
			if m.backupNeeded(day) {
				m.runBackup(ctx, now)
			}
			lastBackup = day
		}
		if lastPurge != day {
			m.purge(ctx, now)
			lastPurge = day
		}
		if sched := m.pruneSchedule(ctx); sched.Enabled && lastPrune != day &&
			int(now.Weekday()) == sched.Weekday && now.Hour() >= sched.Hour {
			m.runPrune(ctx, sched)
			lastPrune = day
		}

		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (m *Monitor) backupNeeded(day string) bool {
	files, _ := filepath.Glob(filepath.Join(m.backups, "cockpit-"+strings.ReplaceAll(day, "-", "")+"-*.db"))
	return len(files) == 0
}

func (m *Monitor) runBackup(ctx context.Context, now time.Time) {
	if err := os.MkdirAll(m.backups, 0o750); err != nil {
		m.logger.Warn("backup: create dir", "err", err)
		return
	}
	path, err := m.store.Backup(ctx, m.backups, keepBackups, now)
	if err != nil {
		m.logger.Warn("backup failed", "err", err)
		return
	}
	m.logger.Info("database backup written", "path", path)
}

func (m *Monitor) purge(ctx context.Context, now time.Time) {
	if err := m.store.PurgeMetrics(ctx, now.Add(-keepMetricsFor)); err != nil {
		m.logger.Warn("purge metrics", "err", err)
	}
	if err := m.store.PurgeAudit(ctx, now.Add(-keepAuditFor)); err != nil {
		m.logger.Warn("purge audit", "err", err)
	}
}

func (m *Monitor) pruneSchedule(ctx context.Context) PruneSchedule {
	var s PruneSchedule
	raw, err := m.store.GetSetting(ctx, PruneKey)
	if err == nil && raw != "" {
		_ = json.Unmarshal([]byte(raw), &s)
	}
	return s
}

func (m *Monitor) runPrune(ctx context.Context, sched PruneSchedule) {
	kinds := []string{}
	if sched.DanglingImage {
		kinds = append(kinds, protocol.PruneDanglingImages)
	}
	if sched.BuildCache {
		kinds = append(kinds, protocol.PruneBuildCache)
	}
	list, err := m.store.ListHosts(ctx)
	if err != nil {
		return
	}
	for _, h := range list {
		agent, err := m.registry.Get(h.ID)
		if err != nil {
			continue
		}
		for _, kind := range kinds {
			cctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
			var res protocol.PruneResult
			err := agent.Call(cctx, protocol.MethodPrune, protocol.PruneParams{Kind: kind}, &res)
			cancel()
			detail := kind
			if err != nil {
				detail += ": " + err.Error()
			} else {
				detail += ": deleted " + strconv.Itoa(res.Deleted) + ", reclaimed " + strconv.FormatInt(res.SpaceReclaimed, 10) + " bytes"
			}
			m.logger.Info("scheduled prune", "host", h.ID, "detail", detail)
			_ = m.store.AddAudit(ctx, storage.AuditEntry{Actor: "system", Action: "prune.scheduled", HostID: h.ID, Detail: detail})
		}
	}
}
