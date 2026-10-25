package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestTurnChangeSummaryProjectionOmitsPrivateCaptureIdentityAndContent(t *testing.T) {
	changeSet := &models.TurnChangeSet{
		ID: "set-1", TaskID: "task-1", TaskSessionID: "session-1", TurnID: "turn-1", Revision: 2,
		RuntimeExecutionID: "private-runtime", StartupAttemptID: "private-startup", SettingsUserID: "private-user",
		ActorID: "private-actor", CaptureEnabled: true, Availability: models.TurnChangeAvailabilityReady,
		Complete: true, SummaryComplete: true, ContentComplete: true, TurnOrdinal: 1,
		FinalAssistantMessageID: "message-1", FallbackAnchor: "turn-changes:turn-1", FileCount: 1,
	}
	repositories := []*models.TurnRepositoryChangeSet{{
		ID: "repo-change-1", TurnChangeSetID: "set-1", CheckoutID: "checkout-1", DisplayName: "app",
		Availability: models.TurnChangeAvailabilityReady, EnumerationComplete: true,
		ComparisonComplete: true, ContentComplete: true, StartReachabilityRef: "private-ref",
	}}
	payload, err := json.Marshal(projectTurnChangeSummaryEvent(changeSet, repositories))
	if err != nil {
		t.Fatal(err)
	}
	body := string(payload)
	for _, privateValue := range []string{"private-runtime", "private-startup", "private-user", "private-actor", "private-ref", "content_bytes"} {
		if strings.Contains(body, privateValue) {
			t.Errorf("summary projection leaked %q: %s", privateValue, body)
		}
	}
	if !strings.Contains(body, `"fallback_anchor":"turn-changes:turn-1"`) || !strings.Contains(body, `"display_name":"app"`) {
		t.Fatalf("summary projection is missing safe navigation metadata: %s", body)
	}
}

func TestTurnChangeHistoryReadsEnforceSessionOwnershipAndRelationships(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedScopedWorkspaces(t, repo)
	svc.turnChanges = repo
	ctx := context.Background()
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "env-b", TaskID: "task-b"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "sess-other", TaskID: "task-b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB().ExecContext(ctx, `UPDATE task_sessions SET task_environment_id = ? WHERE id = ?`, "env-b", "sess-b"); err != nil {
		t.Fatal(err)
	}
	startedAt := time.Now().UTC()
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-b", TaskID: "task-b", TaskSessionID: "sess-b", StartedAt: startedAt}); err != nil {
		t.Fatal(err)
	}
	changeSet := &models.TurnChangeSet{
		ID: "set-b", TaskID: "task-b", TaskSessionID: "sess-b", TurnID: "turn-b", TaskEnvironmentID: "env-b",
		Availability: models.TurnChangeAvailabilityExpired, Reason: models.TurnChangeReasonExpiredAge,
	}
	if err := repo.CreateTurnChangeSet(ctx, changeSet); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.ListTurnChangeHistory(ctxAs("user-a"), "sess-b", 0, 10); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("foreign history read error = %v, want hidden session", err)
	}
	if sets, total, err := svc.ListTurnChangeHistory(ctxAs("user-b"), "sess-b", 0, 10); err != nil || total != 1 || len(sets) != 1 {
		t.Fatalf("owner history read = %d/%d, %v", len(sets), total, err)
	}
	if _, err := svc.GetTurnChangeHistory(ctxAs("user-b"), "sess-other", "set-b"); !errors.Is(err, repoerrors.ErrTurnChangeSetNotFound) {
		t.Fatalf("cross-session history read error = %v, want change-set not found", err)
	}
	if _, err := svc.ReadTurnChangeContent(ctxAs("user-b"), "sess-b", "set-b", "file-not-owned", models.TurnChangeContentCanonicalPatch); !errors.Is(err, ErrTurnChangeContentExpired) {
		t.Fatalf("expired content read error = %v, want explicit expiry", err)
	}
}
