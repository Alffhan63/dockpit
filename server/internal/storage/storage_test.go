package storage

import (
	"context"
	"errors"
	"path/filepath"
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

	info := protocol.HostInfo{OS: "darwin", Arch: "arm64", DockerVersion: "29.1.3", AgentVersion: "dev"}
	seen := time.Unix(1700000000, 0)
	if err := s.RecordHello(ctx, "mac-mini", info, seen); err != nil {
		t.Fatal(err)
	}
	h, err := s.GetHost(ctx, "mac-mini")
	if err != nil {
		t.Fatal(err)
	}
	if h.Name != "Mac mini" || h.Info != info || h.LastSeenAt == nil || !h.LastSeenAt.Equal(seen) {
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
