package sqlite

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func createClarificationBundleMessage(
	t *testing.T,
	repo *Repository,
	id, taskID, sessionID, turnID, pendingID, questionID string,
	createdAt time.Time,
) {
	t.Helper()
	if err := repo.CreateMessage(context.Background(), &models.Message{
		ID:            id,
		TaskSessionID: sessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		AuthorType:    models.MessageAuthorAgent,
		Type:          models.MessageTypeClarificationRequest,
		Metadata: map[string]interface{}{
			"pending_id":  pendingID,
			"question_id": questionID,
			"status":      "pending",
		},
		CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("CreateMessage(%s): %v", id, err)
	}
}

func seedPendingActionSession(t *testing.T, repo *Repository, taskID, sessionID string) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: taskID}); err != nil {
		t.Fatalf("CreateTask(%s): %v", taskID, err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession(%s): %v", sessionID, err)
	}
}

func createPendingActionTurn(
	t *testing.T,
	repo *Repository,
	taskID, sessionID, turnID string,
	startedAt, createdAt time.Time,
) {
	t.Helper()
	if err := repo.CreateTurn(context.Background(), &models.Turn{
		ID: turnID, TaskSessionID: sessionID, TaskID: taskID,
		StartedAt: startedAt, CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("CreateTurn(%s): %v", turnID, err)
	}
}

func TestFindActiveClarificationMessagesBySessionIDUsesNewestDurableTurn(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 14, 12, 0, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-active", "session-active")
	createPendingActionTurn(t, repo, "task-active", "session-active", "turn-older", base, base)
	createPendingActionMessage(t, repo, "clarification-older", "task-active", "session-active", "turn-older", models.MessageTypeClarificationRequest, "pending", base)
	createPendingActionTurn(t, repo, "task-active", "session-active", "turn-newer", base.Add(time.Minute), base.Add(time.Minute))
	createPendingActionMessage(t, repo, "ordinary-newer", "task-active", "session-active", "turn-newer", models.MessageTypeMessage, "<missing>", base.Add(time.Minute))

	got, err := repo.FindActiveClarificationMessagesBySessionID(ctx, "session-active")
	if err != nil {
		t.Fatalf("FindActiveClarificationMessagesBySessionID: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("active clarifications = %v, want none from older turn", messageIDs(got))
	}
	actions, err := repo.GetPendingActionsBySessionIDs(ctx, []string{"session-active"})
	if err != nil {
		t.Fatalf("GetPendingActionsBySessionIDs: %v", err)
	}
	if _, ok := actions["session-active"]; ok {
		t.Fatalf("pending action reactivated from older turn: %#v", actions)
	}

	if err := repo.DeleteMessage(ctx, "ordinary-newer"); err != nil {
		t.Fatalf("DeleteMessage(newer): %v", err)
	}
	got, err = repo.FindActiveClarificationMessagesBySessionID(ctx, "session-active")
	if err != nil {
		t.Fatalf("FindActiveClarificationMessagesBySessionID after delete: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("message deletion reactivated older clarification: %v", messageIDs(got))
	}
	actions, err = repo.GetPendingActionsBySessionIDs(ctx, []string{"session-active"})
	if err != nil {
		t.Fatalf("GetPendingActionsBySessionIDs after delete: %v", err)
	}
	if _, ok := actions["session-active"]; ok {
		t.Fatalf("message deletion reactivated older pending action: %#v", actions)
	}
}

func TestPendingClarificationIgnoresEmptyUnpublishedSuccessor(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 15, 18, 0, 0, 0, time.UTC)
	const taskID = "task-unpublished"
	const sessionID = "session-unpublished"
	seedPendingActionSession(t, repo, taskID, sessionID)
	createPendingActionTurn(t, repo, taskID, sessionID, "turn-clarification", base, base)
	createPendingActionMessage(t, repo, "clarification-current", taskID, sessionID, "turn-clarification", models.MessageTypeClarificationRequest, "pending", base)
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-unpublished", TaskSessionID: sessionID, TaskID: taskID,
		StartedAt: base.Add(time.Minute), CreatedAt: base.Add(time.Minute),
		Metadata: map[string]interface{}{models.TurnMetaKeyPromptDispatchPending: true},
	}); err != nil {
		t.Fatalf("CreateTurn(unpublished): %v", err)
	}

	active, err := repo.FindActiveClarificationMessagesBySessionID(ctx, sessionID)
	if err != nil {
		t.Fatalf("FindActiveClarificationMessagesBySessionID: %v", err)
	}
	if ids := messageIDs(active); len(ids) != 1 || ids[0] != "clarification-current" {
		t.Fatalf("active clarification IDs = %v, want predecessor bundle", ids)
	}
	actions, err := repo.GetPendingActionsBySessionIDs(ctx, []string{sessionID})
	if err != nil {
		t.Fatalf("GetPendingActionsBySessionIDs: %v", err)
	}
	if actions[sessionID] != models.TaskPendingActionClarification {
		t.Fatalf("pending action = %q, want clarification", actions[sessionID])
	}

	createPendingActionMessage(t, repo, "successor-output", taskID, sessionID, "turn-unpublished", models.MessageTypeMessage, "<missing>", base.Add(2*time.Minute))
	actions, err = repo.GetPendingActionsBySessionIDs(ctx, []string{sessionID})
	if err != nil {
		t.Fatalf("GetPendingActionsBySessionIDs after output: %v", err)
	}
	if _, ok := actions[sessionID]; ok {
		t.Fatalf("message-backed successor did not supersede clarification: %#v", actions)
	}
}

func TestAmbiguousEmptySuccessorSupersedesClarification(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 15, 18, 30, 0, 0, time.UTC)
	const taskID = "task-accepted-successor"
	const sessionID = "session-accepted-successor"
	seedPendingActionSession(t, repo, taskID, sessionID)
	createPendingActionTurn(t, repo, taskID, sessionID, "turn-clarification", base, base)
	createPendingActionMessage(
		t, repo, "clarification-predecessor", taskID, sessionID, "turn-clarification",
		models.MessageTypeClarificationRequest, "pending", base,
	)
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-accepted-empty", TaskSessionID: sessionID, TaskID: taskID,
		StartedAt: base.Add(time.Minute), CreatedAt: base.Add(time.Minute),
		Metadata: map[string]interface{}{
			models.TurnMetaKeyPromptDispatchPending:   true,
			models.TurnMetaKeyPromptDispatchAttempted: true,
		},
	}); err != nil {
		t.Fatalf("CreateTurn(accepted empty): %v", err)
	}

	active, err := repo.FindActiveClarificationMessagesBySessionID(ctx, sessionID)
	if err != nil {
		t.Fatalf("FindActiveClarificationMessagesBySessionID: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("accepted successor left predecessor active: %v", messageIDs(active))
	}
	actions, err := repo.GetPendingActionsBySessionIDs(ctx, []string{sessionID})
	if err != nil {
		t.Fatalf("GetPendingActionsBySessionIDs: %v", err)
	}
	if _, ok := actions[sessionID]; ok {
		t.Fatalf("accepted successor left predecessor pending: %#v", actions)
	}
}

func TestFindActiveClarificationMessagesSupportsMissingStatusInCurrentTurn(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 14, 13, 0, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-status", "session-status")
	createPendingActionTurn(t, repo, "task-status", "session-status", "turn-status", base, base)
	createPendingActionMessage(t, repo, "clarification-missing", "task-status", "session-status", "turn-status", models.MessageTypeClarificationRequest, "<missing>", base)
	createPendingActionMessage(t, repo, "clarification-pending", "task-status", "session-status", "turn-status", models.MessageTypeClarificationRequest, "pending", base.Add(time.Second))
	createPendingActionMessage(t, repo, "clarification-answered", "task-status", "session-status", "turn-status", models.MessageTypeClarificationRequest, "answered", base.Add(2*time.Second))

	got, err := repo.FindActiveClarificationMessagesBySessionID(ctx, "session-status")
	if err != nil {
		t.Fatalf("FindActiveClarificationMessagesBySessionID: %v", err)
	}
	if ids := messageIDs(got); len(ids) != 2 || ids[0] != "clarification-missing" || ids[1] != "clarification-pending" {
		t.Fatalf("active clarification IDs = %v, want missing and pending current-turn rows", ids)
	}
}

func TestGetPendingActionsIgnoresMessagesWithoutDurableTurn(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedPendingActionSession(t, repo, "task-orphan", "session-orphan")
	if _, err := repo.db.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		t.Fatalf("disable foreign keys for malformed-history fixture: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.db.Exec("PRAGMA foreign_keys=ON") })
	createPendingActionMessage(
		t,
		repo,
		"clarification-orphan",
		"task-orphan",
		"session-orphan",
		"missing-turn",
		models.MessageTypeClarificationRequest,
		"pending",
		time.Date(2026, time.August, 15, 14, 30, 0, 0, time.UTC),
	)

	actions, err := repo.GetPendingActionsBySessionIDs(ctx, []string{"session-orphan"})
	if err != nil {
		t.Fatalf("GetPendingActionsBySessionIDs: %v", err)
	}
	if _, ok := actions["session-orphan"]; ok {
		t.Fatalf("orphan message became authoritative without a durable turn: %#v", actions)
	}
}

func TestFindActiveClarificationMessagesUsesDeterministicTurnTieBreak(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	stamp := time.Date(2026, time.August, 14, 14, 0, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-tie", "session-tie")
	createPendingActionTurn(t, repo, "task-tie", "session-tie", "turn-a", stamp, stamp)
	createPendingActionMessage(t, repo, "clarification-tie-old", "task-tie", "session-tie", "turn-a", models.MessageTypeClarificationRequest, "pending", stamp)
	createPendingActionTurn(t, repo, "task-tie", "session-tie", "turn-z", stamp, stamp)

	got, err := repo.FindActiveClarificationMessagesBySessionID(ctx, "session-tie")
	if err != nil {
		t.Fatalf("FindActiveClarificationMessagesBySessionID: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("turn id descending tie-break did not select turn-z: %v", messageIDs(got))
	}
}

func TestCompleteActiveClarificationBundleClaimsExactlyOnce(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-claim", "session-claim")
	createPendingActionTurn(t, repo, "task-claim", "session-claim", "turn-claim", base, base)
	createClarificationBundleMessage(t, repo, "message-q1", "task-claim", "session-claim", "turn-claim", "pending-claim", "q1", base)
	createClarificationBundleMessage(t, repo, "message-q2", "task-claim", "session-claim", "turn-claim", "pending-claim", "q2", base.Add(time.Nanosecond))

	responses := map[string]interface{}{
		"q1": map[string]interface{}{"question_id": "q1", "custom_text": "first"},
		"q2": map[string]interface{}{"question_id": "q2", "custom_text": "second"},
	}
	start := make(chan struct{})
	results := make(chan bool, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, claimed, err := repo.CompleteActiveClarificationBundle(ctx, "pending-claim", "answered", responses)
			results <- claimed
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)

	claims := 0
	for claimed := range results {
		if claimed {
			claims++
		}
	}
	if claims != 1 {
		t.Fatalf("successful claims = %d, want 1", claims)
	}
	for err := range errs {
		if err != nil {
			t.Fatalf("CompleteActiveClarificationBundle: %v", err)
		}
	}
	for _, id := range []string{"message-q1", "message-q2"} {
		message, err := repo.GetMessage(ctx, id)
		if err != nil {
			t.Fatalf("GetMessage(%s): %v", id, err)
		}
		if message.Metadata["status"] != "answered" {
			t.Fatalf("%s status = %v, want answered", id, message.Metadata["status"])
		}
		if message.Metadata["response"] == nil {
			t.Fatalf("%s response was not persisted", id)
		}
	}
}

func TestCompleteActiveClarificationBundleRejectsSupersededTurn(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 15, 13, 0, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-superseded", "session-superseded")
	createPendingActionTurn(t, repo, "task-superseded", "session-superseded", "turn-old", base, base)
	createClarificationBundleMessage(t, repo, "message-old", "task-superseded", "session-superseded", "turn-old", "pending-old", "q1", base)
	createPendingActionTurn(t, repo, "task-superseded", "session-superseded", "turn-new", base.Add(time.Second), base.Add(time.Second))

	_, claimed, err := repo.CompleteActiveClarificationBundle(ctx, "pending-old", "answered", map[string]interface{}{
		"q1": map[string]interface{}{"question_id": "q1"},
	})
	if err != nil {
		t.Fatalf("CompleteActiveClarificationBundle: %v", err)
	}
	if claimed {
		t.Fatal("superseded clarification bundle was claimed")
	}
	message, err := repo.GetMessage(ctx, "message-old")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if message.Metadata["status"] != "pending" {
		t.Fatalf("status = %v, want pending", message.Metadata["status"])
	}
}

func TestCompleteActiveClarificationBundleRejectsTerminalSession(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 15, 13, 30, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-terminal", "session-terminal")
	createPendingActionTurn(t, repo, "task-terminal", "session-terminal", "turn-terminal", base, base)
	createClarificationBundleMessage(
		t, repo, "message-terminal", "task-terminal", "session-terminal", "turn-terminal",
		"pending-terminal", "q1", base,
	)
	if err := repo.UpdateTaskSessionState(
		ctx, "session-terminal", models.TaskSessionStateCancelled, "cancelled",
	); err != nil {
		t.Fatalf("cancel session: %v", err)
	}

	_, claimed, err := repo.CompleteActiveClarificationBundle(
		ctx,
		"pending-terminal",
		"answered",
		map[string]interface{}{"q1": map[string]interface{}{"question_id": "q1"}},
	)
	if err != nil {
		t.Fatalf("CompleteActiveClarificationBundle: %v", err)
	}
	if claimed {
		t.Fatal("terminal session clarification bundle was claimed")
	}
	message, err := repo.GetMessage(ctx, "message-terminal")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if message.Metadata["status"] != "pending" {
		t.Fatalf("status = %v, want pending quarantine", message.Metadata["status"])
	}
	actions, err := repo.GetPendingActionsBySessionIDs(ctx, []string{"session-terminal"})
	if err != nil {
		t.Fatalf("GetPendingActionsBySessionIDs: %v", err)
	}
	if _, ok := actions["session-terminal"]; ok {
		t.Fatalf("terminal session retained actionable projection: %#v", actions)
	}
}

func TestCompleteActiveClarificationBundleClaimsDetachedCurrentTurn(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 15, 13, 40, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-detached-claim", "session-detached-claim")
	createPendingActionTurn(
		t, repo, "task-detached-claim", "session-detached-claim", "turn-detached-claim", base, base,
	)
	createClarificationBundleMessage(
		t, repo, "message-detached-claim", "task-detached-claim", "session-detached-claim",
		"turn-detached-claim", "pending-detached-claim", "q1", base,
	)
	detached, err := repo.DetachActiveClarificationMessagesBySessionID(ctx, "session-detached-claim")
	if err != nil || len(detached) != 1 {
		t.Fatalf("DetachActiveClarificationMessagesBySessionID = %d rows, %v; want 1, nil", len(detached), err)
	}

	claimedMessages, claimed, err := repo.CompleteActiveClarificationBundle(
		ctx,
		"pending-detached-claim",
		clarificationStatusAnswered,
		map[string]interface{}{"q1": map[string]interface{}{"question_id": "q1"}},
	)
	if err != nil {
		t.Fatalf("CompleteActiveClarificationBundle: %v", err)
	}
	if !claimed {
		t.Fatal("detached current-turn clarification bundle was not claimed")
	}
	if ids := messageIDs(claimedMessages); len(ids) != 1 || ids[0] != "message-detached-claim" {
		t.Fatalf("claimed messages = %v, want detached current-turn row", ids)
	}
	message, err := repo.GetMessage(ctx, "message-detached-claim")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if message.Metadata["status"] != clarificationStatusAnswered || message.Metadata["agent_disconnected"] != true {
		t.Fatalf("detached message metadata = %#v, want answered and disconnected", message.Metadata)
	}
}

func TestLoadClaimedClarificationBundleUsesCurrentTurn(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 15, 14, 30, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-load-claim", "session-load-claim")
	createPendingActionTurn(t, repo, "task-load-claim", "session-load-claim", "turn-load-old", base, base)
	createClarificationBundleMessage(
		t, repo, "message-load-old", "task-load-claim", "session-load-claim", "turn-load-old",
		"pending-load", "q-old", base,
	)
	setClarificationMessageMetadata(t, repo, "message-load-old", func(metadata map[string]interface{}) {
		metadata["status"] = clarificationStatusResponding
	})
	createPendingActionTurn(
		t, repo, "task-load-claim", "session-load-claim", "turn-load-current",
		base.Add(time.Second), base.Add(time.Second),
	)
	createClarificationBundleMessage(
		t, repo, "message-load-current", "task-load-claim", "session-load-claim", "turn-load-current",
		"pending-load", "q-current", base.Add(time.Second),
	)
	setClarificationMessageMetadata(t, repo, "message-load-current", func(metadata map[string]interface{}) {
		metadata["status"] = clarificationStatusResponding
	})

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	loaded, err := repo.loadClaimedClarificationBundle(
		ctx,
		tx,
		repo.db.DriverName(),
		"pending-load",
	)
	if err != nil {
		t.Fatalf("loadClaimedClarificationBundle: %v", err)
	}
	if ids := messageIDs(loaded); len(ids) != 1 || ids[0] != "message-load-current" {
		t.Fatalf("claimed messages = %v, want only current-turn row", ids)
	}
}

func TestCompleteActiveClarificationBundleRecoversMixedStatusBundle(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 15, 15, 0, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-mixed", "session-mixed")
	createPendingActionTurn(t, repo, "task-mixed", "session-mixed", "turn-mixed", base, base)
	createClarificationBundleMessage(
		t, repo, "message-mixed-terminal", "task-mixed", "session-mixed", "turn-mixed",
		"pending-mixed", "q1", base,
	)
	createClarificationBundleMessage(
		t, repo, "message-mixed-pending", "task-mixed", "session-mixed", "turn-mixed",
		"pending-mixed", "q2", base.Add(time.Nanosecond),
	)
	terminal, err := repo.GetMessage(ctx, "message-mixed-terminal")
	if err != nil {
		t.Fatalf("GetMessage terminal sibling: %v", err)
	}
	terminal.Metadata["status"] = "rejected"
	if err := repo.UpdateMessage(ctx, terminal); err != nil {
		t.Fatalf("UpdateMessage terminal sibling: %v", err)
	}

	claimedMessages, claimed, err := repo.CompleteActiveClarificationBundle(
		ctx,
		"pending-mixed",
		"answered",
		map[string]interface{}{"q2": map[string]interface{}{"question_id": "q2"}},
	)
	if err != nil || !claimed {
		t.Fatalf("complete mixed bundle: claimed=%v err=%v", claimed, err)
	}
	if ids := messageIDs(claimedMessages); len(ids) != 1 || ids[0] != "message-mixed-pending" {
		t.Fatalf("claimed messages = %v, want only pending sibling", ids)
	}
	terminal, err = repo.GetMessage(ctx, "message-mixed-terminal")
	if err != nil {
		t.Fatalf("GetMessage preserved sibling: %v", err)
	}
	if terminal.Metadata["status"] != "rejected" {
		t.Fatalf("terminal sibling status = %v, want rejected", terminal.Metadata["status"])
	}
	completed, err := repo.GetMessage(ctx, "message-mixed-pending")
	if err != nil {
		t.Fatalf("GetMessage completed sibling: %v", err)
	}
	if completed.Metadata["status"] != "answered" {
		t.Fatalf("pending sibling status = %v, want answered", completed.Metadata["status"])
	}
	_, restored, err := repo.RestoreActiveClarificationBundle(
		ctx,
		"pending-mixed",
		"answered",
		claimedMessages,
	)
	if err != nil || !restored {
		t.Fatalf("restore claimed mixed rows: restored=%v err=%v", restored, err)
	}
	terminal, err = repo.GetMessage(ctx, "message-mixed-terminal")
	if err != nil {
		t.Fatalf("GetMessage terminal sibling after restore: %v", err)
	}
	if terminal.Metadata["status"] != "rejected" {
		t.Fatalf("terminal sibling changed during restore: %#v", terminal.Metadata)
	}
	completed, err = repo.GetMessage(ctx, "message-mixed-pending")
	if err != nil {
		t.Fatalf("GetMessage restored sibling: %v", err)
	}
	if completed.Metadata["status"] != "pending" || completed.Metadata["response"] != nil {
		t.Fatalf("restored sibling metadata = %#v, want pending without response", completed.Metadata)
	}
}

func TestRestoreActiveClarificationBundleAllowsRetryAfterDeliveryFailure(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 15, 13, 30, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-restore", "session-restore")
	createPendingActionTurn(t, repo, "task-restore", "session-restore", "turn-restore", base, base)
	createClarificationBundleMessage(
		t, repo, "message-restore", "task-restore", "session-restore", "turn-restore",
		"pending-restore", "q1", base,
	)
	responses := map[string]interface{}{
		"q1": map[string]interface{}{"question_id": "q1", "custom_text": "continue"},
	}

	claimedMessages, claimed, err := repo.CompleteActiveClarificationBundle(
		ctx, "pending-restore", "answered", responses,
	)
	if err != nil || !claimed {
		t.Fatalf("complete before restore: claimed=%v err=%v", claimed, err)
	}
	restoredMessages, restored, err := repo.RestoreActiveClarificationBundle(
		ctx,
		"pending-restore",
		"answered",
		claimedMessages,
	)
	if err != nil || !restored {
		t.Fatalf("restore: restored=%v err=%v", restored, err)
	}
	if len(restoredMessages) != 1 || restoredMessages[0].Metadata["status"] != "pending" {
		t.Fatalf("returned restored messages = %#v, want one pending row", restoredMessages)
	}
	message, err := repo.GetMessage(ctx, "message-restore")
	if err != nil {
		t.Fatalf("GetMessage after restore: %v", err)
	}
	if message.Metadata["status"] != "pending" || message.Metadata["response"] != nil {
		t.Fatalf("restored metadata = %#v, want pending without response", message.Metadata)
	}

	_, claimed, err = repo.CompleteActiveClarificationBundle(
		ctx, "pending-restore", "answered", responses,
	)
	if err != nil || !claimed {
		t.Fatalf("retry completion: claimed=%v err=%v", claimed, err)
	}
}

func TestRestoreDetachedClarificationClaimAfterTerminalTransitionStaysTerminal(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 15, 13, 45, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-terminal-restore", "session-terminal-restore")
	createPendingActionTurn(
		t, repo, "task-terminal-restore", "session-terminal-restore", "turn-terminal-restore", base, base,
	)
	createClarificationBundleMessage(
		t, repo, "message-terminal-restore", "task-terminal-restore", "session-terminal-restore",
		"turn-terminal-restore", "pending-terminal-restore", "q1", base,
	)
	detached, err := repo.DetachActiveClarificationMessagesBySessionID(ctx, "session-terminal-restore")
	if err != nil || len(detached) != 1 {
		t.Fatalf("detach before claim: detached=%d err=%v", len(detached), err)
	}
	claimedMessages, claimed, err := repo.CompleteActiveClarificationBundle(
		ctx,
		"pending-terminal-restore",
		"answered",
		map[string]interface{}{"q1": map[string]interface{}{"question_id": "q1"}},
	)
	if err != nil || !claimed {
		t.Fatalf("complete before terminal transition: claimed=%v err=%v", claimed, err)
	}
	if err := repo.UpdateTaskSessionState(
		ctx, "session-terminal-restore", models.TaskSessionStateCancelled, "cancelled",
	); err != nil {
		t.Fatalf("cancel session: %v", err)
	}

	_, restored, err := repo.RestoreActiveClarificationBundle(
		ctx, "pending-terminal-restore", "answered", claimedMessages,
	)
	if err != nil {
		t.Fatalf("RestoreActiveClarificationBundle: %v", err)
	}
	if restored {
		t.Fatal("terminal session clarification bundle was restored to pending")
	}
	message, err := repo.GetMessage(ctx, "message-terminal-restore")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if message.Metadata["status"] != "answered" || message.Metadata["agent_disconnected"] != true {
		t.Fatalf("metadata = %#v, want answered disconnected quarantine", message.Metadata)
	}
}

func TestRestoreClarificationMessagesLeavesInputUnchangedOnWriteFailure(t *testing.T) {
	repo := newRepoForSessionTests(t)
	tx, err := repo.db.BeginTxx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	message := &models.Message{
		ID: "missing-message",
		Metadata: map[string]interface{}{
			"status":   "answered",
			"response": map[string]interface{}{"custom_text": "continue"},
		},
	}

	err = repo.restoreClarificationMessages(
		context.Background(),
		tx,
		repo.db.DriverName(),
		[]*models.Message{message},
		"answered",
	)
	if err == nil {
		t.Fatal("restoreClarificationMessages succeeded for a missing row")
	}
	if message.Metadata["status"] != "answered" || message.Metadata["response"] == nil {
		t.Fatalf("failed restore mutated input metadata: %#v", message.Metadata)
	}
}

func TestCompleteClaimedClarificationMessagesLeavesInputUnchangedOnWriteFailure(t *testing.T) {
	repo := newRepoForSessionTests(t)
	base := time.Date(2026, time.August, 15, 22, 0, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-complete-input", "session-complete-input")
	createPendingActionTurn(
		t, repo, "task-complete-input", "session-complete-input", "turn-complete-input", base, base,
	)
	createClarificationBundleMessage(
		t, repo, "existing-message", "task-complete-input", "session-complete-input",
		"turn-complete-input", "pending-complete-input", "question-1", base,
	)
	setClarificationMessageMetadata(t, repo, "existing-message", func(metadata map[string]interface{}) {
		metadata["status"] = clarificationStatusResponding
	})
	tx, err := repo.db.BeginTxx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	existingMessage := &models.Message{
		ID: "existing-message",
		Metadata: map[string]interface{}{
			"question_id": "question-1",
			"status":      clarificationStatusResponding,
		},
	}
	missingMessage := &models.Message{
		ID: "missing-message",
		Metadata: map[string]interface{}{
			"question_id": "question-2",
			"status":      clarificationStatusResponding,
		},
	}

	err = repo.completeClaimedClarificationMessages(
		context.Background(),
		tx,
		repo.db.DriverName(),
		[]*models.Message{existingMessage, missingMessage},
		clarificationStatusAnswered,
		map[string]interface{}{"question-1": "continue", "question-2": "continue"},
	)
	if err == nil {
		t.Fatal("completeClaimedClarificationMessages succeeded for a missing row")
	}
	for _, message := range []*models.Message{existingMessage, missingMessage} {
		if message.Metadata["status"] != clarificationStatusResponding || message.Metadata["response"] != nil {
			t.Fatalf("failed completion mutated input metadata: %#v", message.Metadata)
		}
	}
}

func TestClaimActiveClarificationBundleUsesRepositoryClock(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 15, 22, 30, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-claim-clock", "session-claim-clock")
	createPendingActionTurn(
		t, repo, "task-claim-clock", "session-claim-clock", "turn-claim-clock", base, base,
	)
	createClarificationBundleMessage(
		t, repo, "message-claim-clock", "task-claim-clock", "session-claim-clock",
		"turn-claim-clock", "pending-claim-clock", "question-clock", base,
	)
	claimAt := time.Date(2037, time.March, 4, 5, 6, 7, 0, time.UTC)
	repo.clockNow = func() time.Time { return claimAt }

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	claimed, err := repo.claimActiveClarificationBundle(
		ctx, tx, repo.db.DriverName(), "pending-claim-clock",
	)
	if err != nil || claimed != 1 {
		t.Fatalf("claimActiveClarificationBundle = %d, %v; want 1, nil", claimed, err)
	}
	var updatedAt time.Time
	if err := tx.GetContext(
		ctx,
		&updatedAt,
		repo.db.Rebind(`SELECT updated_at FROM task_session_messages WHERE id = ?`),
		"message-claim-clock",
	); err != nil {
		t.Fatalf("load claimed timestamp: %v", err)
	}
	if !updatedAt.Equal(claimAt) {
		t.Fatalf("claimed updated_at = %v, want repository time %v", updatedAt, claimAt)
	}
}

func TestRestoreActiveClarificationBundleDoesNotReactivateSupersededTurn(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.August, 15, 13, 45, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-restore-old", "session-restore-old")
	createPendingActionTurn(t, repo, "task-restore-old", "session-restore-old", "turn-restore-old", base, base)
	createClarificationBundleMessage(
		t, repo, "message-restore-old", "task-restore-old", "session-restore-old", "turn-restore-old",
		"pending-restore-old", "q1", base,
	)
	claimedMessages, claimed, err := repo.CompleteActiveClarificationBundle(
		ctx,
		"pending-restore-old",
		"answered",
		map[string]interface{}{"q1": map[string]interface{}{"question_id": "q1"}},
	)
	if err != nil || !claimed {
		t.Fatalf("complete before supersession: claimed=%v err=%v", claimed, err)
	}
	createPendingActionTurn(
		t, repo, "task-restore-old", "session-restore-old", "turn-restore-new",
		base.Add(time.Second), base.Add(time.Second),
	)

	_, restored, err := repo.RestoreActiveClarificationBundle(
		ctx,
		"pending-restore-old",
		"answered",
		claimedMessages,
	)
	if err != nil {
		t.Fatalf("RestoreActiveClarificationBundle: %v", err)
	}
	if restored {
		t.Fatal("superseded terminal bundle was restored")
	}
}
