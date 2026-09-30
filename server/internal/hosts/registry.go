// Package hosts tracks connected agents and sends requests to them.
package hosts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"dockpit/agent/protocol"
)

const (
	helloTimeout = 10 * time.Second
	writeTimeout = 10 * time.Second
)

// ErrOffline is returned when the host has no live agent connection.
var ErrOffline = errors.New("host is offline")

// AgentError is an error reported by the agent itself, e.g. a Docker failure.
// Code is set for errors the caller caused (4xx), such as an unknown container.
type AgentError struct {
	Msg  string
	Code int
}

func (e *AgentError) Error() string { return e.Msg }

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// ValidID reports whether id is a well-formed host ID.
func ValidID(id string) bool { return idPattern.MatchString(id) }

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Slug derives a host ID from a display name, e.g. "Mac mini" -> "mac-mini".
func Slug(name string) string {
	s := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(s) > 40 {
		s = strings.TrimRight(s[:40], "-")
	}
	if s == "" {
		return "host"
	}
	return s
}

// Host is a snapshot of a connected host.
type Host struct {
	ID          string            `json:"id"`
	Info        protocol.HostInfo `json:"info"`
	ConnectedAt time.Time         `json:"connected_at"`
}

// Registry holds the live agent connections, keyed by host ID.
type Registry struct {
	mu     sync.RWMutex
	agents map[string]*Agent
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{agents: make(map[string]*Agent)}
}

// Get returns the live agent for a host.
func (r *Registry) Get(id string) (*Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.agents[id]
	if !ok {
		return nil, ErrOffline
	}
	return a, nil
}

// List returns all connected hosts sorted by ID.
func (r *Registry) List() []Host {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Host, 0, len(r.agents))
	for id, a := range r.agents {
		out = append(out, Host{ID: id, Info: a.info, ConnectedAt: a.connectedAt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Disconnect closes the live connection of a host, if any.
func (r *Registry) Disconnect(id string) {
	r.mu.Lock()
	a := r.agents[id]
	delete(r.agents, id)
	r.mu.Unlock()
	if a != nil {
		a.conn.Close(websocket.StatusPolicyViolation, "host removed or token rotated")
	}
}

// Serve runs an authenticated agent connection for hostID until it closes.
// A newer connection for the same host replaces the old one. onHello is
// called with the agent's hello before the host is marked online.
func (r *Registry) Serve(ctx context.Context, hostID string, conn *websocket.Conn, onHello func(protocol.HostInfo)) error {
	helloCtx, cancel := context.WithTimeout(ctx, helloTimeout)
	var hello protocol.Message
	err := wsjson.Read(helloCtx, conn, &hello)
	cancel()
	if err != nil {
		return fmt.Errorf("read hello: %w", err)
	}
	if hello.Type != protocol.TypeHello || hello.Hello == nil {
		conn.Close(websocket.StatusPolicyViolation, "expected hello")
		return errors.New("first message was not a hello")
	}

	if onHello != nil {
		onHello(*hello.Hello)
	}

	a := &Agent{
		info:        *hello.Hello,
		connectedAt: time.Now(),
		conn:        conn,
		pending:     make(map[string]*call),
		done:        make(chan struct{}),
	}

	r.mu.Lock()
	old := r.agents[hostID]
	r.agents[hostID] = a
	r.mu.Unlock()
	if old != nil {
		old.conn.Close(websocket.StatusGoingAway, "replaced by a newer connection")
	}

	err = a.readLoop(ctx)

	r.mu.Lock()
	if r.agents[hostID] == a {
		delete(r.agents, hostID)
	}
	r.mu.Unlock()
	a.shutdown()
	return err
}

// ErrSlowConsumer ends a stream whose reader cannot keep up.
var ErrSlowConsumer = errors.New("stream reader too slow")

// streamBuffer is how many chunks a stream may queue before it is dropped.
// Dropping one slow stream keeps it from stalling the whole agent connection.
const streamBuffer = 64

// Agent is one live agent connection.
type Agent struct {
	info        protocol.HostInfo
	connectedAt time.Time
	conn        *websocket.Conn

	writeMu sync.Mutex
	nextID  atomic.Uint64

	mu      sync.Mutex
	pending map[string]*call
	closed  bool
	done    chan struct{}
}

// call is one in-flight request.
type call struct {
	ch       chan protocol.Message
	overflow chan struct{} // closed when ch was full
	once     sync.Once
}

// start registers a request and sends it.
func (a *Agent) start(ctx context.Context, method string, params any, buffer int) (string, *call, error) {
	rawParams, err := json.Marshal(params)
	if err != nil {
		return "", nil, err
	}
	id := strconv.FormatUint(a.nextID.Add(1), 10)
	c := &call{ch: make(chan protocol.Message, buffer), overflow: make(chan struct{})}

	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return "", nil, ErrOffline
	}
	a.pending[id] = c
	a.mu.Unlock()

	if err := a.write(ctx, protocol.Message{Type: protocol.TypeRequest, ID: id, Method: method, Params: rawParams}); err != nil {
		a.finish(id)
		return "", nil, fmt.Errorf("%w: %v", ErrOffline, err)
	}
	return id, c, nil
}

func (a *Agent) finish(id string) {
	a.mu.Lock()
	delete(a.pending, id)
	a.mu.Unlock()
}

func responseError(m protocol.Message) error {
	if m.Error != "" {
		return &AgentError{Msg: m.Error, Code: m.Code}
	}
	return nil
}

// Call sends a request to the agent and decodes the result into result.
func (a *Agent) Call(ctx context.Context, method string, params, result any) error {
	id, c, err := a.start(ctx, method, params, 1)
	if err != nil {
		return err
	}
	defer a.finish(id)

	select {
	case <-ctx.Done():
		a.cancel(id)
		return ctx.Err()
	case <-a.done:
		return ErrOffline
	case resp := <-c.ch:
		if err := responseError(resp); err != nil {
			return err
		}
		if result == nil {
			return nil
		}
		if err := json.Unmarshal(resp.Result, result); err != nil {
			return fmt.Errorf("decode agent result: %w", err)
		}
		return nil
	}
}

// Stream sends a streaming request and calls onChunk for every chunk until
// the agent ends the stream, ctx is cancelled or onChunk fails. The agent is
// told to stop whenever the caller gives up early.
func (a *Agent) Stream(ctx context.Context, method string, params any, onChunk func(json.RawMessage) error) error {
	id, c, err := a.start(ctx, method, params, streamBuffer)
	if err != nil {
		return err
	}
	defer a.finish(id)

	for {
		select {
		case <-ctx.Done():
			a.cancel(id)
			return ctx.Err()
		case <-a.done:
			return ErrOffline
		case <-c.overflow:
			a.cancel(id)
			return ErrSlowConsumer
		case m := <-c.ch:
			if m.Type == protocol.TypeResponse {
				return responseError(m)
			}
			if err := onChunk(m.Result); err != nil {
				a.cancel(id)
				return err
			}
		}
	}
}

// cancel tells the agent to stop working on request id. Best effort.
func (a *Agent) cancel(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	defer cancel()
	a.write(ctx, protocol.Message{Type: protocol.TypeCancel, ID: id})
}

func (a *Agent) write(ctx context.Context, msg protocol.Message) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	return wsjson.Write(ctx, a.conn, msg)
}

func (a *Agent) readLoop(ctx context.Context) error {
	for {
		var msg protocol.Message
		if err := wsjson.Read(ctx, a.conn, &msg); err != nil {
			return err
		}
		if msg.Type != protocol.TypeResponse && msg.Type != protocol.TypeStream {
			continue
		}
		a.mu.Lock()
		c := a.pending[msg.ID]
		a.mu.Unlock()
		if c == nil {
			continue // cancelled or unknown request
		}
		select {
		case c.ch <- msg:
		default:
			// Never block the read loop: other requests share it.
			c.once.Do(func() { close(c.overflow) })
		}
	}
}

func (a *Agent) shutdown() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.closed {
		a.closed = true
		close(a.done)
	}
	a.conn.CloseNow()
}
