package backendapp

import (
	"context"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	sqlitetaskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type journeyReadFixture struct {
	databasePath  string
	workspaceID   string
	workflowID    string
	taskIDs       []string
	params        routeParams
	taskRepo      *sqlitetaskrepo.Repository
	workflowSvc   *workflowservice.Service
	writer        *sqlx.DB
	reader        *sqlx.DB
	largeContentA string
	largeContentB string
}

func newJourneyReadFixture(tb testing.TB, taskCount int) *journeyReadFixture {
	tb.Helper()
	databasePath := filepath.Join(tb.TempDir(), "journey-read.db")
	writerRaw, err := db.OpenSQLite(databasePath)
	if err != nil {
		tb.Fatalf("open SQLite writer: %v", err)
	}
	readerRaw, err := db.OpenSQLiteReader(databasePath)
	if err != nil {
		_ = writerRaw.Close()
		tb.Fatalf("open SQLite reader: %v", err)
	}
	writer := sqlx.NewDb(writerRaw, "sqlite3")
	reader := sqlx.NewDb(readerRaw, "sqlite3")
	repo, repoCleanup, err := taskrepo.Provide(writer, reader, nil)
	if err != nil {
		_ = reader.Close()
		_ = writer.Close()
		tb.Fatalf("create task repository: %v", err)
	}
	wfRepo, err := workflowrepo.NewWithDB(writer, reader, nil)
	if err != nil {
		_ = repoCleanup()
		_ = reader.Close()
		_ = writer.Close()
		tb.Fatalf("create workflow repository: %v", err)
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		tb.Fatalf("create test logger: %v", err)
	}
	eventBus := bus.NewMemoryEventBus(log)
	wfSvc := workflowservice.NewService(wfRepo, log)
	taskSvc := taskservice.NewService(
		taskservice.Repos{
			Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo,
			Messages: repo, Turns: repo, Sessions: repo, GitSnapshots: repo,
			RepoEntities: repo, RepositorySets: repo, Executors: repo,
			Environments: repo, TaskEnvironments: repo, RecoveryOperations: repo,
			Reviews: repo, StatusSummaries: repo, WorkspaceFolders: repo,
		},
		eventBus,
		log,
		taskservice.RepositoryDiscoveryConfig{},
	)
	taskSvc.SetWorkflowStepCreator(wfSvc)
	taskSvc.SetWorkspaceBootstrapper(repo)
	taskSvc.SetWorkflowStepGetter(&workflowStepGetterAdapter{svc: wfSvc})
	taskSvc.SetStartStepResolver(&startStepResolverAdapter{svc: wfSvc})
	taskSvc.SetWorkspacePolicyAttacher(testWorkspacePolicyAttacher{})
	wfSvc.SetWorkflowProvider(&workflowProviderAdapter{svc: taskSvc})

	fixture := &journeyReadFixture{
		databasePath: databasePath,
		workspaceID:  "workspace-journey-read",
		taskRepo:     repo,
		workflowSvc:  wfSvc,
		writer:       writer,
		reader:       reader,
		params: routeParams{
			taskSvc:  taskSvc,
			taskRepo: repo,
			services: &Services{Workflow: wfSvc},
			log:      log,
		},
	}
	tb.Cleanup(func() {
		_ = wfSvc.Close()
		_ = repoCleanup()
		_ = reader.Close()
		_ = writer.Close()
	})
	ctx := context.Background()
	workflow, err := repo.CreateWorkspaceWithKanban(ctx, &models.Workspace{
		ID:   fixture.workspaceID,
		Name: "Journey read fixture",
	})
	if err != nil {
		tb.Fatalf("create workspace and Kanban: %v", err)
	}
	fixture.workflowID = workflow.ID
	steps, err := wfSvc.ListStepsByWorkflow(ctx, workflow.ID)
	if err != nil || len(steps) == 0 {
		tb.Fatalf("list Kanban steps: steps=%d err=%v", len(steps), err)
	}
	startStep := steps[0]
	for _, step := range steps {
		if step.IsStartStep {
			startStep = step
			break
		}
	}
	fixture.taskIDs = make([]string, 0, taskCount)
	for index := 0; index < taskCount; index++ {
		taskID := fmt.Sprintf("journey-task-%04d", index)
		err := repo.CreateTask(ctx, &models.Task{
			ID: taskID, WorkspaceID: fixture.workspaceID, WorkflowID: workflow.ID,
			WorkflowStepID: startStep.ID, Title: taskID, State: v1.TaskStateCreated,
			Priority: "medium",
		})
		if err != nil {
			tb.Fatalf("create journey task %q: %v", taskID, err)
		}
		fixture.taskIDs = append(fixture.taskIDs, taskID)
	}
	fixture.largeContentA = strings.Repeat("a", 16*1024*1024)
	fixture.largeContentB = strings.Repeat("b", 16*1024*1024)
	return fixture
}

func (f *journeyReadFixture) homeSnapshot(ctx context.Context) map[string]any {
	state := map[string]any{}
	request := httptest.NewRequest("GET", "/?workspaceId="+f.workspaceID+"&workflowId="+f.workflowID, nil)
	bootStateBuilder{p: f.params}.addHomeKanbanRouteState(ctx, request, state)
	return state
}

func (f *journeyReadFixture) assertHomeSnapshot(tb testing.TB, state map[string]any) {
	tb.Helper()
	kanban, ok := state["kanban"].(map[string]any)
	if !ok {
		tb.Fatalf("homepage has no active kanban snapshot: %#v", state)
	}
	tasks, ok := kanban["tasks"].([]map[string]any)
	if !ok {
		tb.Fatalf("kanban tasks have type %T", kanban["tasks"])
	}
	if len(tasks) != len(f.taskIDs) {
		tb.Fatalf("kanban tasks = %d, want %d", len(tasks), len(f.taskIDs))
	}
}

func (f *journeyReadFixture) startLargeMessageWriters(tb testing.TB) (func(), <-chan error) {
	tb.Helper()
	const writerCount = 8
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{}, writerCount)
	errs := make(chan error, writerCount)
	var workers sync.WaitGroup
	for index := 0; index < writerCount; index++ {
		index := index
		taskID := fmt.Sprintf("journey-writer-task-%02d", index)
		sessionID := fmt.Sprintf("journey-writer-session-%02d", index)
		turnID := fmt.Sprintf("journey-writer-turn-%02d", index)
		if err := f.taskRepo.CreateTask(context.Background(), &models.Task{
			ID: taskID, WorkspaceID: f.workspaceID, Title: taskID,
		}); err != nil {
			tb.Fatalf("create workload task: %v", err)
		}
		if err := f.taskRepo.CreateTaskSession(context.Background(), &models.TaskSession{ID: sessionID, TaskID: taskID}); err != nil {
			tb.Fatalf("create workload session: %v", err)
		}
		if err := f.taskRepo.CreateTurn(context.Background(), &models.Turn{
			ID: turnID, TaskSessionID: sessionID, TaskID: taskID,
		}); err != nil {
			tb.Fatalf("create workload turn: %v", err)
		}
		message := &models.Message{
			ID: "journey-writer-message-" + fmt.Sprint(index), TaskID: taskID,
			TaskSessionID: sessionID, TurnID: turnID,
			AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeMessage,
			Content: f.largeContentA, Metadata: map[string]any{},
		}
		if err := f.taskRepo.CreateMessage(context.Background(), message); err != nil {
			tb.Fatalf("create workload message: %v", err)
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			content, nextContent := f.largeContentB, f.largeContentA
			started <- struct{}{}
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				message.Content = content
				content, nextContent = nextContent, content
				if err := f.taskRepo.UpdateMessage(ctx, message); err != nil {
					if ctx.Err() == nil {
						errs <- err
					}
					return
				}
			}
		}()
	}
	for index := 0; index < writerCount; index++ {
		<-started
	}
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			cancel()
			workers.Wait()
			close(errs)
		})
	}
	tb.Cleanup(stop)
	return stop, errs
}

func TestJourneyReadDuringWriter(t *testing.T) {
	fixture := newJourneyReadFixture(t, 3)
	ctx := context.Background()
	fixture.assertHomeSnapshot(t, fixture.homeSnapshot(ctx))

	blockerRaw, err := db.OpenSQLite(fixture.databasePath)
	if err != nil {
		t.Fatalf("open external writer: %v", err)
	}
	blocker := sqlx.NewDb(blockerRaw, "sqlite3")
	t.Cleanup(func() { _ = blocker.Close() })
	blockerTx, err := blocker.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("hold external writer: %v", err)
	}
	defer func() { _ = blockerTx.Rollback() }()

	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	done := make(chan map[string]any, 1)
	errs := make(chan error, 1)
	go func() {
		state := fixture.homeSnapshot(readCtx)
		if _, ok := state["kanban"].(map[string]any); !ok {
			errs <- fmt.Errorf("homepage snapshot is incomplete: %#v", state)
			return
		}
		done <- state
	}()
	select {
	case state := <-done:
		fixture.assertHomeSnapshot(t, state)
	case err := <-errs:
		t.Fatalf("homepage read while writer was held: %v", err)
	case <-time.After(2 * time.Second):
		cancel()
		_ = blockerTx.Rollback()
		select {
		case <-done:
		case <-errs:
		case <-time.After(6 * time.Second):
			t.Fatal("homepage read did not stop after cancellation")
		}
		t.Fatal("warm homepage and board snapshot did not finish while writer was held")
	}
}
