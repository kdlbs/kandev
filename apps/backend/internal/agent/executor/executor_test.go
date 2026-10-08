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

func TestExecutorTypeToBackendMapsCursorCloudWithoutStandaloneFallback(t *testing.T) {
	t.Parallel()

	if got := ExecutorTypeToBackend(models.ExecutorTypeCursorCloud); got != agentruntime.RuntimeCursorCloud {
		t.Fatalf("ExecutorTypeToBackend(cursor_cloud) = %q, want %q", got, agentruntime.RuntimeCursorCloud)
	}
}

func TestExecutorTypeToBackendDoesNotFallBackForUnknownOrPluginRemote(t *testing.T) {
	if got := ExecutorTypeToBackend(models.ExecutorType("unknown-provider")); got != NameUnknown {
		t.Fatalf("unknown executor mapped to %q, want NameUnknown", got)
	}
	if got := ExecutorTypeToBackend(models.ExecutorTypePluginRemote); got != NamePluginRemote {
		t.Fatalf("plugin remote executor mapped to %q, want %q", got, NamePluginRemote)
	}
}

func TestExecutorTypeToBackendPreservesLegacyLocalPCAlias(t *testing.T) {
	if got := ExecutorTypeToBackend(models.ExecutorType("local_pc")); got != NameStandalone {
		t.Fatalf("legacy local_pc executor mapped to %q, want %q", got, NameStandalone)
	}
}
