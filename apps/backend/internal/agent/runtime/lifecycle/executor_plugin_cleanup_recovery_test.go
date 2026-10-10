package lifecycle

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

func TestPluginExecutorResetRecoversMissingCleanupHandle(t *testing.T) {
	resource := &pluginsdk.ExecutorResourceDescriptor{
		ResourceHandle: "recovered-resource", StateJson: `{"resource":"one"}`, Platform: "linux-amd64", StateVersion: 1,
	}
	tests := []struct {
		name        string
		outcome     string
		wantDestroy bool
		wantError   bool
	}{
		{name: "found", outcome: "found", wantDestroy: true},
		{name: "absent", outcome: "absent"},
		{name: "unknown", outcome: "unknown", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			operations := &pluginExecutorOperationsFake{
				recoverResponse: &pluginsdk.RecoverExecutorOperationResponse{Outcome: test.outcome, Resource: resource},
				destroyResponse: &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true},
			}
			loader := &pluginExecutorRecoveryProfileLoaderFake{profile: models.ExecutorProviderLaunchProfile{
				Provider: testPluginExecutorLaunchProvider(), ProfileID: "profile-plugin-recovery",
			}}
			store := &pluginExecutorInventoryStoreFake{record: pluginExecutorRecoveryRecord(t, "cleanup_pending", nil)}
			runtime := NewPluginRemoteExecutor(operations, newTestLogger())
			runtime.SetRecoveryDependencies(loader, store)
			ctx := recoveryclaim.WithTaskCleanupJob(context.Background(), recoveryclaim.TaskCleanupJob{
				ID: "reset-job-plugin-recovery", TaskID: "task-plugin-recovery",
			})

			err := runtime.DestroyTaskEnvironment(ctx, &models.TaskEnvironment{
				ID: "environment-plugin-recovery", TaskID: "task-plugin-recovery", OwnershipGeneration: 7,
			})
			if (err != nil) != test.wantError {
				t.Fatalf("DestroyTaskEnvironment() error = %v, wantError %v", err, test.wantError)
			}
			if operations.recoverRequest == nil {
				t.Fatal("missing-handle cleanup did not recover the provider operation")
			}
			if (operations.destroyRequest != nil) != test.wantDestroy {
				t.Fatalf("destroy request = %#v, wantDestroy %v", operations.destroyRequest, test.wantDestroy)
			}
			if test.wantDestroy && operations.destroyRequest.GetResource().GetResourceHandle() != resource.GetResourceHandle() {
				t.Fatalf("destroyed resource = %q, want recovered resource", operations.destroyRequest.GetResource().GetResourceHandle())
			}
			if test.wantError {
				if !strings.Contains(err.Error(), "unknown") {
					t.Fatalf("unknown result error = %v", err)
				}
				inventory, decodeErr := decodePluginExecutorInventory(store.record.Metadata)
				if decodeErr != nil || inventory.Phase != pluginExecutorPhaseCleanupPending {
					t.Fatalf("unknown operation did not retain cleanup inventory: %+v, %v", inventory, decodeErr)
				}
			}
		})
	}
}
