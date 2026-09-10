package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestSwitchSessionForStep_ParkOnEndPreservesSourceWhenPromotionFails(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	if _, err := fixture.svc.messageQueue.QueueMessage(
		ctx, fixture.current.ID, "t1", "queued promotion handoff", "", messagequeue.QueuedByUser, false, nil,
	); err != nil {
		t.Fatalf("queue promotion handoff: %v", err)
	}
	if err := fixture.svc.messageQueue.SetPendingMove(ctx, fixture.current.ID, &messagequeue.PendingMove{
		TaskID:         "t1",
		WorkflowID:     "wf1",
		WorkflowStepID: "step-b",
	}); err != nil {
		t.Fatalf("set pending move: %v", err)
	}
	promotionErr := errors.New("destination promotion failed")
	fixture.svc.repo = failProfileSwitchPromotionRepo{repoStore: fixture.repo, err: promotionErr}

	_, _, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current, &wfmodels.WorkflowStep{
		ID: "step-b", WorkflowID: "wf1", Position: 1, ProfileSessionStartPolicy: fixture.startPolicy,
	}, &wfmodels.WorkflowStep{ID: "step-a", WorkflowID: "wf1", ProfileSessionEndPolicy: fixture.endPolicy})
	if !errors.Is(err, promotionErr) {
		t.Fatalf("parked profile switch error = %v, want promotion failure", err)
	}

	source, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	if err != nil {
		t.Fatalf("reload source session: %v", err)
	}
	if source.State != models.TaskSessionStateRunning || !source.IsPrimary {
		t.Fatalf("source after promotion failure = state %s primary %t, want running primary", source.State, source.IsPrimary)
	}
	if source.CompletedAt != nil {
		t.Fatal("source after promotion failure must not be completed")
	}
	if _, ok := source.Metadata[models.SessionMetaKeyWorkflowProfileSwitchStopIntent]; ok {
		t.Fatal("promotion failure must not leave stop metadata")
	}
	status := fixture.svc.messageQueue.GetStatus(ctx, fixture.current.ID)
	if status.Count != 1 || status.Entries[0].Content != "queued promotion handoff" {
		t.Fatalf("source queue after promotion failure = %+v, want queued promotion handoff preserved", status.Entries)
	}
	move, exists := fixture.svc.messageQueue.GetPendingMove(ctx, fixture.current.ID)
	if !exists || move == nil || move.WorkflowStepID != "step-b" {
		t.Fatalf("source pending move after promotion failure = %+v exists=%t, want step-b preserved", move, exists)
	}
}

type failProfileSwitchQueueTransferRepository struct {
	messagequeue.Repository
	err           error
	beforeFailure func(context.Context, messagequeue.QueueSessionIdentity) error
}

func (r *failProfileSwitchQueueTransferRepository) TransferSessionIdentities(
	ctx context.Context,
	_ messagequeue.QueueSessionIdentity,
	destination messagequeue.QueueSessionIdentity,
) error {
	if r.beforeFailure != nil {
		if err := r.beforeFailure(ctx, destination); err != nil {
			return err
		}
	}
	return r.err
}

func installFailingProfileSwitchQueue(
	t *testing.T,
	fixture *profileSwitchFixture,
	transferErr error,
) *failProfileSwitchQueueTransferRepository {
	t.Helper()
	queueRepo := messagequeue.NewMemoryRepositoryWithAuthority(
		func(ctx context.Context, taskID, sessionID string) (messagequeue.QueueSessionIdentity, error) {
			session, err := fixture.repo.GetTaskSession(ctx, sessionID)
			if err != nil {
				return messagequeue.QueueSessionIdentity{}, err
			}
			if session.TaskID != taskID || session.QueueIncarnationID == "" {
				return messagequeue.QueueSessionIdentity{}, messagequeue.ErrSessionIdentityMismatch
			}
			return messagequeue.QueueSessionIdentity{
				TaskID: taskID, SessionID: sessionID, SessionIncarnationID: session.QueueIncarnationID,
			}, nil
		},
	)
	failingRepo := &failProfileSwitchQueueTransferRepository{Repository: queueRepo, err: transferErr}
	fixture.svc.messageQueue = messagequeue.NewService(
		failingRepo,
		messagequeue.DefaultMaxPerSession,
		fixture.svc.logger,
	)
	return failingRepo
}

func queueProfileSwitchHandoff(t *testing.T, fixture *profileSwitchFixture, content string) {
	t.Helper()
	if _, err := fixture.svc.messageQueue.QueueMessage(
		context.Background(), fixture.current.ID, fixture.current.TaskID,
		content, "", messagequeue.QueuedByUser, false, nil,
	); err != nil {
		t.Fatalf("queue profile-switch handoff: %v", err)
	}
}

func TestCreateNewSessionForStep_TransferFailureRestoresSourceAndRemovesDestination(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(
		t,
		models.WorkflowProfileSessionStartPolicyNew,
		models.WorkflowProfileSessionEndPolicyComplete,
	)
	transferErr := errors.New("queue transfer failed")
	installFailingProfileSwitchQueue(t, fixture, transferErr)
	queueProfileSwitchHandoff(t, fixture, "new-session handoff")

	_, err := fixture.svc.createNewSessionForStepWithEndPolicy(
		ctx, "t1", fixture.current, "profile-b", fixture.endPolicy,
	)
	if !errors.Is(err, transferErr) {
		t.Fatalf("profile switch error = %v, want queue transfer failure", err)
	}

	source, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	if err != nil {
		t.Fatalf("reload source: %v", err)
	}
	if source.State != models.TaskSessionStateRunning || !source.IsPrimary || source.CompletedAt != nil {
		t.Fatalf("source after transfer failure = state %s primary %t completed %v", source.State, source.IsPrimary, source.CompletedAt)
	}
	sessions, err := fixture.repo.ListTaskSessions(ctx, fixture.current.TaskID)
	if err != nil {
		t.Fatalf("list task sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != fixture.current.ID {
		t.Fatalf("sessions after transfer failure = %+v, want only source", sessions)
	}
	status := fixture.svc.messageQueue.GetStatus(ctx, fixture.current.ID)
	if status.Count != 1 || status.Entries[0].Content != "new-session handoff" {
		t.Fatalf("source queue after transfer failure = %+v", status.Entries)
	}
}

func TestReuseSessionForStep_TransferFailureRestoresSourcePrimary(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(
		t,
		models.WorkflowProfileSessionStartPolicyReuse,
		models.WorkflowProfileSessionEndPolicyPark,
	)
	destination := &models.TaskSession{
		ID: "session-b-existing", TaskID: fixture.current.TaskID, AgentProfileID: "profile-b",
		ExecutorID: "exec-local", ExecutorProfileID: "ep1",
		State: models.TaskSessionStateWaitingForInput, StartedAt: fixture.current.StartedAt,
		UpdatedAt: fixture.current.UpdatedAt,
	}
	if err := fixture.repo.CreateTaskSession(ctx, destination); err != nil {
		t.Fatalf("create reusable destination: %v", err)
	}
	transferErr := errors.New("queue transfer failed")
	installFailingProfileSwitchQueue(t, fixture, transferErr)
	queueProfileSwitchHandoff(t, fixture, "reuse-session handoff")

	_, err := fixture.svc.reuseSessionForStepWithEndPolicy(
		ctx, fixture.current.TaskID, fixture.current, destination, fixture.endPolicy,
	)
	if !errors.Is(err, transferErr) {
		t.Fatalf("profile switch error = %v, want queue transfer failure", err)
	}

	source, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	if err != nil {
		t.Fatalf("reload source: %v", err)
	}
	if source.State != models.TaskSessionStateWaitingForInput || !source.IsPrimary {
		t.Fatalf("source after transfer failure = state %s primary %t, want waiting primary", source.State, source.IsPrimary)
	}
	reused, err := fixture.repo.GetTaskSession(ctx, destination.ID)
	if err != nil {
		t.Fatalf("reload reusable destination: %v", err)
	}
	if reused.IsPrimary || reused.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("destination after transfer failure = state %s primary %t", reused.State, reused.IsPrimary)
	}
	status := fixture.svc.messageQueue.GetStatus(ctx, fixture.current.ID)
	if status.Count != 1 || status.Entries[0].Content != "reuse-session handoff" {
		t.Fatalf("source queue after transfer failure = %+v", status.Entries)
	}
}

func TestCreateNewSessionForStep_TransferFailurePreservesDestinationArrival(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(
		t,
		models.WorkflowProfileSessionStartPolicyNew,
		models.WorkflowProfileSessionEndPolicyComplete,
	)
	transferErr := errors.New("queue transfer failed")
	queueRepo := installFailingProfileSwitchQueue(t, fixture, transferErr)
	queueRepo.beforeFailure = func(
		ctx context.Context,
		destination messagequeue.QueueSessionIdentity,
	) error {
		return queueRepo.InsertForSession(ctx, destination, &messagequeue.QueuedMessage{
			ID:        "destination-arrival",
			SessionID: destination.SessionID,
			TaskID:    destination.TaskID,
			Content:   "arrived during switch",
			QueuedBy:  messagequeue.QueuedByUser,
		}, messagequeue.DefaultMaxPerSession)
	}
	queueProfileSwitchHandoff(t, fixture, "source handoff")

	_, err := fixture.svc.createNewSessionForStepWithEndPolicy(
		ctx, fixture.current.TaskID, fixture.current, "profile-b", fixture.endPolicy,
	)
	if !errors.Is(err, transferErr) {
		t.Fatalf("profile switch error = %v, want queue transfer failure", err)
	}

	source, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	if err != nil || !source.IsPrimary {
		t.Fatalf("source after transfer failure = %+v err=%v, want primary", source, err)
	}
	sessions, err := fixture.repo.ListTaskSessions(ctx, fixture.current.TaskID)
	if err != nil {
		t.Fatalf("list task sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("session count after destination arrival = %d, want 2", len(sessions))
	}
	for _, session := range sessions {
		if session.ID == source.ID {
			continue
		}
		if session.State != models.TaskSessionStateFailed || session.IsPrimary {
			t.Fatalf("retained destination = state %s primary %t, want failed nonprimary", session.State, session.IsPrimary)
		}
		status := fixture.svc.messageQueue.GetStatus(ctx, session.ID)
		if status.Count != 1 || status.Entries[0].ID != "destination-arrival" {
			t.Fatalf("retained destination queue = %+v", status.Entries)
		}
	}
}
