package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

func TestPluginExecutorInspectionDoesNotExtendRecordedExpiry(t *testing.T) {
	resource := &pluginsdk.ExecutorResourceDescriptor{
		ResourceHandle: "resource-expiry", StateJson: `{"resource":"one"}`, StateVersion: 1,
	}
	record := pluginExecutorRecoveryRecord(t, "ready", resource)
	inventory, err := decodePluginExecutorInventory(record.Metadata)
	if err != nil {
		t.Fatalf("decode inventory: %v", err)
	}
	inventory.ExpiresAt = "2026-09-26T14:00:00Z"
	store := &pluginExecutorInventoryStoreFake{record: record}
	runtime := NewPluginRemoteExecutor(&pluginExecutorOperationsFake{}, newTestLogger())
	runtime.SetRecoveryDependencies(&pluginExecutorRecoveryProfileLoaderFake{profile: models.ExecutorProviderLaunchProfile{
		Provider: testPluginExecutorLaunchProvider(), ProfileID: inventory.ProfileID,
	}}, store)

	if err := runtime.applyPluginExecutorInspection(context.Background(), record, &inventory,
		&pluginsdk.InspectExecutorEnvironmentResponse{State: "running", ExpiresAt: "2026-09-27T14:00:00Z"}); err != nil {
		t.Fatalf("apply inspection: %v", err)
	}
	if inventory.ExpiresAt != "2026-09-26T14:00:00Z" {
		t.Fatalf("inspection extended expiry to %q", inventory.ExpiresAt)
	}
	persisted, err := decodePluginExecutorInventory(store.record.Metadata)
	if err != nil || persisted.ExpiresAt != "2026-09-26T14:00:00Z" {
		t.Fatalf("persisted expiry = %q, err=%v", persisted.ExpiresAt, err)
	}
}

func TestPluginExecutorExpiry(t *testing.T) {
	resource := &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-expiry", StateJson: `{"resource":"one"}`, StateVersion: 1}
	record := pluginExecutorRecoveryRecord(t, "ready", resource)
	inventory, err := decodePluginExecutorInventory(record.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	inventory.ExpiresAt = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	inventory.Capabilities.Retention = "bounded"
	record.Metadata[MetadataKeyPluginExecutor] = inventory
	store := &pluginExecutorInventoryStoreFake{record: record}
	operations := &pluginExecutorOperationsFake{inspectResponses: []*pluginsdk.InspectExecutorEnvironmentResponse{{State: "absent", Reason: "expired"}}}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.SetRecoveryDependencies(&pluginExecutorRecoveryProfileLoaderFake{profile: models.ExecutorProviderLaunchProfile{
		Provider: testPluginExecutorLaunchProvider(), ProfileID: inventory.ProfileID,
	}}, store)

	status, err := runtime.GetEnvironmentStatus(context.Background(), record)
	if err != nil {
		t.Fatalf("GetEnvironmentStatus(): %v", err)
	}
	if status.State != "expired" || status.Retention != "bounded" || !status.DeadlinePassed || len(operations.inspectRequests) != 1 {
		t.Fatalf("status=%+v inspect_count=%d", status, len(operations.inspectRequests))
	}
	persisted, err := decodePluginExecutorInventory(store.record.Metadata)
	if err != nil || persisted.Phase != "expired" || persisted.Resource != nil || persisted.ExpiresAt == "" {
		t.Fatalf("expired inventory=%+v err=%v", persisted, err)
	}
	if err := runtime.rejectExpiredPluginExecutorEnvironment(context.Background(), inventory.EnvironmentID, inventory.EnvironmentGeneration); err == nil {
		t.Fatal("expired environment was eligible for a replacement allocation")
	}
	if record.ResumeToken != "resume-preserved" || record.LastMessageUUID != "message-preserved" {
		t.Fatalf("expiry changed conversation history: %+v", record)
	}
}

func TestPluginExecutorUnknownRetentionAndInspectionFailure(t *testing.T) {
	base := testPluginExecutorLaunchProvider().Capabilities
	if got := intersectPluginExecutorCapabilities(base, nil).Retention; got != "unknown" {
		t.Fatalf("missing instance retention = %q", got)
	}
	if got := intersectPluginExecutorCapabilities(base, &pluginsdk.ExecutorProviderCapabilities{Retention: "persistent"}, "bounded").Retention; got != "bounded" {
		t.Fatalf("bounded resource retention was elevated to %q", got)
	}
	restrictedBase := base
	restrictedBase.Retention = "bounded"
	if got := intersectPluginExecutorCapabilities(restrictedBase, &pluginsdk.ExecutorProviderCapabilities{Retention: "persistent"}, "persistent").Retention; got != "bounded" {
		t.Fatalf("provider retention ceiling was elevated to %q", got)
	}

	resource := &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-expiry", StateJson: `{"resource":"one"}`, StateVersion: 1}
	record := pluginExecutorRecoveryRecord(t, "ready", resource)
	inventory, err := decodePluginExecutorInventory(record.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	inventory.ExpiresAt = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	inventory.LastInspectionState = "unknown"
	inventory.Capabilities.Retention = "unknown"
	inventory.ExpiryCheckAt = time.Now().UTC().Format(time.RFC3339Nano)
	record.Metadata[MetadataKeyPluginExecutor] = inventory
	store := &pluginExecutorInventoryStoreFake{record: record}
	operations := &pluginExecutorOperationsFake{inspectErr: errors.New("provider is unreachable")}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.SetRecoveryDependencies(&pluginExecutorRecoveryProfileLoaderFake{profile: models.ExecutorProviderLaunchProfile{
		Provider: testPluginExecutorLaunchProvider(), ProfileID: inventory.ProfileID,
	}}, store)

	status, err := runtime.GetEnvironmentStatus(context.Background(), record)
	if err != nil || status.State != "unknown" || !status.DeadlinePassed || status.Retention != "unknown" {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	if len(operations.inspectRequests) != 0 {
		t.Fatalf("expiry-check cooldown failed, made %d inspections", len(operations.inspectRequests))
	}
}

func TestPluginExecutorExpiryInspectionRetriesAreBounded(t *testing.T) {
	resource := &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-expiry", StateJson: `{"resource":"one"}`, StateVersion: 1}
	record := pluginExecutorRecoveryRecord(t, "ready", resource)
	inventory, err := decodePluginExecutorInventory(record.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	inventory.ExpiresAt = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	record.Metadata[MetadataKeyPluginExecutor] = inventory
	store := &pluginExecutorInventoryStoreFake{record: record}
	operations := &pluginExecutorOperationsFake{inspectErr: errors.New("provider is unreachable")}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.SetRecoveryDependencies(&pluginExecutorRecoveryProfileLoaderFake{profile: models.ExecutorProviderLaunchProfile{
		Provider: testPluginExecutorLaunchProvider(), ProfileID: inventory.ProfileID,
	}}, store)

	status, err := runtime.GetEnvironmentStatus(context.Background(), record)
	if err != nil || status.State != "unknown" || len(operations.inspectRequests) != 2 {
		t.Fatalf("status=%+v inspect_count=%d err=%v", status, len(operations.inspectRequests), err)
	}
	if next, err := runtime.GetEnvironmentStatus(context.Background(), store.record); err != nil || next.State != "unknown" || len(operations.inspectRequests) != 2 {
		t.Fatalf("cooldown status=%+v inspect_count=%d err=%v", next, len(operations.inspectRequests), err)
	}
}

type barrierPluginExecutorOperations struct {
	*pluginExecutorOperationsFake
	entered chan struct{}
	release chan struct{}
}

func (o *barrierPluginExecutorOperations) InspectExecutorEnvironment(context.Context, *pluginsdk.InspectExecutorEnvironmentRequest) (*pluginsdk.InspectExecutorEnvironmentResponse, error) {
	close(o.entered)
	<-o.release
	return &pluginsdk.InspectExecutorEnvironmentResponse{State: "running"}, nil
}

func TestPluginExecutorStaleInspectionCannotResurrectCleanedInventory(t *testing.T) {
	resource := &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-expiry", StateJson: `{"resource":"one"}`, StateVersion: 1}
	record := pluginExecutorRecoveryRecord(t, "ready", resource)
	inventory, err := decodePluginExecutorInventory(record.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	inventory.ExpiresAt = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	record.Metadata[MetadataKeyPluginExecutor] = inventory
	store := &pluginExecutorInventoryStoreFake{record: record}
	staleInspectionRecord := *record
	staleInspectionRecord.Metadata = clonePluginExecutorMetadata(record.Metadata)
	operations := &barrierPluginExecutorOperations{
		pluginExecutorOperationsFake: &pluginExecutorOperationsFake{},
		entered:                      make(chan struct{}),
		release:                      make(chan struct{}),
	}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.SetRecoveryDependencies(&pluginExecutorRecoveryProfileLoaderFake{profile: models.ExecutorProviderLaunchProfile{
		Provider: testPluginExecutorLaunchProvider(), ProfileID: inventory.ProfileID,
	}}, store)
	statusResult := make(chan error, 1)
	go func() {
		_, statusErr := runtime.GetEnvironmentStatus(context.Background(), &staleInspectionRecord)
		statusResult <- statusErr
	}()
	select {
	case <-operations.entered:
	case <-time.After(time.Second):
		t.Fatal("provider inspection did not reach the barrier")
	}

	cleanupInventory, err := decodePluginExecutorInventory(store.record.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	cleanupInventory.Phase = "absent"
	cleanupInventory.Resource = nil
	if err := runtime.checkpointPluginExecutorRecord(context.Background(), store.record, cleanupInventory); err != nil {
		t.Fatalf("cleanup checkpoint: %v", err)
	}
	close(operations.release)
	select {
	case err := <-statusResult:
		if !errors.Is(err, models.ErrExecutionRotated) {
			t.Fatalf("stale inspection checkpoint error = %v, want ErrExecutionRotated", err)
		}
	case <-time.After(time.Second):
		t.Fatal("status inspection did not return after releasing provider barrier")
	}
	persisted, err := decodePluginExecutorInventory(store.record.Metadata)
	if err != nil || persisted.Phase != "absent" || persisted.Resource != nil {
		t.Fatalf("cleanup state after stale inspection = %+v, err=%v", persisted, err)
	}
}
