package executor

import (
	"testing"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

func TestExecutorTypeToBackendMapsKubernetes(t *testing.T) {
	t.Parallel()

	got := ExecutorTypeToBackend(models.ExecutorTypeKubernetes)
	if got != agentruntime.RuntimeKubernetes {
		t.Fatalf("ExecutorTypeToBackend(k8s) = %q, want k8s", got)
	}
}
