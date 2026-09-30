package docker

import (
	"context"
	"net/http"
	"testing"

	"dockpit/agent/protocol"
)

func TestDiskUsageSummaries(t *testing.T) {
	sock := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"ImageUsage": {"ActiveCount": 2, "TotalCount": 5, "TotalSize": 1000, "Reclaimable": 600},
			"ContainerUsage": {"ActiveCount": 1, "TotalCount": 3, "TotalSize": 30, "Reclaimable": 20},
			"VolumeUsage": {"ActiveCount": 1, "TotalCount": 2, "TotalSize": 500, "Reclaimable": 100},
			"BuildCacheUsage": {"TotalCount": 9, "TotalSize": 900, "Reclaimable": 700}
		}`))
	})
	du, err := New(sock).DiskUsage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := protocol.DiskUsage{
		Images:     protocol.DiskUsageItem{Count: 5, Active: 2, Size: 1000, Reclaimable: 600},
		Containers: protocol.DiskUsageItem{Count: 3, Active: 1, Size: 30, Reclaimable: 20},
		Volumes:    protocol.DiskUsageItem{Count: 2, Active: 1, Size: 500, Reclaimable: 100},
		BuildCache: protocol.DiskUsageItem{Count: 9, Size: 900, Reclaimable: 700},
	}
	if du != want {
		t.Errorf("got %+v\nwant %+v", du, want)
	}
}

// Older engines have no *Usage summaries; totals come from the items.
func TestDiskUsageComputed(t *testing.T) {
	sock := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"LayersSize": 1000,
			"Images": [
				{"Size": 400, "SharedSize": 100, "Containers": 0},
				{"Size": 300, "SharedSize": -1, "Containers": 0},
				{"Size": 300, "SharedSize": 0, "Containers": 2}
			],
			"Containers": [{"State": "running", "SizeRw": 10}, {"State": "exited", "SizeRw": 5}],
			"Volumes": [{"UsageData": {"Size": 50, "RefCount": 0}}, {"UsageData": {"Size": 70, "RefCount": 1}}, {"UsageData": {"Size": -1, "RefCount": 0}}],
			"BuildCache": [{"Size": 8, "InUse": false, "Shared": false}, {"Size": 4, "InUse": true}, {"Size": 2, "Shared": true}]
		}`))
	})
	du, err := New(sock).DiskUsage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := protocol.DiskUsage{
		Images:     protocol.DiskUsageItem{Count: 3, Active: 1, Size: 1000, Reclaimable: 300 + 300},
		Containers: protocol.DiskUsageItem{Count: 2, Active: 1, Size: 15, Reclaimable: 5},
		Volumes:    protocol.DiskUsageItem{Count: 3, Active: 1, Size: 120, Reclaimable: 50},
		BuildCache: protocol.DiskUsageItem{Count: 3, Active: 1, Size: 14, Reclaimable: 8},
	}
	if du != want {
		t.Errorf("got %+v\nwant %+v", du, want)
	}
}

func TestPrune(t *testing.T) {
	var calls []string
	sock := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		switch r.URL.Path {
		case "/build/prune":
			w.Write([]byte(`{"CachesDeleted": ["a", "b"], "SpaceReclaimed": 123}`))
		case "/images/prune":
			w.Write([]byte(`{"ImagesDeleted": [{"Untagged": "x"}, {"Deleted": "sha256:1"}], "SpaceReclaimed": 45}`))
		}
	})
	c := New(sock)
	ctx := context.Background()
	if r, err := c.Prune(ctx, protocol.PruneBuildCache); err != nil || r.Deleted != 2 || r.SpaceReclaimed != 123 {
		t.Errorf("build cache: %+v %v", r, err)
	}
	if r, err := c.Prune(ctx, protocol.PruneDanglingImages); err != nil || r.Deleted != 1 || r.SpaceReclaimed != 45 {
		t.Errorf("dangling: %+v %v", r, err)
	}
	if _, err := c.Prune(ctx, "volumes"); err == nil {
		t.Error("volume prune accepted")
	}
	want := []string{
		"POST /build/prune?all=1",
		`POST /images/prune?filters=%7B%22dangling%22%3A%5B%22true%22%5D%7D`,
	}
	if len(calls) != 2 || calls[0] != want[0] || calls[1] != want[1] {
		t.Errorf("calls = %q", calls)
	}
}
