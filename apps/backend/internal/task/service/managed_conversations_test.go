package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestManagedConversationIdentity(t *testing.T) {
	svc, deps := newACTestService()
	spec := pluginsdk.ManagedAgentConversationSpec{
		WorkspaceID: "ws-1", InstanceKey: "delivery-lead", AgentProfileID: "profile-gpt4",
		ApprovalRevision: 1, ManifestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ExecutorID: "exec-worktree", ExecutorProfileID: "executor-profile-1", BasePrompt: "Review the workspace",
		InstructionVersion: "instructions-7",
	}

	first, outcome, err := svc.EnsureManaged(context.Background(), "plugin-coordinator", "installation-old", spec, "op-old-1", "payload-old-1")
	if err != nil || outcome != AgentConversationStatusCreated {
		t.Fatalf("EnsureManaged first install: outcome=%q err=%v", outcome, err)
	}
	if first.InstallationID != "installation-old" || first.Revision != 1 || first.RetentionMode != "retain_on_uninstall" {
		t.Fatalf("first descriptor = %+v", first)
	}

	spec.ExpectedRevision = first.Revision
	again, outcome, err := svc.EnsureManaged(context.Background(), "plugin-coordinator", "installation-old", spec, "op-old-2", "payload-old-2")
	if err != nil || outcome != AgentConversationStatusExists || again.TaskID != first.TaskID {
		t.Fatalf("EnsureManaged same identity: descriptor=%+v outcome=%q err=%v", again, outcome, err)
	}

	newInstallationSpec := spec
	newInstallationSpec.ExpectedRevision = 0
	other, outcome, err := svc.EnsureManaged(context.Background(), "plugin-coordinator", "installation-new", newInstallationSpec, "op-new-1", "payload-new-1")
	if err != nil || outcome != AgentConversationStatusCreated || other.TaskID == first.TaskID {
		t.Fatalf("EnsureManaged replacement install: descriptor=%+v outcome=%q err=%v", other, outcome, err)
	}
	if got := deps.tasks.count(); got != 2 {
		t.Fatalf("task count = %d, want one retained conversation per installation", got)
	}
}

func TestManagedConversationEnsureOperationReplay(t *testing.T) {
	svc, deps := newACTestService()
	spec := pluginsdk.ManagedAgentConversationSpec{
		WorkspaceID: "ws-1", InstanceKey: "delivery-lead", AgentProfileID: "profile-gpt4",
		ApprovalRevision: 1, ManifestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		BasePrompt: "Review the workspace",
	}
	first, outcome, err := svc.EnsureManaged(context.Background(), "plugin-coordinator", "installation-1", spec, "op-replay", "digest-replay")
	if err != nil || outcome != AgentConversationStatusCreated {
		t.Fatalf("initial EnsureManaged = %+v, %q, %v", first, outcome, err)
	}

	replayed, outcome, err := svc.EnsureManaged(context.Background(), "plugin-coordinator", "installation-1", spec, "op-replay", "digest-replay")
	if err != nil || outcome != AgentConversationStatusAlreadyApplied || replayed.TaskID != first.TaskID || replayed.Revision != first.Revision {
		t.Fatalf("replayed EnsureManaged = %+v, %q, %v; want original result", replayed, outcome, err)
	}
	if got := deps.tasks.count(); got != 1 {
		t.Fatalf("task count after replay = %d, want 1", got)
	}
}

func TestManagedConversationEnsureRepairsAcceptedOperation(t *testing.T) {
	svc, deps := newACTestService()
	spec := pluginsdk.ManagedAgentConversationSpec{
		WorkspaceID: "ws-1", InstanceKey: "delivery-lead", AgentProfileID: "profile-gpt4",
		ApprovalRevision: 1, ManifestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		BasePrompt: "Review the workspace",
	}
	deps.sess.createErr = errors.New("temporary session write failure")
	if _, _, err := svc.EnsureManaged(context.Background(), "plugin-coordinator", "installation-1", spec, "op-repair", "digest-repair"); err == nil {
		t.Fatal("initial EnsureManaged error = nil, want failed session creation")
	}
	if got := deps.tasks.count(); got != 1 {
		t.Fatalf("task count after partial creation = %d, want 1", got)
	}
	deps.sess.createErr = nil

	repaired, outcome, err := svc.EnsureManaged(context.Background(), "plugin-coordinator", "installation-1", spec, "op-repair", "digest-repair")
	if err != nil || outcome != AgentConversationStatusAlreadyApplied || repaired.SessionID == "" {
		t.Fatalf("replayed EnsureManaged = %+v, %q, %v; want repaired original operation", repaired, outcome, err)
	}
}

func TestManagedConversationConfigurationUsesRevisionAndIdleBoundary(t *testing.T) {
	svc, deps := newACTestService()
	spec := pluginsdk.ManagedAgentConversationSpec{
		WorkspaceID: "ws-1", InstanceKey: "delivery-lead", AgentProfileID: "profile-gpt4",
		ApprovalRevision: 1, ManifestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ExecutorID: "exec-worktree", ExecutorProfileID: "executor-profile-1", BasePrompt: "Initial instructions",
	}
	created, _, err := svc.EnsureManaged(context.Background(), "plugin-coordinator", "installation-1", spec, "op-config-1", "payload-config-1")
	if err != nil {
		t.Fatalf("EnsureManaged: %v", err)
	}

	changed := spec
	changed.ExpectedRevision = created.Revision - 1
	changed.BasePrompt = "Stale update"
	if _, _, err := svc.EnsureManaged(context.Background(), "plugin-coordinator", "installation-1", changed, "op-stale", "payload-stale"); status.Code(err) != codes.Aborted {
		t.Fatalf("stale configuration error = %v, want Aborted", err)
	}

	deps.sess.setState(created.SessionID, models.TaskSessionStateRunning)
	changed.ExpectedRevision = created.Revision
	changed.BasePrompt = "New instructions"
	if _, _, err := svc.EnsureManaged(context.Background(), "plugin-coordinator", "installation-1", changed, "op-running", "payload-running"); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("running configuration error = %v, want FailedPrecondition", err)
	}

	deps.sess.setState(created.SessionID, models.TaskSessionStateIdle)
	updated, outcome, err := svc.EnsureManaged(context.Background(), "plugin-coordinator", "installation-1", changed, "op-config-2", "payload-config-2")
	if err != nil || outcome != AgentConversationStatusExists || updated.Revision != created.Revision+1 {
		t.Fatalf("idle configuration update = %+v outcome=%q err=%v", updated, outcome, err)
	}
	if updated.BasePrompt != "New instructions" || updated.ExecutorProfileID != "executor-profile-1" {
		t.Fatalf("updated configuration = %+v", updated)
	}
}

func TestManagedConversationConcurrentRevisionConflict(t *testing.T) {
	svc, _ := newACTestService()
	spec := pluginsdk.ManagedAgentConversationSpec{
		WorkspaceID: "ws-1", InstanceKey: "delivery-lead", AgentProfileID: "profile-gpt4", BasePrompt: "Initial",
		ApprovalRevision: 1, ManifestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	created, _, err := svc.EnsureManaged(context.Background(), "plugin-coordinator", "installation-1", spec, "op-concurrent-0", "payload-concurrent-0")
	if err != nil {
		t.Fatalf("EnsureManaged: %v", err)
	}
	start := make(chan struct{})
	type result struct {
		descriptor pluginsdk.ManagedAgentConversationDescriptor
		err        error
	}
	results := make(chan result, 2)
	var wait sync.WaitGroup
	for _, prompt := range []string{"First concurrent edit", "Second concurrent edit"} {
		wait.Add(1)
		go func(prompt string) {
			defer wait.Done()
			<-start
			update := spec
			update.ExpectedRevision = created.Revision
			update.BasePrompt = prompt
			value, _, err := svc.EnsureManaged(context.Background(), "plugin-coordinator", "installation-1", update, "op-"+prompt, "payload-"+prompt)
			results <- result{descriptor: value, err: err}
		}(prompt)
	}
	close(start)
	wait.Wait()
	close(results)
	applied, conflicts := 0, 0
	for outcome := range results {
		if outcome.err == nil {
			applied++
			continue
		}
		if status.Code(outcome.err) == codes.Aborted {
			conflicts++
			continue
		}
		t.Fatalf("concurrent update error = %v, want success or stale revision", outcome.err)
	}
	if applied != 1 || conflicts != 1 {
		t.Fatalf("concurrent updates: applied=%d conflicts=%d, want one of each", applied, conflicts)
	}
}

func TestManagedConversationSurvivesLegacyPluginCleanup(t *testing.T) {
	svc, deps := newACTestService()
	managed, _, err := svc.EnsureManaged(context.Background(), "plugin-coordinator", "installation-1", pluginsdk.ManagedAgentConversationSpec{
		WorkspaceID: "ws-1", InstanceKey: "delivery-lead", AgentProfileID: "profile-gpt4",
		ApprovalRevision: 1, ManifestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}, "op-legacy-cleanup", "payload-legacy-cleanup")
	if err != nil {
		t.Fatalf("EnsureManaged: %v", err)
	}
	if _, _, err := svc.Ensure(context.Background(), "plugin-coordinator", pluginsdk.AgentConversationSpec{
		WorkspaceID: "ws-1", ConversationKey: "legacy", AgentProfileID: "profile-gpt4",
	}); err != nil {
		t.Fatalf("legacy Ensure: %v", err)
	}

	deleted, err := svc.DeleteAllForPlugin(context.Background(), "plugin-coordinator")
	if err != nil || deleted != 1 {
		t.Fatalf("DeleteAllForPlugin = %d, err=%v, want only legacy conversation deleted", deleted, err)
	}
	if got := deps.tasks.count(); got != 1 {
		t.Fatalf("retained task rows = %d, want one retained transcript", got)
	}
	if found, err := svc.GetManaged(context.Background(), "installation-1", "ws-1", "delivery-lead"); err != nil || found.TaskID != managed.TaskID {
		t.Fatalf("retained conversation readback = %+v err=%v", found, err)
	}
}

func TestManagedConversationUninstallDetachesAndHidesRetainedTranscript(t *testing.T) {
	svc, deps := newACTestService()
	conversation, _, err := svc.EnsureManaged(context.Background(), "plugin-coordinator", "installation-1", pluginsdk.ManagedAgentConversationSpec{
		WorkspaceID: "ws-1", InstanceKey: "delivery-lead", AgentProfileID: "profile-gpt4",
		ApprovalRevision: 1, ManifestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}, "op-detach", "payload-detach")
	if err != nil {
		t.Fatalf("EnsureManaged: %v", err)
	}
	if err := svc.DetachManagedForInstallation(context.Background(), "installation-1"); err != nil {
		t.Fatalf("DetachManagedForInstallation: %v", err)
	}
	if got := deps.tasks.count(); got != 1 {
		t.Fatalf("retained transcript row count = %d, want 1", got)
	}
	if _, err := svc.GetManaged(context.Background(), "installation-1", "ws-1", "delivery-lead"); status.Code(err) != codes.NotFound {
		t.Fatalf("plugin read after uninstall = %v, want NotFound", err)
	}
	listed, err := svc.ListManaged(context.Background(), "installation-1", "ws-1")
	if err != nil || len(listed) != 0 {
		t.Fatalf("plugin list after uninstall = %+v, err=%v, want no detached conversations", listed, err)
	}
	row, err := svc.findRetainedManagedConversation(context.Background(), "installation-1", "ws-1", "delivery-lead")
	if err != nil || row == nil || !managedConversationDetached(row) || !managedConversationPaused(row) {
		t.Fatalf("host-retained transcript = %+v, err=%v, want detached and paused", row, err)
	}
	if managedConversationRevision(row) != conversation.Revision+1 {
		t.Fatalf("detached transcript revision = %d, want %d", managedConversationRevision(row), conversation.Revision+1)
	}
}

func TestManagedConversationPauseStopsActiveExecution(t *testing.T) {
	svc, deps := newACTestService()
	conversation, _, err := svc.EnsureManaged(context.Background(), "plugin-coordinator", "installation-1", pluginsdk.ManagedAgentConversationSpec{
		WorkspaceID: "ws-1", InstanceKey: "delivery-lead", AgentProfileID: "profile-gpt4",
		ApprovalRevision: 1, ManifestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}, "op-active", "payload-active")
	if err != nil {
		t.Fatalf("EnsureManaged: %v", err)
	}
	deps.sess.setState(conversation.SessionID, models.TaskSessionStateRunning)
	var stoppedTask string
	svc.SetManagedExecutionStopper(func(_ context.Context, taskID string) error {
		stoppedTask = taskID
		return nil
	})

	if err := svc.PauseManagedForInstallation(context.Background(), "installation-1"); err != nil {
		t.Fatalf("PauseManagedForInstallation: %v", err)
	}
	if stoppedTask != conversation.TaskID {
		t.Fatalf("stopped task = %q, want %q", stoppedTask, conversation.TaskID)
	}
	paused, err := svc.GetManaged(context.Background(), "installation-1", "ws-1", "delivery-lead")
	if err != nil || !paused.DesiredPaused || paused.Revision != conversation.Revision+1 {
		t.Fatalf("paused conversation = %+v, err=%v", paused, err)
	}
}

func TestManagedConversationApprovalChangeInvalidatesAndStopsPolicy(t *testing.T) {
	svc, deps := newACTestService()
	spec := pluginsdk.ManagedAgentConversationSpec{
		WorkspaceID: "ws-1", InstanceKey: "delivery-lead", AgentProfileID: "profile-gpt4",
		ApprovalRevision: 1, ManifestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		AgentToolNames: []string{"read_task"},
	}
	conversation, _, err := svc.EnsureManaged(context.Background(), "plugin-coordinator", "installation-1", spec, "op-invalidation", "payload-invalidation")
	if err != nil {
		t.Fatalf("EnsureManaged: %v", err)
	}
	otherSpec := spec
	otherSpec.WorkspaceID = "ws-2"
	otherSpec.InstanceKey = "observer"
	other, _, err := svc.EnsureManaged(context.Background(), "plugin-coordinator", "installation-1", otherSpec, "op-other-workspace", "payload-other-workspace")
	if err != nil {
		t.Fatalf("EnsureManaged other workspace: %v", err)
	}
	if err := svc.sess.UpdateTaskSession(context.Background(), &models.TaskSession{
		ID: conversation.SessionID, TaskID: conversation.TaskID, IsPrimary: true,
		AgentProfileID: conversation.AgentProfileID, State: models.TaskSessionStateRunning,
	}); err != nil {
		t.Fatalf("mark managed session running: %v", err)
	}
	if err := svc.sess.UpdateTaskSession(context.Background(), &models.TaskSession{
		ID: other.SessionID, TaskID: other.TaskID, IsPrimary: true,
		AgentProfileID: other.AgentProfileID, State: models.TaskSessionStateRunning,
	}); err != nil {
		t.Fatalf("mark other managed session running: %v", err)
	}
	var stoppedTasks []string
	svc.SetManagedExecutionStopper(func(_ context.Context, taskID string) error {
		stoppedTasks = append(stoppedTasks, taskID)
		return nil
	})

	if err := svc.InvalidateManagedForInstallationWorkspace(context.Background(), "installation-1", "ws-1"); err != nil {
		t.Fatalf("InvalidateManagedForInstallationWorkspace: %v", err)
	}
	if len(stoppedTasks) != 1 || stoppedTasks[0] != conversation.TaskID {
		t.Fatalf("stopped tasks = %v, want only %q", stoppedTasks, conversation.TaskID)
	}
	deps.sess.setState(conversation.SessionID, models.TaskSessionStateIdle)
	invalidated, err := svc.findRetainedManagedConversation(context.Background(), "installation-1", "ws-1", "delivery-lead")
	if err != nil || invalidated == nil || managedConversationRevision(invalidated) != conversation.Revision+1 {
		t.Fatalf("invalidated task = %+v err=%v", invalidated, err)
	}
	if _, managed, err := models.ManagedToolPolicyFromTask(invalidated); !managed || err == nil {
		t.Fatalf("invalidated managed policy = managed %t, err %v; want denied", managed, err)
	}
	if _, managed, err := models.ManagedToolPolicyFromTask(mustManagedTask(t, svc, "installation-1", "ws-2", "observer")); err != nil || !managed {
		t.Fatalf("other workspace policy = managed %t, err %v; want active", managed, err)
	}

	updatedSpec := spec
	updatedSpec.ExpectedRevision = managedConversationRevision(invalidated)
	updatedSpec.ApprovalRevision = 2
	updatedSpec.ManifestDigest = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	updated, _, err := svc.EnsureManaged(context.Background(), "plugin-coordinator", "installation-1", updatedSpec, "op-policy-refresh", "payload-policy-refresh")
	if err != nil {
		t.Fatalf("EnsureManaged refreshed policy: %v", err)
	}
	refreshed, managed, err := models.ManagedToolPolicyFromTask(mustManagedTask(t, svc, "installation-1", "ws-1", "delivery-lead"))
	if err != nil || !managed || refreshed.ApprovalRevision != 2 || updated.Revision != managedConversationRevision(mustManagedTask(t, svc, "installation-1", "ws-1", "delivery-lead")) {
		t.Fatalf("refreshed policy = %+v, managed %t, err %v", refreshed, managed, err)
	}
}

func mustManagedTask(t *testing.T, svc *AgentConversationService, installationID, workspaceID, instanceKey string) *models.Task {
	t.Helper()
	task, err := svc.findRetainedManagedConversation(context.Background(), installationID, workspaceID, instanceKey)
	if err != nil || task == nil {
		t.Fatalf("find retained managed conversation: task=%+v err=%v", task, err)
	}
	return task
}
