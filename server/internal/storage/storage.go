// Package storage persists controller state in SQLite: registered hosts,
// dashboard sessions and the admin password hash.
//
// Secrets are never stored in plain text: agent tokens and session IDs are
// stored as SHA-256 hashes, the admin password as a bcrypt hash.
package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver, no cgo

	"dockpit/agent/protocol"
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("not found")

const schema = `
CREATE TABLE IF NOT EXISTS hosts (
	id             TEXT PRIMARY KEY,
	name           TEXT NOT NULL,
	token_hash     TEXT NOT NULL UNIQUE,
	created_at     INTEGER NOT NULL,
	last_seen_at   INTEGER,
	os             TEXT NOT NULL DEFAULT '',
	arch           TEXT NOT NULL DEFAULT '',
	docker_version TEXT NOT NULL DEFAULT '',
	agent_version  TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS sessions (
	id_hash    TEXT PRIMARY KEY,
	expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`

// Store is the controller database.
type Store struct {
	db *sql.DB
}

// Open opens (and creates if needed) the database at path.
func Open(path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One writer at a time avoids SQLITE_BUSY; load here is tiny.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	if _, err := db.Exec(extraSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// columns added after the first release; ALTER TABLE only when missing.
var hostColumnsAdded = map[string]string{
	"public_ip": "TEXT NOT NULL DEFAULT ''",
	"addresses": "TEXT NOT NULL DEFAULT '[]'",
}

func migrate(db *sql.DB) error {
	rows, err := db.Query(`SELECT name FROM pragma_table_info('hosts')`)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		have[name] = true
	}
	rows.Close()
	for col, def := range hostColumnsAdded {
		if have[col] {
			continue
		}
		if _, err := db.Exec(`ALTER TABLE hosts ADD COLUMN ` + col + ` ` + def); err != nil {
			return err
		}
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// Host is a registered host.
type Host struct {
	ID         string
	Name       string
	CreatedAt  time.Time
	LastSeenAt *time.Time
	// Info is the last hello the agent sent. Name is unused.
	Info protocol.HostInfo
	// PublicIP is where the agent last connected from, as seen by the
	// controller (behind NAT: the public address of the network).
	PublicIP string
}

const hostColumns = `id, name, created_at, last_seen_at, os, arch, docker_version, agent_version, public_ip, addresses`

func scanHost(row interface{ Scan(...any) error }) (Host, error) {
	var h Host
	var created int64
	var lastSeen sql.NullInt64
	var addresses string
	err := row.Scan(&h.ID, &h.Name, &created, &lastSeen, &h.Info.OS, &h.Info.Arch, &h.Info.DockerVersion, &h.Info.AgentVersion, &h.PublicIP, &addresses)
	if errors.Is(err, sql.ErrNoRows) {
		return h, ErrNotFound
	}
	if err != nil {
		return h, err
	}
	h.CreatedAt = time.Unix(created, 0)
	if addresses != "" && addresses != "[]" {
		if err := json.Unmarshal([]byte(addresses), &h.Info.Addresses); err != nil {
			return h, fmt.Errorf("host %s addresses: %w", h.ID, err)
		}
	}
	if lastSeen.Valid {
		t := time.Unix(lastSeen.Int64, 0)
		h.LastSeenAt = &t
	}
	return h, nil
}

// CreateHost inserts a host. It fails if the ID is taken.
func (s *Store) CreateHost(ctx context.Context, id, name, tokenHash string) (Host, error) {
	now := time.Now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO hosts (id, name, token_hash, created_at) VALUES (?, ?, ?, ?)`,
		id, name, tokenHash, now.Unix())
	if err != nil {
		return Host{}, err
	}
	return Host{ID: id, Name: name, CreatedAt: time.Unix(now.Unix(), 0)}, nil
}

// HostExists reports whether a host ID is taken.
func (s *Store) HostExists(ctx context.Context, id string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM hosts WHERE id = ?`, id).Scan(&n)
	return n > 0, err
}

// ListHosts returns all hosts ordered by name.
func (s *Store) ListHosts(ctx context.Context) ([]Host, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+hostColumns+` FROM hosts ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hosts := []Host{}
	for rows.Next() {
		h, err := scanHost(rows)
		if err != nil {
			return nil, err
		}
		hosts = append(hosts, h)
	}
	return hosts, rows.Err()
}

// GetHost returns one host.
func (s *Store) GetHost(ctx context.Context, id string) (Host, error) {
	return scanHost(s.db.QueryRowContext(ctx, `SELECT `+hostColumns+` FROM hosts WHERE id = ?`, id))
}

// HostIDByTokenHash finds the host that owns a token.
func (s *Store) HostIDByTokenHash(ctx context.Context, tokenHash string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM hosts WHERE token_hash = ?`, tokenHash).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}

// SetHostToken replaces a host's token hash.
func (s *Store) SetHostToken(ctx context.Context, id, tokenHash string) error {
	return expectOne(s.db.ExecContext(ctx, `UPDATE hosts SET token_hash = ? WHERE id = ?`, tokenHash, id))
}

// RecordHello stores what a connecting agent reported, and the address it
// connected from.
func (s *Store) RecordHello(ctx context.Context, id string, info protocol.HostInfo, remoteIP string, at time.Time) error {
	addresses, err := json.Marshal(info.Addresses)
	if err != nil {
		return err
	}
	if info.Addresses == nil {
		addresses = []byte("[]")
	}
	return expectOne(s.db.ExecContext(ctx,
		`UPDATE hosts SET last_seen_at = ?, os = ?, arch = ?, docker_version = ?, agent_version = ?, public_ip = ?, addresses = ? WHERE id = ?`,
		at.Unix(), info.OS, info.Arch, info.DockerVersion, info.AgentVersion, remoteIP, string(addresses), id))
}

// TouchHost updates a host's last-seen time.
func (s *Store) TouchHost(ctx context.Context, id string, at time.Time) error {
	return expectOne(s.db.ExecContext(ctx, `UPDATE hosts SET last_seen_at = ? WHERE id = ?`, at.Unix(), id))
}

// DeleteHost removes a host, revoking its token.
func (s *Store) DeleteHost(ctx context.Context, id string) error {
	if err := expectOne(s.db.ExecContext(ctx, `DELETE FROM hosts WHERE id = ?`, id)); err != nil {
		return err
	}
	return s.DeleteHostData(ctx, id)
}

// CreateSession stores a session that expires at expires.
func (s *Store) CreateSession(ctx context.Context, idHash string, expires time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions (id_hash, expires_at) VALUES (?, ?)`, idHash, expires.Unix())
	return err
}

// SessionValid reports whether a session exists and has not expired.
func (s *Store) SessionValid(ctx context.Context, idHash string, now time.Time) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sessions WHERE id_hash = ? AND expires_at > ?`, idHash, now.Unix()).Scan(&n)
	return n > 0, err
}

// DeleteSession removes a session.
func (s *Store) DeleteSession(ctx context.Context, idHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id_hash = ?`, idHash)
	return err
}

// DeleteAllSessions logs every dashboard session out.
func (s *Store) DeleteAllSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions`)
	return err
}

// PurgeExpiredSessions removes expired sessions.
func (s *Store) PurgeExpiredSessions(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, now.Unix())
	return err
}

const adminPasswordKey = "admin_password_hash"

// AdminPasswordHash returns the stored bcrypt hash, or "" if none is set.
func (s *Store) AdminPasswordHash(ctx context.Context) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, adminPasswordKey).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// SetAdminPasswordHash stores the admin password hash.
func (s *Store) SetAdminPasswordHash(ctx context.Context, hash string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		adminPasswordKey, hash)
	return err
}

func expectOne(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
