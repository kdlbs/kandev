package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type kubernetesInventoryEnvRepo struct {
	stubEnvRepo
	inventory    *models.KubernetesEnvironment
	inventoryErr error
}

func (r *kubernetesInventoryEnvRepo) GetKubernetesEnvironment(context.Context, string) (*models.KubernetesEnvironment, error) {
	return r.inventory, r.inventoryErr
}

type kubernetesEnvironmentDestroyer struct {
	stubDestroyer
	deleted string
	err     error
}

func (d *kubernetesEnvironmentDestroyer) DestroyKubernetesEnvironment(_ context.Context, env *models.TaskEnvironment) error {
	d.deleted = env.ID
	return d.err
}

func TestKubernetesEnvironmentCleanupUsesTaskOwner(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "successful teardown", true: "failed teardown retains inventory"}[fail], func(t *testing.T) {
			env := &models.TaskEnvironment{ID: "env-kube", TaskID: "task-kube", ExecutorType: "k8s", OwnershipGeneration: 1}
			destroyer := &kubernetesEnvironmentDestroyer{}
			if fail {
				destroyer.err = errors.New("cluster unavailable")
			}
			service := &Service{taskEnvironments: &kubernetesInventoryEnvRepo{inventory: &models.KubernetesEnvironment{EnvironmentID: env.ID, TaskID: env.TaskID}}, envDestroyer: destroyer}
			err := service.teardownEnvironmentRuntimeResources(context.Background(), env)
			require.Equal(t, env.ID, destroyer.deleted)
			require.ErrorIs(t, err, destroyer.err)
		})
	}
}

func (r *kubernetesInventoryEnvRepo) ListKubernetesEnvironments(context.Context) ([]*models.KubernetesEnvironment, error) {
	return []*models.KubernetesEnvironment{r.inventory}, r.inventoryErr
}

func TestKubernetesRetainedEnvironmentBlocksExecutorDeletion(t *testing.T) {
	service := &Service{taskEnvironments: &kubernetesInventoryEnvRepo{inventory: &models.KubernetesEnvironment{Metadata: map[string]interface{}{"kubernetes_resource_executor_id": "executor-kube"}}}}
	checker, ok := interface{}(service).(interface {
		hasKubernetesEnvironmentInventory(context.Context, string) (bool, error)
	})
	require.True(t, ok, "executor mutation guard must include retained environments")
	retained, err := checker.hasKubernetesEnvironmentInventory(context.Background(), "executor-kube")
	require.NoError(t, err)
	require.True(t, retained)
}
