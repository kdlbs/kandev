package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type workflowReadBarrier struct {
	repository.WorkflowRepository
	armed  atomic.Bool
	read   chan struct{}
	resume chan struct{}
}

func (r *workflowReadBarrier) GetWorkflow(ctx context.Context, id string) (*models.Workflow, error) {
	w, err := r.WorkflowRepository.GetWorkflow(ctx, id)
	if err != nil || !r.armed.Swap(false) {
		return w, err
	}
	close(r.read)
	select {
	case <-r.resume:
		return w, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func workflowPatchServices(t *testing.T) ([2]*Service, [2]*workflowReadBarrier, [2]*MockEventBus) {
	t.Helper()
	first, path := serviceTestSQLiteTemplate.Open(t)
	raw, err := internaldb.OpenSQLite(path)
	require.NoError(t, err)
	second := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { require.NoError(t, second.Close()) })
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	var services [2]*Service
	var gates [2]*workflowReadBarrier
	var buses [2]*MockEventBus
	var identities [2]any
	for i, database := range []*sqlx.DB{first, second} {
		database.SetMaxOpenConns(1)
		database.SetMaxIdleConns(1)
		conn, err := database.Conn(context.Background())
		require.NoError(t, err)
		require.NoError(t, conn.Raw(func(driver any) error { identities[i] = driver; return nil }))
		require.NoError(t, conn.Close())
		repo := sqliterepo.NewWithInitializedDB(database, database, nil)
		if i == 0 {
			require.NoError(t, repo.CreateWorkspace(context.Background(), &models.Workspace{ID: "workflow-ws", Name: "Workflow"}))
		}
		gates[i] = &workflowReadBarrier{WorkflowRepository: repo, read: make(chan struct{}), resume: make(chan struct{})}
		buses[i] = &MockEventBus{}
		services[i] = &Service{workflows: gates[i], logger: log, eventBus: buses[i]}
	}
	require.NotEqual(t, fmt.Sprintf("%p", identities[0]), fmt.Sprintf("%p", identities[1]))
	t.Logf("independent SQLite physical connections: %p / %p", identities[0], identities[1])
	require.NoError(t, gates[0].CreateWorkflow(context.Background(), &models.Workflow{
		ID: "workflow-fields", WorkspaceID: "workflow-ws", Name: "before name", Prompt: "before prompt", Description: "keep description", Style: models.WorkflowStyleKanban,
	}))
	return services, gates, buses
}

func workflowOverlap(t *testing.T, gate *workflowReadBarrier, first, second func(context.Context) error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	var workers sync.WaitGroup
	var release sync.Once
	defer func() { release.Do(func() { close(gate.resume) }); cancel(); workers.Wait() }()
	gate.armed.Store(true)
	done := make(chan error, 1)
	workers.Go(func() { done <- first(ctx) })
	select {
	case <-gate.read:
	case <-ctx.Done():
		t.Fatal("workflow SQL read barrier not reached", ctx.Err())
	}
	require.NoError(t, second(ctx))
	release.Do(func() { close(gate.resume) })
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("held workflow API did not settle", ctx.Err())
	}
}

// @covers AC-TASKS-FIELD-UPDATES-002.1, AC-TASKS-FIELD-UPDATES-002.4
func TestWorkflowFieldUpdatesConcurrentSQLite(t *testing.T) {
	for held := range 2 {
		t.Run(fmt.Sprintf("held_field_%d", held), func(t *testing.T) {
			svcs, gates, buses := workflowPatchServices(t)
			name, prompt := "saved name", "saved prompt"
			requests := [2]*UpdateWorkflowRequest{{Name: &name}, {Prompt: &prompt}}
			workflowOverlap(t, gates[held], func(ctx context.Context) error {
				_, err := svcs[held].UpdateWorkflow(ctx, "workflow-fields", requests[held])
				return err
			}, func(ctx context.Context) error {
				_, err := svcs[1-held].UpdateWorkflow(ctx, "workflow-fields", requests[1-held])
				return err
			})
			stored, err := gates[held].GetWorkflow(context.Background(), "workflow-fields")
			require.NoError(t, err)
			require.Equal(t, name, stored.Name, "success must retain the other writer's name")
			require.Equal(t, prompt, stored.Prompt, "success must retain the other writer's prompt")
			require.Equal(t, "keep description", stored.Description)
			for _, bus := range buses {
				eventsSeen := bus.GetPublishedEvents()
				require.Len(t, eventsSeen, 1)
				require.Equal(t, events.WorkflowUpdated, eventsSeen[0].Type)
			}
		})
	}
}

// @covers AC-TASKS-FIELD-UPDATES-002.1, AC-TASKS-FIELD-UPDATES-002.3
func TestWorkflowFieldUpdatesDomainWriters(t *testing.T) {
	for _, domain := range []string{"hidden", "source"} {
		for held := range 2 {
			t.Run(fmt.Sprintf("%s_held_%d", domain, held), func(t *testing.T) {
				svcs, gates, _ := workflowPatchServices(t)
				prompt := "saved prompt"
				ops := [2]func(context.Context) error{
					func(ctx context.Context) error {
						_, err := svcs[0].UpdateWorkflow(ctx, "workflow-fields", &UpdateWorkflowRequest{Prompt: &prompt})
						return err
					},
					func(ctx context.Context) error {
						if domain == "hidden" {
							return svcs[1].SetWorkflowHidden(ctx, "workflow-fields", true)
						}
						return svcs[1].SetWorkflowSource(ctx, "workflow-fields", models.WorkflowSourceGitHub, "workflow.yaml")
					},
				}
				workflowOverlap(t, gates[held], ops[held], ops[1-held])
				stored, err := gates[0].GetWorkflow(context.Background(), "workflow-fields")
				require.NoError(t, err)
				require.Equal(t, prompt, stored.Prompt)
				if domain == "hidden" {
					require.True(t, stored.Hidden)
				} else {
					require.Equal(t, models.WorkflowSourceGitHub, stored.Source)
					require.Equal(t, "workflow.yaml", stored.SourcePath)
				}
			})
		}
	}
}

// @covers AC-TASKS-FIELD-UPDATES-002.2
func TestWorkflowFieldUpdatesPresence(t *testing.T) {
	svcs, gates, buses := workflowPatchServices(t)
	ctx := context.Background()
	empty, profile := "", "  profile-id  "
	updated, err := svcs[0].UpdateWorkflow(ctx, "workflow-fields", &UpdateWorkflowRequest{Prompt: &empty, AgentProfileID: &profile})
	require.NoError(t, err)
	require.Empty(t, updated.Prompt)
	require.Equal(t, "profile-id", updated.AgentProfileID)
	require.Equal(t, "before name", updated.Name)
	require.Equal(t, "keep description", updated.Description)
	updated, err = svcs[1].UpdateWorkflow(ctx, "workflow-fields", &UpdateWorkflowRequest{Name: &empty, Description: &empty, AgentProfileID: &empty})
	require.NoError(t, err)
	require.Empty(t, updated.Name)
	require.Empty(t, updated.Description)
	require.Empty(t, updated.AgentProfileID)
	require.NoError(t, svcs[0].SetWorkflowHidden(ctx, "workflow-fields", true))
	require.NoError(t, svcs[1].SetWorkflowHidden(ctx, "workflow-fields", false))
	require.NoError(t, svcs[0].SetWorkflowSource(ctx, "workflow-fields", models.WorkflowSourceGitHub, "workflow.yaml"))
	require.NoError(t, svcs[1].SetWorkflowSource(ctx, "workflow-fields", models.WorkflowSourceManual, ""))
	stored, err := gates[0].GetWorkflow(ctx, "workflow-fields")
	require.NoError(t, err)
	require.False(t, stored.Hidden)
	require.Empty(t, stored.SourcePath)
	count := len(buses[0].GetPublishedEvents()) + len(buses[1].GetPublishedEvents())
	require.NoError(t, svcs[0].SetWorkflowHidden(ctx, "workflow-fields", false))
	require.NoError(t, svcs[1].SetWorkflowSource(ctx, "workflow-fields", models.WorkflowSourceManual, ""))
	require.Equal(t, count, len(buses[0].GetPublishedEvents())+len(buses[1].GetPublishedEvents()))
}

// @covers AC-TASKS-FIELD-UPDATES-002.2
func TestWorkflowFieldUpdatesFullDraftControl(t *testing.T) {
	svcs, gates, _ := workflowPatchServices(t)
	ctx := context.Background()
	prompt, name, description, profile := "new prompt", "draft name", "draft description", "draft profile"
	_, err := svcs[0].UpdateWorkflow(ctx, "workflow-fields", &UpdateWorkflowRequest{Prompt: &prompt})
	require.NoError(t, err)
	draftPrompt := "draft prompt"
	_, err = svcs[1].UpdateWorkflow(ctx, "workflow-fields", &UpdateWorkflowRequest{
		Name: &name, Description: &description, Prompt: &draftPrompt, AgentProfileID: &profile,
	})
	require.NoError(t, err)
	stored, err := gates[0].GetWorkflow(ctx, "workflow-fields")
	require.NoError(t, err)
	require.Equal(t, draftPrompt, stored.Prompt, "all supplied draft fields remain authoritative")
	require.Equal(t, name, stored.Name)
	require.Equal(t, description, stored.Description)
	require.Equal(t, profile, stored.AgentProfileID)
}

type fencedWorkflowBarrier struct{ *workflowReadBarrier }

func (r fencedWorkflowBarrier) UpdateWorkflowIfUnchanged(ctx context.Context, workflow *models.Workflow, expected time.Time) error {
	return r.WorkflowRepository.(exactWorkflowVersionUpdater).UpdateWorkflowIfUnchanged(ctx, workflow, expected)
}

// @covers AC-TASKS-FIELD-UPDATES-002.3
func TestWorkflowFieldUpdatesExactFence(t *testing.T) {
	t.Run("stale_at_SQL_admission", func(t *testing.T) {
		svcs, gates, buses := workflowPatchServices(t)
		svcs[0].workflows = fencedWorkflowBarrier{gates[0]}
		before, err := gates[0].GetWorkflow(context.Background(), "workflow-fields")
		require.NoError(t, err)
		name, prompt := "rejected name", "saved prompt"
		workflowOverlap(t, gates[0], func(ctx context.Context) error {
			_, err := svcs[0].UpdateWorkflow(ctx, before.ID, &UpdateWorkflowRequest{Name: &name, ExpectedUpdatedAt: &before.UpdatedAt})
			require.ErrorIs(t, err, repoerrors.ErrTaskVersionConflict)
			return nil
		}, func(ctx context.Context) error {
			_, err := svcs[1].UpdateWorkflow(ctx, before.ID, &UpdateWorkflowRequest{Prompt: &prompt})
			return err
		})
		stored, err := gates[0].GetWorkflow(context.Background(), before.ID)
		require.NoError(t, err)
		require.Equal(t, before.Name, stored.Name)
		require.Equal(t, prompt, stored.Prompt)
		require.Empty(t, buses[0].GetPublishedEvents())
	})
	t.Run("fence_unavailable_fails_closed", func(t *testing.T) {
		svcs, gates, buses := workflowPatchServices(t)
		before, err := gates[0].GetWorkflow(context.Background(), "workflow-fields")
		require.NoError(t, err)
		name := "rejected name"
		_, err = svcs[0].UpdateWorkflow(context.Background(), before.ID, &UpdateWorkflowRequest{Name: &name, ExpectedUpdatedAt: &before.UpdatedAt})
		require.EqualError(t, err, "workflow version fencing is unavailable")
		stored, err := gates[0].GetWorkflow(context.Background(), before.ID)
		require.NoError(t, err)
		require.Equal(t, before, stored)
		require.Empty(t, buses[0].GetPublishedEvents())
	})
}

// @covers AC-TASKS-FIELD-UPDATES-002.3
func TestWorkflowFieldUpdatesFailures(t *testing.T) {
	t.Run("cancelled_after_read_before_write", func(t *testing.T) {
		svcs, gates, buses := workflowPatchServices(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		before, err := gates[0].GetWorkflow(ctx, "workflow-fields")
		require.NoError(t, err)
		name := "cancelled name"
		workflowOverlap(t, gates[0], func(context.Context) error {
			_, err := svcs[0].UpdateWorkflow(ctx, before.ID, &UpdateWorkflowRequest{Name: &name})
			require.ErrorIs(t, err, context.Canceled)
			return nil
		}, func(context.Context) error { cancel(); return nil })
		stored, err := gates[0].GetWorkflow(context.Background(), before.ID)
		require.NoError(t, err)
		require.Equal(t, before, stored)
		require.Empty(t, buses[0].GetPublishedEvents())
	})
	t.Run("foreign_and_missing", func(t *testing.T) {
		svcs, gates, buses := workflowPatchServices(t)
		repo := gates[0].WorkflowRepository.(*sqliterepo.Repository)
		svcs[0].workspaces = repo
		ctx := context.Background()
		require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "foreign-ws", Name: "Foreign", OwnerID: "other"}))
		require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "foreign", WorkspaceID: "foreign-ws", Name: "private"}))
		name := "forbidden"
		_, err := svcs[0].UpdateWorkflow(ctxAs("caller"), "foreign", &UpdateWorkflowRequest{Name: &name})
		require.ErrorIs(t, err, repoerrors.ErrWorkflowNotFound)
		_, err = svcs[0].UpdateWorkflow(ctx, "missing", &UpdateWorkflowRequest{Name: &name})
		require.ErrorIs(t, err, repoerrors.ErrWorkflowNotFound)
		stored, err := repo.GetWorkflow(ctx, "foreign")
		require.NoError(t, err)
		require.Equal(t, "private", stored.Name)
		require.Empty(t, buses[0].GetPublishedEvents())
	})
}
