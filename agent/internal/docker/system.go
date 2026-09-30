package docker

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"dockpit/agent/protocol"
)

type usageSummary struct {
	ActiveCount int   `json:"ActiveCount"`
	TotalCount  int   `json:"TotalCount"`
	TotalSize   int64 `json:"TotalSize"`
	Reclaimable int64 `json:"Reclaimable"`
}

func (u *usageSummary) item() protocol.DiskUsageItem {
	return protocol.DiskUsageItem{Count: u.TotalCount, Active: u.ActiveCount, Size: u.TotalSize, Reclaimable: u.Reclaimable}
}

// DiskUsage returns a `docker system df` summary. Newer engines (API 1.52+)
// report the totals themselves; for older ones they are computed from the
// item lists the same way the docker CLI does.
func (c *Client) DiskUsage(ctx context.Context) (protocol.DiskUsage, error) {
	var v struct {
		LayersSize int64 `json:"LayersSize"`
		Images     []struct {
			Size       int64 `json:"Size"`
			SharedSize int64 `json:"SharedSize"`
			Containers int   `json:"Containers"`
		} `json:"Images"`
		Containers []struct {
			State  string `json:"State"`
			SizeRw int64  `json:"SizeRw"`
		} `json:"Containers"`
		Volumes []struct {
			UsageData *struct {
				Size     int64 `json:"Size"`
				RefCount int   `json:"RefCount"`
			} `json:"UsageData"`
		} `json:"Volumes"`
		BuildCache []struct {
			Size   int64 `json:"Size"`
			InUse  bool  `json:"InUse"`
			Shared bool  `json:"Shared"`
		} `json:"BuildCache"`

		ImageUsage      *usageSummary `json:"ImageUsage"`
		ContainerUsage  *usageSummary `json:"ContainerUsage"`
		VolumeUsage     *usageSummary `json:"VolumeUsage"`
		BuildCacheUsage *usageSummary `json:"BuildCacheUsage"`
	}
	if err := c.get(ctx, "/system/df", &v); err != nil {
		return protocol.DiskUsage{}, err
	}

	var du protocol.DiskUsage
	if v.ImageUsage != nil {
		du.Images = v.ImageUsage.item()
	} else {
		du.Images = protocol.DiskUsageItem{Count: len(v.Images), Size: v.LayersSize}
		for _, i := range v.Images {
			if i.Containers > 0 {
				du.Images.Active++
				continue
			}
			if i.SharedSize > 0 {
				du.Images.Reclaimable += i.Size - i.SharedSize
			} else {
				du.Images.Reclaimable += i.Size
			}
		}
	}
	if v.ContainerUsage != nil {
		du.Containers = v.ContainerUsage.item()
	} else {
		du.Containers.Count = len(v.Containers)
		for _, ct := range v.Containers {
			du.Containers.Size += ct.SizeRw
			if ct.State == "running" {
				du.Containers.Active++
			} else {
				du.Containers.Reclaimable += ct.SizeRw
			}
		}
	}
	if v.VolumeUsage != nil {
		du.Volumes = v.VolumeUsage.item()
	} else {
		du.Volumes.Count = len(v.Volumes)
		for _, vol := range v.Volumes {
			if vol.UsageData == nil || vol.UsageData.Size < 0 {
				continue // size unknown
			}
			du.Volumes.Size += vol.UsageData.Size
			if vol.UsageData.RefCount > 0 {
				du.Volumes.Active++
			} else {
				du.Volumes.Reclaimable += vol.UsageData.Size
			}
		}
	}
	if v.BuildCacheUsage != nil {
		du.BuildCache = v.BuildCacheUsage.item()
	} else {
		du.BuildCache.Count = len(v.BuildCache)
		for _, b := range v.BuildCache {
			du.BuildCache.Size += b.Size
			if b.InUse {
				du.BuildCache.Active++
			} else if !b.Shared {
				du.BuildCache.Reclaimable += b.Size
			}
		}
	}
	return du, nil
}

// Prune runs one of the low-risk cleanups in protocol.Prune*.
func (c *Client) Prune(ctx context.Context, kind string) (protocol.PruneResult, error) {
	switch kind {
	case protocol.PruneBuildCache:
		// all=1: every unused cache entry, not only dangling ones. It is only
		// cache: the worst case is a slower next build.
		var v struct {
			CachesDeleted  []string `json:"CachesDeleted"`
			SpaceReclaimed int64    `json:"SpaceReclaimed"`
		}
		if err := c.do(ctx, http.MethodPost, "/build/prune?all=1", &v); err != nil {
			return protocol.PruneResult{}, err
		}
		return protocol.PruneResult{Deleted: len(v.CachesDeleted), SpaceReclaimed: v.SpaceReclaimed}, nil

	case protocol.PruneDanglingImages:
		// Untagged images no container uses; tagged images are kept.
		var v struct {
			ImagesDeleted []struct {
				Deleted string `json:"Deleted"`
			} `json:"ImagesDeleted"`
			SpaceReclaimed int64 `json:"SpaceReclaimed"`
		}
		filters := url.QueryEscape(`{"dangling":["true"]}`)
		if err := c.do(ctx, http.MethodPost, "/images/prune?filters="+filters, &v); err != nil {
			return protocol.PruneResult{}, err
		}
		n := 0
		for _, d := range v.ImagesDeleted {
			if d.Deleted != "" {
				n++
			}
		}
		return protocol.PruneResult{Deleted: n, SpaceReclaimed: v.SpaceReclaimed}, nil
	}
	return protocol.PruneResult{}, fmt.Errorf("unknown prune kind %q", kind)
}
