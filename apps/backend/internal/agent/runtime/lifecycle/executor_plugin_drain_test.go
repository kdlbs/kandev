package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

const drainTestCleanupClaim = "operation-plugin-recovery:cleanup"

type pluginDrainerFake struct {
	calls     []string
	deadlines []time.Duration
	receipt   *agentctl.RuntimeDrainReceipt
	err       error
	events    *[]string
}

func (f *pluginDrainerFake) drain(_ context.Context, _ agentctl.ConnectionLeaseResolver, _ *logger.Logger, token, claim string, deadline time.Duration) (*agentctl.RuntimeDrainReceipt, error) {
	f.calls = append(f.calls, claim)
	f.deadlines = append(f.deadlines, deadline)
	if f.events != nil {
		*f.events = append(*f.events, "drain")
	}
	if token == "" {
		return nil, errors.New("drain was called without the agentctl credential")
	}
	if f.err != nil {
		return nil, f.err
	}
	if f.receipt != nil {
		return f.receipt, nil
	}
	return &agentctl.RuntimeDrainReceipt{RequestID: claim, Sealed: true, Drained: true, CompletedAt: time.Date(2026, 10, 28, 1, 2, 3, 0, time.UTC)}, nil
}

func drainRuntimeFixture(t *testing.T, negotiated []string, mutate func(*pluginExecutorInventory, *models.ExecutorRunning)) (*PluginRemoteExecutor, *pluginExecutorOperationsFake, *pluginExecutorInventoryStoreFake, *pluginDrainerFake) {
	t.Helper()
	resource := &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-recovery", StateJson: `{"resource":"one"}`, Platform: "linux-amd64", StateVersion: 1}
	runtime, operations, store := claimSettlementRuntime(t, "ready", nil, resource)
	inventory, err := decodePluginExecutorInventory(store.record.Metadata)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	inventory.NegotiatedCapabilities = negotiated
	inventory.RuntimeIdentity = "execution-plugin-recovery"
	if mutate != nil {
		mutate(&inventory, store.record)
	}
	store.record.Metadata = map[string]interface{}{MetadataKeyPluginExecutor: inventory}
	drainer := &pluginDrainerFake{}
	runtime.drainRuntime = drainer.drain
	return runtime, operations, store, drainer
}

func TestDrainRunsBeforePersistenceAndPersistenceBeforeDestroy(t *testing.T) {
	var events []string
	runtime, operations, store, drainer := drainRuntimeFixture(t, []string{"drain_receipt@1"}, nil)
	drainer.events = &events
	operations.beforeDestroy = func() {
		events = append(events, "destroy")
		inventory, err := decodePluginExecutorInventory(store.record.Metadata)
		if err != nil || inventory.DrainOutcome != "drained" || inventory.DrainClaim != drainTestCleanupClaim {
			t.Errorf("destroy ran before the drain outcome was persisted: %+v err=%v", inventory, err)
		}
	}
	if err := destroyClaimEnvironment(runtime); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if fmt.Sprint(events) != "[drain destroy]" {
		t.Fatalf("event order = %v, want drain then destroy", events)
	}
	request := operations.destroyRequest
	if request.GetDrainOutcome() != "drained" || request.GetDrainReceipt().GetReceiptId() == "" {
		t.Fatalf("destroy request = outcome %q receipt %#v, want drained with a receipt", request.GetDrainOutcome(), request.GetDrainReceipt())
	}
	receipt := request.GetDrainReceipt()
	if receipt.GetCleanupClaim() != drainTestCleanupClaim || receipt.GetRuntimeIdentity() != "execution-plugin-recovery" {
		t.Fatalf("receipt = %#v, want it bound to the cleanup claim and runtime identity", receipt)
	}
	if drainer.deadlines[0] != 60*time.Second {
		t.Fatalf("drain deadline = %v, want 60s", drainer.deadlines[0])
	}
}

func TestDrainOutcomeMapping(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		mutate func(*pluginExecutorInventory, *models.ExecutorRunning)
		want   string
		called bool
	}{
		{"refused connection", &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}, nil, "unreachable", true},
		{"identity mismatch", &agentctl.RuntimeDrainRejectedError{StatusCode: http.StatusConflict}, nil, "unreachable", true},
		{"credential no longer accepted", &agentctl.RuntimeDrainRejectedError{StatusCode: http.StatusUnauthorized}, nil, "unreachable", true},
		{"lost reply", errors.New("EOF"), nil, "uncertain", true},
		{"deadline", context.DeadlineExceeded, nil, "uncertain", true},
		{"agentctl drain failed", &agentctl.RuntimeDrainRejectedError{StatusCode: http.StatusInternalServerError}, nil, "uncertain", true},
		{"no instance recorded", nil, func(i *pluginExecutorInventory, _ *models.ExecutorRunning) { i.InstancePort = 0 }, "not_attempted", false},
		{"no credential saved", nil, func(_ *pluginExecutorInventory, r *models.ExecutorRunning) { r.TransientAuthToken = "" }, "not_attempted", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runtime, operations, store, drainer := drainRuntimeFixture(t, []string{"drain_receipt@1"}, tc.mutate)
			drainer.err = tc.err
			if err := destroyClaimEnvironment(runtime); err != nil {
				t.Fatalf("destroy: %v", err)
			}
			if got := operations.destroyRequest.GetDrainOutcome(); got != tc.want {
				t.Fatalf("drain_outcome = %q, want %q", got, tc.want)
			}
			if operations.destroyRequest.GetDrainReceipt() != nil {
				t.Fatalf("a %s outcome carried a receipt: %#v", tc.want, operations.destroyRequest.GetDrainReceipt())
			}
			if (len(drainer.calls) > 0) != tc.called {
				t.Fatalf("drain calls = %v, want called=%v", drainer.calls, tc.called)
			}
			_ = store
		})
	}
}

func TestLostDrainReplyKeepsCleanupPendingUntilProviderConfirmsAbsence(t *testing.T) {
	runtime, operations, store, drainer := drainRuntimeFixture(t, []string{"drain_receipt@1"}, nil)
	drainer.err = errors.New("EOF")
	operations.destroyResponse = &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: false}
	if err := destroyClaimEnvironment(runtime); err == nil {
		t.Fatal("an unconfirmed destroy after an uncertain drain settled")
	}
	inventory, err := decodePluginExecutorInventory(store.record.Metadata)
	if err != nil || inventory.Phase != "cleanup_pending" || inventory.DrainOutcome != "uncertain" || inventory.DrainReceiptID != "" {
		t.Fatalf("inventory = %+v err=%v, want cleanup_pending with an uncertain outcome and no receipt", inventory, err)
	}
	if store.deleted {
		t.Fatal("the inventory row was removed without confirmed absence")
	}
}

func TestDrainReceiptIsNotAbsenceEvidence(t *testing.T) {
	runtime, operations, store, _ := drainRuntimeFixture(t, []string{"drain_receipt@1"}, nil)
	operations.destroyResponse = &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: false}
	if err := destroyClaimEnvironment(runtime); err == nil {
		t.Fatal("a drain receipt settled an environment the provider did not confirm absent")
	}
	if store.deleted {
		t.Fatal("the inventory row was removed on the strength of a drain receipt")
	}
}

func TestSameClaimRetryReusesTheStoredOutcomeWithoutDrainingAgain(t *testing.T) {
	runtime, operations, store, drainer := drainRuntimeFixture(t, []string{"drain_receipt@1"}, nil)
	operations.destroyResponse = &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: false}
	_ = destroyClaimEnvironment(runtime)
	first := operations.destroyRequest.GetDrainReceipt().GetReceiptId()
	_ = destroyClaimEnvironment(runtime)
	if len(drainer.calls) != 1 {
		t.Fatalf("drain calls = %d, want the stored outcome reused under the same claim", len(drainer.calls))
	}
	if again := operations.destroyRequest.GetDrainReceipt().GetReceiptId(); again != first || first == "" {
		t.Fatalf("replayed receipt id = %q, want %q", again, first)
	}
	_ = store
}

func TestSuccessorClaimRunsAFreshDrain(t *testing.T) {
	runtime, operations, store, drainer := drainRuntimeFixture(t, []string{"drain_receipt@1"}, func(i *pluginExecutorInventory, _ *models.ExecutorRunning) {
		i.DrainClaim = "operation-plugin-recovery:recovery"
		i.DrainOutcome = "drained"
		i.DrainReceiptID = pluginDrainReceiptID(*i, "operation-plugin-recovery:recovery")
	})
	if err := destroyClaimEnvironment(runtime); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if len(drainer.calls) != 1 || drainer.calls[0] != drainTestCleanupClaim {
		t.Fatalf("drain calls = %v, want one fresh drain under this claim", drainer.calls)
	}
	if got := operations.destroyRequest.GetDrainReceipt().GetCleanupClaim(); got != drainTestCleanupClaim {
		t.Fatalf("receipt claim = %q, want the active cleanup claim", got)
	}
	_ = store
}

func TestReceiptWithAnotherBindingIsRejected(t *testing.T) {
	cases := map[string]func(*pluginExecutorInventory){
		"another environment": func(i *pluginExecutorInventory) { i.EnvironmentID = "other" },
		"another generation":  func(i *pluginExecutorInventory) { i.EnvironmentGeneration++ },
		"another runtime":     func(i *pluginExecutorInventory) { i.RuntimeIdentity = "other-runtime" },
		"an earlier claim's id": func(i *pluginExecutorInventory) {
			i.DrainReceiptID = pluginDrainReceiptID(*i, "operation-plugin-recovery:recovery")
		},
		"an empty receipt id": func(i *pluginExecutorInventory) { i.DrainReceiptID = "" },
	}
	for name, corrupt := range cases {
		t.Run(name, func(t *testing.T) {
			inventory := pluginExecutorInventory{
				EnvironmentID: "environment-plugin-recovery", EnvironmentGeneration: 7, RuntimeIdentity: "execution-plugin-recovery",
				DrainClaim: drainTestCleanupClaim, DrainOutcome: "drained",
			}
			inventory.DrainReceiptID = pluginDrainReceiptID(inventory, drainTestCleanupClaim)
			if _, ok := storedPluginDrainDecision(inventory, drainTestCleanupClaim); !ok {
				t.Fatal("the untouched receipt was not trusted")
			}
			corrupt(&inventory)
			if decision, ok := storedPluginDrainDecision(inventory, drainTestCleanupClaim); ok {
				t.Fatalf("a receipt with another binding was trusted: %+v", decision)
			}
		})
	}
}

func TestProviderWithoutDrainReceiptNeverDrains(t *testing.T) {
	runtime, operations, _, drainer := drainRuntimeFixture(t, nil, nil)
	if err := destroyClaimEnvironment(runtime); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if len(drainer.calls) != 0 || operations.destroyRequest.GetDrainOutcome() != "" || operations.destroyRequest.GetDrainReceipt() != nil {
		t.Fatalf("a provider without drain_receipt saw a drain: calls=%v request=%#v", drainer.calls, operations.destroyRequest)
	}
}

func TestDrainFailsClosedWhenTheOutcomeCannotBePersisted(t *testing.T) {
	runtime, operations, store, _ := drainRuntimeFixture(t, []string{"drain_receipt@1"}, nil)
	store.checkpointErr = models.ErrExecutionRotated
	if err := destroyClaimEnvironment(runtime); err == nil {
		t.Fatal("destroy proceeded although the drain outcome was not persisted")
	}
	if operations.destroyRequest != nil {
		t.Fatal("the provider was called before the drain outcome was persisted")
	}
}

func TestDestroyUsesTheProviderDeadlineAndDefaultsToThirtySeconds(t *testing.T) {
	for _, tc := range []struct {
		seconds int
		want    time.Duration
	}{{0, 30 * time.Second}, {120, 120 * time.Second}} {
		runtime, operations, _, _ := drainRuntimeFixture(t, []string{"drain_receipt@1"}, nil)
		runtime.profileLoader = &pluginExecutorRecoveryProfileLoaderFake{profile: models.ExecutorProviderLaunchProfile{
			Provider:  withDestroyTimeout(testPluginExecutorLaunchProvider(), tc.seconds),
			ProfileID: "profile-plugin-recovery",
		}}
		start := time.Now()
		if err := destroyClaimEnvironment(runtime); err != nil {
			t.Fatalf("destroy: %v", err)
		}
		remaining := operations.destroyDeadline.Sub(start)
		if remaining > tc.want+time.Second || remaining < tc.want-5*time.Second {
			t.Fatalf("destroy deadline in %v for %ds, want about %v", remaining, tc.seconds, tc.want)
		}
	}
}

func withDestroyTimeout(provider models.ExecutorProvider, seconds int) models.ExecutorProvider {
	provider.DestroyTimeoutSeconds = seconds
	return provider
}

func TestProviderCannotWriteTheHostOnlyDrainReceiptPhase(t *testing.T) {
	manager, store, request := claimCheckpointFixture(t, []string{"destroy_checkpoints@1", "drain_receipt@1"})
	request.Phase = "drain_receipt"
	if err := manager.CheckpointExecutorResource(context.Background(), request); err == nil {
		t.Fatal("a provider checkpoint with the drain_receipt phase was accepted")
	}
	if store.writes != 0 {
		t.Fatalf("a refused drain_receipt checkpoint wrote %d times", store.writes)
	}
}

func TestBootstrapDisablesTheIdleReaperOnlyForDrainReceiptProviders(t *testing.T) {
	launch := func(negotiated []string) *pluginExecutorLaunchState {
		return &pluginExecutorLaunchState{
			request: &ExecutorCreateRequest{InstanceID: "execution-1"},
			apiURL:  "https://kandev.example", nonce: "nonce-1",
			inventory: pluginExecutorInventory{NegotiatedCapabilities: negotiated},
		}
	}
	declared, err := pluginExecutorBootstrapJSON(launch([]string{"drain_receipt@1"}))
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(declared), &fields); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if value, present := fields["idle_timeout_seconds"]; !present || value != float64(0) {
		t.Fatalf("idle_timeout_seconds = %v (present=%v), want an explicit 0", value, present)
	}
	for _, negotiated := range [][]string{nil, {"workspace_descriptor@1"}} {
		other, err := pluginExecutorBootstrapJSON(launch(negotiated))
		if err != nil {
			t.Fatalf("bootstrap: %v", err)
		}
		if strings.Contains(other, "idle_timeout_seconds") {
			t.Fatalf("a provider without drain_receipt received an idle timeout: %s", other)
		}
	}
}

func TestOrdinaryStopAndBackendShutdownNeverSealTheRuntime(t *testing.T) {
	for _, reason := range []string{"", "session stopped", StopReasonBackendShutdown} {
		t.Run("reason="+reason, func(t *testing.T) {
			runtime, operations, _, drainer := drainRuntimeFixture(t, []string{"drain_receipt@1"}, nil)
			record := runtimeRecordForStop(t, runtime)
			err := runtime.StopInstance(context.Background(), &ExecutorInstance{
				InstanceID: record.AgentExecutionID, TaskID: record.TaskID, SessionID: record.SessionID,
				RuntimeName: "plugin_remote", StopReason: reason, Metadata: record.Metadata,
			}, false)
			if err != nil {
				t.Fatalf("StopInstance: %v", err)
			}
			if len(drainer.calls) != 0 || operations.destroyRequest != nil {
				t.Fatalf("a %q stop drained or destroyed the environment: drains=%v destroy=%#v", reason, drainer.calls, operations.destroyRequest)
			}
		})
	}
}

func runtimeRecordForStop(t *testing.T, runtime *PluginRemoteExecutor) *models.ExecutorRunning {
	t.Helper()
	store, ok := runtime.inventoryStore.(*pluginExecutorInventoryStoreFake)
	if !ok {
		t.Fatalf("inventory store = %T", runtime.inventoryStore)
	}
	return store.record
}

func TestTaskTeardownStopDrainsBeforeDestroy(t *testing.T) {
	runtime, operations, _, drainer := drainRuntimeFixture(t, []string{"drain_receipt@1"}, nil)
	record := runtimeRecordForStop(t, runtime)
	if err := runtime.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID: record.AgentExecutionID, TaskID: record.TaskID, SessionID: record.SessionID,
		RuntimeName: "plugin_remote", StopReason: StopReasonTaskDeleted, Metadata: record.Metadata,
	}, false); err != nil {
		t.Fatalf("StopInstance: %v", err)
	}
	if len(drainer.calls) != 1 || operations.destroyRequest.GetDrainOutcome() != "drained" {
		t.Fatalf("task teardown drains=%v destroy outcome=%q, want one drain then a drained destroy", drainer.calls, operations.destroyRequest.GetDrainOutcome())
	}
}
