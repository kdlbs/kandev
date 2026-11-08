package backendapp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	taskrepository "github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
)

// failingActiveTurnRepo forces the non-creating active-turn lookup to fail so
// the adapter cannot silently treat a persistence error as an idle session.
type failingActiveTurnRepo struct {
	taskrepository.TurnRepository
	err error
}

func (r failingActiveTurnRepo) GetActiveTurnBySessionID(context.Context, string) (*models.Turn, error) {
	return nil, r.err
}

// blockingActiveTurnRepo captures the first active-turn snapshot and holds it
// until the test releases the lookup. Only the first lookup blocks so the
// test's own service calls continue to see authoritative turn state.
type blockingActiveTurnRepo struct {
	taskrepository.TurnRepository
	captured    chan *models.Turn
	release     chan struct{}
	blockOnce   sync.Once
	releaseOnce sync.Once
}

func newBlockingActiveTurnRepo(inner taskrepository.TurnRepository) *blockingActiveTurnRepo {
	return &blockingActiveTurnRepo{
		TurnRepository: inner,
		captured:       make(chan *models.Turn, 1),
		release:        make(chan struct{}),
	}
}

func (r *blockingActiveTurnRepo) GetActiveTurnBySessionID(ctx context.Context, sessionID string) (*models.Turn, error) {
	turn, err := r.TurnRepository.GetActiveTurnBySessionID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	blocked := false
	r.blockOnce.Do(func() { blocked = true })
	if !blocked {
		return turn, nil
	}
	r.captured <- turn
	<-r.release
	return turn, nil
}

func (r *blockingActiveTurnRepo) Release() {
	r.releaseOnce.Do(func() { close(r.release) })
}

func newScriptTurnTaskService(t *testing.T, repo *sqliterepo.Repository, turns taskrepository.TurnRepository) *taskservice.Service {
	t.Helper()
	log := newTestLogger()
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	if turns == nil {
		turns = repo
	}
	return taskservice.NewService(taskservice.Repos{
		Workspaces:       repo,
		Tasks:            repo,
		TaskRepos:        repo,
		WorkspaceFolders: repo,
		Workflows:        repo,
		Messages:         repo,
		Turns:            turns,
		Sessions:         repo,
		RepoEntities:     repo,
		TaskEnvironments: repo,
		BackgroundWork:   repo,
		GitSnapshots:     repo,
		Executors:        repo,
		Environments:     repo,
		Usage:            repo,
	}, eventBus, log, taskservice.RepositoryDiscoveryConfig{})
}

func seedScriptTurnSession(t *testing.T, repo *sqliterepo.Repository) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-script", Name: "Script turns"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID:          "task-script",
		WorkspaceID: "ws-script",
		Title:       "Script turns",
		Priority:    "medium",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:     "session-script",
		TaskID: "task-script",
		State:  models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
}

func scriptMessageRequest(turnID string) *worktree.CreateMessageRequest {
	return &worktree.CreateMessageRequest{
		TaskSessionID: "session-script",
		TaskID:        "task-script",
		TurnID:        turnID,
		AuthorType:    "agent",
		Type:          "script_execution",
		Metadata:      map[string]interface{}{"script_type": "setup"},
	}
}

// TestWorktreeScriptMessageAdapter_IdleLifecycle covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.2
// and AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.5: an idle session must receive a
// script message bound to a completed lifecycle turn, not a phantom active turn.
func TestWorktreeScriptMessageAdapter_IdleLifecycle(t *testing.T) {
	repo := newMaterializerRepo(t)
	seedScriptTurnSession(t, repo)
	svc := newScriptTurnTaskService(t, repo, nil)
	ctx := context.Background()

	message, err := (&taskServiceAdapter{svc: svc}).CreateMessage(ctx, scriptMessageRequest(""))
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if message.TurnID == "" {
		t.Fatal("script message has no turn binding")
	}
	active, err := svc.GetActiveTurn(ctx, "session-script")
	if err != nil {
		t.Fatalf("GetActiveTurn: %v", err)
	}
	if active != nil {
		t.Fatalf("idle script message left active turn %q", active.ID)
	}
	turn, err := svc.GetTurn(ctx, message.TurnID)
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if turn.CompletedAt == nil {
		t.Fatalf("lifecycle turn %q is not completed", turn.ID)
	}
	if turn.Metadata[models.TurnMetaKeyLifecycleOnly] != true {
		t.Fatalf("lifecycle turn metadata = %+v, want lifecycle_only", turn.Metadata)
	}
	if _, err := svc.StartTurn(ctx, "session-script"); err != nil {
		t.Fatalf("next user turn rejected: %v", err)
	}
}

// TestWorktreeScriptMessageAdapter_PreservesTurnBinding covers explicit and
// observed turn IDs. A supplied ID must be preserved even after that turn
// completed, and an observed active turn must remain the owner.
func TestWorktreeScriptMessageAdapter_PreservesTurnBinding(t *testing.T) {
	t.Run("supplied_completed_turn", func(t *testing.T) {
		repo := newMaterializerRepo(t)
		seedScriptTurnSession(t, repo)
		svc := newScriptTurnTaskService(t, repo, nil)
		ctx := context.Background()
		completed, err := svc.StartTurn(ctx, "session-script")
		if err != nil {
			t.Fatalf("StartTurn: %v", err)
		}
		if err := svc.CompleteTurn(ctx, completed.ID); err != nil {
			t.Fatalf("CompleteTurn: %v", err)
		}

		message, err := (&taskServiceAdapter{svc: svc}).CreateMessage(ctx, scriptMessageRequest(completed.ID))
		if err != nil {
			t.Fatalf("CreateMessage: %v", err)
		}
		if message.TurnID != completed.ID {
			t.Fatalf("message turn = %q, want supplied %q", message.TurnID, completed.ID)
		}
		active, err := svc.GetActiveTurn(ctx, "session-script")
		if err != nil {
			t.Fatalf("GetActiveTurn: %v", err)
		}
		if active != nil {
			t.Fatalf("supplied turn created active turn %q", active.ID)
		}
	})

	t.Run("observed_active_turn", func(t *testing.T) {
		repo := newMaterializerRepo(t)
		seedScriptTurnSession(t, repo)
		svc := newScriptTurnTaskService(t, repo, nil)
		ctx := context.Background()
		active, err := svc.StartTurn(ctx, "session-script")
		if err != nil {
			t.Fatalf("StartTurn: %v", err)
		}

		message, err := (&taskServiceAdapter{svc: svc}).CreateMessage(ctx, scriptMessageRequest(""))
		if err != nil {
			t.Fatalf("CreateMessage: %v", err)
		}
		if message.TurnID != active.ID {
			t.Fatalf("message turn = %q, want observed %q", message.TurnID, active.ID)
		}
		current, err := svc.GetActiveTurn(ctx, "session-script")
		if err != nil {
			t.Fatalf("GetActiveTurn: %v", err)
		}
		if current == nil || current.ID != active.ID {
			t.Fatalf("active turn = %+v, want %q", current, active.ID)
		}
	})
}

// TestWorktreeScriptMessageAdapter_ObservedTurnCompletesBeforePersistence covers
// the concurrency boundary in the turn-binding design: a turn observed by the
// non-creating lookup remains the message owner after it completes and a
// successor begins before the adapter persists the script message. A second
// active-turn lookup during creation would redirect the message to the successor.
func TestWorktreeScriptMessageAdapter_ObservedTurnCompletesBeforePersistence(t *testing.T) {
	repo := newMaterializerRepo(t)
	seedScriptTurnSession(t, repo)
	barrier := newBlockingActiveTurnRepo(repo)
	t.Cleanup(barrier.Release)
	svc := newScriptTurnTaskService(t, repo, barrier)
	ctx := context.Background()

	first, err := svc.StartTurn(ctx, "session-script")
	if err != nil {
		t.Fatalf("StartTurn first: %v", err)
	}

	type outcome struct {
		message *models.Message
		err     error
	}
	done := make(chan outcome, 1)
	go func() {
		message, createErr := (&taskServiceAdapter{svc: svc}).CreateMessage(ctx, scriptMessageRequest(""))
		done <- outcome{message: message, err: createErr}
	}()
	var joinOnce sync.Once
	var joined outcome
	join := func() outcome {
		joinOnce.Do(func() {
			select {
			case joined = <-done:
			case <-time.After(scriptTurnHandlerBudget):
				t.Error("adapter CreateMessage did not finish after the lookup was released")
			}
		})
		return joined
	}
	t.Cleanup(func() {
		barrier.Release()
		_ = join()
	})

	var observed *models.Turn
	select {
	case observed = <-barrier.captured:
	case <-time.After(scriptTurnHandlerBudget):
		t.Fatal("adapter did not perform a non-creating active-turn lookup")
	}
	if observed == nil || observed.ID != first.ID {
		t.Fatalf("observed turn = %+v, want %q", observed, first.ID)
	}

	if err := svc.CompleteTurn(ctx, first.ID); err != nil {
		t.Fatalf("CompleteTurn: %v", err)
	}
	successor, err := svc.StartTurn(ctx, "session-script")
	if err != nil {
		t.Fatalf("StartTurn successor: %v", err)
	}
	barrier.Release()

	result := join()
	if result.err != nil {
		t.Fatalf("CreateMessage: %v", result.err)
	}
	if result.message == nil {
		t.Fatal("CreateMessage returned no message")
	}
	if result.message.TurnID != first.ID {
		t.Fatalf("message turn = %q, want captured %q", result.message.TurnID, first.ID)
	}

	active, err := svc.GetActiveTurn(ctx, "session-script")
	if err != nil {
		t.Fatalf("GetActiveTurn: %v", err)
	}
	if active == nil || active.ID != successor.ID {
		t.Fatalf("active turn = %+v, want successor %q", active, successor.ID)
	}

	turns, err := svc.ListTurnsBySession(ctx, "session-script")
	if err != nil {
		t.Fatalf("ListTurnsBySession: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("session turns = %d, want first and successor only", len(turns))
	}
	seen := map[string]bool{}
	for _, turn := range turns {
		seen[turn.ID] = true
	}
	if !seen[first.ID] || !seen[successor.ID] {
		t.Fatalf("session turns = %+v, want %q and %q", turns, first.ID, successor.ID)
	}

	messages, err := repo.ListMessages(ctx, "session-script")
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(messages) != 1 || messages[0].TurnID != first.ID {
		t.Fatalf("script messages = %+v, want one bound to %q", messages, first.ID)
	}
}

// TestWorktreeScriptMessageAdapter_ActiveTurnLookupFailure covers the design
// rule that a lookup error does not prove the session is idle: no fallback turn
// and no message may be created.
func TestWorktreeScriptMessageAdapter_ActiveTurnLookupFailure(t *testing.T) {
	repo := newMaterializerRepo(t)
	seedScriptTurnSession(t, repo)
	lookupErr := errors.New("turn lookup unavailable")
	svc := newScriptTurnTaskService(t, repo, failingActiveTurnRepo{TurnRepository: repo, err: lookupErr})
	ctx := context.Background()

	_, err := (&taskServiceAdapter{svc: svc}).CreateMessage(ctx, scriptMessageRequest(""))
	if !errors.Is(err, lookupErr) {
		t.Fatalf("CreateMessage error = %v, want %v", err, lookupErr)
	}
	messages, listErr := repo.ListMessages(ctx, "session-script")
	if listErr != nil {
		t.Fatalf("ListMessages: %v", listErr)
	}
	if len(messages) != 0 {
		t.Fatalf("lookup failure persisted %d messages", len(messages))
	}
	turns, listErr := repo.ListTurnsBySession(ctx, "session-script")
	if listErr != nil {
		t.Fatalf("ListTurnsBySession: %v", listErr)
	}
	if len(turns) != 0 {
		t.Fatalf("lookup failure persisted %d turns", len(turns))
	}
}
