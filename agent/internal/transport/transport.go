// Package transport keeps the agent's outbound WebSocket connection to the
// controller alive and dispatches incoming requests to a handler.
package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"dockpit/agent/protocol"
)

const (
	dialTimeout  = 10 * time.Second
	writeTimeout = 10 * time.Second
	// Well below nginx's default proxy_read_timeout of 60s.
	pingInterval = 20 * time.Second
	maxBackoff   = 30 * time.Second
)

// Request is one request from the controller.
type Request struct {
	Method string
	Params json.RawMessage
	// Emit sends one chunk of a streaming result. Unary handlers ignore it.
	Emit func(ctx context.Context, chunk any) error
}

// Handler answers one request. The context is cancelled when the controller
// cancels the request or the connection drops; handlers set their own
// timeouts. An error implementing StatusCode() int passes that code on.
type Handler func(ctx context.Context, req Request) (any, error)

// Config configures Run.
type Config struct {
	URL   string // connect URL, from ConnectURL
	Token string
	// TLS overrides the TLS settings, e.g. to trust a private CA. Optional.
	TLS    *tls.Config
	Hello  func(ctx context.Context) protocol.HostInfo
	Handle Handler
	Logger *slog.Logger
}

// ConnectURL turns the controller base URL into the agent connect URL.
//
// http/https are mapped to ws/wss. Unencrypted schemes are only accepted for
// loopback hosts so a token is never sent in clear text over the network.
func ConnectURL(base string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("invalid controller URL: %w", err)
	}
	switch u.Scheme {
	case "https", "wss":
		u.Scheme = "wss"
	case "http", "ws":
		if !isLoopback(u.Hostname()) {
			return "", fmt.Errorf("controller URL %q is not encrypted: use https:// (plain http is only allowed for localhost)", base)
		}
		u.Scheme = "ws"
	default:
		return "", fmt.Errorf("controller URL %q must start with https://", base)
	}
	if u.Host == "" {
		return "", fmt.Errorf("controller URL %q has no host", base)
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + protocol.ConnectPath
	u.RawQuery, u.Fragment = "", ""
	return u.String(), nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// TLSConfigWithCA returns a TLS config that trusts the system roots plus the
// PEM certificates in caFile, for controllers with a private or self-signed
// certificate. Verification stays on.
func TLSConfigWithCA(caFile string) (*tls.Config, error) {
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("%s: no PEM certificates found", caFile)
	}
	return &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}, nil
}

// ErrUnauthorized is returned when the controller rejects the token.
var ErrUnauthorized = errors.New("controller rejected the agent token")

// Run connects to the controller and serves requests until ctx is done,
// reconnecting with exponential backoff.
func Run(ctx context.Context, cfg Config) error {
	backoff := time.Second
	for {
		connected, err := runOnce(ctx, cfg)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if connected {
			backoff = time.Second
		}
		cfg.Logger.Warn("controller connection lost", "err", err, "retry_in", backoff.String())
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, maxBackoff)
	}
}

func runOnce(ctx context.Context, cfg Config) (connected bool, err error) {
	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	opts := &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Bearer " + cfg.Token}},
	}
	if cfg.TLS != nil {
		opts.HTTPClient = &http.Client{Transport: &http.Transport{TLSClientConfig: cfg.TLS, Proxy: http.ProxyFromEnvironment}}
	}
	conn, resp, err := websocket.Dial(dialCtx, cfg.URL, opts)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			return false, ErrUnauthorized
		}
		return false, err
	}
	defer conn.CloseNow()

	ctx, cancel = context.WithCancel(ctx)
	defer cancel()

	s := &session{conn: conn, ctx: ctx, cancels: make(map[string]context.CancelFunc)}
	info := cfg.Hello(ctx)
	if err := s.send(ctx, protocol.Message{Type: protocol.TypeHello, Hello: &info}); err != nil {
		return false, fmt.Errorf("send hello: %w", err)
	}
	cfg.Logger.Info("connected to controller", "url", cfg.URL)

	go s.keepAlive(ctx)

	for {
		var msg protocol.Message
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			return true, err
		}
		switch msg.Type {
		case protocol.TypeRequest:
			reqCtx, cancel := context.WithCancel(ctx)
			s.track(msg.ID, cancel)
			go s.serve(reqCtx, cfg, msg)
		case protocol.TypeCancel:
			s.cancel(msg.ID)
		default:
			cfg.Logger.Debug("ignoring message", "type", msg.Type)
		}
	}
}

type session struct {
	conn *websocket.Conn
	ctx  context.Context // lives as long as the connection
	mu   sync.Mutex      // serialises writes so messages never interleave

	cancelMu sync.Mutex
	cancels  map[string]context.CancelFunc
}

func (s *session) track(id string, cancel context.CancelFunc) {
	s.cancelMu.Lock()
	defer s.cancelMu.Unlock()
	s.cancels[id] = cancel
}

func (s *session) cancel(id string) {
	s.cancelMu.Lock()
	defer s.cancelMu.Unlock()
	if cancel, ok := s.cancels[id]; ok {
		cancel()
		delete(s.cancels, id)
	}
}

func (s *session) send(ctx context.Context, msg protocol.Message) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	return wsjson.Write(ctx, s.conn, msg)
}

func (s *session) serve(ctx context.Context, cfg Config, req protocol.Message) {
	defer s.cancel(req.ID)

	emit := func(ctx context.Context, chunk any) error {
		raw, err := json.Marshal(chunk)
		if err != nil {
			return err
		}
		return s.send(ctx, protocol.Message{Type: protocol.TypeStream, ID: req.ID, Result: raw})
	}
	result, err := cfg.Handle(ctx, Request{Method: req.Method, Params: req.Params, Emit: emit})

	resp := protocol.Message{Type: protocol.TypeResponse, ID: req.ID}
	if err == nil && result != nil {
		resp.Result, err = json.Marshal(result)
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return // the controller no longer wants an answer
		}
		resp.Error = err.Error()
		var coded interface{ StatusCode() int }
		if errors.As(err, &coded) {
			resp.Code = coded.StatusCode()
		}
		cfg.Logger.Warn("request failed", "method", req.Method, "err", err)
	}
	// Use the connection context: ctx may already be cancelled.
	if err := s.send(s.ctx, resp); err != nil {
		cfg.Logger.Warn("send response", "method", req.Method, "err", err)
	}
}

// keepAlive pings the controller so idle proxies do not drop the connection
// and dead connections are noticed quickly.
func (s *session) keepAlive(ctx context.Context) {
	t := time.NewTicker(pingInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pingCtx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := s.conn.Ping(pingCtx)
			cancel()
			if err != nil {
				s.conn.CloseNow()
				return
			}
		}
	}
}
