package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestConversationForkTaskAdmissionBindsSnapshotAndAttachmentsAtomically(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-fork-admission", Name: "Fork admission", OwnerID: "owner-a"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	now := time.Now().UTC()
	fork := newConversationForkDraft("draft-admission", "fork-content-hash", now.Add(time.Hour))
	fork.WorkspaceID = "workspace-fork-admission"
	fork.Descriptor.AttachmentDescriptors = []models.ConversationForkAttachment{{ID: "fork-copy-admission", Name: "notes.txt", MediaType: "text/plain", Size: 12, Available: true}}
	if _, err := repo.CreateConversationForkDraft(ctx, fork); err != nil {
		t.Fatalf("create fork draft: %v", err)
	}
	if err := repo.CreateMessageAttachment(ctx, &models.TaskMessageAttachment{
		ID: "fork-copy-admission", OwnerID: "owner-a", WorkspaceID: "workspace-fork-admission", Name: "notes.txt",
		MimeType: "text/plain", Kind: "resource", DeliveryMode: "prompt", SizeBytes: 12,
		StorageKey: "fork-copy-admission", State: models.AttachmentStateStaged, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("create staged fork copy: %v", err)
	}
	task := &models.Task{ID: "destination-task-admission", WorkspaceID: "workspace-fork-admission", Title: "Destination"}
	admission := models.ConversationForkAdmission{
		OwnerID: "owner-a", WorkspaceID: task.WorkspaceID, ForkID: fork.Descriptor.ID, DestinationRequestID: "create-request-admission",
		DestinationKind: "task", RequestFingerprint: "destination-fingerprint", DestinationTaskID: task.ID,
	}
	if err := repo.CreateTaskWithConversationFork(ctx, task, admission); err != nil {
		t.Fatalf("create destination with fork: %v", err)
	}
	var state, destinationTask, requestFingerprint string
	if err := repo.db.QueryRowContext(ctx, repo.db.Rebind(`
		SELECT state, destination_task_id, destination_request_fingerprint FROM task_conversation_forks WHERE id = ?
	`), fork.Descriptor.ID).Scan(&state, &destinationTask, &requestFingerprint); err != nil {
		t.Fatalf("read admitted fork: %v", err)
	}
	if state != "attached" || destinationTask != task.ID || requestFingerprint != admission.RequestFingerprint {
		t.Fatalf("fork admission = (%q, %q, %q)", state, destinationTask, requestFingerprint)
	}
	attachment, err := repo.GetMessageAttachment(ctx, "fork-copy-admission")
	if err != nil || attachment.State != models.AttachmentStateClaimed || attachment.TaskID != task.ID || attachment.SessionID != "" {
		t.Fatalf("fork attachment claim = %+v, %v", attachment, err)
	}
	if err := repo.DiscardConversationForkDraft(ctx, admission.OwnerID, fork.Descriptor.ID); !errors.Is(err, models.ErrConversationForkConflict) {
		t.Fatalf("discard attached fork error = %v, want conflict", err)
	}
}

func TestConversationForkTaskAdmissionFailureLeavesNoDestination(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-fork-admission-fail", Name: "Fork admission", OwnerID: "owner-a"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	fork := newConversationForkDraft("draft-expired-admission", "expired-hash", time.Now().UTC().Add(-time.Minute))
	fork.WorkspaceID = "workspace-fork-admission-fail"
	if _, err := repo.CreateConversationForkDraft(ctx, fork); err != nil {
		t.Fatalf("create expiring fork draft: %v", err)
	}
	task := &models.Task{ID: "destination-task-rejected", WorkspaceID: "workspace-fork-admission-fail", Title: "Rejected"}
	err := repo.CreateTaskWithConversationFork(ctx, task, models.ConversationForkAdmission{
		OwnerID: "owner-a", WorkspaceID: task.WorkspaceID, ForkID: fork.Descriptor.ID, DestinationRequestID: "create-request-expired",
		DestinationKind: "task", RequestFingerprint: "expired-fingerprint", DestinationTaskID: task.ID,
	})
	if !errors.Is(err, models.ErrConversationForkExpired) {
		t.Fatalf("expired fork admission error = %v, want expired", err)
	}
	if _, err := repo.GetTask(ctx, task.ID); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("rejected destination lookup = %v, want no task", err)
	}
}

func TestConversationForkTaskDestinationCompletionAndCreateRollback(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	workspaceID := "workspace-fork-rollback"
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Fork rollback", OwnerID: "owner-a"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	now := time.Now().UTC()
	fork := newConversationForkDraft("draft-rollback", "rollback-hash", now.Add(time.Hour))
	fork.Descriptor.ID = "fork-rollback"
	fork.WorkspaceID = workspaceID
	fork.Descriptor.AttachmentDescriptors = []models.ConversationForkAttachment{{
		ID: "fork-rollback-copy", Name: "notes.txt", Size: 12, Available: true,
	}}
	if _, err := repo.CreateConversationForkDraft(ctx, fork); err != nil {
		t.Fatalf("create fork draft: %v", err)
	}
	if err := repo.CreateMessageAttachment(ctx, &models.TaskMessageAttachment{
		ID: "fork-rollback-copy", OwnerID: "owner-a", WorkspaceID: workspaceID, Name: "notes.txt",
		MimeType: "text/plain", Kind: "resource", DeliveryMode: "prompt", SizeBytes: 12,
		StorageKey: "fork-rollback-copy", State: models.AttachmentStateStaged, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("create staged fork copy: %v", err)
	}
	firstTask := &models.Task{ID: "task-fork-rollback-first", WorkspaceID: workspaceID, Title: "Rollback destination"}
	firstAdmission := models.ConversationForkAdmission{
		OwnerID: "owner-a", WorkspaceID: workspaceID, ForkID: fork.Descriptor.ID,
		DestinationKind: "task", DestinationRequestID: "rollback-create-first",
		RequestFingerprint: "rollback-fingerprint-first", DestinationTaskID: firstTask.ID,
	}
	if err := repo.CreateTaskWithConversationFork(ctx, firstTask, firstAdmission); err != nil {
		t.Fatalf("create first destination: %v", err)
	}
	if err := repo.MarkConversationForkTaskDestinationComplete(ctx, "owner-a", fork.Descriptor.ID, firstTask.ID); err != nil {
		t.Fatalf("mark destination complete: %v", err)
	}
	completed, err := repo.GetConversationForkByDestinationRequest(ctx, "owner-a", firstAdmission.DestinationRequestID)
	if err != nil || !completed.Descriptor.DestinationComplete {
		t.Fatalf("completed destination state = %+v, %v", completed.Descriptor, err)
	}

	if err := repo.RestoreConversationForkTaskDestinationForRollback(ctx, firstTask.ID); err != nil {
		t.Fatalf("restore fork after create rollback: %v", err)
	}
	restored, err := repo.GetConversationForkDraft(ctx, "owner-a", fork.Descriptor.ID, now)
	if err != nil || restored.Descriptor.State != "draft" || restored.Descriptor.DestinationComplete || restored.Descriptor.DestinationTaskID != "" {
		t.Fatalf("restored draft = %+v, %v", restored.Descriptor, err)
	}
	copy, err := repo.GetMessageAttachment(ctx, "fork-rollback-copy")
	if err != nil || copy.State != models.AttachmentStateStaged || copy.TaskID != "" {
		t.Fatalf("restaged fork copy = %+v, %v", copy, err)
	}
	secondTask := &models.Task{ID: "task-fork-rollback-second", WorkspaceID: workspaceID, Title: "Retry destination"}
	secondAdmission := firstAdmission
	secondAdmission.DestinationRequestID = "rollback-create-second"
	secondAdmission.RequestFingerprint = "rollback-fingerprint-second"
	secondAdmission.DestinationTaskID = secondTask.ID
	if err := repo.CreateTaskWithConversationFork(ctx, secondTask, secondAdmission); err != nil {
		t.Fatalf("admit restored fork to retry destination: %v", err)
	}
}

func TestConversationForkSessionAdmissionBindsDraftInSessionTransaction(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-fork-session", Name: "Fork session", OwnerID: "owner-a"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	task := &models.Task{ID: "task-fork-session", WorkspaceID: "workspace-fork-session", Title: "Existing task"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	now := time.Now().UTC()
	fork := newConversationForkDraft("draft-session-admission", "session-hash", now.Add(time.Hour))
	fork.Descriptor.ID = "fork-session-admission"
	fork.WorkspaceID = task.WorkspaceID
	fork.Descriptor.AttachmentDescriptors = []models.ConversationForkAttachment{{ID: "fork-session-copy", Name: "notes.txt", MediaType: "text/plain", Size: 12, Available: true}}
	if _, err := repo.CreateConversationForkDraft(ctx, fork); err != nil {
		t.Fatalf("create fork draft: %v", err)
	}
	if err := repo.CreateMessageAttachment(ctx, &models.TaskMessageAttachment{
		ID: "fork-session-copy", OwnerID: "owner-a", WorkspaceID: task.WorkspaceID, Name: "notes.txt",
		MimeType: "text/plain", Kind: "resource", DeliveryMode: "prompt", SizeBytes: 12,
		StorageKey: "fork-session-copy", State: models.AttachmentStateStaged, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("create staged fork copy: %v", err)
	}
	admission := models.ConversationForkAdmission{
		OwnerID: "owner-a", WorkspaceID: task.WorkspaceID, ForkID: fork.Descriptor.ID,
		DestinationKind:      "agent",
		DestinationRequestID: "agent-session-request", RequestFingerprint: "agent-session-fingerprint",
		DestinationTaskID: task.ID, DestinationSessionID: "session-fork-bound",
	}
	session := &models.TaskSession{ID: admission.DestinationSessionID, TaskID: task.ID, State: models.TaskSessionStateCreated, ConversationForkAdmission: &admission}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("create fork session: %v", err)
	}
	draft, err := repo.GetConversationForkByDestinationRequest(ctx, admission.OwnerID, admission.DestinationRequestID)
	if err != nil || draft.Descriptor.DestinationSessionID != session.ID || draft.Descriptor.DestinationTaskID != task.ID {
		t.Fatalf("destination fork = %+v, %v", draft.Descriptor, err)
	}
	attachment, err := repo.GetMessageAttachment(ctx, "fork-session-copy")
	if err != nil || attachment.TaskID != task.ID || attachment.SessionID != "" || attachment.State != models.AttachmentStateClaimed {
		t.Fatalf("fork attachment claim = %+v, %v", attachment, err)
	}
}

func TestConversationForkPendingTaskBindsFirstSessionAtomically(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-fork-pending-session", Name: "Fork pending session", OwnerID: "owner-a"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	fork := newConversationForkDraft("draft-pending-session", "pending-session-hash", time.Now().UTC().Add(time.Hour))
	fork.Descriptor.ID = "fork-pending-session"
	fork.WorkspaceID = "workspace-fork-pending-session"
	if _, err := repo.CreateConversationForkDraft(ctx, fork); err != nil {
		t.Fatalf("create fork draft: %v", err)
	}
	task := &models.Task{ID: "task-fork-pending-session", WorkspaceID: fork.WorkspaceID, Title: "Forked task"}
	admission := models.ConversationForkAdmission{
		OwnerID: "owner-a", WorkspaceID: task.WorkspaceID, ForkID: fork.Descriptor.ID,
		DestinationKind:      "task",
		DestinationRequestID: "task-pending-session-request", RequestFingerprint: "task-pending-session-fingerprint",
		DestinationTaskID: task.ID,
	}
	if err := repo.CreateTaskWithConversationFork(ctx, task, admission); err != nil {
		t.Fatalf("create task with fork: %v", err)
	}
	session := &models.TaskSession{ID: "session-pending-fork", TaskID: task.ID, State: models.TaskSessionStateCreated, ConversationForkPendingTask: true}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("create first task session: %v", err)
	}
	firstStored, err := repo.GetTaskSession(ctx, session.ID)
	if err != nil || firstStored.Metadata[models.MetaKeyConversationForkID] != fork.Descriptor.ID {
		t.Fatalf("first session fork delivery metadata = %+v, %v", firstStored, err)
	}
	draft, err := repo.GetConversationForkByDestinationRequest(ctx, admission.OwnerID, admission.DestinationRequestID)
	if err != nil || draft.Descriptor.DestinationSessionID != session.ID {
		t.Fatalf("pending task fork = %+v, %v", draft.Descriptor, err)
	}

	for _, create := range []struct {
		name string
		call func(*models.TaskSession) error
	}{
		{name: "ordinary additional session", call: func(session *models.TaskSession) error { return repo.CreateTaskSession(ctx, session) }},
		{name: "workflow-created session", call: func(session *models.TaskSession) error {
			return repo.CreateTaskSessionWithWorkflowSessionRoute(ctx, session, nil)
		}},
	} {
		t.Run(create.name, func(t *testing.T) {
			later := &models.TaskSession{
				ID:     "session-later-" + strings.ReplaceAll(create.name, " ", "-"),
				TaskID: task.ID, State: models.TaskSessionStateCreated, ConversationForkPendingTask: true,
			}
			if err := create.call(later); err != nil {
				t.Fatalf("create later session: %v", err)
			}
			stored, err := repo.GetTaskSession(ctx, later.ID)
			if err != nil {
				t.Fatalf("get later session: %v", err)
			}
			if _, found := stored.Metadata[models.MetaKeyConversationForkID]; found {
				t.Fatalf("later session inherited consumed fork delivery metadata: %#v", stored.Metadata)
			}
		})
	}
}

func TestConversationForkSessionAdmissionFailureRollsBackSession(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-fork-session-fail", Name: "Fork session failure", OwnerID: "owner-a"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	task := &models.Task{ID: "task-fork-session-fail", WorkspaceID: "workspace-fork-session-fail", Title: "Existing task"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	fork := newConversationForkDraft("draft-expired-session", "expired-session-hash", time.Now().UTC().Add(-time.Minute))
	fork.Descriptor.ID = "fork-expired-session"
	fork.WorkspaceID = task.WorkspaceID
	if _, err := repo.CreateConversationForkDraft(ctx, fork); err != nil {
		t.Fatalf("create expired fork draft: %v", err)
	}
	session := &models.TaskSession{
		ID: "session-rejected-fork", TaskID: task.ID, State: models.TaskSessionStateCreated,
		ConversationForkAdmission: &models.ConversationForkAdmission{
			OwnerID: "owner-a", WorkspaceID: task.WorkspaceID, ForkID: fork.Descriptor.ID,
			DestinationKind:      "agent",
			DestinationRequestID: "expired-session-request", RequestFingerprint: "expired-session-fingerprint",
			DestinationTaskID: task.ID, DestinationSessionID: "session-rejected-fork",
		},
	}
	if err := repo.CreateTaskSession(ctx, session); !errors.Is(err, models.ErrConversationForkExpired) {
		t.Fatalf("create session with expired fork error = %v, want expired", err)
	}
	if _, err := repo.GetTaskSession(ctx, session.ID); !errors.Is(err, models.ErrTaskSessionNotFound) {
		t.Fatalf("rejected session lookup = %v, want no session", err)
	}
}

func TestConversationForkDestinationRetention(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-fork-retention", Name: "Fork retention", OwnerID: "owner-a"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	source := &models.Task{ID: "task-fork-retention-source", WorkspaceID: "workspace-fork-retention", Title: "Source"}
	if err := repo.CreateTask(ctx, source); err != nil {
		t.Fatalf("create source task: %v", err)
	}
	now := time.Now().UTC()
	draft := newConversationForkDraft("draft-retention", "retention-hash", now.Add(time.Hour))
	draft.WorkspaceID = "workspace-fork-retention"
	draft.Descriptor.SourceTaskID = source.ID
	draft.CompiledText = "copied history"
	created, err := repo.CreateConversationForkDraft(ctx, draft)
	if err != nil {
		t.Fatalf("create fork snapshot: %v", err)
	}
	destination := &models.Task{ID: "task-fork-retention-destination", WorkspaceID: "workspace-fork-retention", Title: "Destination"}
	if err := repo.CreateTaskWithConversationFork(ctx, destination, models.ConversationForkAdmission{
		OwnerID: "owner-a", WorkspaceID: destination.WorkspaceID, ForkID: created.Descriptor.ID,
		DestinationKind: "task", DestinationRequestID: "destination-retention-request",
		RequestFingerprint: "destination-retention-fingerprint", DestinationTaskID: destination.ID,
	}); err != nil {
		t.Fatalf("create destination task: %v", err)
	}
	if err := repo.DeleteTask(ctx, source.ID); err != nil {
		t.Fatalf("delete source task: %v", err)
	}
	retained, err := repo.GetConversationForkByDestinationRequest(ctx, "owner-a", "destination-retention-request")
	if err != nil || retained.CompiledText != "copied history" {
		t.Fatalf("fork after source deletion = %+v, %v", retained, err)
	}
	if err := repo.DeleteTask(ctx, destination.ID); err != nil {
		t.Fatalf("delete destination task: %v", err)
	}
	if _, err := repo.GetConversationForkByDestinationRequest(ctx, "owner-a", "destination-retention-request"); !errors.Is(err, models.ErrConversationForkNotFound) {
		t.Fatalf("fork after destination deletion = %v, want not found", err)
	}

	orphanDraft := newConversationForkDraft("workspace-retention", "workspace-retention-hash", now.Add(time.Hour))
	orphanDraft.WorkspaceID = "workspace-fork-retention"
	orphan, err := repo.CreateConversationForkDraft(ctx, orphanDraft)
	if err != nil {
		t.Fatalf("create workspace snapshot: %v", err)
	}
	if _, _, err := repo.DeleteWorkspaceCascade(ctx, "workspace-fork-retention"); err != nil {
		t.Fatalf("delete workspace with fork snapshots: %v", err)
	}
	if _, err := repo.GetConversationForkDraft(ctx, "owner-a", orphan.Descriptor.ID, now); !errors.Is(err, models.ErrConversationForkNotFound) {
		t.Fatalf("fork after workspace deletion = %v, want not found", err)
	}
}
