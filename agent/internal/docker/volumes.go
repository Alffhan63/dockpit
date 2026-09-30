package docker

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"

	"dockpit/agent/protocol"
)

var anonymousVolume = regexp.MustCompile(`^[0-9a-f]{64}$`)

// volumeUsers counts containers (any state) that mount each volume, and how
// many of those are running.
func (c *Client) volumeUsers(ctx context.Context) (used, running map[string]int, err error) {
	containers, err := c.listContainers(ctx, true)
	if err != nil {
		return nil, nil, err
	}
	used, running = map[string]int{}, map[string]int{}
	for _, ct := range containers {
		for _, m := range ct.Mounts {
			if m.Type != "volume" || m.Name == "" {
				continue
			}
			used[m.Name]++
			if ct.State == "running" {
				running[m.Name]++
			}
		}
	}
	return used, running, nil
}

// ListVolumes lists volumes with their size and the containers using them.
func (c *Client) ListVolumes(ctx context.Context) ([]protocol.Volume, error) {
	var list struct {
		Volumes []struct {
			Name      string            `json:"Name"`
			Driver    string            `json:"Driver"`
			CreatedAt string            `json:"CreatedAt"`
			Labels    map[string]string `json:"Labels"`
		} `json:"Volumes"`
	}
	if err := c.get(ctx, "/volumes", &list); err != nil {
		return nil, err
	}
	// Sizes are only reported by system df; type=volume skips images and
	// build cache on engines that support it.
	var df struct {
		Volumes []struct {
			Name      string `json:"Name"`
			UsageData *struct {
				Size int64 `json:"Size"`
			} `json:"UsageData"`
		} `json:"Volumes"`
	}
	if err := c.get(ctx, "/system/df?type=volume", &df); err != nil {
		return nil, err
	}
	sizes := map[string]int64{}
	for _, v := range df.Volumes {
		if v.UsageData != nil {
			sizes[v.Name] = v.UsageData.Size
		}
	}
	used, running, err := c.volumeUsers(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]protocol.Volume, 0, len(list.Volumes))
	for _, v := range list.Volumes {
		size, ok := sizes[v.Name]
		if !ok {
			size = -1
		}
		project := v.Labels[composeProjectLabel]
		out = append(out, protocol.Volume{
			Name:           v.Name,
			Driver:         v.Driver,
			CreatedAt:      v.CreatedAt,
			Size:           size,
			Containers:     used[v.Name],
			Running:        running[v.Name],
			ComposeProject: project,
			Anonymous:      project == "" && anonymousVolume.MatchString(v.Name),
		})
	}
	return out, nil
}

// RemoveVolume deletes a volume and its data. It refuses volumes any
// container mounts, running or not, and never forces.
func (c *Client) RemoveVolume(ctx context.Context, name string) error {
	used, _, err := c.volumeUsers(ctx)
	if err != nil {
		return err
	}
	if n := used[name]; n > 0 {
		return &APIError{Status: http.StatusConflict, Message: fmt.Sprintf("volume is used by %d container(s); remove them first", n)}
	}
	return c.do(ctx, http.MethodDelete, "/volumes/"+url.PathEscape(name), nil)
}
