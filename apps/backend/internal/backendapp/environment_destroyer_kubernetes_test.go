package backendapp

import (
	"context"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestKubernetesEnvironmentDestroyerWiring(t *testing.T) {
	_, ok := interface{}(&environmentDestroyerAdapter{}).(interface {
		DestroyKubernetesEnvironment(context.Context, *models.TaskEnvironment) error
	})
	require.True(t, ok, "task cleanup adapter must expose shared Kubernetes teardown")
}
