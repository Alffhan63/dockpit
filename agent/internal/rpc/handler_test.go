package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"dockpit/agent/internal/docker"
	"dockpit/agent/internal/transport"
	"dockpit/agent/protocol"
)

func TestRejectsBadRequests(t *testing.T) {
	// The Docker client must never be reached for these requests.
	h := &Handler{Docker: docker.New(func() (string, error) {
		t.Fatal("docker was called")
		return "", nil
	})}
	bad := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }

	cases := []transport.Request{
		{Method: protocol.MethodStopContainer, Params: bad(protocol.ContainerParams{ID: "../../info"})},
		{Method: protocol.MethodRemoveContainer, Params: bad(protocol.ContainerParams{ID: ""})},
		{Method: protocol.MethodStartContainer, Params: json.RawMessage(`{"id": 5}`)},
		{Method: protocol.MethodRemoveImage, Params: bad(protocol.ImageParams{ID: "redis:7"})},
		{Method: protocol.MethodRemoveImage, Params: bad(protocol.ImageParams{ID: "../../containers/x"})},
		{Method: protocol.MethodPrune, Params: bad(protocol.PruneParams{Kind: "volumes"})},
		{Method: protocol.MethodPrune, Params: bad(protocol.PruneParams{Kind: "containers"})},
		{Method: protocol.MethodRemoveVolume, Params: bad(protocol.VolumeParams{Name: "../x"})},
		{Method: protocol.MethodRemoveVolume, Params: bad(protocol.VolumeParams{Name: ""})},
		{Method: "exec"},
	}
	for _, req := range cases {
		_, err := h.Handle(context.Background(), req)
		var br *BadRequest
		if !errors.As(err, &br) || br.StatusCode() != http.StatusBadRequest {
			t.Errorf("%s %s: err = %v, want BadRequest", req.Method, req.Params, err)
		}
	}
}
