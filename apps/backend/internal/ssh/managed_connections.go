package ssh

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kandev/kandev/internal/sshconnections"
	"github.com/kandev/kandev/internal/sshtransport"
	"golang.org/x/crypto/ssh"
)

// UnmarshalJSON enforces the same saved-connection selector for HTTP and WebSocket.
func (r *TestRequest) UnmarshalJSON(data []byte) error {
	type wire TestRequest
	var req wire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || req.ConnectionID == "" || req.Stamp == "" {
		return sshconnections.ErrInvalidRoute
	}
	*r = TestRequest(req)
	return nil
}

type ManagedConnections interface {
	OpenConnectionForManagement(context.Context, string, string) (*ssh.Client, *sshtransport.SSHTarget, func(), error)
	OpenExecutorForManagement(context.Context, string) (*ssh.Client, *sshtransport.SSHTarget, func(), error)
}

func (h *Handler) openExecutor(ctx context.Context, id string) (*ssh.Client, *sshtransport.SSHTarget, func(), int, error) {
	empty := func() {}
	executor, status, err := h.loadSSHExecutor(ctx, id)
	if err != nil {
		return nil, nil, empty, status, err
	}
	if h.managedConnections == nil {
		return nil, nil, empty, http.StatusServiceUnavailable, sshconnections.ErrDisabled
	}
	if executor.Config[sshconnections.ConnectionConfigKey] == "" {
		return nil, nil, empty, http.StatusBadRequest, sshconnections.ErrInvalidRoute
	}
	client, target, cleanup, err := h.managedConnections.OpenExecutorForManagement(ctx, id)
	switch {
	case errors.Is(err, sshconnections.ErrForbidden):
		status = http.StatusForbidden
	case errors.Is(err, sshconnections.ErrDisabled):
		status = http.StatusServiceUnavailable
	case err != nil:
		status = http.StatusBadGateway
	}
	return client, target, cleanup, status, err
}

func (h *Handler) testOpen(ctx context.Context, req TestRequest, result *TestResult) (*ssh.Client, func(), error) {
	if req.ConnectionID == "" || req.Stamp == "" {
		result.Error = sshconnections.ErrInvalidRoute.Error()
		return nil, func() {}, sshconnections.ErrInvalidRoute
	}
	if h.managedConnections == nil {
		result.Error = sshconnections.ErrDisabled.Error()
		return nil, func() {}, sshconnections.ErrDisabled
	}
	client, target, cleanup, err := h.managedConnections.OpenConnectionForManagement(ctx, req.ConnectionID, req.Stamp)
	if err != nil {
		result.Error = err.Error()
		return nil, func() {}, err
	}
	result.Fingerprint = target.PinnedFingerprint
	result.Steps = append(result.Steps, TestStep{Name: "SSH handshake", Success: true})
	return client, cleanup, nil
}
