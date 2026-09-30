package docker

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"

	"dockpit/agent/protocol"
)

type apiInspect struct {
	ID           string `json:"Id"`
	Name         string `json:"Name"`
	RestartCount int    `json:"RestartCount"`
	State        struct {
		Status     string `json:"Status"`
		ExitCode   int    `json:"ExitCode"`
		OOMKilled  bool   `json:"OOMKilled"`
		Error      string `json:"Error"`
		StartedAt  string `json:"StartedAt"`
		FinishedAt string `json:"FinishedAt"`
		Health     *struct {
			Status string `json:"Status"`
			Log    []struct {
				Output string `json:"Output"`
			} `json:"Log"`
		} `json:"Health"`
	} `json:"State"`
	Config struct {
		Image      string   `json:"Image"`
		Env        []string `json:"Env"`
		Cmd        []string `json:"Cmd"`
		Entrypoint []string `json:"Entrypoint"`
	} `json:"Config"`
	HostConfig struct {
		RestartPolicy struct {
			Name              string `json:"Name"`
			MaximumRetryCount int    `json:"MaximumRetryCount"`
		} `json:"RestartPolicy"`
		LogConfig struct {
			Type string `json:"Type"`
		} `json:"LogConfig"`
		Memory   int64 `json:"Memory"`
		NanoCpus int64 `json:"NanoCpus"`
	} `json:"HostConfig"`
	Mounts []struct {
		Type        string `json:"Type"`
		Name        string `json:"Name"`
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
		RW          bool   `json:"RW"`
	} `json:"Mounts"`
	NetworkSettings struct {
		Networks map[string]struct {
			IPAddress string   `json:"IPAddress"`
			Aliases   []string `json:"Aliases"`
		} `json:"Networks"`
	} `json:"NetworkSettings"`
}

func (c *Client) inspect(ctx context.Context, id string) (apiInspect, error) {
	var v apiInspect
	err := c.get(ctx, containerPath(id, "/json"), &v)
	return v, err
}

// InspectContainer returns a read-only view of one container. Environment
// values that look like secrets are masked here, so they never leave the host.
func (c *Client) InspectContainer(ctx context.Context, id string) (protocol.ContainerDetail, error) {
	v, err := c.inspect(ctx, id)
	if err != nil {
		return protocol.ContainerDetail{}, err
	}
	d := protocol.ContainerDetail{
		ID:           v.ID,
		Name:         strings.TrimPrefix(v.Name, "/"),
		Image:        v.Config.Image,
		State:        v.State.Status,
		Command:      strings.Join(append(append([]string{}, v.Config.Entrypoint...), v.Config.Cmd...), " "),
		StartedAt:    zeroTime(v.State.StartedAt),
		FinishedAt:   zeroTime(v.State.FinishedAt),
		ExitCode:     v.State.ExitCode,
		OOMKilled:    v.State.OOMKilled,
		Error:        v.State.Error,
		RestartCount: v.RestartCount,
		LogDriver:    v.HostConfig.LogConfig.Type,
		MemLimit:     v.HostConfig.Memory,
		CPUs:         float64(v.HostConfig.NanoCpus) / 1e9,
		Env:          make([]protocol.EnvVar, 0, len(v.Config.Env)),
		Mounts:       make([]protocol.Mount, 0, len(v.Mounts)),
		Networks:     make([]protocol.ContainerNetwork, 0, len(v.NetworkSettings.Networks)),
	}
	if p := v.HostConfig.RestartPolicy; p.Name != "" {
		d.RestartPolicy = p.Name
		if p.Name == "on-failure" && p.MaximumRetryCount > 0 {
			d.RestartPolicy += ":" + itoa(p.MaximumRetryCount)
		}
	}
	if h := v.State.Health; h != nil {
		d.Health = h.Status
		if n := len(h.Log); n > 0 {
			d.HealthOutput = truncate(strings.TrimSpace(h.Log[n-1].Output), 500)
		}
	}
	for _, kv := range v.Config.Env {
		d.Env = append(d.Env, maskEnv(kv))
	}
	for _, m := range v.Mounts {
		d.Mounts = append(d.Mounts, protocol.Mount{Type: m.Type, Name: m.Name, Source: m.Source, Destination: m.Destination, ReadOnly: !m.RW})
	}
	for name, n := range v.NetworkSettings.Networks {
		d.Networks = append(d.Networks, protocol.ContainerNetwork{Name: name, IP: n.IPAddress, Aliases: n.Aliases})
	}
	return d, nil
}

func zeroTime(s string) string {
	if strings.HasPrefix(s, "0001-") {
		return ""
	}
	return s
}

func itoa(n int) string {
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

var (
	secretKey = regexp.MustCompile(`(?i)(pass(word|wd)?|secret|token|api[_-]?key|private|credential|auth|cookie|session|salt|signature|dsn|\bkey\b|_key$|^key_)`)
	urlCreds  = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://[^:/@\s]+):[^@\s]+@`)
)

const maskedValue = "••••••••"

// maskEnv splits KEY=VALUE and hides values that are probably secrets: keys
// with words like PASSWORD or TOKEN, and passwords inside URLs.
func maskEnv(kv string) protocol.EnvVar {
	key, val, _ := strings.Cut(kv, "=")
	if secretKey.MatchString(key) && val != "" {
		return protocol.EnvVar{Key: key, Value: maskedValue, Masked: true}
	}
	if urlCreds.MatchString(val) {
		return protocol.EnvVar{Key: key, Value: urlCreds.ReplaceAllString(val, "${1}:"+maskedValue+"@"), Masked: true}
	}
	return protocol.EnvVar{Key: key, Value: truncate(val, 500)}
}

// extra is the per-container data ListContainers cannot get from the list
// endpoint. It is cached because it needs one inspect call per container.
type extra struct {
	restartCount int
	exitCode     int
	oom          bool
	logDriver    string
	at           time.Time
	status       string
}

type extraCache struct {
	mu sync.Mutex
	m  map[string]extra
}

const extraTTL = 20 * time.Second

// enrich fills restart count, exit code, OOM flag and log driver. Failures are
// ignored: these fields are informational.
func (c *Client) enrich(ctx context.Context, list []apiContainer, out []protocol.Container) {
	c.extras.mu.Lock()
	if c.extras.m == nil {
		c.extras.m = map[string]extra{}
	}
	live := map[string]bool{}
	var todo []int
	for i, rc := range list {
		live[rc.ID] = true
		e, ok := c.extras.m[rc.ID]
		if !ok || e.status != rc.Status || time.Since(e.at) > extraTTL {
			todo = append(todo, i)
		}
	}
	for id := range c.extras.m {
		if !live[id] {
			delete(c.extras.m, id)
		}
	}
	c.extras.mu.Unlock()

	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for _, i := range todo {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			v, err := c.inspect(ctx, list[i].ID)
			if err != nil {
				return
			}
			c.extras.mu.Lock()
			c.extras.m[list[i].ID] = extra{
				restartCount: v.RestartCount, exitCode: v.State.ExitCode, oom: v.State.OOMKilled,
				logDriver: v.HostConfig.LogConfig.Type, at: time.Now(), status: list[i].Status,
			}
			c.extras.mu.Unlock()
		}()
	}
	wg.Wait()

	c.extras.mu.Lock()
	defer c.extras.mu.Unlock()
	for i, rc := range list {
		e, ok := c.extras.m[rc.ID]
		if !ok {
			continue
		}
		out[i].RestartCount = e.restartCount
		out[i].OOMKilled = e.oom
		out[i].LogDriver = e.logDriver
		if rc.State != "running" && rc.State != "restarting" && rc.State != "created" {
			code := e.exitCode
			out[i].ExitCode = &code
		}
	}
}

// healthFromStatus reads the health suffix Docker adds to the status text,
// e.g. "Up 3 hours (unhealthy)" or "Up 5 seconds (health: starting)".
func healthFromStatus(status string) string {
	switch {
	case strings.Contains(status, "(unhealthy)"):
		return "unhealthy"
	case strings.Contains(status, "(healthy)"):
		return "healthy"
	case strings.Contains(status, "(health: starting)"):
		return "starting"
	}
	return ""
}
