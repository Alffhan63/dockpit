package docker

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestVolumes(t *testing.T) {
	anon := strings.Repeat("ab", 32)
	var deleted []string
	sock := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/volumes" && r.Method == "GET":
			w.Write([]byte(`{"Volumes": [
				{"Name": "demo_db", "Driver": "local", "Labels": {"com.docker.compose.project": "demo"}},
				{"Name": "` + anon + `", "Driver": "local"},
				{"Name": "old-data", "Driver": "local"}
			]}`))
		case r.URL.Path == "/system/df":
			if r.URL.Query().Get("type") != "volume" {
				t.Errorf("df query = %q", r.URL.RawQuery)
			}
			w.Write([]byte(`{"Volumes": [{"Name": "demo_db", "UsageData": {"Size": 1000}}, {"Name": "old-data", "UsageData": {"Size": 5}}]}`))
		case r.URL.Path == "/containers/json":
			w.Write([]byte(`[
				{"Id": "c1", "State": "exited", "Mounts": [{"Type": "volume", "Name": "demo_db"}, {"Type": "bind", "Name": ""}]}
			]`))
		case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/volumes/"):
			deleted = append(deleted, r.URL.RequestURI())
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	c := New(sock)
	vols, err := c.ListVolumes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(vols) != 3 {
		t.Fatalf("vols = %+v", vols)
	}
	db, a, old := vols[0], vols[1], vols[2]
	if db.Size != 1000 || db.Containers != 1 || db.Running != 0 || db.ComposeProject != "demo" || db.Anonymous {
		t.Errorf("demo_db = %+v", db)
	}
	if !a.Anonymous || a.Size != -1 {
		t.Errorf("anonymous = %+v", a)
	}
	if old.Containers != 0 || old.Anonymous {
		t.Errorf("old-data = %+v", old)
	}

	// Mounted by a stopped container: refused, nothing deleted.
	var apiErr *APIError
	if err := c.RemoveVolume(context.Background(), "demo_db"); !errors.As(err, &apiErr) || apiErr.StatusCode() != 409 {
		t.Errorf("in-use volume: err = %v", err)
	}
	if err := c.RemoveVolume(context.Background(), "old-data"); err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0] != "/volumes/old-data" {
		t.Errorf("deletes = %v (force must never be set)", deleted)
	}
}
