package backendapp

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

type providerRefresherRepoFake struct {
	repositories          []*models.TaskRepository
	entities              map[string]*models.Repository
	sessions              []*models.TaskSession
	providerValues        []string
	providerQueryCalls    int
	repositoryLookupCalls int
}

func (r *providerRefresherRepoFake) ListTaskRepositories(context.Context, string) ([]*models.TaskRepository, error) {
	return r.repositories, nil
}

func (r *providerRefresherRepoFake) GetRepository(_ context.Context, id string) (*models.Repository, error) {
	r.repositoryLookupCalls++
	return r.entities[id], nil
}

func (r *providerRefresherRepoFake) ListTaskRepositoryProviders(context.Context, string) ([]string, error) {
	r.providerQueryCalls++
	if r.providerValues != nil {
		return append([]string(nil), r.providerValues...), nil
	}
	providers := make([]string, 0, len(r.repositories))
	for _, taskRepository := range r.repositories {
		if taskRepository == nil {
			continue
		}
		if repository := r.entities[taskRepository.RepositoryID]; repository != nil {
			providers = append(providers, repository.Provider)
		}
	}
	return providers, nil
}

func (r *providerRefresherRepoFake) ListTaskSessions(context.Context, string) ([]*models.TaskSession, error) {
	return r.sessions, nil
}

type providerRefreshCall struct {
	sessionID string
	providers []string
}

type providerRefresherLifecycleFake struct {
	calls       []providerRefreshCall
	errors      map[string]error
	previousSet map[string][]string
}

func (l *providerRefresherLifecycleFake) SetMcpProvidersForSession(_ context.Context, sessionID string, providers []string) error {
	l.calls = append(l.calls, providerRefreshCall{sessionID: sessionID, providers: append([]string(nil), providers...)})
	if err := l.errors[sessionID]; err != nil {
		return err
	}
	l.previousSet[sessionID] = append([]string(nil), providers...)
	return nil
}

func TestTaskMCPProviderRefresher_RefreshesEveryActiveSessionWithProviderUnion(t *testing.T) {
	refresher := newTaskMCPProviderRefresher(
		&providerRefresherRepoFake{
			repositories: []*models.TaskRepository{
				{RepositoryID: "repo-gitlab"},
				{RepositoryID: "repo-github"},
				{RepositoryID: "repo-unknown"},
			},
			entities: map[string]*models.Repository{
				"repo-gitlab":  {ID: "repo-gitlab", Provider: " GITLAB "},
				"repo-github":  {ID: "repo-github", Provider: "github"},
				"repo-unknown": {ID: "repo-unknown", Provider: "local"},
			},
			sessions: []*models.TaskSession{
				{ID: "running", State: models.TaskSessionStateRunning},
				{ID: "waiting", State: models.TaskSessionStateWaitingForInput},
				{ID: "completed", State: models.TaskSessionStateCompleted},
			},
		},
		&providerRefresherLifecycleFake{previousSet: make(map[string][]string)},
		newTestLogger(),
	)

	if err := refresher.RefreshTaskMCPProviders(context.Background(), "task-1"); err != nil {
		t.Fatalf("RefreshTaskMCPProviders: %v", err)
	}
	if got := len(refresher.lifecycle.(*providerRefresherLifecycleFake).calls); got != 2 {
		t.Fatalf("provider refresh calls = %d, want running and waiting sessions only", got)
	}
	for _, call := range refresher.lifecycle.(*providerRefresherLifecycleFake).calls {
		if !reflect.DeepEqual(call.providers, []string{"github", "gitlab"}) {
			t.Fatalf("session %q providers = %v, want [github gitlab]", call.sessionID, call.providers)
		}
	}
}

func TestTaskMCPProviderRefresher_NoActiveSessionsIsNoOp(t *testing.T) {
	lifecycle := &providerRefresherLifecycleFake{previousSet: make(map[string][]string)}
	refresher := newTaskMCPProviderRefresher(
		&providerRefresherRepoFake{sessions: nil}, lifecycle, newTestLogger(),
	)

	if err := refresher.RefreshTaskMCPProviders(context.Background(), "task-1"); err != nil {
		t.Fatalf("RefreshTaskMCPProviders: %v", err)
	}
	if len(lifecycle.calls) != 0 {
		t.Fatalf("provider refresh calls = %#v, want no calls", lifecycle.calls)
	}
}

func TestTaskMCPProviderRefresher_UsesOneJoinedProviderQuery(t *testing.T) {
	repo := &providerRefresherRepoFake{
		providerValues: []string{" GITLAB ", "github"},
		sessions:       []*models.TaskSession{{ID: "running", State: models.TaskSessionStateRunning}},
	}
	lifecycle := &providerRefresherLifecycleFake{previousSet: make(map[string][]string)}
	refresher := newTaskMCPProviderRefresher(repo, lifecycle, newTestLogger())

	if err := refresher.RefreshTaskMCPProviders(context.Background(), "task-1"); err != nil {
		t.Fatalf("RefreshTaskMCPProviders: %v", err)
	}
	if repo.providerQueryCalls != 1 {
		t.Fatalf("joined provider query calls = %d, want 1", repo.providerQueryCalls)
	}
	if repo.repositoryLookupCalls != 0 {
		t.Fatalf("individual repository lookups = %d, want none", repo.repositoryLookupCalls)
	}
	if !reflect.DeepEqual(lifecycle.previousSet["running"], []string{"github", "gitlab"}) {
		t.Fatalf("providers = %v, want [github gitlab]", lifecycle.previousSet["running"])
	}
}

func TestTaskMCPProviderRefresher_ContinuesAfterSessionFailure(t *testing.T) {
	refreshErr := errors.New("agentctl unavailable")
	lifecycle := &providerRefresherLifecycleFake{
		errors:      map[string]error{"running": refreshErr},
		previousSet: make(map[string][]string),
	}
	refresher := newTaskMCPProviderRefresher(
		&providerRefresherRepoFake{
			repositories: []*models.TaskRepository{{RepositoryID: "repo-github"}},
			entities:     map[string]*models.Repository{"repo-github": {ID: "repo-github", Provider: "github"}},
			sessions: []*models.TaskSession{
				{ID: "running", State: models.TaskSessionStateRunning},
				{ID: "waiting", State: models.TaskSessionStateWaitingForInput},
			},
		},
		lifecycle,
		newTestLogger(),
	)

	if err := refresher.RefreshTaskMCPProviders(context.Background(), "task-1"); !errors.Is(err, refreshErr) {
		t.Fatalf("RefreshTaskMCPProviders error = %v, want %v", err, refreshErr)
	}
	if len(lifecycle.calls) != 2 {
		t.Fatalf("provider refresh calls = %#v, want both active sessions attempted", lifecycle.calls)
	}
	if _, ok := lifecycle.previousSet["running"]; ok {
		t.Fatal("failed session should not replace its previous provider subset")
	}
	if !reflect.DeepEqual(lifecycle.previousSet["waiting"], []string{"github"}) {
		t.Fatalf("successful session providers = %v, want [github]", lifecycle.previousSet["waiting"])
	}
}
