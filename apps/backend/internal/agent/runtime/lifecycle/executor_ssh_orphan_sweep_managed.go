package lifecycle

import (
	"context"

	"github.com/kandev/kandev/internal/sshconnections"
	"github.com/kandev/kandev/internal/sshtransport"
	"github.com/kandev/kandev/internal/task/models"
	"golang.org/x/crypto/ssh"
)

// ManagedSSHExecutorConnection opens the saved executor route and holds its
// transport references until the returned cleanup function is called.
type ManagedSSHExecutorConnection interface {
	OpenExecutor(context.Context, string) (*ssh.Client, *sshtransport.SSHTarget, func(), error)
}

func (s *OrphanSweepScheduler) openConnection(ctx context.Context, executor *models.Executor) (*ssh.Client, func(), error) {
	if s.managed == nil {
		return nil, nil, sshconnections.ErrDisabled
	}
	client, _, cleanup, err := s.managed.OpenExecutor(ctx, executor.ID)
	return client, cleanup, err
}
