package orchestrator

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	repository "github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.4
// @covers AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.7
// @covers AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.8
func TestSessionOpenRecoveryEligibility(t *testing.T) {
	route := models.WorkflowSessionRoute{
		OperationID:       "route-reused",
		DestinationStepID: "step-current",
		EntryIdentity:     "entry:00000000000000000001",
		TargetKind:        "new_session",
		AgentProfileID:    "profile-reused",
		DestinationID:     "session-reused",
		Phase:             workflowSessionRouteCommitted,
	}

	tests := []struct {
		name         string
		taskMetadata map[string]interface{}
		sessionMeta  map[string]interface{}
		wantAllowed  bool
	}{
		{
			name: "reused destination ignores consumed historical stop",
			taskMetadata: map[string]interface{}{
				models.MetaKeyWorkflowSessionRoute: route,
			},
			sessionMeta: map[string]interface{}{
				models.SessionMetaKeyWorkflowProfileSwitchStopIntent: models.WorkflowProfileSwitchStopIntent{
					ExecutionID: "execution-old",
					Stamp:       "stop-old",
					Consumed:    true,
				},
			},
			wantAllowed: true,
		},
		{
			name: "settled empty ceiling record has no pending work",
			taskMetadata: map[string]interface{}{
				models.MetaKeyDeferredLaunch: map[string]interface{}{},
			},
			wantAllowed: true,
		},
		{
			name: "reused destination with settled empty ceiling record",
			taskMetadata: map[string]interface{}{
				models.MetaKeyWorkflowSessionRoute: route,
				models.MetaKeyDeferredLaunch:       map[string]interface{}{},
			},
			sessionMeta: map[string]interface{}{
				models.SessionMetaKeyWorkflowProfileSwitchStopIntent: models.WorkflowProfileSwitchStopIntent{
					ExecutionID: "execution-old",
					Stamp:       "stop-old",
					Consumed:    true,
				},
			},
			wantAllowed: true,
		},
		{
			name:        "absent deferred record remains eligible",
			wantAllowed: true,
		},
		{
			name: "null deferred record remains eligible",
			taskMetadata: map[string]interface{}{
				models.MetaKeyDeferredLaunch: nil,
			},
			wantAllowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &models.Task{
				ID:             "task-reused",
				WorkflowStepID: "step-current",
				Metadata:       tt.taskMetadata,
			}
			session := &models.TaskSession{
				ID:             "session-reused",
				TaskID:         "task-reused",
				AgentProfileID: "profile-reused",
				IsPrimary:      true,
				Metadata:       tt.sessionMeta,
			}

			allowed, reason := (&Service{}).autoResumeEligibility(context.Background(), task, session)
			if allowed != tt.wantAllowed {
				t.Fatalf("allowed = %t, want %t (reason %q)", allowed, tt.wantAllowed, reason)
			}
		})
	}
}

// @covers AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.2
// @covers AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.5
// @covers AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.6
// @covers AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.9
func TestSessionOpenRecoveryOwnership(t *testing.T) {
	queuedRecord := func(sessionID string) map[string]interface{} {
		return models.CeilingRecordKeys(models.CeilingDeferral{
			Kind: models.CeilingLaunchStartCreated,
			Payload: map[string]interface{}{
				metaKeySessionID:      sessionID,
				metaKeyAgentProfileID: "profile-reused",
			},
			Origin:   string(launchOriginAutomatic),
			QueuedAt: time.Date(2026, 9, 16, 20, 0, 0, 0, time.UTC),
		})
	}
	consumedStop := models.WorkflowProfileSwitchStopIntent{
		ExecutionID: "execution-old",
		Stamp:       "stop-old",
		Consumed:    true,
	}
	build := func() (*models.Task, *models.TaskSession) {
		return &models.Task{
				ID:             "task-reused",
				WorkflowStepID: "step-current",
			}, &models.TaskSession{
				ID:             "session-reused",
				TaskID:         "task-reused",
				AgentProfileID: "profile-reused",
			}
	}
	tests := []struct {
		name          string
		mutate        func(*models.Task, *models.TaskSession)
		wantAllowed   bool
		wantBlockCode string
	}{
		{
			name: "current parking marker does not block recovery",
			mutate: func(_ *models.Task, session *models.TaskSession) {
				session.Metadata = map[string]interface{}{
					models.SessionMetaKeyWorkflowParking: models.WorkflowParking{
						Stamp: "parking-current", SourceSessionID: session.ID,
					},
				}
			},
			wantAllowed: true,
		},
		{
			name: "historical stop intent does not block recovery",
			mutate: func(_ *models.Task, session *models.TaskSession) {
				session.Metadata = map[string]interface{}{
					models.SessionMetaKeyWorkflowProfileSwitchStopIntent: consumedStop,
				}
			},
			wantAllowed: true,
		},
		{
			name: "malformed parking metadata does not block recovery",
			mutate: func(_ *models.Task, session *models.TaskSession) {
				session.Metadata = map[string]interface{}{
					models.SessionMetaKeyWorkflowParking: map[string]interface{}{"stamp": "legacy"},
				}
			},
			wantAllowed: true,
		},
		{
			name: "current queued destination remains owned",
			mutate: func(task *models.Task, _ *models.TaskSession) {
				task.Metadata = map[string]interface{}{
					models.MetaKeyWorkflowSessionRoute: routeForTestSessionOpenRecovery(),
					models.MetaKeyDeferredLaunch:       queuedRecord("session-reused"),
				}
			},
			wantBlockCode: autoResumeBlockedLaunchQueued,
		},
		{
			name: "malformed deferred ownership remains blocked",
			mutate: func(task *models.Task, _ *models.TaskSession) {
				task.Metadata = map[string]interface{}{
					models.MetaKeyDeferredLaunch: map[string]interface{}{"wip_task_id": "other-task"},
				}
			},
			wantBlockCode: autoResumeBlockedOwnershipUnavailable,
		},
		{
			name: "queued sibling does not block selected recovery",
			mutate: func(task *models.Task, _ *models.TaskSession) {
				task.Metadata = map[string]interface{}{
					models.MetaKeyDeferredLaunch: queuedRecord("session-other"),
				}
			},
			wantAllowed: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			task, session := build()
			test.mutate(task, session)
			allowed, reason := (&Service{}).autoResumeEligibility(context.Background(), task, session)
			if allowed != test.wantAllowed || reason != test.wantBlockCode {
				t.Fatalf("eligibility = %t, %q; want %t, %q", allowed, reason, test.wantAllowed, test.wantBlockCode)
			}
		})
	}
}

func routeForTestSessionOpenRecovery() models.WorkflowSessionRoute {
	return models.WorkflowSessionRoute{
		OperationID:       "route-reused",
		DestinationStepID: "step-current",
		EntryIdentity:     "entry:00000000000000000001",
		TargetKind:        "new_session",
		AgentProfileID:    "profile-reused",
		DestinationID:     "session-reused",
		Phase:             workflowSessionRouteCommitted,
	}
}

// @covers AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.4
// @covers AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.7
// @covers AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.8
// @covers AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.4
func TestSessionOpenRecoveryAfterRestart(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "session-open-recovery.db")
	repo, closeRepo := openSessionOpenRecoveryRepo(t, dbPath)
	seedSessionOpenRecoveryState(t, repo, models.TaskSessionStateRunning)
	closeRepo()

	restartedRepo, _ := openSessionOpenRecoveryRepo(t, dbPath)
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task-reused"] = &v1.Task{ID: "task-reused", State: v1.TaskStateInProgress}
	agentMgr := &mockAgentManager{repoForExecutionLookup: restartedRepo}
	svc := createTestServiceWithAgent(restartedRepo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, restartedRepo, testLogger(), executor.ExecutorConfig{})
	svc.reconcileExecutorSessionsOnStartup(ctx)

	session, err := restartedRepo.GetTaskSession(ctx, "session-reused")
	if err != nil {
		t.Fatalf("load session after restart: %v", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state after restart = %q, want WAITING_FOR_INPUT", session.State)
	}
	if parking, ok := models.LoadWorkflowParking(session.Metadata); !ok || parking.Stamp == "" {
		t.Fatalf("workflow parking after restart = %#v, want retained marker", session.Metadata[models.SessionMetaKeyWorkflowParking])
	}
	stop, ok := workflowProfileSwitchStopIntentFromMetadata(session.Metadata)
	if !ok || !stop.Consumed {
		t.Fatalf("stop intent after restart = %#v, want retained consumed tombstone", session.Metadata[models.SessionMetaKeyWorkflowProfileSwitchStopIntent])
	}

	task, err := restartedRepo.GetTask(ctx, "task-reused")
	if err != nil {
		t.Fatalf("load task after restart: %v", err)
	}
	deferred, ok := task.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	if !ok || len(deferred) != 0 {
		t.Fatalf("deferred launch after restart = %#v, want settled empty record", task.Metadata[models.MetaKeyDeferredLaunch])
	}
	running, err := restartedRepo.GetExecutorRunningBySessionID(ctx, "session-reused")
	if err != nil {
		t.Fatalf("load executor row after restart: %v", err)
	}
	if running.ResumeToken != "resume-reused" || !running.Resumable {
		t.Fatalf("executor row after restart = %+v, want retained resumable token", running)
	}

	status, err := svc.GetTaskSessionStatus(ctx, "task-reused", "session-reused")
	if err != nil {
		t.Fatalf("GetTaskSessionStatus after restart: %v", err)
	}
	if !status.AutoResumeAllowed || !status.NeedsResume {
		t.Fatalf("status after restart = %+v, want passive recovery", status)
	}
}

// @covers AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.2
// @covers AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.5
func openSessionOpenRecoveryRepo(t *testing.T, dbPath string) (*sqliterepo.Repository, func()) {
	t.Helper()
	dbConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open session recovery database: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
	if err != nil {
		_ = sqlxDB.Close()
		t.Fatalf("create session recovery repository: %v", err)
	}
	closed := false
	closeRepo := func() {
		if closed {
			return
		}
		closed = true
		_ = cleanup()
		_ = sqlxDB.Close()
	}
	t.Cleanup(closeRepo)
	return repo, closeRepo
}

func seedSessionOpenRecoveryState(t *testing.T, repo *sqliterepo.Repository, state models.TaskSessionState) {
	t.Helper()
	ctx := context.Background()
	seedTaskAndSession(t, repo, "task-reused", "session-reused", state)

	task, err := repo.GetTask(ctx, "task-reused")
	if err != nil {
		t.Fatalf("load task while seeding recovery state: %v", err)
	}
	task.WorkflowStepID = "step-current"
	task.Metadata = map[string]interface{}{
		models.MetaKeyWorkflowSessionRoute: routeForTestSessionOpenRecovery(),
		models.MetaKeyDeferredLaunch:       map[string]interface{}{},
	}
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("save task recovery state: %v", err)
	}

	session, err := repo.GetTaskSession(ctx, "session-reused")
	if err != nil {
		t.Fatalf("load session while seeding recovery state: %v", err)
	}
	session.AgentProfileID = "profile-reused"
	session.IsPrimary = false
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("save session recovery state: %v", err)
	}
	parking := models.WorkflowParking{
		Stamp:           "parking-reused",
		ParkedAt:        time.Date(2026, 9, 16, 20, 0, 0, 0, time.UTC),
		SourceSessionID: session.ID,
	}
	if err := repo.SetSessionMetadataKey(ctx, session.ID, models.SessionMetaKeyWorkflowParking, parking); err != nil {
		t.Fatalf("save parking recovery state: %v", err)
	}
	if err := repo.SetSessionMetadataKey(ctx, session.ID, models.SessionMetaKeyWorkflowProfileSwitchStopIntent, models.WorkflowProfileSwitchStopIntent{
		ExecutionID: "execution-reused",
		Stamp:       parking.Stamp,
		Consumed:    true,
	}); err != nil {
		t.Fatalf("save stop intent recovery state: %v", err)
	}

	now := time.Date(2026, 9, 16, 20, 0, 0, 0, time.UTC)
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               session.ID,
		SessionID:        session.ID,
		TaskID:           task.ID,
		Runtime:          agentruntime.RuntimeStandalone,
		Status:           models.ExecutorRunningStatusRunning,
		Resumable:        true,
		ResumeToken:      "resume-reused",
		AgentExecutionID: "execution-reused",
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("save executor recovery state: %v", err)
	}
}
