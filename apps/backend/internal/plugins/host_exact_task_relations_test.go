package plugins

import (
	"context"
	"testing"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

type exactTaskRelationFake struct {
	*fakeTaskWriter
	edges map[string]bool
	calls int
}

func (f *exactTaskRelationFake) AddTaskRelationExact(_ context.Context, input ExactTaskRelationInput) (bool, error) {
	if f.edges == nil {
		f.edges = make(map[string]bool)
	}
	f.calls++
	key := input.TaskID + ":" + input.RelatedTaskID
	if f.edges[key] {
		return true, nil
	}
	f.edges[key] = true
	return false, nil
}

func (f *exactTaskRelationFake) RemoveTaskRelationExact(_ context.Context, input ExactTaskRelationInput) (bool, error) {
	if f.edges == nil {
		f.edges = make(map[string]bool)
	}
	f.calls++
	key := input.TaskID + ":" + input.RelatedTaskID
	if !f.edges[key] {
		return true, nil
	}
	delete(f.edges, key)
	return false, nil
}

func TestExactTaskRelationCommandsRecoverAndRoundTrip(t *testing.T) {
	host, _ := newExactTaskCommandHost(t)
	writer := &exactTaskRelationFake{fakeTaskWriter: &fakeTaskWriter{}}
	host.taskWriter = writer
	manager, ok := pluginsdk.HostTaskCommands(host)
	if !ok {
		t.Fatal("plugin Host does not expose exact task commands")
	}
	digest := ManifestCapabilityDigest(host.service.installedRecordByInstallationID(host.installationID).Manifest)
	input := pluginsdk.ExactTaskRelation{
		RequestID: "relation-request", WorkspaceID: "workspace-task-commands", TaskID: "task-a",
		RelatedTaskID: "task-b", ExpectedTaskResourceVersion: "2026-09-26T10:00:00Z",
		ExpectedRelatedResourceVersion: "2026-09-26T10:00:00Z",
		IdempotencyKey:                 "relation-add", ApprovalRevision: 1, ManifestDigest: digest,
	}

	added, err := manager.AddRelation(context.Background(), input)
	if err != nil || added == nil || added.Status != pluginsdk.CommandApplied {
		t.Fatalf("AddRelation = result:%+v err:%v", added, err)
	}
	replayed, err := manager.AddRelation(context.Background(), input)
	if err != nil || replayed == nil || replayed.Status != pluginsdk.CommandApplied || writer.calls != 1 {
		t.Fatalf("replayed AddRelation = result:%+v calls:%d err:%v", replayed, writer.calls, err)
	}

	duplicate := input
	duplicate.RequestID = "relation-duplicate-request"
	duplicate.IdempotencyKey = "relation-add-duplicate"
	duplicateResult, err := manager.AddRelation(context.Background(), duplicate)
	if err != nil || duplicateResult == nil || duplicateResult.Status != pluginsdk.CommandAlreadyApplied || writer.calls != 2 {
		t.Fatalf("duplicate AddRelation = result:%+v calls:%d err:%v", duplicateResult, writer.calls, err)
	}

	wireHost := dialPluginHostOverWire(t, host)
	wireManager, ok := pluginsdk.HostTaskCommands(wireHost)
	if !ok {
		t.Fatal("gRPC Host client does not expose exact task commands")
	}
	remove := input
	remove.RequestID = "relation-remove-request"
	remove.IdempotencyKey = "relation-remove"
	removed, err := wireManager.RemoveRelation(context.Background(), remove)
	if err != nil || removed == nil || removed.Status != pluginsdk.CommandApplied || writer.calls != 3 {
		t.Fatalf("wire RemoveRelation = result:%+v calls:%d err:%v", removed, writer.calls, err)
	}
}

var _ exactTaskRelationWriter = (*exactTaskRelationFake)(nil)
