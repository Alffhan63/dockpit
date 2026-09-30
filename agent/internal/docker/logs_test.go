package docker

import (
	"bytes"
	"context"
	"encoding/binary"
	"net/http"
	"strings"
	"testing"

	"dockpit/agent/protocol"
)

func frame(stream byte, s string) []byte {
	hdr := make([]byte, 8)
	hdr[0] = stream
	binary.BigEndian.PutUint32(hdr[4:], uint32(len(s)))
	return append(hdr, s...)
}

func collect(t *testing.T, data []byte, tty bool) []protocol.LogLine {
	t.Helper()
	var got []protocol.LogLine
	if err := readLogs(bytes.NewReader(data), tty, func(l protocol.LogLine) { got = append(got, l) }); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestReadLogsMultiplexed(t *testing.T) {
	var data []byte
	data = append(data, frame(1, "2024-01-01T00:00:00.000000001Z hello\n")...)
	data = append(data, frame(2, "2024-01-01T00:00:01Z boom\r\n2024-01-01T00:00:02Z par")...)
	data = append(data, frame(2, "tial\n")...)
	data = append(data, frame(1, "no timestamp")...) // unterminated: flushed at EOF

	got := collect(t, data, false)
	want := []protocol.LogLine{
		{Stream: "stdout", Time: "2024-01-01T00:00:00.000000001Z", Text: "hello"},
		{Stream: "stderr", Time: "2024-01-01T00:00:01Z", Text: "boom"},
		{Stream: "stderr", Time: "2024-01-01T00:00:02Z", Text: "partial"},
		{Stream: "stdout", Text: "no timestamp"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestReadLogsTTYAndLongLines(t *testing.T) {
	long := strings.Repeat("x", maxLineBytes+10)
	got := collect(t, []byte("2024-01-01T00:00:00Z tty line\n"+long+"\n"), true)
	if len(got) != 3 || got[0].Stream != "stdout" || got[0].Text != "tty line" {
		t.Fatalf("got %d lines: %+v", len(got), got[0])
	}
	if len(got[1].Text) != maxLineBytes || len(got[2].Text) != 10 {
		t.Errorf("long line split into %d + %d", len(got[1].Text), len(got[2].Text))
	}
}

func TestFollowLogs(t *testing.T) {
	var logsQuery string
	sock := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/containers/web/json":
			w.Write([]byte(`{"Config":{"Tty":false}}`))
		case "/containers/web/logs":
			logsQuery = r.URL.RawQuery
			w.Write(frame(1, "2024-01-01T00:00:00Z up\n"))
		default:
			http.NotFound(w, r)
		}
	})
	var got []protocol.LogLine
	err := New(sock).FollowLogs(context.Background(), "web", 50, func(l protocol.LogLine) { got = append(got, l) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logsQuery, "follow=1") || !strings.Contains(logsQuery, "tail=50") || !strings.Contains(logsQuery, "timestamps=1") {
		t.Errorf("query = %q", logsQuery)
	}
	if len(got) != 1 || got[0].Text != "up" {
		t.Errorf("got %+v", got)
	}
}
