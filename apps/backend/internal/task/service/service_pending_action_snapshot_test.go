package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestPendingActionSnapshotRevisionTracksProjectionChanges(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	turnID := setupTestTurn(t, repo, sessionID, "task-123", "turn-pending")
	message := &models.Message{
		ID:            "message-pending",
		TaskSessionID: sessionID,
		TaskID:        "task-123",
		TurnID:        turnID,
		AuthorType:    models.MessageAuthorAgent,
		Content:       "Choose",
		Type:          models.MessageTypeClarificationRequest,
		Metadata:      map[string]interface{}{"pending_id": "pending-1", "status": "pending"},
	}
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatalf("create pending message: %v", err)
	}
	svc.SetPendingActionProjectionEpoch(7)

	actions, revisions, err := svc.GetPendingActionSnapshotProjectionsForSessions(ctx, []string{sessionID})
	if err != nil {
		t.Fatalf("get initial snapshot: %v", err)
	}
	if actions[sessionID] != models.TaskPendingActionClarification {
		t.Fatalf("initial action = %q, want clarification", actions[sessionID])
	}
	initialRevision := revisions[sessionID]
	if initialRevision.Epoch != "7" || initialRevision.Sequence == 0 {
		t.Fatalf("initial revision = %#v, want non-zero revision in epoch 7", initialRevision)
	}

	_, unchangedRevisions, err := svc.GetPendingActionSnapshotProjectionsForSessions(ctx, []string{sessionID})
	if err != nil {
		t.Fatalf("get unchanged snapshot: %v", err)
	}
	if unchangedRevisions[sessionID] != initialRevision {
		t.Fatalf("unchanged revision = %#v, want stable %#v", unchangedRevisions[sessionID], initialRevision)
	}

	message.Metadata["status"] = "answered"
	if err := repo.UpdateMessage(ctx, message); err != nil {
		t.Fatalf("resolve pending message: %v", err)
	}
	clearedActions, clearedRevisions, err := svc.GetPendingActionSnapshotProjectionsForSessions(ctx, []string{sessionID})
	if err != nil {
		t.Fatalf("get changed snapshot: %v", err)
	}
	if _, exists := clearedActions[sessionID]; exists {
		t.Fatalf("cleared snapshot action = %q, want absent", clearedActions[sessionID])
	}
	changedRevision := clearedRevisions[sessionID]
	if changedRevision.Epoch != initialRevision.Epoch || changedRevision.Sequence <= initialRevision.Sequence {
		t.Fatalf("changed revision = %#v, want newer than %#v", changedRevision, initialRevision)
	}

	_, repeatedRevisions, err := svc.GetPendingActionSnapshotProjectionsForSessions(ctx, []string{sessionID})
	if err != nil {
		t.Fatalf("get repeated snapshot: %v", err)
	}
	if repeatedRevisions[sessionID] != changedRevision {
		t.Fatalf("repeated revision = %#v, want stable %#v", repeatedRevisions[sessionID], changedRevision)
	}
}

func TestPendingActionSnapshotRejectsOlderReadAfterNewerEvent(t *testing.T) {
	svc, _, _ := createTestService(t)
	svc.SetPendingActionProjectionEpoch(7)
	older := models.PendingActionRevision{Epoch: "7", Sequence: 1}
	newer := models.PendingActionRevision{Epoch: "7", Sequence: 2}
	svc.pendingActionProjectionObserved = map[string]pendingActionProjectionState{
		"session-1": {action: models.TaskPendingActionPermission, revision: newer},
	}
	svc.pendingActionSnapshotValues = map[string]pendingActionProjectionState{
		"session-1": {action: models.TaskPendingActionPermission, revision: newer},
	}
	svc.lastPendingActionProjections = map[string]pendingActionProjectionState{
		"session-1": {action: models.TaskPendingActionPermission, revision: newer},
	}

	actions, revisions := svc.stabilizePendingActionSnapshotProjections(
		[]string{"session-1"},
		map[string]models.TaskPendingAction{"session-1": models.TaskPendingActionClarification},
		map[string]models.PendingActionRevision{"session-1": older},
	)
	if actions["session-1"] != models.TaskPendingActionPermission {
		t.Fatalf("stale action = %q, want the newer event's permission", actions["session-1"])
	}
	if revisions["session-1"] != newer {
		t.Fatalf("stale revision = %#v, want newer event revision %#v", revisions["session-1"], newer)
	}
}

func TestPendingActionNewerUnchangedEventWatermarkRejectsDelayedChange(t *testing.T) {
	svc, _, _ := createTestService(t)
	svc.SetPendingActionProjectionEpoch(7)
	initial := models.PendingActionRevision{Epoch: "7", Sequence: 1}
	older := models.PendingActionRevision{Epoch: "7", Sequence: 2}
	newer := models.PendingActionRevision{Epoch: "7", Sequence: 3}
	seedPendingActionProjection(svc, models.TaskPendingAction(""), initial)

	olderFinished := make(chan struct{})
	releaseOlder := make(chan struct{})
	olderResult := make(chan pendingActionObservationResult, 1)
	go func() {
		close(olderFinished)
		<-releaseOlder
		actions, revisions := svc.stabilizePendingActionEventProjections(
			[]string{"session-1"},
			map[string]models.TaskPendingAction{"session-1": models.TaskPendingActionPermission},
			map[string]models.PendingActionRevision{"session-1": older},
		)
		changed := svc.pendingActionProjectionChanged("session-1", actions["session-1"], revisions["session-1"])
		olderResult <- pendingActionObservationResult{actions: actions, revisions: revisions, changed: changed}
	}()
	<-olderFinished

	newerActions, newerRevisions := svc.stabilizePendingActionEventProjections(
		[]string{"session-1"},
		map[string]models.TaskPendingAction{"session-1": ""},
		map[string]models.PendingActionRevision{"session-1": newer},
	)
	if svc.pendingActionProjectionChanged("session-1", newerActions["session-1"], newerRevisions["session-1"]) {
		t.Fatal("unchanged newer event should not emit a duplicate pending-action transition")
	}
	close(releaseOlder)
	result := <-olderResult
	if result.actions["session-1"] != "" || result.revisions["session-1"] != newer || result.changed {
		t.Fatalf("delayed event result = %#v, want unchanged action at newer watermark", result)
	}
}

func TestPendingActionNewerUnchangedSnapshotWatermarkRejectsDelayedChange(t *testing.T) {
	svc, _, _ := createTestService(t)
	svc.SetPendingActionProjectionEpoch(7)
	initial := models.PendingActionRevision{Epoch: "7", Sequence: 1}
	older := models.PendingActionRevision{Epoch: "7", Sequence: 2}
	newer := models.PendingActionRevision{Epoch: "7", Sequence: 3}
	seedPendingActionProjection(svc, "", initial)

	olderFinished := make(chan struct{})
	releaseOlder := make(chan struct{})
	olderResult := make(chan pendingActionObservationResult, 1)
	go func() {
		close(olderFinished)
		<-releaseOlder
		actions, revisions := svc.stabilizePendingActionSnapshotProjections(
			[]string{"session-1"},
			map[string]models.TaskPendingAction{"session-1": models.TaskPendingActionPermission},
			map[string]models.PendingActionRevision{"session-1": older},
		)
		olderResult <- pendingActionObservationResult{actions: actions, revisions: revisions}
	}()
	<-olderFinished

	newerActions, newerRevisions := svc.stabilizePendingActionSnapshotProjections(
		[]string{"session-1"},
		map[string]models.TaskPendingAction{"session-1": ""},
		map[string]models.PendingActionRevision{"session-1": newer},
	)
	if newerActions["session-1"] != "" || newerRevisions["session-1"] != initial {
		t.Fatalf("unchanged snapshot = %#v / %#v, want original stable representation", newerActions, newerRevisions)
	}
	close(releaseOlder)
	result := <-olderResult
	if result.actions["session-1"] != "" || result.revisions["session-1"] != initial {
		t.Fatalf("delayed snapshot = %#v, want unchanged action at stable revision %#v", result, initial)
	}
}

type pendingActionObservationResult struct {
	actions   map[string]models.TaskPendingAction
	revisions map[string]models.PendingActionRevision
	changed   bool
}

func seedPendingActionProjection(svc *Service, action models.TaskPendingAction, revision models.PendingActionRevision) {
	state := pendingActionProjectionState{action: action, revision: revision}
	svc.pendingActionProjectionObserved = map[string]pendingActionProjectionState{"session-1": state}
	svc.pendingActionSnapshotValues = map[string]pendingActionProjectionState{"session-1": state}
	svc.lastPendingActionProjections = map[string]pendingActionProjectionState{"session-1": state}
}
