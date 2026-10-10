package plugins

import (
	"context"
	"testing"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

type fakeExactTaskMessageMessenger struct {
	input          ExactTaskMessageInput
	calls          int
	alreadyApplied bool
	err            error
}

func (f *fakeExactTaskMessageMessenger) SendMessageExact(_ context.Context, input ExactTaskMessageInput) (string, bool, error) {
	f.calls++
	f.input = input
	return "queued-message-1", f.alreadyApplied, f.err
}

func (*fakeExactTaskMessageMessenger) SendMessage(context.Context, string, string, string, string) (PluginMessageResult, error) {
	return PluginMessageResult{SessionID: "session-message", Status: "queued"}, nil
}

func TestExactTaskMessageCommandReturnsAcceptedQueueReceipt(t *testing.T) {
	host, _ := newExactTaskCommandHost(t)
	messenger := &fakeExactTaskMessageMessenger{alreadyApplied: true}
	host.service.SetWriteDeps(messenger, nil)
	manager, ok := pluginsdk.HostTaskCommands(host)
	if !ok {
		t.Fatal("plugin Host does not expose exact task commands")
	}
	manifestDigest := ManifestCapabilityDigest(host.service.installedRecordByInstallationID(host.installationID).Manifest)
	input := pluginsdk.ExactTaskMessage{
		RequestID: "request-message", WorkspaceID: "workspace-task-commands", TaskID: "task-message",
		SessionID: "session-message", IdempotencyKey: "message-1",
		ExpectedTaskResourceVersion: "2026-09-26T10:00:00Z", ExpectedSessionResourceVersion: "2026-09-26T10:01:00Z",
		Content: "Continue the accepted task.", ApprovalRevision: 1, ManifestDigest: manifestDigest,
	}
	result, err := manager.SendMessage(context.Background(), input)
	if err != nil || result == nil || result.Status != pluginsdk.CommandAlreadyApplied || result.Receipt == nil {
		t.Fatalf("SendTaskMessageExact = result:%+v err:%v", result, err)
	}
	if messenger.calls != 1 || messenger.input.WorkspaceID != input.WorkspaceID ||
		messenger.input.TaskID != input.TaskID || messenger.input.SessionID != input.SessionID ||
		messenger.input.ExpectedTaskResourceVersion != input.ExpectedTaskResourceVersion ||
		messenger.input.ExpectedSessionResourceVersion != input.ExpectedSessionResourceVersion ||
		messenger.input.Content != input.Content {
		t.Fatalf("messenger calls/input = %d/%+v", messenger.calls, messenger.input)
	}
	if result.Receipt.TargetID != "queued-message-1" || result.Receipt.ResourceVersion != input.ExpectedSessionResourceVersion {
		t.Fatalf("accepted message receipt = %+v", result.Receipt)
	}

	replayed, err := manager.SendMessage(context.Background(), input)
	if err != nil || replayed == nil || replayed.Status != pluginsdk.CommandAlreadyApplied || messenger.calls != 1 {
		t.Fatalf("replayed SendTaskMessageExact = result:%+v calls:%d err:%v", replayed, messenger.calls, err)
	}
}
