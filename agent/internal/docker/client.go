// Package docker is a minimal client for the local Docker Engine API over a
// unix socket. It implements only the endpoints the agent needs.
package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"dockpit/agent/protocol"
)

const (
	composeProjectLabel = "com.docker.compose.project"
	composeServiceLabel = "com.docker.compose.service"
	composeDirLabel     = "com.docker.compose.project.working_dir"
)

// Client talks to the Docker Engine API on a unix socket.
type Client struct {
	http *http.Client
}

// New returns a client that connects to the socket returned by socketPath.
// The path is resolved on every new connection, so the agent keeps working
// when Docker starts after the agent or its socket moves.
func New(socketPath func() (string, error)) *Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			path, err := socketPath()
			if err != nil {
				return nil, err
			}
			var d net.Dialer
			return d.DialContext(ctx, "unix", path)
		},
	}
	return &Client{http: &http.Client{Transport: transport}}
}

// Version returns the Docker Engine version string.
func (c *Client) Version(ctx context.Context) (string, error) {
	var v struct {
		Version string `json:"Version"`
	}
	if err := c.get(ctx, "/version", &v); err != nil {
		return "", err
	}
	return v.Version, nil
}

type apiContainer struct {
	ID      string   `json:"Id"`
	Names   []string `json:"Names"`
	Image   string   `json:"Image"`
	ImageID string   `json:"ImageID"`
	State   string   `json:"State"`
	Status  string   `json:"Status"`
	Created int64    `json:"Created"`
	Ports   []struct {
		IP          string `json:"IP"`
		PrivatePort uint16 `json:"PrivatePort"`
		PublicPort  uint16 `json:"PublicPort"`
		Type        string `json:"Type"`
	} `json:"Ports"`
	Labels map[string]string `json:"Labels"`
	Mounts []struct {
		Type string `json:"Type"`
		Name string `json:"Name"`
	} `json:"Mounts"`
}

// ListContainers lists containers. When all is false only running containers
// are returned.
func (c *Client) ListContainers(ctx context.Context, all bool) ([]protocol.Container, error) {
	raw, err := c.listContainers(ctx, all)
	if err != nil {
		return nil, err
	}

	out := make([]protocol.Container, 0, len(raw))
	for _, rc := range raw {
		name := ""
		if len(rc.Names) > 0 {
			name = strings.TrimPrefix(rc.Names[0], "/")
		}
		out = append(out, protocol.Container{
			ID:             rc.ID,
			Name:           name,
			Image:          rc.Image,
			ImageID:        rc.ImageID,
			State:          rc.State,
			Status:         rc.Status,
			Created:        rc.Created,
			Ports:          dedupePorts(rc),
			ComposeProject: rc.Labels[composeProjectLabel],
			ComposeService: rc.Labels[composeServiceLabel],
			ComposeDir:     rc.Labels[composeDirLabel],
		})
	}
	return out, nil
}

func (c *Client) listContainers(ctx context.Context, all bool) ([]apiContainer, error) {
	q := url.Values{}
	if all {
		q.Set("all", "1")
	}
	var raw []apiContainer
	err := c.get(ctx, "/containers/json?"+q.Encode(), &raw)
	return raw, err
}

// dedupePorts drops the duplicate entries Docker reports when a port is
// published on both IPv4 and IPv6.
func dedupePorts(rc apiContainer) []protocol.Port {
	type key struct {
		priv, pub uint16
		typ       string
	}
	seen := make(map[key]bool)
	ports := make([]protocol.Port, 0, len(rc.Ports))
	for _, p := range rc.Ports {
		k := key{p.PrivatePort, p.PublicPort, p.Type}
		if seen[k] {
			continue
		}
		seen[k] = true
		ports = append(ports, protocol.Port{IP: p.IP, PrivatePort: p.PrivatePort, PublicPort: p.PublicPort, Type: p.Type})
	}
	return ports
}

// StartContainer starts a container. Starting a running container is a no-op.
func (c *Client) StartContainer(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, containerPath(id, "/start"), nil)
}

// StopContainer stops a container, killing it after stopTimeout seconds.
func (c *Client) StopContainer(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, containerPath(id, "/stop")+stopTimeoutQuery, nil)
}

// RestartContainer restarts a container.
func (c *Client) RestartContainer(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, containerPath(id, "/restart")+stopTimeoutQuery, nil)
}

// RemoveContainer removes a container. Volumes are never removed. Without
// force Docker refuses to remove a running container (409).
func (c *Client) RemoveContainer(ctx context.Context, id string, force bool) error {
	path := containerPath(id, "")
	if force {
		path += "?force=1"
	}
	return c.do(ctx, http.MethodDelete, path, nil)
}

// stopTimeoutQuery gives containers 10 seconds to exit before SIGKILL.
const stopTimeoutQuery = "?t=10"

func containerPath(id, suffix string) string {
	return "/containers/" + url.PathEscape(id) + suffix
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, out)
}

func (c *Client) do(ctx context.Context, method, path string, out any) error {
	// The host part is ignored; the transport always dials the unix socket.
	req, err := http.NewRequestWithContext(ctx, method, "http://docker"+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("docker %s: %w", path, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
	case http.StatusNotModified:
		// Already started/stopped: the desired state holds.
		return nil
	default:
		return apiError(resp)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("docker %s: decode response: %w", path, err)
	}
	return nil
}

// APIError is an error response from the Docker Engine.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return "docker: " + e.Message }

// StatusCode reports the Docker status for client errors (4xx) so they can be
// passed through to the dashboard, and 0 otherwise.
func (e *APIError) StatusCode() int {
	if e.Status >= 400 && e.Status < 500 {
		return e.Status
	}
	return 0
}

func apiError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var e struct {
		Message string `json:"message"`
	}
	msg := strings.TrimSpace(string(body))
	if json.Unmarshal(body, &e) == nil && e.Message != "" {
		msg = e.Message
	}
	return &APIError{Status: resp.StatusCode, Message: msg}
}
