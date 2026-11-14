package lifecycle

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

const (
	registrationRaceTaskID    = "task-registration-race"
	registrationRaceSessionID = "session-registration-race"
	registrationRaceEnvID     = "environment-registration-race"
)

type registrationBarrierWriter struct {
	*tasksqlite.Repository

	upsertEntered chan struct{}
	releaseUpsert chan struct{}
	upsertErr     error
	enteredOnce   sync.Once
	releaseOnce   sync.Once
}

func (w *registrationBarrierWriter) UpsertExecutorRunning(ctx context.Context, running *models.ExecutorRunning) error {
	w.enteredOnce.Do(func() { close(w.upsertEntered) })
	<-w.releaseUpsert
	if w.upsertErr != nil {
		return w.upsertErr
	}
	return w.Repository.UpsertExecutorRunning(ctx, running)
}

func (w *registrationBarrierWriter) release() {
	w.releaseOnce.Do(func() { close(w.releaseUpsert) })
}

type registrationRaceFixture struct {
	manager         *Manager
	writer          *registrationBarrierWriter
	registration    *executionCreationInputs
	info            *WorkspaceInfo
	runtime         *ExecutorInstance
	execution       *AgentExecution
	backend         *MockExecutor
	registrationCtx context.Context
}

func newRegistrationRaceFixture(t *testing.T, upsertErr error, reattached bool) *registrationRaceFixture {
	t.Helper()

	database, err := db.OpenSQLite(filepath.Join(t.TempDir(), "registration-race.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close SQLite database: %v", err)
		}
	})
	connection := sqlx.NewDb(database, "sqlite3")
	repository, err := tasksqlite.NewWithDB(connection, connection, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		ID: "running-registration-race", TaskID: registrationRaceTaskID,
		SessionID: registrationRaceSessionID, AgentExecutionID: "exec-old",
		Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusRunning,
	}); err != nil {
		t.Fatal(err)
	}

	writer := &registrationBarrierWriter{
		Repository: repository, upsertEntered: make(chan struct{}),
		releaseUpsert: make(chan struct{}), upsertErr: upsertErr,
	}
	backend := &MockExecutor{name: executor.NameStandalone}
	manager, _ := newRuntimeSelectionManager(t, backend)
	manager.SetExecutorRunningWriter(writer)
	t.Cleanup(writer.release)

	metadata := map[string]interface{}{}
	if reattached {
		metadata[MetadataKeyReuseExistingProcess] = true
	}
	request := &ExecutorCreateRequest{
		InstanceID: "exec-new", TaskID: registrationRaceTaskID,
		SessionID: registrationRaceSessionID, TaskEnvironmentID: registrationRaceEnvID,
		ExecutorType: "local_pc", WorkspacePath: t.TempDir(),
	}
	preparation := &executionCreatePreparation{request: request}
	info := &WorkspaceInfo{
		TaskID: registrationRaceTaskID, SessionID: registrationRaceSessionID,
		TaskEnvironmentID: registrationRaceEnvID, WorkspacePath: request.WorkspacePath,
	}
	runtimeInstance := &ExecutorInstance{
		InstanceID: "exec-new", TaskID: registrationRaceTaskID,
		SessionID: registrationRaceSessionID, RuntimeName: agentruntime.RuntimeStandalone,
		WorkspacePath: request.WorkspacePath, Metadata: metadata,
	}
	execution := manager.initializeCreatedExecution(
		context.Background(), registrationRaceTaskID, info, backend, runtimeInstance, preparation,
	)
	return &registrationRaceFixture{
		manager: manager, writer: writer,
		registration: &executionCreationInputs{
			runtime: backend, executionID: "exec-new", preparation: preparation,
		},
		info: info, runtime: runtimeInstance, execution: execution, backend: backend,
		registrationCtx: context.Background(),
	}
}

func (f *registrationRaceFixture) startRegistration(t *testing.T) <-chan error {
	t.Helper()
	result := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_, err := f.manager.registerAndPublishCreatedExecution(
			f.registrationCtx, registrationRaceTaskID, f.info, false,
			f.registration, f.runtime, f.execution,
		)
		result <- err
	}()
	t.Cleanup(func() {
		f.writer.release()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("execution registration did not finish")
		}
	})
	return result
}

// TestValidateExecutionRuntimeWaitsForRegistrationSettle covers
// AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.10. A page-open workspace
// registration must finish writing its row before resume compares identities.
func TestValidateExecutionRuntimeWaitsForRegistrationSettle(t *testing.T) {
	fixture := newRegistrationRaceFixture(t, nil, false)
	registrationDone := fixture.startRegistration(t)
	<-fixture.writer.upsertEntered

	validationStarted := make(chan struct{})
	validationDone := make(chan error, 1)
	go func() {
		close(validationStarted)
		validationDone <- fixture.manager.ValidateExecutionRuntime(
			context.Background(), registrationRaceSessionID, registrationRaceEnvID, "local_pc",
		)
	}()
	<-validationStarted
	select {
	case err := <-validationDone:
		fixture.writer.release()
		<-registrationDone
		t.Fatalf("runtime validation completed before registration settled: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	fixture.writer.release()
	if err := <-registrationDone; err != nil {
		t.Fatalf("register execution: %v", err)
	}
	if err := <-validationDone; err != nil {
		t.Fatalf("validate settled execution: %v", err)
	}
	running, err := fixture.writer.GetExecutorRunningBySessionID(context.Background(), registrationRaceSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if running.AgentExecutionID != "exec-new" {
		t.Fatalf("persisted execution ID = %q, want exec-new", running.AgentExecutionID)
	}
}

// TestValidateExecutionRuntimeAfterRegistrationRollback covers
// AC-EXECUTORS-RUNTIME-SELECTION-002.2. A rolled-back registration is removed
// from memory before validation falls back to the predecessor's durable row.
func TestValidateExecutionRuntimeAfterRegistrationRollback(t *testing.T) {
	fixture := newRegistrationRaceFixture(t, errors.New("inventory write failed"), true)
	registrationDone := fixture.startRegistration(t)
	<-fixture.writer.upsertEntered

	validationDone := make(chan error, 1)
	go func() {
		validationDone <- fixture.manager.ValidateExecutionRuntime(
			context.Background(), registrationRaceSessionID, registrationRaceEnvID, "local_pc",
		)
	}()
	fixture.writer.release()
	if err := <-registrationDone; err == nil {
		t.Fatal("registration error = nil, want persistence failure")
	}
	if err := <-validationDone; err != nil {
		t.Fatalf("validate predecessor after registration rollback: %v", err)
	}
	if _, exists := fixture.manager.GetExecutionBySessionID(registrationRaceSessionID); exists {
		t.Fatal("rolled-back execution remains in memory")
	}
	running, err := fixture.writer.GetExecutorRunningBySessionID(context.Background(), registrationRaceSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if running.AgentExecutionID != "exec-old" {
		t.Fatalf("durable execution ID = %q, want predecessor exec-old", running.AgentExecutionID)
	}
}

// TestValidateExecutionRuntimeRegistrationWaitHonorsContext covers the
// caller's cancellation boundary while a registration writer remains blocked.
func TestValidateExecutionRuntimeRegistrationWaitHonorsContext(t *testing.T) {
	fixture := newRegistrationRaceFixture(t, nil, false)
	registrationDone := fixture.startRegistration(t)
	<-fixture.writer.upsertEntered

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	validationStarted := make(chan struct{})
	validationDone := make(chan error, 1)
	go func() {
		close(validationStarted)
		validationDone <- fixture.manager.ValidateExecutionRuntime(
			ctx, registrationRaceSessionID, registrationRaceEnvID, "local_pc",
		)
	}()
	<-validationStarted
	cancel()
	if err := <-validationDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled validation error = %v, want context.Canceled", err)
	}

	fixture.writer.release()
	if err := <-registrationDone; err != nil {
		t.Fatalf("register execution: %v", err)
	}
}

// TestValidateExecutionRuntimeSettledMismatchStillRefused covers
// AC-EXECUTORS-RUNTIME-SELECTION-002.2. Settled identity conflicts remain
// fail-closed.
func TestValidateExecutionRuntimeSettledMismatchStillRefused(t *testing.T) {
	fixture := newRegistrationRaceFixture(t, nil, false)
	fixture.manager.SetExecutorRunningWriter(fixture.writer.Repository)
	fixture.execution.ID = "exec-new"
	fixture.execution.settleRegistration()
	if err := fixture.manager.executionStore.Add(fixture.execution); err != nil {
		t.Fatal(err)
	}
	err := fixture.manager.ValidateExecutionRuntime(
		context.Background(), registrationRaceSessionID, registrationRaceEnvID, "local_pc",
	)
	if !errors.Is(err, ErrExecutorIdentityUnverified) {
		t.Fatalf("settled mismatch error = %v, want identity unverified", err)
	}
}
