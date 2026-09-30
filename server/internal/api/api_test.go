package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"dockpit/agent/protocol"
	"dockpit/server/internal/auth"
	"dockpit/server/internal/hosts"
	"dockpit/server/internal/storage"
)

const testPassword = "correct horse battery"

type env struct {
	t      *testing.T
	srv    *httptest.Server
	reg    *hosts.Registry
	store  *storage.Store
	client *http.Client // logged in
	token  string       // agent token of host "local"
}

func newEnv(t *testing.T) *env {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "cockpit.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()
	_, token, err := hosts.Register(ctx, store, "local")
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := auth.HashPassword(testPassword)
	store.SetAdminPasswordHash(ctx, hash)

	reg := hosts.NewRegistry()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(New(store, reg, logger, nil).Handler())
	t.Cleanup(srv.Close)

	e := &env{t: t, srv: srv, reg: reg, store: store, token: token, client: newClient()}
	if status, body := e.doWith(e.client, "POST", "/api/v1/auth/login", map[string]string{"password": testPassword}); status != 204 {
		t.Fatalf("login: %d %v", status, body)
	}
	return e
}

func newClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

func (e *env) do(method, path string, body any) (int, map[string]json.RawMessage) {
	return e.doWith(e.client, method, path, body)
}

func (e *env) doWith(c *http.Client, method, path string, body any) (int, map[string]json.RawMessage) {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, rd)
	req.Header.Set(csrfHeader, csrfValue)
	resp, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]json.RawMessage
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (e *env) dialAgent(token string) (*websocket.Conn, *http.Response, error) {
	return websocket.Dial(context.Background(), e.srv.URL+protocol.ConnectPath, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Bearer " + token}},
	})
}

// fakeAgent connects as host "local", sends a hello and answers requests
// with answer (nil closes the connection). It returns once the host is online.
func (e *env) fakeAgent(answer func(protocol.Message) *protocol.Message) *websocket.Conn {
	e.t.Helper()
	ctx := context.Background()
	conn, _, err := e.dialAgent(e.token)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { conn.CloseNow() })

	hello := protocol.Message{Type: protocol.TypeHello, Hello: &protocol.HostInfo{OS: "linux", Arch: "amd64", DockerVersion: "27.0.0"}}
	if err := wsjson.Write(ctx, conn, hello); err != nil {
		e.t.Fatal(err)
	}
	go func() {
		for {
			var req protocol.Message
			if err := wsjson.Read(ctx, conn, &req); err != nil {
				return
			}
			if req.Type != protocol.TypeRequest {
				continue
			}
			resp := answer(req)
			if resp == nil {
				conn.Close(websocket.StatusNormalClosure, "bye")
				return
			}
			wsjson.Write(ctx, conn, resp)
		}
	}()
	e.waitOnline("local", true)
	return conn
}

func (e *env) waitOnline(id string, want bool) {
	e.t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, err := e.reg.Get(id)
		if (err == nil) == want {
			return
		}
		if time.Now().After(deadline) {
			e.t.Fatalf("host %s online=%v never happened", id, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestHealth(t *testing.T) {
	e := newEnv(t)
	status, body := e.doWith(http.DefaultClient, "GET", "/healthz", nil)
	if status != http.StatusOK || string(body["status"]) != `"ok"` {
		t.Fatalf("status %d body %v", status, body)
	}
}

func TestAuth(t *testing.T) {
	e := newEnv(t)
	anon := newClient()

	if status, _ := e.doWith(anon, "GET", "/api/v1/hosts", nil); status != 401 {
		t.Errorf("anonymous hosts: %d, want 401", status)
	}
	status, body := e.doWith(anon, "GET", "/api/v1/auth/session", nil)
	if status != 200 || string(body["authenticated"]) != "false" || string(body["password_set"]) != "true" {
		t.Errorf("anonymous session: %d %v", status, body)
	}
	if _, body := e.do("GET", "/api/v1/auth/session", nil); string(body["authenticated"]) != "true" {
		t.Errorf("logged-in session: %v", body)
	}

	// CSRF: state-changing requests without the header are refused.
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/hosts", strings.NewReader(`{"name":"x"}`))
	resp, _ := e.client.Do(req)
	if resp.StatusCode != 403 {
		t.Errorf("POST without CSRF header: %d, want 403", resp.StatusCode)
	}
	resp.Body.Close()

	// Brute force is rate limited per client.
	for i := 0; i < 5; i++ {
		if status, _ := e.doWith(anon, "POST", "/api/v1/auth/login", map[string]string{"password": "wrong password!"}); status != 401 {
			t.Fatalf("wrong password #%d: %d", i, status)
		}
	}
	if status, _ := e.doWith(anon, "POST", "/api/v1/auth/login", map[string]string{"password": testPassword}); status != 429 {
		t.Errorf("after 5 failures: %d, want 429", status)
	}

	if status, _ := e.do("POST", "/api/v1/auth/logout", nil); status != 204 {
		t.Errorf("logout: %d", status)
	}
	if status, _ := e.do("GET", "/api/v1/hosts", nil); status != 401 {
		t.Errorf("after logout: %d, want 401", status)
	}
}

func TestHostLifecycle(t *testing.T) {
	e := newEnv(t)

	status, body := e.do("POST", "/api/v1/hosts", map[string]string{"name": "Mac mini"})
	if status != 201 {
		t.Fatalf("create: %d %v", status, body)
	}
	var created hostView
	json.Unmarshal(body["host"], &created)
	var token string
	json.Unmarshal(body["token"], &token)
	if created.ID != "mac-mini" || created.Online || len(token) != 64 {
		t.Fatalf("created %+v token len %d", created, len(token))
	}
	if status, _ := e.do("POST", "/api/v1/hosts", map[string]string{"name": " "}); status != 400 {
		t.Errorf("blank name: %d", status)
	}

	// The new token connects as mac-mini.
	conn, _, err := e.dialAgent(token)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	wsjson.Write(context.Background(), conn, protocol.Message{Type: protocol.TypeHello, Hello: &protocol.HostInfo{OS: "darwin", Arch: "arm64"}})
	e.waitOnline("mac-mini", true)

	_, body = e.do("GET", "/api/v1/hosts", nil)
	var list []hostView
	json.Unmarshal(body["hosts"], &list)
	if len(list) != 2 || list[0].ID != "local" || list[0].Online || list[1].ID != "mac-mini" || !list[1].Online || list[1].Info.OS != "darwin" {
		t.Fatalf("list = %+v", list)
	}

	// Rotation disconnects the agent and invalidates the old token.
	status, body = e.do("POST", "/api/v1/hosts/mac-mini/token", nil)
	var newToken string
	json.Unmarshal(body["token"], &newToken)
	if status != 200 || newToken == token {
		t.Fatalf("rotate: %d", status)
	}
	e.waitOnline("mac-mini", false)
	if _, resp, err := e.dialAgent(token); err == nil || resp.StatusCode != 401 {
		t.Error("old token still accepted")
	}

	// The stored info survives disconnects.
	_, body = e.do("GET", "/api/v1/hosts/mac-mini", nil)
	var got hostView
	json.Unmarshal(mustJSON(body), &got)
	if got.Online || got.Info.OS != "darwin" || got.LastSeenAt == nil {
		t.Errorf("offline host view = %+v", got)
	}

	if status, _ := e.do("DELETE", "/api/v1/hosts/mac-mini", nil); status != 204 {
		t.Errorf("delete: %d", status)
	}
	if status, _ := e.do("GET", "/api/v1/hosts/mac-mini", nil); status != 404 {
		t.Errorf("get deleted: %d", status)
	}
	if _, resp, err := e.dialAgent(newToken); err == nil || resp.StatusCode != 401 {
		t.Error("deleted host's token still accepted")
	}
}

func mustJSON(m map[string]json.RawMessage) []byte {
	b, _ := json.Marshal(m)
	return b
}

func TestListContainersThroughAgent(t *testing.T) {
	e := newEnv(t)
	e.fakeAgent(func(req protocol.Message) *protocol.Message {
		result, _ := json.Marshal([]protocol.Container{{ID: "abc", Name: "web", State: "running"}})
		return &protocol.Message{Type: protocol.TypeResponse, ID: req.ID, Result: result}
	})
	status, body := e.do("GET", "/api/v1/hosts/local/containers", nil)
	var containers []protocol.Container
	json.Unmarshal(body["containers"], &containers)
	if status != 200 || len(containers) != 1 || containers[0].Name != "web" {
		t.Fatalf("status %d containers %s", status, body["containers"])
	}
}

func TestAgentErrors(t *testing.T) {
	e := newEnv(t)
	if status, _ := e.do("GET", "/api/v1/hosts/local/containers", nil); status != 503 {
		t.Errorf("offline: %d, want 503", status)
	}
	if status, _ := e.do("GET", "/api/v1/hosts/nope/containers", nil); status != 404 {
		t.Errorf("unknown host: %d, want 404", status)
	}
	if status, _ := e.do("GET", "/api/v1/hosts/Bad_ID/containers", nil); status != 400 {
		t.Errorf("invalid host: %d, want 400", status)
	}
	if _, resp, err := e.dialAgent("wrong"); err == nil || resp.StatusCode != 401 {
		t.Errorf("bad agent token accepted")
	}

	e.fakeAgent(func(req protocol.Message) *protocol.Message {
		if req.Method == protocol.MethodStopContainer {
			return nil // drop the connection mid-call
		}
		return &protocol.Message{Type: protocol.TypeResponse, ID: req.ID, Error: "docker is down"}
	})
	status, body := e.do("GET", "/api/v1/hosts/local/containers", nil)
	if status != 502 || !strings.Contains(string(body["error"]), "docker is down") {
		t.Errorf("agent error: %d %v", status, body)
	}
	if status, _ := e.do("POST", "/api/v1/hosts/local/containers/web/stop", nil); status != 503 {
		t.Errorf("disconnect mid-call: %d, want 503", status)
	}
}

func TestContainerActions(t *testing.T) {
	e := newEnv(t)
	type seen struct {
		method string
		params protocol.ContainerParams
	}
	calls := make(chan seen, 10)
	e.fakeAgent(func(req protocol.Message) *protocol.Message {
		var p protocol.ContainerParams
		json.Unmarshal(req.Params, &p)
		calls <- seen{req.Method, p}
		if p.ID == "missing" {
			return &protocol.Message{Type: protocol.TypeResponse, ID: req.ID, Error: "No such container: missing", Code: 404}
		}
		return &protocol.Message{Type: protocol.TypeResponse, ID: req.ID}
	})
	base := "/api/v1/hosts/local/containers/"

	cases := []struct {
		method, path string
		status       int
		want         *seen
	}{
		{"POST", "web/start", 204, &seen{protocol.MethodStartContainer, protocol.ContainerParams{ID: "web"}}},
		{"POST", "web/stop", 204, &seen{protocol.MethodStopContainer, protocol.ContainerParams{ID: "web"}}},
		{"POST", "web/restart", 204, &seen{protocol.MethodRestartContainer, protocol.ContainerParams{ID: "web"}}},
		{"DELETE", "web", 204, &seen{protocol.MethodRemoveContainer, protocol.ContainerParams{ID: "web"}}},
		{"DELETE", "web?force=true", 204, &seen{protocol.MethodRemoveContainer, protocol.ContainerParams{ID: "web", Force: true}}},
		{"POST", "missing/start", 404, &seen{protocol.MethodStartContainer, protocol.ContainerParams{ID: "missing"}}},
		{"POST", "web/exec", 404, nil},
		{"POST", "bad%20id/start", 400, nil},
	}
	for _, tc := range cases {
		status, body := e.do(tc.method, base+tc.path, nil)
		if status != tc.status {
			t.Errorf("%s %s: status %d, want %d (%v)", tc.method, tc.path, status, tc.status, body)
		}
		if tc.want == nil {
			continue
		}
		select {
		case got := <-calls:
			if got != *tc.want {
				t.Errorf("%s %s: agent got %+v, want %+v", tc.method, tc.path, got, *tc.want)
			}
		case <-time.After(time.Second):
			t.Errorf("%s %s: agent not called", tc.method, tc.path)
		}
	}
	select {
	case got := <-calls:
		t.Errorf("unexpected agent call %+v", got)
	default:
	}
}

func (e *env) dialLogs(path string) *websocket.Conn {
	e.t.Helper()
	u, _ := url.Parse(e.srv.URL)
	conn, _, err := websocket.Dial(context.Background(), e.srv.URL+path, &websocket.DialOptions{
		HTTPClient: e.client,
		HTTPHeader: http.Header{"Origin": {e.srv.URL}, "Host": {u.Host}},
	})
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { conn.CloseNow() })
	return conn
}

func readEvent(t *testing.T, conn *websocket.Conn) logEvent {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var ev logEvent
	if err := wsjson.Read(ctx, conn, &ev); err != nil {
		t.Fatal(err)
	}
	return ev
}

func TestLogsStream(t *testing.T) {
	e := newEnv(t)
	cancelled := make(chan string, 1)
	e.fakeAgentRaw(func(ctx context.Context, conn *websocket.Conn, req protocol.Message) {
		switch req.Type {
		case protocol.TypeCancel:
			cancelled <- req.ID
		case protocol.TypeRequest:
			var p protocol.LogsParams
			json.Unmarshal(req.Params, &p)
			if p.ID != "web" || p.Tail != 50 {
				t.Errorf("params = %+v", p)
			}
			for i := 0; i < 2; i++ {
				chunk, _ := json.Marshal([]protocol.LogLine{{Stream: "stderr", Text: "boom"}})
				wsjson.Write(ctx, conn, protocol.Message{Type: protocol.TypeStream, ID: req.ID, Result: chunk})
			}
		}
	})

	conn := e.dialLogs("/api/v1/hosts/local/containers/web/logs?tail=50")
	for i := 0; i < 2; i++ {
		ev := readEvent(t, conn)
		if ev.Type != "lines" || !strings.Contains(string(ev.Lines), "boom") {
			t.Fatalf("event %d = %+v", i, ev)
		}
	}
	// Closing the browser side cancels the stream on the agent.
	conn.Close(websocket.StatusNormalClosure, "")
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("agent never got a cancel")
	}
}

func TestLogsErrors(t *testing.T) {
	e := newEnv(t)
	ev := readEvent(t, e.dialLogs("/api/v1/hosts/local/containers/web/logs"))
	if ev.Type != "error" || ev.Error != "host is offline" {
		t.Errorf("offline: %+v", ev)
	}

	agentConn := e.fakeAgentRaw(func(ctx context.Context, conn *websocket.Conn, req protocol.Message) {
		// Only "nope" fails; other streams stay open until the agent drops.
		if req.Type == protocol.TypeRequest && strings.Contains(string(req.Params), `"nope"`) {
			wsjson.Write(ctx, conn, protocol.Message{Type: protocol.TypeResponse, ID: req.ID, Error: "docker: No such container: nope", Code: 404})
		}
	})
	ev = readEvent(t, e.dialLogs("/api/v1/hosts/local/containers/nope/logs"))
	if ev.Type != "error" || !strings.Contains(ev.Error, "No such container") {
		t.Errorf("agent error: %+v", ev)
	}

	// Agent disconnecting mid-stream is reported to the browser.
	conn := e.dialLogs("/api/v1/hosts/local/containers/web/logs")
	time.Sleep(100 * time.Millisecond)
	agentConn.Close(websocket.StatusGoingAway, "")
	for {
		ev = readEvent(t, conn)
		if ev.Type == "error" {
			break
		}
	}
	if ev.Error != "host went offline" {
		t.Errorf("disconnect: %+v", ev)
	}

	// Cross-origin pages cannot open the socket with the session cookie.
	_, resp, err := websocket.Dial(context.Background(), e.srv.URL+"/api/v1/hosts/local/containers/web/logs", &websocket.DialOptions{
		HTTPClient: e.client,
		HTTPHeader: http.Header{"Origin": {"https://evil.example"}},
	})
	if err == nil || resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-origin logs socket allowed: %v", err)
	}
	// And anonymous users cannot open it at all.
	_, resp, err = websocket.Dial(context.Background(), e.srv.URL+"/api/v1/hosts/local/containers/web/logs", nil)
	if err == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous logs socket allowed: %v", err)
	}
}

// fakeAgentRaw connects as host "local" and passes every message to handle.
func (e *env) fakeAgentRaw(handle func(ctx context.Context, conn *websocket.Conn, msg protocol.Message)) *websocket.Conn {
	e.t.Helper()
	ctx := context.Background()
	conn, _, err := e.dialAgent(e.token)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { conn.CloseNow() })
	wsjson.Write(ctx, conn, protocol.Message{Type: protocol.TypeHello, Hello: &protocol.HostInfo{}})
	go func() {
		for {
			var msg protocol.Message
			if err := wsjson.Read(ctx, conn, &msg); err != nil {
				return
			}
			handle(ctx, conn, msg)
		}
	}()
	e.waitOnline("local", true)
	return conn
}

func TestHostStatus(t *testing.T) {
	e := newEnv(t)
	if status, _ := e.do("GET", "/api/v1/hosts/local/status", nil); status != 503 {
		t.Errorf("offline: %d, want 503", status)
	}
	e.fakeAgent(func(req protocol.Message) *protocol.Message {
		if req.Method != protocol.MethodHostStatus {
			return &protocol.Message{Type: protocol.TypeResponse, ID: req.ID, Error: "unexpected"}
		}
		result, _ := json.Marshal(protocol.HostStatus{
			Metrics: protocol.HostMetrics{CPUPercent: 32, MemUsed: 47, MemTotal: 100},
			Docker:  protocol.DockerInfo{Version: "27.3.1", Containers: 8, Running: 7},
		})
		return &protocol.Message{Type: protocol.TypeResponse, ID: req.ID, Result: result}
	})
	status, body := e.do("GET", "/api/v1/hosts/local/status", nil)
	var metrics protocol.HostMetrics
	var docker protocol.DockerInfo
	json.Unmarshal(body["metrics"], &metrics)
	json.Unmarshal(body["docker"], &docker)
	if status != 200 || metrics.CPUPercent != 32 || docker.Running != 7 {
		t.Fatalf("status %d body %v", status, body)
	}
}

func TestWebCacheHeaders(t *testing.T) {
	store, _ := storage.Open(filepath.Join(t.TempDir(), "c.db"))
	defer store.Close()
	web := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	srv := httptest.NewServer(New(store, hosts.NewRegistry(), slog.New(slog.NewTextHandler(io.Discard, nil)), web).Handler())
	defer srv.Close()
	for path, want := range map[string]string{
		"/":                     "no-cache",
		"/sw.js":                "no-cache",
		"/manifest.webmanifest": "no-cache",
		"/assets/index-abc.js":  "public, max-age=31536000, immutable",
	} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if got := resp.Header.Get("Cache-Control"); got != want {
			t.Errorf("%s: Cache-Control = %q, want %q", path, got, want)
		}
		if resp.Header.Get("Content-Security-Policy") == "" {
			t.Errorf("%s: missing CSP", path)
		}
	}
}

func TestImages(t *testing.T) {
	e := newEnv(t)
	removed := make(chan string, 4)
	e.fakeAgent(func(req protocol.Message) *protocol.Message {
		switch req.Method {
		case protocol.MethodListImages:
			result, _ := json.Marshal([]protocol.Image{{ID: "sha256:abc123abc123", Tags: []string{"redis:7"}, Size: 10}})
			return &protocol.Message{Type: protocol.TypeResponse, ID: req.ID, Result: result}
		case protocol.MethodRemoveImage:
			var p protocol.ImageParams
			json.Unmarshal(req.Params, &p)
			removed <- p.ID
			if p.ID == "sha256:0000000000000000" {
				return &protocol.Message{Type: protocol.TypeResponse, ID: req.ID, Error: "image is used by 1 container(s)", Code: 409}
			}
			return &protocol.Message{Type: protocol.TypeResponse, ID: req.ID}
		}
		return &protocol.Message{Type: protocol.TypeResponse, ID: req.ID, Error: "unexpected"}
	})

	status, body := e.do("GET", "/api/v1/hosts/local/images", nil)
	var images []protocol.Image
	json.Unmarshal(body["images"], &images)
	if status != 200 || len(images) != 1 || images[0].Tags[0] != "redis:7" {
		t.Fatalf("list: %d %v", status, body)
	}
	if status, _ := e.do("DELETE", "/api/v1/hosts/local/images/sha256:abc123abc123", nil); status != 204 {
		t.Errorf("remove: %d", status)
	}
	if got := <-removed; got != "sha256:abc123abc123" {
		t.Errorf("agent got %q", got)
	}
	if status, _ := e.do("DELETE", "/api/v1/hosts/local/images/sha256:0000000000000000", nil); status != 409 {
		t.Errorf("in use: %d, want 409", status)
	}
	<-removed
	// Tags are never accepted: images are removed by ID only.
	if status, _ := e.do("DELETE", "/api/v1/hosts/local/images/redis:7", nil); status != 400 {
		t.Errorf("by tag: %d, want 400", status)
	}
}

func TestDiskUsageAndPrune(t *testing.T) {
	e := newEnv(t)
	kinds := make(chan string, 4)
	e.fakeAgent(func(req protocol.Message) *protocol.Message {
		var result any
		switch req.Method {
		case protocol.MethodDiskUsage:
			result = protocol.DiskUsage{BuildCache: protocol.DiskUsageItem{Count: 3, Size: 900, Reclaimable: 700}}
		case protocol.MethodPrune:
			var p protocol.PruneParams
			json.Unmarshal(req.Params, &p)
			kinds <- p.Kind
			result = protocol.PruneResult{Deleted: 3, SpaceReclaimed: 700}
		}
		raw, _ := json.Marshal(result)
		return &protocol.Message{Type: protocol.TypeResponse, ID: req.ID, Result: raw}
	})

	status, body := e.do("GET", "/api/v1/hosts/local/disk", nil)
	var cache protocol.DiskUsageItem
	json.Unmarshal(body["build_cache"], &cache)
	if status != 200 || cache.Reclaimable != 700 {
		t.Fatalf("disk: %d %v", status, body)
	}
	for _, kind := range []string{protocol.PruneBuildCache, protocol.PruneDanglingImages} {
		status, body := e.do("POST", "/api/v1/hosts/local/prune/"+kind, nil)
		if status != 200 || string(body["space_reclaimed"]) != "700" {
			t.Errorf("prune %s: %d %v", kind, status, body)
		}
		if got := <-kinds; got != kind {
			t.Errorf("agent got kind %q", got)
		}
	}
	// Risky prunes do not exist, and never reach the agent.
	for _, kind := range []string{"volumes", "containers", "system"} {
		if status, _ := e.do("POST", "/api/v1/hosts/local/prune/"+kind, nil); status != 404 {
			t.Errorf("prune %s: %d, want 404", kind, status)
		}
	}
	select {
	case k := <-kinds:
		t.Errorf("agent asked to prune %q", k)
	default:
	}
}

func TestHistoryAndVolumes(t *testing.T) {
	e := newEnv(t)
	removed := make(chan string, 2)
	e.fakeAgent(func(req protocol.Message) *protocol.Message {
		var result any
		switch req.Method {
		case protocol.MethodHostHistory:
			result = protocol.HostHistory{StepSeconds: 30, Points: []protocol.HistoryPoint{{T: 1, CPU: 12.5, Mem: 40}}}
		case protocol.MethodListVolumes:
			result = []protocol.Volume{{Name: "demo_db", Size: 1000, Containers: 1}}
		case protocol.MethodRemoveVolume:
			var p protocol.VolumeParams
			json.Unmarshal(req.Params, &p)
			removed <- p.Name
			if p.Name == "demo_db" {
				return &protocol.Message{Type: protocol.TypeResponse, ID: req.ID, Error: "volume is used by 1 container(s)", Code: 409}
			}
		}
		raw, _ := json.Marshal(result)
		return &protocol.Message{Type: protocol.TypeResponse, ID: req.ID, Result: raw}
	})

	status, body := e.do("GET", "/api/v1/hosts/local/history", nil)
	var points []protocol.HistoryPoint
	json.Unmarshal(body["points"], &points)
	if status != 200 || len(points) != 1 || points[0].CPU != 12.5 || string(body["step_seconds"]) != "30" {
		t.Fatalf("history: %d %v", status, body)
	}
	status, body = e.do("GET", "/api/v1/hosts/local/volumes", nil)
	var vols []protocol.Volume
	json.Unmarshal(body["volumes"], &vols)
	if status != 200 || len(vols) != 1 || vols[0].Name != "demo_db" {
		t.Fatalf("volumes: %d %v", status, body)
	}
	if status, _ := e.do("DELETE", "/api/v1/hosts/local/volumes/demo_db", nil); status != 409 {
		t.Errorf("in-use volume: %d, want 409", status)
	}
	<-removed
	if status, _ := e.do("DELETE", "/api/v1/hosts/local/volumes/old-data", nil); status != 204 {
		t.Errorf("remove: %d", status)
	}
	if got := <-removed; got != "old-data" {
		t.Errorf("agent got %q", got)
	}
	if status, _ := e.do("DELETE", "/api/v1/hosts/local/volumes/..%2Fetc", nil); status != 400 {
		t.Errorf("bad name: %d, want 400", status)
	}
}
