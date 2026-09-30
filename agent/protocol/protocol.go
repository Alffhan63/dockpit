// Package protocol defines the messages exchanged between the controller and
// an agent over the agent's WebSocket connection.
//
// The agent dials the controller and keeps one connection open. The first
// message the agent sends is a hello. After that the controller sends
// requests and the agent answers each one with a response carrying the same ID.
// Streaming requests (logs) send any number of stream messages before the
// response. The controller may cancel any request by ID.
package protocol

import (
	"encoding/json"
	"regexp"
)

// Message types.
const (
	TypeHello    = "hello"
	TypeRequest  = "request"
	TypeResponse = "response"
	// TypeStream carries one chunk of a streaming result (agent -> controller).
	// The stream ends with a TypeResponse for the same ID.
	TypeStream = "stream"
	// TypeCancel asks the agent to stop working on a request (controller -> agent).
	TypeCancel = "cancel"
)

// Methods the agent understands.
const (
	MethodListContainers   = "containers.list"
	MethodStartContainer   = "containers.start"
	MethodStopContainer    = "containers.stop"
	MethodRestartContainer = "containers.restart"
	MethodRemoveContainer  = "containers.remove"
	// MethodContainerLogs streams []LogLine chunks until the log ends or the
	// request is cancelled.
	MethodContainerLogs = "containers.logs"
	MethodHostStatus    = "host.status"
	MethodListImages    = "images.list"
	MethodRemoveImage   = "images.remove"
	MethodDiskUsage     = "system.df"
	MethodPrune         = "system.prune"
	MethodHostHistory   = "host.history"
	MethodListVolumes   = "volumes.list"
	MethodRemoveVolume  = "volumes.remove"
)

// Prune kinds for MethodPrune. Only low-risk cleanups exist: caches and
// untagged images nothing uses. Containers and volumes are never pruned.
const (
	PruneBuildCache     = "build-cache"
	PruneDanglingImages = "dangling-images"
)

// ConnectPath is the controller endpoint agents dial.
const ConnectPath = "/agent/connect"

// Message is the single envelope used on the wire.
type Message struct {
	Type   string          `json:"type"`
	ID     string          `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
	// Code is an HTTP-style status for errors the caller caused, such as
	// 404 for an unknown container or 409 for a conflicting state.
	Code  int       `json:"code,omitempty"`
	Hello *HostInfo `json:"hello,omitempty"`
}

// HostInfo describes the machine an agent runs on.
type HostInfo struct {
	Name          string    `json:"name"`
	OS            string    `json:"os"`
	Arch          string    `json:"arch"`
	DockerVersion string    `json:"docker_version"`
	AgentVersion  string    `json:"agent_version"`
	Addresses     []Address `json:"addresses,omitempty"`
}

// Address is an IPv4 address of one of the host's network interfaces.
type Address struct {
	Interface string `json:"interface"`
	IP        string `json:"ip"`
	// Scope is "private" (RFC 1918, or 100.64/10 as used by Tailscale and
	// carrier NAT) or "public".
	Scope string `json:"scope"`
}

// ListContainersParams are the params for MethodListContainers.
type ListContainersParams struct {
	All bool `json:"all"`
}

// ContainerParams identify a container for an action.
type ContainerParams struct {
	ID string `json:"id"`
	// Force removes a running container. Only used by MethodRemoveContainer.
	Force bool `json:"force,omitempty"`
}

var containerRefPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)

// ValidContainerRef reports whether ref is a well-formed container ID or name.
func ValidContainerRef(ref string) bool { return containerRefPattern.MatchString(ref) }

// LogsParams are the params for MethodContainerLogs.
type LogsParams struct {
	ID string `json:"id"`
	// Tail is the number of existing lines to send before following.
	Tail int `json:"tail"`
}

// Limits for LogsParams.Tail.
const (
	DefaultLogTail = 200
	MaxLogTail     = 5000
)

// LogLine is one line of container output. Field names are short because
// logs are the bulk of the traffic.
type LogLine struct {
	Stream string `json:"s"`           // "stdout" or "stderr"
	Time   string `json:"t,omitempty"` // RFC 3339 timestamp from Docker
	Text   string `json:"m"`
}

var imageIDPattern = regexp.MustCompile(`^(sha256:)?[0-9a-f]{12,64}$`)

// ValidImageID reports whether id is an image ID (full or short, optionally
// prefixed with "sha256:"). Images are only addressed by ID, never by tag.
func ValidImageID(id string) bool { return imageIDPattern.MatchString(id) }

// ImageParams identify an image for MethodRemoveImage.
type ImageParams struct {
	ID string `json:"id"`
}

// Image is a summary of one local image.
type Image struct {
	ID      string   `json:"id"`
	Tags    []string `json:"tags"` // empty for dangling images
	Size    int64    `json:"size"`
	Created int64    `json:"created"`
	// Containers using this image (any state), and how many of them run.
	Containers int `json:"containers"`
	Running    int `json:"running"`
}

// HostHistory is recent host metrics, oldest first, one point per Step.
type HostHistory struct {
	StepSeconds int            `json:"step_seconds"`
	Points      []HistoryPoint `json:"points"`
}

// HistoryPoint is one bucket of host metrics. CPU is the bucket average,
// Mem the last reading; both in percent.
type HistoryPoint struct {
	T   int64   `json:"t"` // bucket start, unix seconds
	CPU float64 `json:"cpu"`
	Mem float64 `json:"mem"`
}

var volumeNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,254}$`)

// ValidVolumeName reports whether name is a well-formed Docker volume name.
func ValidVolumeName(name string) bool { return volumeNamePattern.MatchString(name) }

// VolumeParams identify a volume for MethodRemoveVolume.
type VolumeParams struct {
	Name string `json:"name"`
}

// Volume is a summary of one volume.
type Volume struct {
	Name           string `json:"name"`
	Driver         string `json:"driver"`
	CreatedAt      string `json:"created_at,omitempty"`
	Size           int64  `json:"size"` // bytes; -1 when unknown
	Containers     int    `json:"containers"`
	Running        int    `json:"running"`
	ComposeProject string `json:"compose_project,omitempty"`
	// Anonymous volumes have a generated 64-hex name and no compose label.
	Anonymous bool `json:"anonymous"`
}

// DiskUsage is a `docker system df` summary.
type DiskUsage struct {
	Images     DiskUsageItem `json:"images"`
	Containers DiskUsageItem `json:"containers"`
	Volumes    DiskUsageItem `json:"volumes"`
	BuildCache DiskUsageItem `json:"build_cache"`
}

// DiskUsageItem summarises one kind of object. Sizes are bytes.
type DiskUsageItem struct {
	Count       int   `json:"count"`
	Active      int   `json:"active"`
	Size        int64 `json:"size"`
	Reclaimable int64 `json:"reclaimable"`
}

// PruneParams are the params for MethodPrune.
type PruneParams struct {
	Kind string `json:"kind"`
}

// PruneResult reports what a prune freed.
type PruneResult struct {
	Deleted        int   `json:"deleted"`
	SpaceReclaimed int64 `json:"space_reclaimed"`
}

// Container is a summary of one container.
type Container struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Image          string `json:"image"`
	ImageID        string `json:"image_id"`
	State          string `json:"state"`
	Status         string `json:"status"`
	Created        int64  `json:"created"`
	Ports          []Port `json:"ports"`
	ComposeProject string `json:"compose_project,omitempty"`
	ComposeService string `json:"compose_service,omitempty"`
	// ComposeDir is the directory docker compose ran in, i.e. the project.
	ComposeDir string `json:"compose_dir,omitempty"`

	// Resource usage of running containers, when sampled.
	CPUPercent *float64 `json:"cpu_percent,omitempty"`
	MemUsage   uint64   `json:"mem_usage,omitempty"`
	MemLimit   uint64   `json:"mem_limit,omitempty"`
}

// HostStatus is the result of MethodHostStatus.
type HostStatus struct {
	Metrics   HostMetrics `json:"metrics"`
	Docker    DockerInfo  `json:"docker"`
	Addresses []Address   `json:"addresses"`
}

// HostMetrics are OS-level metrics of the machine the agent runs on.
type HostMetrics struct {
	CPUPercent    float64 `json:"cpu_percent"`
	CPUCores      int     `json:"cpu_cores"`
	MemUsed       uint64  `json:"mem_used"`
	MemTotal      uint64  `json:"mem_total"`
	DiskPath      string  `json:"disk_path"`
	DiskUsed      uint64  `json:"disk_used"`
	DiskTotal     uint64  `json:"disk_total"`
	UptimeSeconds uint64  `json:"uptime_seconds"`
	SampledAt     int64   `json:"sampled_at"` // unix seconds; 0 = not yet sampled
}

// DockerInfo summarises the Docker Engine. On macOS, NCPU and MemTotal
// describe the Docker VM, not the Mac.
type DockerInfo struct {
	Version         string `json:"version"`
	OperatingSystem string `json:"operating_system"`
	Containers      int    `json:"containers"`
	Running         int    `json:"running"`
	Paused          int    `json:"paused"`
	Stopped         int    `json:"stopped"`
	Images          int    `json:"images"`
	NCPU            int    `json:"ncpu"`
	MemTotal        int64  `json:"mem_total"`
}

// Port is a container port mapping.
type Port struct {
	IP          string `json:"ip,omitempty"`
	PrivatePort uint16 `json:"private_port"`
	PublicPort  uint16 `json:"public_port,omitempty"`
	Type        string `json:"type"`
}
