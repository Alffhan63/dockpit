package docker

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"dockpit/agent/protocol"
)

// ListImages lists local images with how many containers use each.
func (c *Client) ListImages(ctx context.Context) ([]protocol.Image, error) {
	var raw []struct {
		ID       string   `json:"Id"`
		RepoTags []string `json:"RepoTags"`
		Created  int64    `json:"Created"`
		Size     int64    `json:"Size"`
	}
	if err := c.get(ctx, "/images/json", &raw); err != nil {
		return nil, err
	}
	// Docker only reports per-image container counts from `system df`, so
	// count them from the container list.
	containers, err := c.listContainers(ctx, true)
	if err != nil {
		return nil, err
	}
	used := map[string]int{}
	running := map[string]int{}
	for _, ct := range containers {
		used[ct.ImageID]++
		if ct.State == "running" {
			running[ct.ImageID]++
		}
	}

	out := make([]protocol.Image, 0, len(raw))
	for _, img := range raw {
		tags := make([]string, 0, len(img.RepoTags))
		for _, t := range img.RepoTags {
			if t != "<none>:<none>" {
				tags = append(tags, t)
			}
		}
		out = append(out, protocol.Image{
			ID:         img.ID,
			Tags:       tags,
			Size:       img.Size,
			Created:    img.Created,
			Containers: used[img.ID],
			Running:    running[img.ID],
		})
	}
	return out, nil
}

// RemoveImage deletes an image, with all its tags, if no container uses it.
//
// Docker itself would refuse to delete a used image only without force, but
// without force it also refuses images with several tags. So check usage
// here, then force: containers never lose their image, and multi-tag images
// can still be removed.
func (c *Client) RemoveImage(ctx context.Context, id string) error {
	containers, err := c.listContainers(ctx, true)
	if err != nil {
		return err
	}
	short := strings.TrimPrefix(id, "sha256:")
	n := 0
	for _, ct := range containers {
		if strings.HasPrefix(strings.TrimPrefix(ct.ImageID, "sha256:"), short) {
			n++
		}
	}
	if n > 0 {
		return &APIError{Status: http.StatusConflict, Message: fmt.Sprintf("image is used by %d container(s); remove them first", n)}
	}
	return c.do(ctx, http.MethodDelete, "/images/"+url.PathEscape(id)+"?force=1", nil)
}
