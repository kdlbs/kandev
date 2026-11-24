package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestUpdateResumeTokenWithACPAdapterIdentityCommitsBothValues(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-antigravity-identity", "session-antigravity-identity", "turn-antigravity-identity")
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		SessionID: "session-antigravity-identity", TaskID: "task-antigravity-identity",
		AgentExecutionID: "execution-antigravity-identity", Status: "running",
	}); err != nil {
		t.Fatal(err)
	}
	identity := models.ACPAdapterIdentity{
		NativeSessionID: "native-antigravity-identity", AdapterVersion: "antigravity-cli/1.5.15",
	}

	if err := repo.UpdateResumeTokenWithACPAdapterIdentity(
		ctx, "session-antigravity-identity", "execution-antigravity-identity",
		identity.NativeSessionID, "", identity,
	); err != nil {
		t.Fatalf("UpdateResumeTokenWithACPAdapterIdentity: %v", err)
	}
	running, err := repo.GetExecutorRunningBySessionID(ctx, "session-antigravity-identity")
	if err != nil || running.ResumeToken != identity.NativeSessionID {
		t.Fatalf("resume token = %q, %v; want %q", running.ResumeToken, err, identity.NativeSessionID)
	}
	session, err := repo.GetTaskSession(ctx, "session-antigravity-identity")
	if err != nil || models.ACPAdapterVersionForNativeSession(session.Metadata, identity.NativeSessionID) != identity.AdapterVersion {
		t.Fatalf("persisted ACP adapter identity = %v, %v; want %q", session.Metadata, err, identity.AdapterVersion)
	}
}

func TestUpdateResumeTokenWithACPAdapterIdentityRollsBackTokenOnMetadataFailure(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-antigravity-rollback", "session-antigravity-rollback", "turn-antigravity-rollback")
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		SessionID: "session-antigravity-rollback", TaskID: "task-antigravity-rollback",
		AgentExecutionID: "execution-antigravity-rollback", Status: "running", ResumeToken: "prior-token",
	}); err != nil {
		t.Fatal(err)
	}
	_, err := repo.db.ExecContext(ctx, `
		CREATE TRIGGER reject_antigravity_identity_update
		BEFORE UPDATE OF metadata ON task_sessions
		BEGIN SELECT RAISE(ABORT, 'injected ACP identity metadata failure'); END
	`)
	if err != nil {
		t.Fatalf("create metadata failure trigger: %v", err)
	}
	identity := models.ACPAdapterIdentity{
		NativeSessionID: "native-antigravity-rollback", AdapterVersion: "antigravity-cli/1.5.15",
	}

	err = repo.UpdateResumeTokenWithACPAdapterIdentity(
		ctx, "session-antigravity-rollback", "execution-antigravity-rollback",
		identity.NativeSessionID, "", identity,
	)
	if err == nil {
		t.Fatal("UpdateResumeTokenWithACPAdapterIdentity succeeded despite metadata write failure")
	}
	running, err := repo.GetExecutorRunningBySessionID(ctx, "session-antigravity-rollback")
	if err != nil || running.ResumeToken != "prior-token" {
		t.Fatalf("resume token after rollback = %q, %v; want prior-token", running.ResumeToken, err)
	}
	session, err := repo.GetTaskSession(ctx, "session-antigravity-rollback")
	if err != nil || models.ACPAdapterVersionForNativeSession(session.Metadata, identity.NativeSessionID) != "" {
		t.Fatalf("ACP adapter identity after rollback = %v, %v; want absent", session.Metadata, err)
	}
}
