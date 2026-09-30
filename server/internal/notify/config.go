package notify

// Rules choose which events raise an alert. A percentage of 0 turns that
// check off.
type Rules struct {
	HostOffline     bool `json:"host_offline"`
	ContainerHealth bool `json:"container_unhealthy"`
	ContainerExit   bool `json:"container_exit"`
	RestartLoop     bool `json:"restart_loop"`
	DiskPercent     int  `json:"disk_percent"`
	MemPercent      int  `json:"mem_percent"`
}

// Config is the stored notification setup.
type Config struct {
	Enabled  bool     `json:"enabled"`
	Channels Channels `json:"channels"`
	Rules    Rules    `json:"rules"`
}

// DefaultConfig is what a fresh install starts with (disabled).
func DefaultConfig() Config {
	return Config{Rules: Rules{
		HostOffline: true, ContainerHealth: true, ContainerExit: true, RestartLoop: true,
		DiskPercent: 90, MemPercent: 90,
	}}
}
