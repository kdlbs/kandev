package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

// slowProvisionFake blocks ProvisionExecutorEnvironment the way a provider
// that sleeps before returning does.
type slowProvisionFake struct {
	pluginExecutorOperationsFake
	entered chan struct{}
	release chan struct{}
}

func (f *slowProvisionFake) ProvisionExecutorEnvironment(ctx context.Context, req *pluginsdk.ProvisionExecutorEnvironmentRequest) (*pluginsdk.ProvisionExecutorEnvironmentResponse, error) {
	close(f.entered)
	select {
	case <-f.release:
	case <-ctx.Done():
	}
	return f.pluginExecutorOperationsFake.ProvisionExecutorEnvironment(ctx, req)
}

// Archiving a task while its plugin launch is still provisioning must not let
// the environment cleanup conclude the allocation is absent: the provider has
// not published the resource yet, but it will.
func TestEnvironmentCleanupDuringProvisionDefersInsteadOfSettlingAbsent(t *testing.T) {
	operations := &slowProvisionFake{
		pluginExecutorOperationsFake: pluginExecutorOperationsFake{
			destroyResponse: &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true},
			recoverResponse: &pluginsdk.RecoverExecutorOperationResponse{Outcome: pluginExecutorPhaseAbsent},
			provisionErr:    context.Canceled,
		},
		entered: make(chan struct{}), release: make(chan struct{}),
	}
	record := pluginExecutorRecoveryRecord(t, pluginExecutorPhaseAllocating, nil)
	store := &pluginExecutorInventoryStoreFake{
		record:  record,
		session: &models.TaskSession{ID: "session-plugin-recovery", TaskID: "task-plugin-recovery", State: models.TaskSessionStateStarting},
	}
	loader := &pluginExecutorRecoveryProfileLoaderFake{profile: models.ExecutorProviderLaunchProfile{
		Provider: testPluginExecutorLaunchProvider(), ProfileID: "profile-plugin-recovery",
	}}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.SetRecoveryDependencies(loader, store)

	request := pluginExecutorLaunchRequest(testPluginExecutorLaunchProvider())
	request.InstanceID, request.TaskID, request.SessionID = record.AgentExecutionID, record.TaskID, record.SessionID
	request.CheckpointRuntimeInventory = func(context.Context, map[string]interface{}) error { return nil }
	request.ReleaseRuntimeInventory = func(context.Context) error { return nil }
	launched := make(chan struct{})
	go func() {
		defer close(launched)
		_, _ = runtime.CreateInstance(context.Background(), request)
	}()
	select {
	case <-operations.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("provision was never reached")
	}

	err := destroyClaimEnvironment(runtime)
	if err == nil {
		t.Fatal("cleanup reported success while the launch was still provisioning")
	}
	if store.deleted {
		t.Fatal("cleanup removed the inventory row of an in-flight launch")
	}
	if operations.destroyRequest != nil {
		t.Fatal("cleanup destroyed an environment that was not yet recorded")
	}
	close(operations.release)
	<-launched
}
