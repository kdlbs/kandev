package executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// A workflow profile switch parks its source session in WAITING_FOR_INPUT
// while the replacement session takes over. A launch that was already in
// flight must observe that as a supersede, not as a launch failure.
func TestPersistSessionFullRowIfCurrentState_WorkflowParkIsSuperseded(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	repo.sessions["session-1"] = &models.TaskSession{
		ID:     "session-1",
		TaskID: "task-1",
		State:  models.TaskSessionStateWaitingForInput,
		Metadata: map[string]interface{}{
			models.SessionMetaKeyWorkflowParking: models.WorkflowParking{
				Stamp:           "park-1",
				ParkedAt:        time.Now().UTC(),
				SourceSessionID: "session-1",
				DestinationID:   "session-2",
			},
		},
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	stale := &models.TaskSession{ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateCreated}

	err := exec.persistSessionFullRowIfCurrentState(ctx, stale, models.TaskSessionStateCreated)
	if !IsSessionStateSwitched(err) {
		t.Fatalf("persistSessionFullRowIfCurrentState() error = %v, want SessionStateSwitchedError", err)
	}
	var switched *SessionStateSwitchedError
	if !errors.As(err, &switched) {
		t.Fatalf("error %T does not unwrap to *SessionStateSwitchedError", err)
	}
	if switched.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("switched state = %q, want WAITING_FOR_INPUT", switched.State)
	}
}

// An unexpected state without a workflow switch record is still a real launch
// failure, so the supersede classification must not widen.
func TestPersistSessionFullRowIfCurrentState_UnexpectedStateWithoutSwitchIsError(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	repo.sessions["session-1"] = &models.TaskSession{
		ID:     "session-1",
		TaskID: "task-1",
		State:  models.TaskSessionStateWaitingForInput,
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	stale := &models.TaskSession{ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateCreated}

	err := exec.persistSessionFullRowIfCurrentState(ctx, stale, models.TaskSessionStateCreated)
	if err == nil {
		t.Fatal("persistSessionFullRowIfCurrentState() error = nil, want unexpected-state failure")
	}
	if IsSessionStateSwitched(err) {
		t.Fatalf("unexpected state was classified as a workflow switch: %v", err)
	}
}

// A parking marker that names another session (the destination) must not
// excuse this launch's state mismatch.
func TestPersistSessionFullRowIfCurrentState_ForeignParkingMarkerIsNotSuperseded(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	repo.sessions["session-1"] = &models.TaskSession{
		ID:     "session-1",
		TaskID: "task-1",
		State:  models.TaskSessionStateWaitingForInput,
		Metadata: map[string]interface{}{
			models.SessionMetaKeyWorkflowParking: models.WorkflowParking{
				Stamp:           "park-1",
				ParkedAt:        time.Now().UTC(),
				SourceSessionID: "session-other",
			},
		},
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	stale := &models.TaskSession{ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateCreated}

	err := exec.persistSessionFullRowIfCurrentState(ctx, stale, models.TaskSessionStateCreated)
	if err == nil || IsSessionStateSwitched(err) {
		t.Fatalf("persistSessionFullRowIfCurrentState() error = %v, want a plain unexpected-state failure", err)
	}
}

func TestPersistSessionFullRowIfCurrentState_NewParkingStampSupersedesUnchangedWaitingState(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	repo.sessions["session-1"] = &models.TaskSession{
		ID:     "session-1",
		TaskID: "task-1",
		State:  models.TaskSessionStateWaitingForInput,
		Metadata: map[string]interface{}{
			models.SessionMetaKeyWorkflowParking: models.WorkflowParking{
				Stamp:           "park-new",
				ParkedAt:        time.Now().UTC(),
				SourceSessionID: "session-1",
			},
		},
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	launch := &models.TaskSession{
		ID:     "session-1",
		TaskID: "task-1",
		State:  models.TaskSessionStateStarting,
	}

	err := exec.persistSessionFullRowIfCurrentState(ctx, launch, models.TaskSessionStateWaitingForInput)
	if !IsSessionStateSwitched(err) {
		t.Fatalf("persistSessionFullRowIfCurrentState() error = %v, want SessionStateSwitchedError", err)
	}
	if got := repo.sessions["session-1"].State; got != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state = %q, want WAITING_FOR_INPUT", got)
	}
}

func TestLaunchPreparedSession_LateLaunchErrorAfterNewParkingStampIsSuperseded(t *testing.T) {
	repo := newMockRepository()
	env := seedOwnedCreatingClaim(repo, models.TaskSessionStateWaitingForInput)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	repo.getTaskSessionFunc = func(readCtx context.Context, id string) (*models.TaskSession, error) {
		if err := readCtx.Err(); err != nil {
			return nil, err
		}
		repo.mu.Lock()
		defer repo.mu.Unlock()
		current := repo.sessions[id]
		if current == nil {
			return nil, nil
		}
		return cloneMockTaskSession(current), nil
	}
	launchSnapshot, err := repo.GetTaskSession(context.Background(), releaseSessionID)
	if err != nil {
		t.Fatal(err)
	}
	launchSnapshot.Metadata = map[string]interface{}{
		models.SessionMetaKeyWorkflowParking: models.WorkflowParking{
			Stamp: "park-old", ParkedAt: time.Now().UTC(), SourceSessionID: releaseSessionID,
		},
	}
	if err := repo.UpdateTaskSession(context.Background(), launchSnapshot); err != nil {
		t.Fatal(err)
	}

	launchErr := errors.New("agent launch failed after the profile switch")
	agentManager := &mockAgentManager{
		launchAgentFunc: func(ctx context.Context, _ *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			current, getErr := repo.GetTaskSession(ctx, releaseSessionID)
			if getErr != nil {
				return nil, getErr
			}
			current.Metadata[models.SessionMetaKeyWorkflowParking] = models.WorkflowParking{
				Stamp: "park-new", ParkedAt: time.Now().UTC(), SourceSessionID: releaseSessionID,
			}
			if updateErr := repo.UpdateTaskSession(ctx, current); updateErr != nil {
				return nil, updateErr
			}
			cancel()
			return nil, launchErr
		},
	}
	exec := newTestExecutor(t, agentManager, repo)

	_, err = exec.LaunchPreparedSession(ctx, releaseTask(), releaseSessionID,
		LaunchOptions{AgentProfileID: "profile-123", StartAgent: true})

	if !IsSessionStateSwitched(err) {
		t.Fatalf("LaunchPreparedSession error = %v, want a superseded launch", err)
	}
	if env.Status != models.TaskEnvironmentStatusCreating || env.MaterializationSessionID != releaseSessionID {
		t.Fatalf("environment = status %q owner %q, want the parked source claim kept", env.Status, env.MaterializationSessionID)
	}
	current, err := repo.GetTaskSession(context.Background(), releaseSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if current.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state = %q, want WAITING_FOR_INPUT", current.State)
	}
	marker, ok := models.LoadWorkflowParking(current.Metadata)
	if !ok || marker.Stamp != "park-new" {
		t.Fatalf("workflow parking marker = %#v, want park-new", marker)
	}
}
