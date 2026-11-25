package backendapp

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/sshconnections"
	"github.com/kandev/kandev/internal/sshtransport"
	"github.com/kandev/kandev/internal/task/models"
	"golang.org/x/crypto/ssh"
)

type managedSweepWiringRepo struct{ sshOrphanSweepWiringRepo }

func (*managedSweepWiringRepo) ListSSHExecutorsForReachability(context.Context) ([]*models.Executor, error) {
	return []*models.Executor{{ID: "managed-executor", Type: models.ExecutorTypeSSH, Config: map[string]string{"ssh_connection_id": "connection"}}}, nil
}
func (*managedSweepWiringRepo) GetExecutorReachability(context.Context, string) (*models.ExecutorReachability, error) {
	return &models.ExecutorReachability{State: models.ExecutorReachabilityStateReachable}, nil
}

type managedSweepOpener struct{ opened chan string }

func (m *managedSweepOpener) OpenExecutor(_ context.Context, id string) (*ssh.Client, *sshtransport.SSHTarget, func(), error) {
	m.opened <- id
	return nil, nil, nil, sshconnections.ErrDisabled
}
func TestManagedSSHOrphanSweepComposition(t *testing.T) {
	managed := &managedSweepOpener{opened: make(chan string, 1)}
	var cleanup func() error
	scheduler := startSSHOrphanSweepScheduler(context.Background(), &managedSweepWiringRepo{}, nil, logger.Default(), func(fn func() error) func() error { cleanup = fn; return fn }, nil, managed)
	t.Cleanup(scheduler.Stop)
	select {
	case id := <-managed.opened:
		if id != "managed-executor" {
			t.Fatalf("opened executor %q", id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("composition did not forward managed connection capability")
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
}
