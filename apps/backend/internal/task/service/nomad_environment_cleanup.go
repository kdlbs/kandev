package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

// NomadEnvironmentDestroyer tears down a Nomad environment's allocation and
// checkpoints exact remote absence. It returns an error whenever absence is
// not proven, so the caller keeps the owner rows.
type NomadEnvironmentDestroyer interface {
	DestroyNomadEnvironment(ctx context.Context, environmentID string) error
}

// SetNomadEnvironmentDestroyer wires Nomad allocation teardown.
func (s *Service) SetNomadEnvironmentDestroyer(d NomadEnvironmentDestroyer) {
	s.nomadDestroyer = d
}

// nomadEnvironmentRetained reports whether the environment still has a
// non-absent Nomad inventory row. Absent tombstones do not count.
func (s *Service) nomadEnvironmentRetained(ctx context.Context, env *models.TaskEnvironment) (bool, error) {
	if env == nil || env.ExecutorType != string(models.ExecutorTypeNomad) {
		return false, nil
	}
	inventory, ok := s.taskEnvironments.(interface {
		HasNomadEnvironmentInventory(context.Context, string) (bool, error)
	})
	if !ok {
		return false, nil
	}
	return inventory.HasNomadEnvironmentInventory(ctx, env.ID)
}

func (s *Service) teardownNomadEnvironment(ctx context.Context, env *models.TaskEnvironment) error {
	retained, err := s.nomadEnvironmentRetained(ctx, env)
	if err != nil {
		return fmt.Errorf("read nomad environment inventory %s: %w", env.ID, err)
	}
	if !retained {
		return nil
	}
	if s.nomadDestroyer == nil {
		return errors.New("nomad environment destroyer is unavailable")
	}
	if err := s.nomadDestroyer.DestroyNomadEnvironment(ctx, env.ID); err != nil {
		return fmt.Errorf("destroy nomad environment %s: %w", env.ID, err)
	}
	return nil
}
