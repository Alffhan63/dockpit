package docker

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dockpit/agent/protocol"
)

// maxLineBytes caps a single log line; longer lines are split.
const maxLineBytes = 16 << 10

// ContainerTTY reports whether a container was created with a TTY, which
// decides whether its log stream is multiplexed.
func (c *Client) ContainerTTY(ctx context.Context, id string) (bool, error) {
	var v struct {
		Config struct {
			Tty bool `json:"Tty"`
		} `json:"Config"`
	}
	if err := c.get(ctx, containerPath(id, "/json"), &v); err != nil {
		return false, err
	}
	return v.Config.Tty, nil
}

// FollowLogs streams a container's logs, starting with the last tail lines,
// calling emit for each line until the log ends or ctx is cancelled.
func (c *Client) FollowLogs(ctx context.Context, id string, tail int, emit func(protocol.LogLine)) error {
	tty, err := c.ContainerTTY(ctx, id)
	if err != nil {
		return err
	}
	path := containerPath(id, "/logs") + "?stdout=1&stderr=1&follow=1&timestamps=1&tail=" + strconv.Itoa(tail)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker"+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("docker logs: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return apiError(resp)
	}

	err = readLogs(resp.Body, tty, emit)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// readLogs parses a Docker log stream. Without a TTY the stream is
// multiplexed: each frame has an 8-byte header [stream, 0, 0, 0, size(4, BE)].
func readLogs(r io.Reader, tty bool, emit func(protocol.LogLine)) error {
	stdout := &lineSplitter{stream: "stdout", emit: emit}
	stderr := &lineSplitter{stream: "stderr", emit: emit}
	defer stdout.flush()
	defer stderr.flush()

	if tty {
		buf := make([]byte, 32<<10)
		for {
			n, err := r.Read(buf)
			stdout.write(buf[:n])
			if err != nil {
				return eofOK(err)
			}
		}
	}

	var hdr [8]byte
	var payload []byte
	for {
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return eofOK(err)
		}
		size := int(binary.BigEndian.Uint32(hdr[4:]))
		if cap(payload) < size {
			payload = make([]byte, size)
		}
		payload = payload[:size]
		if _, err := io.ReadFull(r, payload); err != nil {
			return eofOK(err)
		}
		switch hdr[0] {
		case 1:
			stdout.write(payload)
		case 2:
			stderr.write(payload)
		}
	}
}

func eofOK(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return nil
	}
	return err
}

// lineSplitter turns a byte stream into lines, keeping partial lines until
// the rest arrives.
type lineSplitter struct {
	stream string
	emit   func(protocol.LogLine)
	buf    []byte
}

func (s *lineSplitter) write(p []byte) {
	s.buf = append(s.buf, p...)
	for {
		i := bytes.IndexByte(s.buf, '\n')
		switch {
		case i < 0 && len(s.buf) < maxLineBytes:
			return // wait for the rest of the line
		case i < 0 || i > maxLineBytes:
			s.line(s.buf[:maxLineBytes])
			s.buf = append(s.buf[:0], s.buf[maxLineBytes:]...)
		default:
			s.line(s.buf[:i])
			s.buf = append(s.buf[:0], s.buf[i+1:]...)
		}
	}
}

func (s *lineSplitter) flush() {
	if len(s.buf) > 0 {
		s.line(s.buf)
		s.buf = s.buf[:0]
	}
}

func (s *lineSplitter) line(b []byte) {
	text := strings.TrimSuffix(string(b), "\r")
	l := protocol.LogLine{Stream: s.stream, Text: text}
	// With timestamps=1 every line starts with "<RFC3339Nano> ".
	if ts, rest, ok := strings.Cut(text, " "); ok {
		if _, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			l.Time, l.Text = ts, rest
		}
	}
	s.emit(l)
}
