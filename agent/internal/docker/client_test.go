package docker

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeDocker serves handler on a unix socket and returns a resolver for it.
func fakeDocker(t *testing.T, handler http.HandlerFunc) func() (string, error) {
	t.Helper()
	// Short dir: unix socket paths are limited to ~104 bytes on macOS.
	dir, err := os.MkdirTemp("", "dk")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "d.sock")

	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(handler)
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return func() (string, error) { return sock, nil }
}

func TestListContainers(t *testing.T) {
	var gotQuery string
	sock := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/containers/json" {
			http.NotFound(w, r)
			return
		}
		gotQuery = r.URL.RawQuery
		w.Write([]byte(`[{
			"Id": "abc123",
			"Names": ["/web"],
			"Image": "nginx:latest",
			"State": "running",
			"Status": "Up 2 hours",
			"Created": 1700000000,
			"Ports": [
				{"IP": "0.0.0.0", "PrivatePort": 80, "PublicPort": 8080, "Type": "tcp"},
				{"IP": "::", "PrivatePort": 80, "PublicPort": 8080, "Type": "tcp"}
			],
			"Labels": {"com.docker.compose.project": "demo"}
		}]`))
	})

	got, err := New(sock).ListContainers(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if gotQuery != "all=1" {
		t.Errorf("query = %q, want all=1", gotQuery)
	}
	if len(got) != 1 {
		t.Fatalf("got %d containers, want 1", len(got))
	}
	c := got[0]
	if c.Name != "web" || c.ID != "abc123" || c.State != "running" || c.ComposeProject != "demo" {
		t.Errorf("unexpected container: %+v", c)
	}
	if len(c.Ports) != 1 || c.Ports[0].PublicPort != 8080 {
		t.Errorf("ports not deduplicated: %+v", c.Ports)
	}
}

func TestListContainersRunningOnly(t *testing.T) {
	var gotQuery string
	sock := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Write([]byte(`[]`))
	})
	got, err := New(sock).ListContainers(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if gotQuery != "" {
		t.Errorf("query = %q, want empty", gotQuery)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("want empty non-nil slice, got %#v", got)
	}
}

func TestAPIError(t *testing.T) {
	sock := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message": "engine exploded"}`))
	})
	_, err := New(sock).ListContainers(context.Background(), true)
	if err == nil || !strings.Contains(err.Error(), "engine exploded") {
		t.Fatalf("err = %v, want docker message", err)
	}
}

func TestNoSocket(t *testing.T) {
	c := New(func() (string, error) { return "", ErrNoSocket })
	if _, err := c.ListContainers(context.Background(), true); !errors.Is(err, ErrNoSocket) {
		t.Fatalf("err = %v, want ErrNoSocket", err)
	}
}

func TestResolveSocketDockerHost(t *testing.T) {
	t.Setenv("DOCKER_HOST", "unix:///tmp/custom.sock")
	got, err := ResolveSocket()
	if err != nil || got != "/tmp/custom.sock" {
		t.Fatalf("got %q, %v", got, err)
	}

	for _, bad := range []string{"tcp://0.0.0.0:2375", "tcp://127.0.0.1:2376", "unix://"} {
		t.Setenv("DOCKER_HOST", bad)
		if _, err := ResolveSocket(); err == nil {
			t.Errorf("DOCKER_HOST=%q: expected error", bad)
		}
	}
}

func TestContainerActions(t *testing.T) {
	type call struct{ method, path string }
	var got []call
	sock := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		got = append(got, call{r.Method, r.URL.RequestURI()})
		switch {
		case strings.HasSuffix(r.URL.Path, "/start"):
			w.WriteHeader(http.StatusNotModified) // already running
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})
	c := New(sock)
	ctx := context.Background()
	for _, err := range []error{
		c.StartContainer(ctx, "web"),
		c.StopContainer(ctx, "web"),
		c.RestartContainer(ctx, "web"),
		c.RemoveContainer(ctx, "web", false),
		c.RemoveContainer(ctx, "web", true),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	want := []call{
		{"POST", "/containers/web/start"},
		{"POST", "/containers/web/stop?t=10"},
		{"POST", "/containers/web/restart?t=10"},
		{"DELETE", "/containers/web"},
		{"DELETE", "/containers/web?force=1"},
	}
	if len(got) != len(want) {
		t.Fatalf("calls = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("call %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestAPIErrorStatus(t *testing.T) {
	sock := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"message": "container is running"}`))
	})
	err := New(sock).RemoveContainer(context.Background(), "web", false)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode() != http.StatusConflict || apiErr.Message != "container is running" {
		t.Fatalf("err = %#v", err)
	}
}

func TestContainerStats(t *testing.T) {
	sock := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/containers/web/stats" || r.URL.Query().Get("one-shot") != "true" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{
			"cpu_stats": {"cpu_usage": {"total_usage": 2000}, "system_cpu_usage": 100000, "online_cpus": 4},
			"memory_stats": {"usage": 1000, "limit": 8000, "stats": {"inactive_file": 300}}
		}`))
	})
	st, err := New(sock).ContainerStats(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	if st.MemUsage != 700 || st.MemLimit != 8000 || st.OnlineCPUs != 4 {
		t.Errorf("stats = %+v", st)
	}

	prev := Stats{CPUTotal: 1000, SystemCPU: 90000}
	pct, ok := CPUPercent(prev, st)
	// (2000-1000)/(100000-90000) * 4 cores * 100 = 40%
	if !ok || pct < 39.99 || pct > 40.01 {
		t.Errorf("CPUPercent = %v, %v", pct, ok)
	}
	if _, ok := CPUPercent(st, prev); ok {
		t.Error("CPUPercent accepted counters going backwards")
	}
}

func TestListAndRemoveImages(t *testing.T) {
	var deleted []string
	sock := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/images/json":
			w.Write([]byte(`[
				{"Id": "sha256:aaa111aaa111", "RepoTags": ["redis:7", "cache:latest"], "Created": 1, "Size": 100},
				{"Id": "sha256:bbb222bbb222", "RepoTags": ["<none>:<none>"], "Created": 2, "Size": 50}
			]`))
		case r.URL.Path == "/containers/json":
			w.Write([]byte(`[
				{"Id": "c1", "ImageID": "sha256:aaa111aaa111", "State": "running"},
				{"Id": "c2", "ImageID": "sha256:aaa111aaa111", "State": "exited"}
			]`))
		case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/images/"):
			deleted = append(deleted, r.URL.RequestURI())
			w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	})
	c := New(sock)
	imgs, err := c.ListImages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(imgs) != 2 || imgs[0].Containers != 2 || imgs[0].Running != 1 || len(imgs[0].Tags) != 2 {
		t.Errorf("image 0 = %+v", imgs[0])
	}
	if len(imgs[1].Tags) != 0 || imgs[1].Containers != 0 {
		t.Errorf("dangling image = %+v", imgs[1])
	}

	// In use (also by a stopped container): refused, nothing deleted.
	err = c.RemoveImage(context.Background(), "sha256:aaa111aaa111")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode() != http.StatusConflict {
		t.Errorf("in-use image: err = %v", err)
	}
	if err := c.RemoveImage(context.Background(), "bbb222bbb222"); err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0] != "/images/bbb222bbb222?force=1" {
		t.Errorf("deletes = %v", deleted)
	}
}
