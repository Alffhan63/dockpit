package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"dockpit/server/internal/auth"
)

func TestTwoFactorLoginFlow(t *testing.T) {
	e := newEnv(t)

	status, body := e.do("POST", "/api/v1/auth/2fa/setup", nil)
	if status != 200 {
		t.Fatalf("setup: %d", status)
	}
	var secret string
	json.Unmarshal(body["secret"], &secret)
	if secret == "" {
		t.Fatal("no secret returned")
	}
	// Not active until a valid code is confirmed.
	if _, body := e.do("GET", "/api/v1/auth/2fa", nil); string(body["enabled"]) != "false" {
		t.Fatalf("enabled before confirmation: %s", body["enabled"])
	}
	if status, _ := e.do("POST", "/api/v1/auth/2fa/enable", map[string]string{"code": "000000"}); status != 401 {
		t.Fatalf("enable with wrong code = %d", status)
	}
	code := codeAt(t, secret, time.Now().Add(-30*time.Second)) // previous step
	if status, _ := e.do("POST", "/api/v1/auth/2fa/enable", map[string]string{"code": code}); status != 204 {
		t.Fatalf("enable = %d", status)
	}

	// Password alone is no longer enough, and asking for the code is not a strike.
	c := newClient()
	for i := 0; i < 8; i++ {
		status, body := e.doWith(c, "POST", "/api/v1/auth/login", map[string]string{"password": testPassword})
		if status != 401 || string(body["totp_required"]) != "true" {
			t.Fatalf("login without code = %d %v", status, body)
		}
	}
	// The code used to enable 2FA cannot be replayed.
	if status, _ := e.doWith(c, "POST", "/api/v1/auth/login", map[string]string{"password": testPassword, "code": code}); status != 401 {
		t.Fatalf("replayed code accepted: %d", status)
	}
	if status, _ := e.doWith(c, "POST", "/api/v1/auth/login", map[string]string{"password": "wrong password!!", "code": code}); status != 401 {
		t.Fatalf("wrong password accepted: %d", status)
	}

	// A code from a newer step works.
	next := currentCode(t, secret)
	if status, body := e.doWith(newClient(), "POST", "/api/v1/auth/login", map[string]string{"password": testPassword, "code": next}); status != 204 {
		t.Fatalf("login with code = %d %v", status, body)
	}

	// Disabling needs password and a current code.
	if status, _ := e.do("POST", "/api/v1/auth/2fa/disable", map[string]string{"password": testPassword, "code": "123456"}); status != 401 {
		t.Fatalf("disable with bad code = %d", status)
	}
	after := codeAt(t, secret, time.Now().Add(30*time.Second))
	if status, _ := e.do("POST", "/api/v1/auth/2fa/disable", map[string]string{"password": testPassword, "code": after}); status != 204 {
		t.Fatalf("disable = %d", status)
	}
	if status, _ := e.doWith(newClient(), "POST", "/api/v1/auth/login", map[string]string{"password": testPassword}); status != 204 {
		t.Fatalf("login after disable = %d", status)
	}
}

func currentCode(t *testing.T, secret string) string { return codeAt(t, secret, time.Now()) }

func codeAt(t *testing.T, secret string, at time.Time) string {
	t.Helper()
	code, err := auth.TOTPCode(secret, at)
	if err != nil {
		t.Fatal(err)
	}
	return code
}

func TestNotificationSecretsAreMaskedAndKept(t *testing.T) {
	e := newEnv(t)
	cfg := map[string]any{
		"enabled":  true,
		"channels": map[string]any{"webhook": map[string]string{"url": "https://hooks.example.com/services/T000/B000/SECRETVALUE"}, "telegram": map[string]string{"bot_token": "123456:ABCDEFsecret", "chat_id": "42"}},
		"rules":    map[string]any{"host_offline": true, "disk_percent": 90},
	}
	if status, body := e.do("PUT", "/api/v1/settings/notifications", cfg); status != 200 {
		t.Fatalf("put = %d %v", status, body)
	}
	status, body := e.do("GET", "/api/v1/settings/notifications", nil)
	if status != 200 {
		t.Fatalf("get = %d", status)
	}
	raw, _ := json.Marshal(body)
	if strings.Contains(string(raw), "SECRETVALUE") || strings.Contains(string(raw), "ABCDEFsecret") {
		t.Fatalf("secrets leaked to the browser: %s", raw)
	}

	// Saving the masked form back must keep the stored secrets.
	var channels map[string]map[string]string
	json.Unmarshal(body["channels"], &channels)
	back := map[string]any{"enabled": true, "channels": channels, "rules": map[string]any{"disk_percent": 80}}
	if status, _ := e.do("PUT", "/api/v1/settings/notifications", back); status != 200 {
		t.Fatalf("put masked = %d", status)
	}
	stored, _ := e.store.GetSetting(t.Context(), "notifications")
	if !strings.Contains(stored, "SECRETVALUE") || !strings.Contains(stored, "ABCDEFsecret") {
		t.Fatalf("masked save overwrote secrets: %s", stored)
	}

	// Validation.
	bad := map[string]any{"enabled": true, "channels": map[string]any{}}
	if status, _ := e.do("PUT", "/api/v1/settings/notifications", bad); status != 400 {
		t.Fatalf("enabled without channel = %d", status)
	}
	bad = map[string]any{"channels": map[string]any{"webhook": map[string]string{"url": "file:///etc/passwd"}}}
	if status, _ := e.do("PUT", "/api/v1/settings/notifications", bad); status != 400 {
		t.Fatalf("file:// webhook = %d", status)
	}
}

func TestAuditRecordsActionsAndRequiresLogin(t *testing.T) {
	e := newEnv(t)
	if status, _ := e.do("POST", "/api/v1/hosts", map[string]string{"name": "Second"}); status != 201 {
		t.Fatalf("create host = %d", status)
	}
	status, body := e.do("GET", "/api/v1/audit", nil)
	if status != 200 {
		t.Fatalf("audit = %d", status)
	}
	var entries []struct{ Action, Target string }
	json.Unmarshal(body["entries"], &entries)
	found := map[string]bool{}
	for _, en := range entries {
		found[en.Action] = true
	}
	if !found["hosts.create"] || !found["auth.login"] {
		t.Fatalf("missing entries: %+v", entries)
	}
	if status, _ := e.doWith(newClient(), "GET", "/api/v1/audit", nil); status != 401 {
		t.Fatalf("audit without session = %d", status)
	}
	if status, _ := e.doWith(newClient(), "GET", "/api/v1/overview", nil); status != 401 {
		t.Fatalf("overview without session = %d", status)
	}
}

func TestOverviewListsEveryHost(t *testing.T) {
	e := newEnv(t)
	status, body := e.do("GET", "/api/v1/overview", nil)
	if status != 200 {
		t.Fatalf("overview = %d", status)
	}
	var hs []struct {
		ID       string
		Online   bool
		Problems []any
		Spark    []float64
	}
	json.Unmarshal(body["hosts"], &hs)
	if len(hs) != 1 || hs[0].ID != "local" || hs[0].Online || hs[0].Problems == nil || hs[0].Spark == nil {
		t.Fatalf("overview = %+v", hs)
	}
}
