package backendapp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agent/agents"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/coordinator"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestContainmentReaders_CredentialAndRepositoryReadersAreAlwaysWired(t *testing.T) {
	r := containmentReaders(containmentDeps{})
	if r.Credentials == nil || r.Repositories == nil {
		t.Fatalf("credentials=%v repositories=%v, want both non-nil", r.Credentials, r.Repositories)
	}
	if _, err := r.Credentials.CredentialValues(context.Background(), "a"); err == nil {
		t.Fatal("an unwired credentials source must fail closed as an error")
	}
	if _, err := r.Repositories.HasRepositoryBinding(context.Background(), "t"); err == nil {
		t.Fatal("an unwired repository source must fail closed as an error")
	}
	if _, err := r.Sessions.ConversationSession(context.Background(), "t"); err == nil {
		t.Fatal("an unwired session source must fail closed as an error")
	}
}

func TestRegisterCoordinatorRoutes_Phase3WiresContainment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	origPass := runCoordinatorBackgroundPass
	runCoordinatorBackgroundPass = func(context.Context, time.Time, []func(context.Context, time.Time)) {}
	t.Cleanup(func() { runCoordinatorBackgroundPass = origPass })
	for _, p3 := range []bool{false, true} {
		svc, err := initCoordinatorWiring(context.Background(), newCoordinatorTestPool(t), newCoordinatorTestTracker(t), nil, nil, nil, true, true, p3, newTestLogger())
		if err != nil {
			t.Fatal(err)
		}
		registerCoordinatorRoutes(routeParams{ctx: context.Background(), router: gin.New(), services: &Services{Coordinator: svc}, log: newTestLogger()})
		if got := svc.Containment() != nil; got != p3 {
			t.Fatalf("phase3=%v: containment wired = %v", p3, got)
		}
	}
}

type fakeContainmentAgent struct {
	agents.Agent
	required []string
}

func (f fakeContainmentAgent) Runtime() *agents.RuntimeConfig {
	return &agents.RuntimeConfig{RequiredEnv: f.required}
}

type fakeAgentRegistry map[string]agents.Agent

func (r fakeAgentRegistry) Get(id string) (agents.Agent, bool) { a, ok := r[id]; return a, ok }

type fakeCredValues map[string]string

func (f fakeCredValues) GetCredentialValue(_ context.Context, key string) (string, error) {
	v, ok := f[key]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

func TestContainmentCredentials_SkipsUnresolvedKeysAndFailsOnRegistryMiss(t *testing.T) {
	c := containmentCredentials{
		settings: containmentAgentSettings("agent-1", "claude"),
		registry: fakeAgentRegistry{"claude": fakeContainmentAgent{required: []string{"A", "B", "C"}}},
		values:   fakeCredValues{"A": "va", "C": ""},
	}
	got, err := c.CredentialValues(context.Background(), "agent-1")
	if err != nil || len(got) != 1 || got[0] != (coordinator.CredentialValue{Key: "A", Value: "va"}) {
		t.Fatalf("got %+v err=%v, want only A (B unresolved, C empty)", got, err)
	}
	c.registry = fakeAgentRegistry{}
	if _, err := c.CredentialValues(context.Background(), "agent-1"); err == nil {
		t.Fatal("an unregistered agent must be an error")
	}
	if _, err := c.CredentialValues(context.Background(), "missing"); err == nil {
		t.Fatal("an unknown agent id must be an error")
	}
}

type fakeContainmentTasks struct {
	coordinator.ContainmentExecutorReader
	task     *taskmodels.Task
	repos    map[string]*taskmodels.Repository
	session  *taskmodels.TaskSession
	sessions error
	listErr  error
}

func (f fakeContainmentTasks) ListTaskRepositories(context.Context, string) ([]*taskmodels.TaskRepository, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if f.task == nil {
		return nil, nil
	}
	return f.task.Repositories, nil
}
func (f fakeContainmentTasks) GetRepository(_ context.Context, id string) (*taskmodels.Repository, error) {
	return f.repos[id], nil
}
func (f fakeContainmentTasks) GetPrimarySession(context.Context, string) (*taskmodels.TaskSession, error) {
	return f.session, f.sessions
}

func TestContainmentRepositories_BindingDetection(t *testing.T) {
	bound := &taskmodels.Repository{SecretBindings: []taskmodels.RepositorySecretBinding{{}}}
	cases := []struct {
		name  string
		task  *taskmodels.Task
		repos map[string]*taskmodels.Repository
		want  bool
	}{
		{"no repositories", &taskmodels.Task{}, nil, false},
		{"repository without bindings", &taskmodels.Task{Repositories: []*taskmodels.TaskRepository{{RepositoryID: "r"}}}, map[string]*taskmodels.Repository{"r": {}}, false},
		{"repository with a binding", &taskmodels.Task{Repositories: []*taskmodels.TaskRepository{{RepositoryID: "r"}}}, map[string]*taskmodels.Repository{"r": bound}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := containmentRepositories{tasks: fakeContainmentTasks{task: tc.task, repos: tc.repos}}.HasRepositoryBinding(context.Background(), "t")
			if err != nil || got != tc.want {
				t.Fatalf("got %v err=%v, want %v", got, err, tc.want)
			}
		})
	}
}

func TestContainmentRepositories_ListErrorIsNotAbsence(t *testing.T) {
	_, err := containmentRepositories{tasks: fakeContainmentTasks{task: &taskmodels.Task{}, listErr: errors.New("db busy")}}.HasRepositoryBinding(context.Background(), "t")
	if err == nil {
		t.Fatal("a failed repository list read must surface as an error, not as no binding")
	}
}

func TestContainmentSessions_NoPrimarySessionIsNil(t *testing.T) {
	s := containmentSessions{tasks: fakeContainmentTasks{sessions: repoerrors.ErrNoPrimarySession}}
	got, err := s.ConversationSession(context.Background(), "t")
	if err != nil || got != nil {
		t.Fatalf("got %+v err=%v, want nil session", got, err)
	}
	started := time.Now()
	s = containmentSessions{tasks: fakeContainmentTasks{session: &taskmodels.TaskSession{State: taskmodels.TaskSessionStateCreated, StartedAt: started}}}
	got, err = s.ConversationSession(context.Background(), "t")
	if err != nil || got == nil || got.State != "CREATED" || !got.StartedAt.Equal(started) {
		t.Fatalf("got %+v err=%v", got, err)
	}
}

type fakeContainmentSettings struct {
	settingsstore.Repository
	id, name string
}

func (f fakeContainmentSettings) GetAgent(_ context.Context, id string) (*settingsmodels.Agent, error) {
	if id != f.id {
		return nil, errors.New("not found")
	}
	return &settingsmodels.Agent{ID: id, Name: f.name}, nil
}

func containmentAgentSettings(id, name string) settingsstore.Repository {
	return fakeContainmentSettings{id: id, name: name}
}
