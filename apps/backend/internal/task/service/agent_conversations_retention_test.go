package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kdlbs/kandev/pluginsdk"
)

func TestDeliveryRetentionDeletion(t *testing.T) {
	t.Run("ConversationIDReuse", func(t *testing.T) {
		svc, repo := newAgentConversationServiceOverRealRepo(t)
		ctx := context.Background()
		seedConversationWorkspace(t, repo, "retention-workspace")
		verifyOrdinaryConversationReuse(t, ctx, svc, repo)
		verifyManagedConversationReuse(t, ctx, svc, repo)
	})

	t.Run("CoordinatorDeleteSchedulesRetirement", func(t *testing.T) {
		svc, repo := newAgentConversationServiceOverRealRepo(t)
		ctx := context.Background()
		seedConversationWorkspace(t, repo, "retention-delete-workspace")
		descriptor := ensureConversation(t, svc, "plugin-coordinator", "retention-delete-workspace", "coordinator")
		session, err := repo.GetTaskSession(ctx, descriptor.SessionID)
		if err != nil {
			t.Fatalf("load coordinator session: %v", err)
		}
		seedConversationDeliveryEvidence(t, repo, session)
		if deleted, err := svc.Delete(ctx, "plugin-coordinator", "retention-delete-workspace", "coordinator"); err != nil || deleted != 1 {
			t.Fatalf("delete coordinator conversation = %d, %v", deleted, err)
		}
		var count int
		err = repo.DB().QueryRowContext(ctx, `
			SELECT COUNT(*) FROM agent_delivery_cleanup_jobs WHERE session_id = ? AND incarnation_id = ?`,
			session.ID, session.QueueIncarnationID).Scan(&count)
		if err != nil || count != 1 {
			t.Fatalf("coordinator deletion retirement job count = %d, err %v", count, err)
		}
	})
}

func verifyOrdinaryConversationReuse(t *testing.T, ctx context.Context, svc *AgentConversationService, repo *sqliterepo.Repository) {
	t.Helper()
	const pluginID, workspaceID, conversationKey = "plugin-retention", "retention-workspace", "ordinary"
	original, _, err := svc.Ensure(ctx, pluginID, pluginsdk.AgentConversationSpec{WorkspaceID: workspaceID, ConversationKey: conversationKey})
	if err != nil {
		t.Fatalf("ensure ordinary conversation: %v", err)
	}
	oldSession, err := repo.GetTaskSession(ctx, original.SessionID)
	if err != nil {
		t.Fatalf("load ordinary primary: %v", err)
	}
	seedConversationDeliveryEvidence(t, repo, oldSession)
	retireLegacyConversationPrimaryForTest(t, ctx, repo, oldSession)
	if _, _, err := svc.Ensure(ctx, pluginID, pluginsdk.AgentConversationSpec{WorkspaceID: workspaceID, ConversationKey: conversationKey}); !errors.Is(err, repoerrors.ErrAgentDeliveryIdentityRetiring) {
		t.Fatalf("ordinary deterministic ID retry error = %v, want retirement conflict", err)
	}
	purgeConversationDelivery(t, ctx, repo, oldSession.ID)
	repaired, outcome, err := svc.Ensure(ctx, pluginID, pluginsdk.AgentConversationSpec{WorkspaceID: workspaceID, ConversationKey: conversationKey})
	if err != nil || outcome != AgentConversationStatusExists || repaired.SessionID != oldSession.ID {
		t.Fatalf("ordinary retry after purge = %+v, %q, %v", repaired, outcome, err)
	}
	assertFreshConversationIncarnation(t, ctx, repo, repaired.SessionID, oldSession.QueueIncarnationID)
}

func verifyManagedConversationReuse(t *testing.T, ctx context.Context, svc *AgentConversationService, repo *sqliterepo.Repository) {
	t.Helper()
	spec := pluginsdk.ManagedAgentConversationSpec{
		WorkspaceID: "retention-workspace", InstanceKey: "managed",
		AgentProfileID: "test-workspace-default-profile", ApprovalRevision: 1,
		ManifestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	original, _, err := svc.EnsureManaged(ctx, "plugin-retention", "installation-retention", spec, "managed-create", "managed-digest")
	if err != nil {
		t.Fatalf("ensure managed conversation: %v", err)
	}
	oldSession, err := repo.GetTaskSession(ctx, original.SessionID)
	if err != nil {
		t.Fatalf("load managed primary: %v", err)
	}
	seedConversationDeliveryEvidence(t, repo, oldSession)
	retireLegacyConversationPrimaryForTest(t, ctx, repo, oldSession)
	spec.ExpectedRevision = original.Revision
	if _, _, err := svc.EnsureManaged(ctx, "plugin-retention", "installation-retention", spec, "managed-repair", "managed-repair-digest"); !errors.Is(err, repoerrors.ErrAgentDeliveryIdentityRetiring) {
		t.Fatalf("managed deterministic ID retry error = %v, want retirement conflict", err)
	}
	purgeConversationDelivery(t, ctx, repo, oldSession.ID)
	repaired, _, err := svc.EnsureManaged(ctx, "plugin-retention", "installation-retention", spec, "managed-repair", "managed-repair-digest")
	if err != nil || repaired.SessionID != oldSession.ID {
		t.Fatalf("managed retry after purge = %+v, %v", repaired, err)
	}
	assertFreshConversationIncarnation(t, ctx, repo, repaired.SessionID, oldSession.QueueIncarnationID)
}

func retireLegacyConversationPrimaryForTest(t *testing.T, ctx context.Context, repo *sqliterepo.Repository, session *models.TaskSession) {
	t.Helper()
	// Model a primary removed before cleanup began protecting primary sessions.
	if _, err := repo.DB().ExecContext(ctx, `DELETE FROM task_sessions WHERE id = ?`, session.ID); err != nil {
		t.Fatalf("remove legacy primary: %v", err)
	}
	if _, err := repo.DB().ExecContext(ctx, `
		INSERT INTO agent_delivery_cleanup_jobs
			(session_id, incarnation_id, retired_at, phase, created_at, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP, 'pending', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, session.ID, session.QueueIncarnationID); err != nil {
		t.Fatalf("schedule legacy delivery retirement: %v", err)
	}
}

type conversationDeliveryRepository interface {
	GetTaskSession(context.Context, string) (*models.TaskSession, error)
	DeleteTaskSession(context.Context, *models.TaskSession) error
	RunAgentDeliveryRetirementBatch(context.Context, models.AgentDeliveryRetirementBatchOptions) (models.AgentDeliveryRetirementBatchResult, error)
	PrepareAgentDeliverySubmission(context.Context, *models.AgentDeliverySubmission) (bool, error)
	ReceiveAgentDeliveryEvent(context.Context, *models.AgentDeliveryEvent, int64) (bool, error)
}

func seedConversationDeliveryEvidence(t *testing.T, repo conversationDeliveryRepository, session *models.TaskSession) {
	t.Helper()
	submission := &models.AgentDeliverySubmission{
		ID: "retention-prompt:" + session.ID, SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, DispatchAttemptID: "retention-attempt:" + session.ID,
		PayloadHash: "retention-hash:" + session.ID, Payload: []byte("retention prompt"), State: models.DeliverySubmissionPrepared,
	}
	if _, err := repo.PrepareAgentDeliverySubmission(context.Background(), submission); err != nil {
		t.Fatalf("prepare conversation prompt: %v", err)
	}
	event := &models.AgentDeliveryEvent{
		SessionID: session.ID, IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1,
		StreamID: "retention-stream:" + session.ID, Sequence: 1, SubmissionID: submission.ID,
		EventType: "assistant_message", Payload: []byte("retention event"),
	}
	if inserted, err := repo.ReceiveAgentDeliveryEvent(context.Background(), event, 1); err != nil || !inserted {
		t.Fatalf("receive conversation event = %v, err %v", inserted, err)
	}
}

func purgeConversationDelivery(t *testing.T, ctx context.Context, repo conversationDeliveryRepository, sessionID string) {
	t.Helper()
	result, err := repo.RunAgentDeliveryRetirementBatch(ctx, models.AgentDeliveryRetirementBatchOptions{})
	if err != nil || result.Completed != 1 {
		t.Fatalf("purge conversation delivery = %+v, %v", result, err)
	}
}

func assertFreshConversationIncarnation(
	t *testing.T,
	ctx context.Context,
	repo interface {
		GetTaskSession(context.Context, string) (*models.TaskSession, error)
	},
	sessionID, oldIncarnation string,
) {
	t.Helper()
	current, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load recreated session: %v", err)
	}
	if current.QueueIncarnationID == oldIncarnation {
		t.Fatalf("recreated session reused incarnation %q", oldIncarnation)
	}
}
