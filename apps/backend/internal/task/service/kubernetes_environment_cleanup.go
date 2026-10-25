package service

import (
	"context"
	"errors"
	"fmt"
	agentkubernetes "github.com/kandev/kandev/internal/agent/kubernetes"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

func (s *Service) teardownKubernetesEnvironment(ctx context.Context, env *models.TaskEnvironment) error {
	if env.ExecutorType != string(models.ExecutorTypeKubernetes) {
		return nil
	}
	inventory, ok := s.taskEnvironments.(interface {
		GetKubernetesEnvironment(context.Context, string) (*models.KubernetesEnvironment, error)
	})
	if !ok {
		return nil
	}
	record, err := inventory.GetKubernetesEnvironment(ctx, env.ID)
	if errors.Is(err, models.ErrKubernetesEnvironmentNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if record == nil || record.TaskID != env.TaskID || record.EnvironmentID != env.ID {
		return models.ErrWorkspaceReuseUnsafe
	}
	destroyer, ok := s.envDestroyer.(interface {
		DestroyKubernetesEnvironment(context.Context, *models.TaskEnvironment) error
	})
	if !ok {
		return fmt.Errorf("kubernetes environment destroyer is unavailable")
	}
	return destroyer.DestroyKubernetesEnvironment(ctx, env)
}

func (s *Service) hasKubernetesEnvironmentInventory(ctx context.Context, executorID string) (bool, error) {
	store, ok := s.taskEnvironments.(interface {
		ListKubernetesEnvironments(context.Context) ([]*models.KubernetesEnvironment, error)
	})
	if !ok {
		return false, nil
	}
	records, err := store.ListKubernetesEnvironments(ctx)
	if err != nil {
		return false, err
	}
	for _, record := range records {
		if record == nil {
			continue
		}
		id, _ := record.Metadata[agentkubernetes.MetadataKeyResourceExecutorID].(string)
		if strings.TrimSpace(id) == "" || id == executorID {
			return true, nil
		}
	}
	return false, nil
}
