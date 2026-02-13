package scheduler

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/task/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// mockAgentManager implements executor.AgentManagerClient for testing
type mockAgentManager struct {
	launchErr   error
	launchedIDs []string
	mu          sync.Mutex
}

func newMockAgentManager() *mockAgentManager {
	return &mockAgentManager{
		launchedIDs: make([]string, 0),
	}
}

func (m *mockAgentManager) LaunchAgent(ctx context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
	if m.launchErr != nil {
		return nil, m.launchErr
	}
	m.mu.Lock()
	m.launchedIDs = append(m.launchedIDs, req.TaskID)
	m.mu.Unlock()

	return &executor.LaunchAgentResponse{
		AgentExecutionID: "agent-" + req.TaskID,
		ContainerID:      "container-" + req.TaskID,
		Status:           v1.AgentStatusRunning,
	}, nil
}

func (m *mockAgentManager) SetExecutionDescription(ctx context.Context, agentExecutionID string, description string) error {
	return nil
}

func (m *mockAgentManager) StartAgentProcess(ctx context.Context, agentExecutionID string) error {
	return nil
}

func (m *mockAgentManager) StopAgent(ctx context.Context, agentExecutionID string, force bool) error {
	return nil
}

func (m *mockAgentManager) PromptAgent(ctx context.Context, agentExecutionID string, prompt string, _ []v1.MessageAttachment) (*executor.PromptResult, error) {
	return &executor.PromptResult{StopReason: "end_turn"}, nil
}

func (m *mockAgentManager) RespondToPermissionBySessionID(ctx context.Context, sessionID, pendingID, optionID string, cancelled bool) error {
	return nil
}

func (m *mockAgentManager) IsAgentRunningForSession(ctx context.Context, sessionID string) bool {
	return false
}

func (m *mockAgentManager) CancelAgent(ctx context.Context, sessionID string) error {
	return nil
}

func (m *mockAgentManager) ResolveAgentProfile(ctx context.Context, profileID string) (*executor.AgentProfileInfo, error) {
	return &executor.AgentProfileInfo{
		ProfileID:   profileID,
		ProfileName: "Mock Profile",
		AgentID:     "mock-agent",
		AgentName:   "Mock Agent",
		Model:       "mock-model",
	}, nil
}

// testTaskRepository is an in-memory task repository for testing
type testTaskRepository struct {
	tasks map[string]*v1.Task
	mu    sync.RWMutex
}

func newTestTaskRepository() *testTaskRepository {
	return &testTaskRepository{
		tasks: make(map[string]*v1.Task),
	}
}

func (r *testTaskRepository) AddTask(task *v1.Task) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks[task.ID] = task
}

func (r *testTaskRepository) GetTask(ctx context.Context, taskID string) (*v1.Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	task, exists := r.tasks[taskID]
	if !exists {
		return nil, ErrTaskNotFound
	}
	copy := *task
	return &copy, nil
}

func (r *testTaskRepository) UpdateTaskState(ctx context.Context, taskID string, state v1.TaskState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, exists := r.tasks[taskID]
	if !exists {
		return ErrTaskNotFound
	}
	task.State = state
	return nil
}

func createTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error", // Suppress logs during tests
		Format: "console",
	})
	return log
}

func createTestExecutor(t *testing.T, agentMgr *mockAgentManager, log *logger.Logger) *executor.Executor {
	tmpDir := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	repoImpl, cleanup, err := repository.Provide(dbConn)
	if err != nil {
		t.Fatalf("failed to create test repository: %v", err)
	}
	repo := repository.Repository(repoImpl)
	t.Cleanup(func() {
		if err := dbConn.Close(); err != nil {
			t.Errorf("failed to close sqlite db: %v", err)
		}
		if err := cleanup(); err != nil {
			t.Errorf("failed to close repo: %v", err)
		}
	})
	cfg := executor.ExecutorConfig{}
	return executor.NewExecutor(agentMgr, repo, log, cfg)
}

func createTestTask(id string, priority int) *v1.Task {
	return &v1.Task{
		ID:        id,
		BoardID:   "test-board",
		Title:     "Test Task " + id,
		Priority:  priority,
		State:     v1.TaskStateTODO,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func TestNewScheduler(t *testing.T) {
	q := queue.NewTaskQueue(100)
	agentMgr := newMockAgentManager()
	log := createTestLogger()
	exec := createTestExecutor(t, agentMgr, log)
	taskRepo := newTestTaskRepository()

	s := NewScheduler(q, exec, taskRepo, log, DefaultSchedulerConfig())
	if s == nil {
		t.Fatal("NewScheduler returned nil")
	}
	if s.IsRunning() {
		t.Error("scheduler should not be running initially")
	}
}

func TestDefaultSchedulerConfig(t *testing.T) {
	cfg := DefaultSchedulerConfig()

	if cfg.ProcessInterval != 5*time.Second {
		t.Errorf("expected ProcessInterval = 5s, got %v", cfg.ProcessInterval)
	}
	if cfg.RetryLimit != 3 {
		t.Errorf("expected RetryLimit = 3, got %d", cfg.RetryLimit)
	}
	if cfg.RetryDelay != 30*time.Second {
		t.Errorf("expected RetryDelay = 30s, got %v", cfg.RetryDelay)
	}
}

func TestStartStop(t *testing.T) {
	q := queue.NewTaskQueue(100)
	agentMgr := newMockAgentManager()
	log := createTestLogger()
	exec := createTestExecutor(t, agentMgr, log)
	taskRepo := newTestTaskRepository()

	s := NewScheduler(q, exec, taskRepo, log, DefaultSchedulerConfig())

	err := s.Start(context.Background())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if !s.IsRunning() {
		t.Error("scheduler should be running after Start")
	}

	err = s.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if s.IsRunning() {
		t.Error("scheduler should not be running after Stop")
	}
}

func TestStartAlreadyRunning(t *testing.T) {
	q := queue.NewTaskQueue(100)
	agentMgr := newMockAgentManager()
	log := createTestLogger()
	exec := createTestExecutor(t, agentMgr, log)
	taskRepo := newTestTaskRepository()

	s := NewScheduler(q, exec, taskRepo, log, DefaultSchedulerConfig())

	_ = s.Start(context.Background())
	defer func() {
		_ = s.Stop()
	}()

	err := s.Start(context.Background())
	if err != ErrSchedulerAlreadyRunning {
		t.Errorf("expected ErrSchedulerAlreadyRunning, got %v", err)
	}
}

func TestStopNotRunning(t *testing.T) {
	q := queue.NewTaskQueue(100)
	agentMgr := newMockAgentManager()
	log := createTestLogger()
	exec := createTestExecutor(t, agentMgr, log)
	taskRepo := newTestTaskRepository()

	s := NewScheduler(q, exec, taskRepo, log, DefaultSchedulerConfig())

	err := s.Stop()
	if err != ErrSchedulerNotRunning {
		t.Errorf("expected ErrSchedulerNotRunning, got %v", err)
	}
}

func TestEnqueueTask(t *testing.T) {
	q := queue.NewTaskQueue(100)
	agentMgr := newMockAgentManager()
	log := createTestLogger()
	exec := createTestExecutor(t, agentMgr, log)
	taskRepo := newTestTaskRepository()

	s := NewScheduler(q, exec, taskRepo, log, DefaultSchedulerConfig())

	task := createTestTask("task-1", 5)
	err := s.EnqueueTask(task)
	if err != nil {
		t.Fatalf("EnqueueTask failed: %v", err)
	}

	if q.Len() != 1 {
		t.Errorf("expected queue length = 1, got %d", q.Len())
	}
}

func TestRemoveTask(t *testing.T) {
	q := queue.NewTaskQueue(100)
	agentMgr := newMockAgentManager()
	log := createTestLogger()
	exec := createTestExecutor(t, agentMgr, log)
	taskRepo := newTestTaskRepository()

	s := NewScheduler(q, exec, taskRepo, log, DefaultSchedulerConfig())

	task := createTestTask("task-1", 5)
	_ = s.EnqueueTask(task)

	removed := s.RemoveTask("task-1")
	if !removed {
		t.Error("RemoveTask should return true for existing task")
	}

	if q.Len() != 0 {
		t.Errorf("expected queue length = 0 after remove, got %d", q.Len())
	}
}

func TestRemoveNonExistentTask(t *testing.T) {
	q := queue.NewTaskQueue(100)
	agentMgr := newMockAgentManager()
	log := createTestLogger()
	exec := createTestExecutor(t, agentMgr, log)
	taskRepo := newTestTaskRepository()

	s := NewScheduler(q, exec, taskRepo, log, DefaultSchedulerConfig())

	removed := s.RemoveTask("non-existent")
	if removed {
		t.Error("RemoveTask should return false for non-existent task")
	}
}

func TestGetQueueStatus(t *testing.T) {
	q := queue.NewTaskQueue(100)
	agentMgr := newMockAgentManager()
	log := createTestLogger()
	exec := createTestExecutor(t, agentMgr, log)
	taskRepo := newTestTaskRepository()

	cfg := DefaultSchedulerConfig()
	s := NewScheduler(q, exec, taskRepo, log, cfg)

	_ = s.EnqueueTask(createTestTask("task-1", 5))
	_ = s.EnqueueTask(createTestTask("task-2", 3))

	status := s.GetQueueStatus()
	if status.QueuedTasks != 2 {
		t.Errorf("expected QueuedTasks = 2, got %d", status.QueuedTasks)
	}
	if status.ActiveExecutions != 0 {
		t.Errorf("expected ActiveExecutions = 0, got %d", status.ActiveExecutions)
	}
}

func TestHandleTaskCompleted(t *testing.T) {
	q := queue.NewTaskQueue(100)
	agentMgr := newMockAgentManager()
	log := createTestLogger()
	exec := createTestExecutor(t, agentMgr, log)
	taskRepo := newTestTaskRepository()

	s := NewScheduler(q, exec, taskRepo, log, DefaultSchedulerConfig())

	// Track initial stats
	initialStatus := s.GetQueueStatus()

	// Handle successful completion
	s.HandleTaskCompleted("task-1", true)

	status := s.GetQueueStatus()
	if status.TotalProcessed != initialStatus.TotalProcessed+1 {
		t.Error("TotalProcessed should increment on success")
	}

	// Handle failed completion
	s.HandleTaskCompleted("task-2", false)

	status = s.GetQueueStatus()
	if status.TotalFailed != initialStatus.TotalFailed+1 {
		t.Error("TotalFailed should increment on failure")
	}
}

func TestPriorityOrdering(t *testing.T) {
	q := queue.NewTaskQueue(100)
	agentMgr := newMockAgentManager()
	log := createTestLogger()
	exec := createTestExecutor(t, agentMgr, log)
	taskRepo := newTestTaskRepository()

	s := NewScheduler(q, exec, taskRepo, log, DefaultSchedulerConfig())

	// Enqueue tasks with different priorities
	_ = s.EnqueueTask(createTestTask("low", 1))
	_ = s.EnqueueTask(createTestTask("high", 10))
	_ = s.EnqueueTask(createTestTask("medium", 5))

	// Verify the underlying queue has correct ordering by dequeueing
	first := q.Dequeue()
	if first == nil || first.TaskID != "high" {
		t.Errorf("expected highest priority task (high) first, got %v", first)
	}
}

func TestIsRunning(t *testing.T) {
	q := queue.NewTaskQueue(100)
	agentMgr := newMockAgentManager()
	log := createTestLogger()
	exec := createTestExecutor(t, agentMgr, log)
	taskRepo := newTestTaskRepository()

	s := NewScheduler(q, exec, taskRepo, log, DefaultSchedulerConfig())

	if s.IsRunning() {
		t.Error("scheduler should not be running before Start")
	}

	_ = s.Start(context.Background())
	if !s.IsRunning() {
		t.Error("scheduler should be running after Start")
	}

	_ = s.Stop()
	if s.IsRunning() {
		t.Error("scheduler should not be running after Stop")
	}
}

func TestSchedulerWithContextCancellation(t *testing.T) {
	q := queue.NewTaskQueue(100)
	agentMgr := newMockAgentManager()
	log := createTestLogger()
	exec := createTestExecutor(t, agentMgr, log)
	taskRepo := newTestTaskRepository()

	cfg := DefaultSchedulerConfig()
	cfg.ProcessInterval = 10 * time.Millisecond
	s := NewScheduler(q, exec, taskRepo, log, cfg)

	ctx, cancel := context.WithCancel(context.Background())

	err := s.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Cancel context
	cancel()

	// Give it time to stop
	time.Sleep(50 * time.Millisecond)

	// The scheduler loop should have exited due to context cancellation
	// We still need to call Stop to clean up the running flag
	_ = s.Stop()
}

func TestEnqueueDuplicateTask(t *testing.T) {
	q := queue.NewTaskQueue(100)
	agentMgr := newMockAgentManager()
	log := createTestLogger()
	exec := createTestExecutor(t, agentMgr, log)
	taskRepo := newTestTaskRepository()

	s := NewScheduler(q, exec, taskRepo, log, DefaultSchedulerConfig())

	task := createTestTask("task-1", 5)
	_ = s.EnqueueTask(task)

	err := s.EnqueueTask(task)
	if err != queue.ErrTaskExists {
		t.Errorf("expected ErrTaskExists, got %v", err)
	}
}

func TestRetryTaskExceedsLimit(t *testing.T) {
	q := queue.NewTaskQueue(100)
	agentMgr := newMockAgentManager()
	log := createTestLogger()
	exec := createTestExecutor(t, agentMgr, log)
	taskRepo := newTestTaskRepository()

	cfg := DefaultSchedulerConfig()
	cfg.RetryLimit = 2
	cfg.RetryDelay = 1 * time.Millisecond
	s := NewScheduler(q, exec, taskRepo, log, cfg)

	task := createTestTask("task-1", 5)
	taskRepo.AddTask(task)

	// First retry should succeed
	result := s.RetryTask("task-1")
	if !result {
		t.Error("first retry should succeed")
	}

	// Second retry should succeed
	result = s.RetryTask("task-1")
	if !result {
		t.Error("second retry should succeed")
	}

	// Third retry should fail (exceeds limit of 2)
	result = s.RetryTask("task-1")
	if result {
		t.Error("third retry should fail (limit exceeded)")
	}
}

func TestRetryTaskNotFound(t *testing.T) {
	q := queue.NewTaskQueue(100)
	agentMgr := newMockAgentManager()
	log := createTestLogger()
	exec := createTestExecutor(t, agentMgr, log)
	taskRepo := newTestTaskRepository()

	cfg := DefaultSchedulerConfig()
	cfg.RetryDelay = 1 * time.Millisecond
	s := NewScheduler(q, exec, taskRepo, log, cfg)

	// Try to retry a task that doesn't exist in the repository
	result := s.RetryTask("non-existent")
	if result {
		t.Error("retry should fail for non-existent task")
	}
}
