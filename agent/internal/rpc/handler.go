// Package rpc maps controller requests to Docker operations.
package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"dockpit/agent/internal/docker"
	"dockpit/agent/internal/metrics"
	"dockpit/agent/internal/netinfo"
	"dockpit/agent/internal/transport"
	"dockpit/agent/protocol"
)

// unaryTimeout bounds non-streaming requests. Stop/restart wait up to 10s for
// the container to exit, so leave room for that. Disk usage and prunes walk
// the whole image/cache store and get longer.
const (
	unaryTimeout  = 30 * time.Second
	systemTimeout = 2 * time.Minute
)

// Handler serves controller requests against the local Docker Engine.
type Handler struct {
	Docker  *docker.Client
	Sampler *metrics.Sampler
}

// BadRequest is an error caused by invalid request parameters.
type BadRequest struct{ Msg string }

func (e *BadRequest) Error() string   { return e.Msg }
func (e *BadRequest) StatusCode() int { return http.StatusBadRequest }

// Handle implements transport.Handler.
func (h *Handler) Handle(ctx context.Context, req transport.Request) (any, error) {
	switch req.Method {
	case protocol.MethodListContainers:
		var p protocol.ListContainersParams
		if err := decode(req.Params, &p); err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(ctx, unaryTimeout)
		defer cancel()
		containers, err := h.Docker.ListContainers(ctx, p.All)
		if err != nil {
			return nil, err
		}
		usage := h.Sampler.ContainerUsage()
		for i := range containers {
			if u, ok := usage[containers[i].ID]; ok && containers[i].State == "running" {
				containers[i].CPUPercent = u.CPUPercent
				containers[i].MemUsage = u.MemUsage
				containers[i].MemLimit = u.MemLimit
			}
		}
		return containers, nil

	case protocol.MethodHostStatus:
		ctx, cancel := context.WithTimeout(ctx, unaryTimeout)
		defer cancel()
		info, err := h.Docker.Info(ctx)
		if err != nil {
			return nil, err
		}
		// Addresses are read on every call: DHCP or a VPN can change them.
		return protocol.HostStatus{Metrics: h.Sampler.HostMetrics(), Docker: info, Addresses: netinfo.Addresses()}, nil

	case protocol.MethodStartContainer, protocol.MethodStopContainer,
		protocol.MethodRestartContainer, protocol.MethodRemoveContainer:
		p, err := containerParams(req.Params)
		if err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(ctx, unaryTimeout)
		defer cancel()
		return nil, h.containerAction(ctx, req.Method, p)

	case protocol.MethodInspectContainer:
		p, err := containerParams(req.Params)
		if err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(ctx, unaryTimeout)
		defer cancel()
		return h.Docker.InspectContainer(ctx, p.ID)

	case protocol.MethodListImages:
		ctx, cancel := context.WithTimeout(ctx, unaryTimeout)
		defer cancel()
		return h.Docker.ListImages(ctx)

	case protocol.MethodRemoveImage:
		var p protocol.ImageParams
		if err := decode(req.Params, &p); err != nil {
			return nil, err
		}
		if !protocol.ValidImageID(p.ID) {
			return nil, &BadRequest{Msg: "invalid image id"}
		}
		ctx, cancel := context.WithTimeout(ctx, unaryTimeout)
		defer cancel()
		return nil, h.Docker.RemoveImage(ctx, p.ID)

	case protocol.MethodHostHistory:
		return h.Sampler.History(), nil

	case protocol.MethodListVolumes:
		ctx, cancel := context.WithTimeout(ctx, systemTimeout)
		defer cancel()
		return h.Docker.ListVolumes(ctx)

	case protocol.MethodRemoveVolume:
		var p protocol.VolumeParams
		if err := decode(req.Params, &p); err != nil {
			return nil, err
		}
		if !protocol.ValidVolumeName(p.Name) {
			return nil, &BadRequest{Msg: "invalid volume name"}
		}
		ctx, cancel := context.WithTimeout(ctx, unaryTimeout)
		defer cancel()
		return nil, h.Docker.RemoveVolume(ctx, p.Name)

	case protocol.MethodDiskUsage:
		ctx, cancel := context.WithTimeout(ctx, systemTimeout)
		defer cancel()
		return h.Docker.DiskUsage(ctx)

	case protocol.MethodPrune:
		var p protocol.PruneParams
		if err := decode(req.Params, &p); err != nil {
			return nil, err
		}
		if p.Kind != protocol.PruneBuildCache && p.Kind != protocol.PruneDanglingImages {
			return nil, &BadRequest{Msg: "unknown prune kind"}
		}
		ctx, cancel := context.WithTimeout(ctx, systemTimeout)
		defer cancel()
		return h.Docker.Prune(ctx, p.Kind)

	case protocol.MethodContainerLogs:
		var p protocol.LogsParams
		if err := decode(req.Params, &p); err != nil {
			return nil, err
		}
		if !protocol.ValidContainerRef(p.ID) {
			return nil, &BadRequest{Msg: "invalid container id"}
		}
		if p.Tail <= 0 {
			p.Tail = protocol.DefaultLogTail
		}
		p.Tail = min(p.Tail, protocol.MaxLogTail)
		return nil, h.streamLogs(ctx, p, req.Emit)

	default:
		return nil, &BadRequest{Msg: fmt.Sprintf("unknown method %q", req.Method)}
	}
}

// Log batching: send up to logBatchLines at once, and never hold a line
// longer than logFlushInterval.
const (
	logBatchLines    = 200
	logFlushInterval = 100 * time.Millisecond
)

func (h *Handler) streamLogs(ctx context.Context, p protocol.LogsParams, emit func(context.Context, any) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	lines := make(chan protocol.LogLine, 1024)
	errc := make(chan error, 1)
	go func() {
		defer close(lines)
		errc <- h.Docker.FollowLogs(ctx, p.ID, p.Tail, func(l protocol.LogLine) {
			select {
			case lines <- l:
			case <-ctx.Done():
			}
		})
	}()

	batch := make([]protocol.LogLine, 0, logBatchLines)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := emit(ctx, batch)
		batch = batch[:0]
		return err
	}
	ticker := time.NewTicker(logFlushInterval)
	defer ticker.Stop()

	for {
		select {
		case l, ok := <-lines:
			if !ok {
				if err := flush(); err != nil {
					return err
				}
				return <-errc
			}
			batch = append(batch, l)
			if len(batch) >= logBatchLines {
				if err := flush(); err != nil {
					return err
				}
			}
		case <-ticker.C:
			if err := flush(); err != nil {
				return err
			}
		}
	}
}

func (h *Handler) containerAction(ctx context.Context, method string, p protocol.ContainerParams) error {
	switch method {
	case protocol.MethodStartContainer:
		return h.Docker.StartContainer(ctx, p.ID)
	case protocol.MethodStopContainer:
		return h.Docker.StopContainer(ctx, p.ID)
	case protocol.MethodRestartContainer:
		return h.Docker.RestartContainer(ctx, p.ID)
	default:
		return h.Docker.RemoveContainer(ctx, p.ID, p.Force)
	}
}

// containerParams decodes and validates container params. The controller
// validates too; the agent re-checks because it owns the Docker socket.
func containerParams(raw json.RawMessage) (protocol.ContainerParams, error) {
	var p protocol.ContainerParams
	if err := decode(raw, &p); err != nil {
		return p, err
	}
	if !protocol.ValidContainerRef(p.ID) {
		return p, &BadRequest{Msg: "invalid container id"}
	}
	return p, nil
}

func decode(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return &BadRequest{Msg: "invalid params: " + err.Error()}
	}
	return nil
}
