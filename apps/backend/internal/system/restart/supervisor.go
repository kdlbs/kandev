package restart

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"runtime"
	"time"
)

const (
	EnvSupervisorSocket   = "KANDEV_SUPERVISOR_SOCKET"
	EnvSupervisorManifest = "KANDEV_SUPERVISOR_MANIFEST"
	EnvRestartAdapter     = "KANDEV_RESTART_ADAPTER"

	controlTimeout = 2 * time.Second
)

type SupervisorManager struct {
	SocketPath string
	Manifest   string
	Dial       func(ctx context.Context, network, address string) (net.Conn, error)
}

func NewManagerFromEnv() Manager {
	adapter := os.Getenv(EnvRestartAdapter)
	socket := os.Getenv(EnvSupervisorSocket)
	if adapter == AdapterSupervisor && socket == "" {
		return NewUnsupportedManager(fmt.Sprintf("%s is required when %s=%s.", EnvSupervisorSocket, EnvRestartAdapter, AdapterSupervisor))
	}
	if adapter == AdapterSupervisor || socket != "" {
		return NewSupervisorManager(socket, os.Getenv(EnvSupervisorManifest))
	}
	return NewUnsupportedManager("")
}

func NewSupervisorManager(socketPath, manifest string) *SupervisorManager {
	return &SupervisorManager{
		SocketPath: socketPath,
		Manifest:   manifest,
		Dial: (&net.Dialer{
			Timeout: controlTimeout,
		}).DialContext,
	}
}

func (m *SupervisorManager) Capability(ctx context.Context) Capability {
	if runtime.GOOS == "windows" {
		return m.unsupported("Supervisor restart is not available on Windows yet.")
	}
	if m.SocketPath == "" {
		return m.unsupported("Supervisor restart is not configured for this launch mode.")
	}
	if err := m.probe(ctx); err != nil {
		return m.unsupported("Supervisor restart is configured but unavailable: " + err.Error())
	}
	return Capability{
		Supported: true,
		Mode:      ModeSupervisor,
		Adapter:   AdapterSupervisor,
		Details: map[string]interface{}{
			"restart_requires_ui_poll": true,
		},
	}
}

func (m *SupervisorManager) RequestRestart(ctx context.Context) (RestartResponse, error) {
	if cap := m.Capability(ctx); !cap.Supported {
		return RestartResponse{Accepted: false, Message: cap.Reason}, ErrUnsupported
	}
	resp, err := m.send(ctx, map[string]string{"action": "restart"})
	if err != nil {
		return RestartResponse{}, err
	}
	if !resp.Accepted {
		return RestartResponse{Accepted: false, Message: resp.Message}, fmt.Errorf("supervisor rejected restart: %s", resp.Message)
	}
	return RestartResponse{Accepted: true, Message: "Restart requested. Kandev will be unavailable briefly."}, nil
}

func (m *SupervisorManager) probe(ctx context.Context) error {
	conn, err := m.dial(ctx)
	if err != nil {
		return err
	}
	return conn.Close()
}

func (m *SupervisorManager) send(ctx context.Context, req map[string]string) (RestartResponse, error) {
	conn, err := m.dial(ctx)
	if err != nil {
		return RestartResponse{}, err
	}
	defer func() { _ = conn.Close() }()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(controlTimeout))
	}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return RestartResponse{}, err
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return RestartResponse{}, err
	}
	var resp RestartResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return RestartResponse{}, err
	}
	return resp, nil
}

func (m *SupervisorManager) dial(ctx context.Context) (net.Conn, error) {
	dial := m.Dial
	if dial == nil {
		dial = (&net.Dialer{Timeout: controlTimeout}).DialContext
	}
	return dial(ctx, "unix", m.SocketPath)
}

func (m *SupervisorManager) unsupported(reason string) Capability {
	return Capability{
		Supported: false,
		Mode:      ModeManual,
		Adapter:   AdapterUnsupported,
		Reason:    reason,
	}
}
