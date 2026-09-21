package executor

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// Reload environment sources independently of the immutable workload snapshot.
// Secret references remain unresolved until lifecycle's launch checkpoint.
func (e *Executor) restoreKubernetesProfileEnvironment(
	ctx context.Context, config *executorConfig, recorded map[string]interface{},
) error {
	profileID, _ := recorded["executor_profile_id"].(string)
	if profileID == "" {
		return nil
	}
	profile, err := e.repo.GetExecutorProfile(ctx, profileID)
	if errors.Is(err, repoerrors.ErrExecutorProfileNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("resume Kubernetes executor: load recorded profile environment: %w", err)
	}
	if profile == nil {
		return nil
	}
	if profile.ExecutorID != config.ExecutorID {
		return errors.New("resume Kubernetes executor: recorded profile belongs to a different executor")
	}
	config.ProfileEnvVars = append([]models.ProfileEnvVar(nil), profile.EnvVars...)
	return nil
}
