package transport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"dockpit/agent/protocol"
)

func TestConnectURL(t *testing.T) {
	ok := map[string]string{
		"https://cockpit.example.com":  "wss://cockpit.example.com/agent/connect",
		"https://cockpit.example.com/": "wss://cockpit.example.com/agent/connect",
		"https://example.com/cockpit":  "wss://example.com/cockpit/agent/connect",
		"wss://cockpit.example.com":    "wss://cockpit.example.com/agent/connect",
		"http://localhost:8080":        "ws://localhost:8080/agent/connect",
		"http://127.0.0.1:8080":        "ws://127.0.0.1:8080/agent/connect",
		"http://[::1]:8080":            "ws://[::1]:8080/agent/connect",
	}
	for in, want := range ok {
		got, err := ConnectURL(in)
		if err != nil || got != want {
			t.Errorf("ConnectURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}

	for _, bad := range []string{
		"http://cockpit.example.com", // clear text over the network
		"ws://192.168.1.10:8080",
		"ftp://example.com",
		"cockpit.example.com",
		"https://",
	} {
		if got, err := ConnectURL(bad); err == nil {
			t.Errorf("ConnectURL(%q) = %q, want error", bad, got)
		}
	}
}

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestRunRoundTrip(t *testing.T) {
	const token = "test-token"
	done := make(chan protocol.Message, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx := r.Context()

		var hello protocol.Message
		if err := wsjson.Read(ctx, conn, &hello); err != nil || hello.Type != protocol.TypeHello || hello.Hello.Name != "test-host" {
			t.Errorf("bad hello: %+v, %v", hello, err)
			return
		}
		params, _ := json.Marshal(protocol.ListContainersParams{All: true})
		wsjson.Write(ctx, conn, protocol.Message{Type: protocol.TypeRequest, ID: "1", Method: protocol.MethodListContainers, Params: params})

		var resp protocol.Message
		if err := wsjson.Read(ctx, conn, &resp); err != nil {
			t.Error(err)
			return
		}
		done <- resp
	}))
	defer srv.Close()

	connectURL, err := ConnectURL(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go Run(ctx, Config{
		URL:    connectURL,
		Token:  token,
		Hello:  func(context.Context) protocol.HostInfo { return protocol.HostInfo{Name: "test-host"} },
		Logger: testLogger(),
		Handle: func(_ context.Context, req Request) (any, error) {
			var p protocol.ListContainersParams
			if err := json.Unmarshal(req.Params, &p); err != nil || !p.All {
				return nil, errors.New("bad params")
			}
			return []protocol.Container{{ID: "abc", Name: "web"}}, nil
		},
	})

	select {
	case resp := <-done:
		if resp.ID != "1" || resp.Error != "" {
			t.Fatalf("unexpected response: %+v", resp)
		}
		var got []protocol.Container
		if err := json.Unmarshal(resp.Result, &got); err != nil || len(got) != 1 || got[0].Name != "web" {
			t.Fatalf("result = %s, %v", resp.Result, err)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for response")
	}
}

func TestRunOnceUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	connectURL, _ := ConnectURL(srv.URL)
	_, err := runOnce(context.Background(), Config{URL: connectURL, Token: "wrong", Logger: testLogger()})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

type codedErr struct{}

func (codedErr) Error() string   { return "no such container" }
func (codedErr) StatusCode() int { return 404 }

// TestStreamCancelAndCode checks streaming chunks, cancellation from the
// controller and error codes, over a real WebSocket.
func TestStreamCancelAndCode(t *testing.T) {
	const token = "test-token"
	type result struct {
		chunks    int
		cancelled bool
		code      int
	}
	done := make(chan result, 1)
	cancelled := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx := r.Context()
		var hello protocol.Message
		wsjson.Read(ctx, conn, &hello)

		var res result
		wsjson.Write(ctx, conn, protocol.Message{Type: protocol.TypeRequest, ID: "s", Method: "stream"})
		for res.chunks < 3 {
			var m protocol.Message
			if err := wsjson.Read(ctx, conn, &m); err != nil || m.Type != protocol.TypeStream {
				t.Errorf("want stream chunk, got %+v %v", m, err)
				return
			}
			res.chunks++
		}
		wsjson.Write(ctx, conn, protocol.Message{Type: protocol.TypeCancel, ID: "s"})
		select {
		case <-cancelled:
			res.cancelled = true
		case <-time.After(2 * time.Second):
		}

		wsjson.Write(ctx, conn, protocol.Message{Type: protocol.TypeRequest, ID: "c", Method: "coded"})
		for {
			var m protocol.Message
			if err := wsjson.Read(ctx, conn, &m); err != nil {
				return
			}
			if m.Type == protocol.TypeResponse && m.ID == "c" {
				res.code = m.Code
				break
			}
		}
		done <- res
	}))
	defer srv.Close()

	connectURL, _ := ConnectURL(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go Run(ctx, Config{
		URL: connectURL, Token: token, Logger: testLogger(),
		Hello: func(context.Context) protocol.HostInfo { return protocol.HostInfo{} },
		Handle: func(ctx context.Context, req Request) (any, error) {
			if req.Method == "coded" {
				return nil, codedErr{}
			}
			for {
				if err := req.Emit(ctx, "line"); err != nil {
					return nil, err
				}
				select {
				case <-ctx.Done():
					close(cancelled)
					return nil, ctx.Err()
				case <-time.After(10 * time.Millisecond):
				}
			}
		},
	})

	select {
	case res := <-done:
		if res.chunks != 3 || !res.cancelled || res.code != 404 {
			t.Fatalf("result = %+v", res)
		}
	case <-ctx.Done():
		t.Fatal("timed out")
	}
}
