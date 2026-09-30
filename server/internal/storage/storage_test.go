package storage

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"dockpit/agent/protocol"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestHosts(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()

	if _, err := s.CreateHost(ctx, "mac-mini", "Mac mini", "hash1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateHost(ctx, "mac-mini", "dup", "hash2"); err == nil {
		t.Error("duplicate id accepted")
	}
	if _, err := s.CreateHost(ctx, "other", "Other", "hash1"); err == nil {
		t.Error("duplicate token hash accepted")
	}

	id, err := s.HostIDByTokenHash(ctx, "hash1")
	if err != nil || id != "mac-mini" {
		t.Fatalf("HostIDByTokenHash = %q, %v", id, err)
	}
	if _, err := s.HostIDByTokenHash(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown token: err = %v", err)
	}

	info := protocol.HostInfo{OS: "darwin", Arch: "arm64", DockerVersion: "29.1.3", AgentVersion: "dev",
		Addresses: []protocol.Address{{Interface: "en0", IP: "192.168.1.20", Scope: "private"}}}
	seen := time.Unix(1700000000, 0)
	if err := s.RecordHello(ctx, "mac-mini", info, "203.0.113.7", seen); err != nil {
		t.Fatal(err)
	}
	h, err := s.GetHost(ctx, "mac-mini")
	if err != nil {
		t.Fatal(err)
	}
	if h.Name != "Mac mini" || !reflect.DeepEqual(h.Info, info) || h.PublicIP != "203.0.113.7" || h.LastSeenAt == nil || !h.LastSeenAt.Equal(seen) {
		t.Errorf("host = %+v", h)
	}

	if err := s.SetHostToken(ctx, "mac-mini", "hash3"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.HostIDByTokenHash(ctx, "hash1"); !errors.Is(err, ErrNotFound) {
		t.Error("old token still valid after rotation")
	}

	list, _ := s.ListHosts(ctx)
	if len(list) != 1 {
		t.Fatalf("ListHosts = %v", list)
	}
	if err := s.DeleteHost(ctx, "mac-mini"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteHost(ctx, "mac-mini"); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete: err = %v", err)
	}
	if _, err := s.HostIDByTokenHash(ctx, "hash3"); !errors.Is(err, ErrNotFound) {
		t.Error("token valid after host deleted")
	}
}

func TestSessions(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	now := time.Now()

	s.CreateSession(ctx, "live", now.Add(time.Hour))
	s.CreateSession(ctx, "old", now.Add(-time.Hour))
	if ok, _ := s.SessionValid(ctx, "live", now); !ok {
		t.Error("live session invalid")
	}
	if ok, _ := s.SessionValid(ctx, "old", now); ok {
		t.Error("expired session valid")
	}
	s.DeleteSession(ctx, "live")
	if ok, _ := s.SessionValid(ctx, "live", now); ok {
		t.Error("deleted session valid")
	}
}

func TestAdminPassword(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if h, err := s.AdminPasswordHash(ctx); err != nil || h != "" {
		t.Fatalf("unset hash = %q, %v", h, err)
	}
	s.SetAdminPasswordHash(ctx, "a")
	s.SetAdminPasswordHash(ctx, "b")
	if h, _ := s.AdminPasswordHash(ctx); h != "b" {
		t.Errorf("hash = %q, want b", h)
	}
}

// A database created before public_ip/addresses existed is migrated in place.
func TestMigrateOldSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE hosts (id TEXT PRIMARY KEY, name TEXT NOT NULL, token_hash TEXT NOT NULL UNIQUE,
		created_at INTEGER NOT NULL, last_seen_at INTEGER, os TEXT NOT NULL DEFAULT '', arch TEXT NOT NULL DEFAULT '',
		docker_version TEXT NOT NULL DEFAULT '', agent_version TEXT NOT NULL DEFAULT '');
		INSERT INTO hosts (id, name, token_hash, created_at) VALUES ('old', 'Old', 'h', 1);`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h, err := s.GetHost(context.Background(), "old")
	if err != nil || h.PublicIP != "" || h.Info.Addresses != nil {
		t.Fatalf("migrated host = %+v, %v", h, err)
	}
	// Opening again is a no-op.
	s.Close()
	if s, err = Open(path); err != nil {
		t.Fatal(err)
	}
}
