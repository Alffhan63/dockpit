package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const extraSchema = `
CREATE TABLE IF NOT EXISTS audit (
	id      INTEGER PRIMARY KEY AUTOINCREMENT,
	ts      INTEGER NOT NULL,
	actor   TEXT NOT NULL,
	action  TEXT NOT NULL,
	host_id TEXT NOT NULL DEFAULT '',
	target  TEXT NOT NULL DEFAULT '',
	detail  TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS audit_ts ON audit (ts DESC);
CREATE TABLE IF NOT EXISTS metrics (
	host_id TEXT NOT NULL,
	t       INTEGER NOT NULL,
	cpu     REAL NOT NULL,
	mem     REAL NOT NULL,
	disk    REAL NOT NULL,
	PRIMARY KEY (host_id, t)
) WITHOUT ROWID;
`

// AuditEntry is one recorded action.
type AuditEntry struct {
	ID     int64     `json:"id"`
	Time   time.Time `json:"time"`
	Actor  string    `json:"actor"` // remote address of the dashboard user, or "system"
	Action string    `json:"action"`
	HostID string    `json:"host_id,omitempty"`
	Target string    `json:"target,omitempty"`
	Detail string    `json:"detail,omitempty"`
}

// AddAudit records an action.
func (s *Store) AddAudit(ctx context.Context, e AuditEntry) error {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit (ts, actor, action, host_id, target, detail) VALUES (?, ?, ?, ?, ?, ?)`,
		e.Time.Unix(), e.Actor, e.Action, e.HostID, e.Target, truncate(e.Detail, 500))
	return err
}

// ListAudit returns entries newest first. before is an entry ID for paging
// (0 = from the newest).
func (s *Store) ListAudit(ctx context.Context, limit int, before int64) ([]AuditEntry, error) {
	if before <= 0 {
		before = 1 << 62
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, ts, actor, action, host_id, target, detail FROM audit WHERE id < ? ORDER BY id DESC LIMIT ?`, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		var ts int64
		if err := rows.Scan(&e.ID, &ts, &e.Actor, &e.Action, &e.HostID, &e.Target, &e.Detail); err != nil {
			return nil, err
		}
		e.Time = time.Unix(ts, 0)
		out = append(out, e)
	}
	return out, rows.Err()
}

// PurgeAudit deletes entries older than cutoff.
func (s *Store) PurgeAudit(ctx context.Context, cutoff time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM audit WHERE ts < ?`, cutoff.Unix())
	return err
}

// MetricPoint is one stored sample of host metrics, in percent.
type MetricPoint struct {
	T    int64   `json:"t"`
	CPU  float64 `json:"cpu"`
	Mem  float64 `json:"mem"`
	Disk float64 `json:"disk"`
}

// AddMetric stores one sample.
func (s *Store) AddMetric(ctx context.Context, hostID string, p MetricPoint) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO metrics (host_id, t, cpu, mem, disk) VALUES (?, ?, ?, ?, ?)`,
		hostID, p.T, p.CPU, p.Mem, p.Disk)
	return err
}

// Metrics returns samples from since until now, averaged into buckets of
// bucketSeconds (0 = raw).
func (s *Store) Metrics(ctx context.Context, hostID string, since time.Time, bucketSeconds int64) ([]MetricPoint, error) {
	if bucketSeconds < 1 {
		bucketSeconds = 1
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT (t / ?) * ? AS b, AVG(cpu), AVG(mem), AVG(disk) FROM metrics
		 WHERE host_id = ? AND t >= ? GROUP BY b ORDER BY b`,
		bucketSeconds, bucketSeconds, hostID, since.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MetricPoint{}
	for rows.Next() {
		var p MetricPoint
		if err := rows.Scan(&p.T, &p.CPU, &p.Mem, &p.Disk); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PurgeMetrics deletes samples older than cutoff.
func (s *Store) PurgeMetrics(ctx context.Context, cutoff time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM metrics WHERE t < ?`, cutoff.Unix())
	return err
}

// DeleteHostData removes a deleted host's stored metrics.
func (s *Store) DeleteHostData(ctx context.Context, hostID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM metrics WHERE host_id = ?`, hostID)
	return err
}

// GetSetting returns a setting, or "" if unset.
func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// SetSetting stores a setting.
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// DeleteSetting removes a setting.
func (s *Store) DeleteSetting(ctx context.Context, key string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM settings WHERE key = ?`, key)
	return err
}

// Backup writes a consistent copy of the database into dir as
// cockpit-YYYYMMDD-HHMMSS.db and keeps only the newest keep copies.
func (s *Store) Backup(ctx context.Context, dir string, keep int, now time.Time) (string, error) {
	dst := filepath.Join(dir, "cockpit-"+now.Format("20060102-150405")+".db")
	// VACUUM INTO takes a snapshot without blocking readers; the path is
	// ours (not user input) but is still passed as a bound parameter.
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, dst); err != nil {
		return "", fmt.Errorf("backup: %w", err)
	}
	return dst, pruneBackups(dir, keep)
}

func pruneBackups(dir string, keep int) error {
	files, err := filepath.Glob(filepath.Join(dir, "cockpit-*.db"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	for len(files) > keep {
		if err := removeFile(files[0]); err != nil {
			return err
		}
		files = files[1:]
	}
	return nil
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n]
}
