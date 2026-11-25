package lifecycle

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/sshconnections"
	"net"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/sshtransport"
	"github.com/kandev/kandev/internal/task/models"
	"golang.org/x/crypto/ssh"
)

type sweepManagedConnection struct {
	client   *ssh.Client
	opened   string
	released bool
	err      error
}

func (m *sweepManagedConnection) OpenExecutor(_ context.Context, id string) (*ssh.Client, *sshtransport.SSHTarget, func(), error) {
	m.opened = id
	if m.err != nil {
		return nil, nil, nil, m.err
	}
	return m.client, &sshtransport.SSHTarget{}, func() { m.released = true; _ = m.client.Close() }, nil
}

// @covers AC-EXECUTORS-SSH-CONNECTIONS-005.3
func TestManagedSSHOrphanSweepStopsArchivedTaskThroughSavedConnection(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	archived := time.Now()
	handler := newSSHScriptedHandler(t,
		sshScriptRule{match: "/fixture/tasks", result: sshOut("PROC\t4242\t1\t/opt/kandev/agentctl --workdir /fixture/tasks/task-archived\n")},
		sshScriptRule{match: "TARGET_PID=4242", result: sshOK},
		sshScriptRule{match: `printf %s "$HOME"`, result: sshOut("/fixture-home")},
		sshScriptRule{match: "/fixture-home/.kandev/tasks", result: sshOK},
	)
	server := newFakeSSHServer(t, handler.handle)
	managed := &sweepManagedConnection{client: server.dial(t)}
	store := &orphanSweepRootTestStore{task: &models.Task{ID: "archived", ArchivedAt: &archived}}
	scheduler := NewOrphanSweepScheduler(&schedulerTestStore{}, store, 3600, logger.Default(), nil, managed)
	executor := &models.Executor{ID: "saved-executor", Type: models.ExecutorTypeSSH, Config: map[string]string{
		MetadataKeySSHConnectionID: "saved-connection", MetadataKeySSHWorkdirRoot: "/fixture",
	}}
	scheduler.runSweep(context.Background(), executor)
	if _, ok := server.lastCommandContaining("TARGET_PID=4242"); !ok {
		t.Fatal("managed cleanup did not stop the archived task's process")
	}
	if managed.opened != executor.ID || !managed.released {
		t.Fatalf("saved executor connection lifecycle: opened=%q released=%v", managed.opened, managed.released)
	}
}

// @covers AC-EXECUTORS-SSH-CONNECTIONS-005.3
func TestManagedSSHOrphanSweepNeverFallsBackToInlineSettings(t *testing.T) {
	stubSSHAgentSocket(t)
	for _, mode := range []string{"legacy", "disabled", "stale", "unavailable"} {
		t.Run(mode, func(t *testing.T) {
			handler := newSSHScriptedHandler(t,
				sshScriptRule{match: `printf %s "$HOME"`, result: sshOut("/fixture-home")},
				sshScriptRule{match: "/fixture-home/.kandev/tasks", result: sshOK},
			)
			server := newFakeSSHServer(t, handler.handle)
			host, port, err := net.SplitHostPort(server.addr())
			if err != nil {
				t.Fatal(err)
			}
			executor := &models.Executor{ID: "saved", Type: models.ExecutorTypeSSH, Config: map[string]string{
				"ssh_host": host, "ssh_port": port, "ssh_user": "fixture",
				"ssh_host_fingerprint": ssh.FingerprintSHA256(server.signer.PublicKey()),
				"ssh_identity_source":  "agent",
			}}
			var managed ManagedSSHExecutorConnection
			if mode != "legacy" {
				executor.Config[MetadataKeySSHConnectionID] = "managed"
			}
			if mode == "stale" {
				managed = &sweepManagedConnection{err: sshconnections.ErrConflict}
			}
			if mode == "unavailable" {
				managed = &sweepManagedConnection{err: errors.New("fixture unavailable")}
			}
			store := &schedulerTestStore{}
			scheduler := NewOrphanSweepScheduler(store, store, 3600, logger.Default(), nil, managed)
			scheduler.runSweep(context.Background(), executor)
			_, ran := server.lastCommandContaining("/fixture-home/.kandev/tasks")
			if ran {
				t.Fatalf("inline inventory ran=%v, mode=%s", ran, mode)
			}
		})
	}
}
