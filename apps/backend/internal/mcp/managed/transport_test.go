package managed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
)

const testGrantToken = "managed-token-not-a-pat"

func TestGrantScope(t *testing.T) {
	fixture := newTransportFixture(t)
	var gotProfile profile.Context
	var gotIdentity authn.Identity
	var gotPrincipal scope.Principal
	transport := fixture.transport(t, func(ctx context.Context, _ *models.ManagedAgentBinding, got profile.Context, _ string) (http.Handler, error) {
		gotProfile = got
		gotIdentity, _ = authn.IdentityFromContext(ctx)
		gotPrincipal, _ = scope.PrincipalFromContext(ctx)
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "scoped")
		}), nil
	})
	response := fixture.request(transport)
	if response.Code != http.StatusOK || response.Body.String() != "scoped" {
		t.Fatalf("callback response = %d %q, want 200 scoped", response.Code, response.Body.String())
	}
	if gotIdentity.UserID != fixture.binding.UserID {
		t.Fatalf("callback identity = %#v, want binding owner", gotIdentity)
	}
	if gotPrincipal.CallerTaskID != fixture.binding.TaskID || gotPrincipal.CallerSessionID != fixture.binding.SessionID {
		t.Fatalf("callback principal = %#v, want binding task/session", gotPrincipal)
	}
	if gotProfile.Surface != profile.SurfaceManagedTask || !gotProfile.HasCapability(profile.CapabilityUserQuestion) {
		t.Fatalf("callback profile = %#v, want task tools and user-question capability", gotProfile)
	}
}

func TestGrantRevocationAfterQuestionWait(t *testing.T) {
	fixture := newTransportFixture(t)
	entered := make(chan struct{})
	resume := make(chan struct{})
	transport := fixture.transport(t, func(context.Context, *models.ManagedAgentBinding, profile.Context, string) (http.Handler, error) {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			close(entered)
			<-resume
			_, _ = io.WriteString(w, "question answer")
		}), nil
	})
	response := httptest.NewRecorder()
	finished := make(chan struct{})
	go func() {
		transport.ServeHTTP(response, fixture.newRequest())
		close(finished)
	}()
	<-entered
	fixture.repo.revoke()
	close(resume)
	<-finished
	if response.Code != http.StatusUnauthorized || strings.Contains(response.Body.String(), "question answer") {
		t.Fatalf("revoked callback response = %d %q, want a generic denial", response.Code, response.Body.String())
	}
}

func TestFeatureDisableDuringQuestionWaitRejectsResult(t *testing.T) {
	fixture := newTransportFixture(t)
	entered := make(chan struct{})
	resume := make(chan struct{})
	var enabled atomic.Bool
	enabled.Store(true)
	transport, err := NewTransport(Config{
		Repository: fixture.repo, Authority: fixtureAuthority{fixture: fixture}, Scope: fixturePrincipal{},
		Enabled: enabled.Load,
		HandlerFactory: func(context.Context, *models.ManagedAgentBinding, profile.Context, string) (http.Handler, error) {
			return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				close(entered)
				<-resume
				_, _ = io.WriteString(w, "question answer")
			}), nil
		},
	})
	if err != nil {
		t.Fatalf("NewTransport() error = %v", err)
	}
	response := httptest.NewRecorder()
	finished := make(chan struct{})
	go func() {
		transport.ServeHTTP(response, fixture.newRequest())
		close(finished)
	}()
	<-entered
	enabled.Store(false)
	close(resume)
	<-finished
	if response.Code != http.StatusUnauthorized || strings.Contains(response.Body.String(), "question answer") {
		t.Fatalf("disabled callback response = %d %q, want a generic denial", response.Code, response.Body.String())
	}
}

func TestDisabledGrant(t *testing.T) {
	fixture := newTransportFixture(t)
	called := false
	transport, err := NewTransport(Config{
		Repository: fixture.repo, Authority: fixtureAuthority{fixture: fixture}, Scope: fixturePrincipal{},
		Enabled: func() bool { return false }, HandlerFactory: func(context.Context, *models.ManagedAgentBinding, profile.Context, string) (http.Handler, error) {
			called = true
			return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), nil
		},
	})
	if err != nil {
		t.Fatalf("NewTransport() error = %v", err)
	}
	response := fixture.request(transport)
	if response.Code != http.StatusNotFound || called {
		t.Fatalf("disabled callback = %d, factory called %t; want 404 and no dispatch", response.Code, called)
	}
}

func TestGrantRejectsCrossSessionBinding(t *testing.T) {
	fixture := newTransportFixture(t)
	fixture.session.TaskID = "other-task"
	called := false
	transport := fixture.transport(t, func(context.Context, *models.ManagedAgentBinding, profile.Context, string) (http.Handler, error) {
		called = true
		return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), nil
	})
	response := fixture.request(transport)
	if response.Code != http.StatusUnauthorized || called {
		t.Fatalf("cross-session callback = %d, factory called %t; want denial without dispatch", response.Code, called)
	}
}

func TestIssueGrantStoresOnlyHashAndCapsLifetime(t *testing.T) {
	fixture := newTransportFixture(t)
	issued, err := IssueGrant(context.Background(), fixture.repo, fixture.binding, fixture.operation,
		profile.New(profile.SurfaceManagedTask, []profile.Capability{profile.CapabilityUserQuestion}, nil),
		time.Now().UTC(), 48*time.Hour)
	if err != nil {
		t.Fatalf("IssueGrant() error = %v", err)
	}
	if issued.Token == "" || issued.Grant.TokenHash == issued.Token {
		t.Fatal("issued bearer must be returned once and persisted only as a hash")
	}
	hash := sha256.Sum256([]byte(issued.Token))
	if issued.Grant.TokenHash != hex.EncodeToString(hash[:]) {
		t.Fatal("persisted token hash does not match the issued bearer")
	}
	if !issued.Grant.ExpiresAt.Equal(issued.Grant.CreatedAt.Add(MaxGrantLifetime)) {
		t.Fatalf("grant lifetime = %s, want maximum %s", issued.Grant.ExpiresAt.Sub(issued.Grant.CreatedAt), MaxGrantLifetime)
	}
	if !strings.HasPrefix(issued.URL, strings.TrimRight(fixture.binding.Launch.CallbackURL, "/")+"/") {
		t.Fatalf("callback URL = %q, want operation grant path", issued.URL)
	}
	transport := fixture.transport(t, func(_ context.Context, _ *models.ManagedAgentBinding, _ profile.Context, _ string) (http.Handler, error) {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }), nil
	})
	request := httptest.NewRequest(http.MethodPost, issued.URL, nil)
	request.Header.Set("Authorization", "Bearer "+issued.Token)
	response := httptest.NewRecorder()
	transport.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("request to issued callback URL %q returned %d, want 204", issued.URL, response.Code)
	}
	var storedProfile profile.Context
	if err := json.Unmarshal([]byte(issued.Grant.Scope), &storedProfile); err != nil {
		t.Fatalf("grant scope is not JSON: %v", err)
	}
	if storedProfile.Surface != profile.SurfaceManagedTask {
		t.Fatalf("persisted grant surface = %q, want managed-task", storedProfile.Surface)
	}
}

type transportFixture struct {
	repo      *memoryManagedRepository
	binding   *models.ManagedAgentBinding
	operation *models.ManagedAgentOperation
	grant     *models.ManagedAgentToolGrant
	task      *models.Task
	session   *models.TaskSession
}

func newTransportFixture(t *testing.T) *transportFixture {
	t.Helper()
	now := time.Now().UTC()
	binding := &models.ManagedAgentBinding{
		ID: "binding-1", SessionID: "session-1", TaskID: "task-1", WorkspaceID: "workspace-1",
		UserID: "user-1", ExecutionID: "execution-1", ProviderKind: "cursor_cloud", ExecutorID: "executor-1",
		ExecutorProfileID: "profile-1", CredentialRef: "secret-ref", RemoteAgentID: "bc-123e4567-e89b-12d3-a456-426614174000",
		Lifecycle: models.ManagedAgentBindingReady, DispatchGeneration: 4, Revision: 3,
		Launch: models.ManagedAgentLaunchSnapshot{RepositoryID: "repo-1", RepositoryURL: "https://github.com/acme/project", StartingRef: "main", Model: "model-1", CallbackURL: "https://kandev.example.test"},
	}
	operation := &models.ManagedAgentOperation{
		ID: "operation-1", BindingID: binding.ID, PromptTurnID: "turn-1", Kind: models.ManagedAgentOperationFollowup,
		DispatchGeneration: binding.DispatchGeneration, State: models.ManagedAgentSubmissionAccepted, Revision: 2,
	}
	grant := &models.ManagedAgentToolGrant{
		ID: "grant-1", BindingID: binding.ID, OperationID: operation.ID, TokenHash: hashToken(testGrantToken),
		Generation: operation.DispatchGeneration, Scope: `{"surface":"managed-task","capabilities":["user-question"]}`,
		ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}
	repo := &memoryManagedRepository{grant: grant, binding: binding, operation: operation}
	return &transportFixture{
		repo: repo, binding: binding, operation: operation, grant: grant,
		task: &models.Task{ID: binding.TaskID, WorkspaceID: binding.WorkspaceID},
		session: &models.TaskSession{ID: binding.SessionID, TaskID: binding.TaskID, AgentExecutionID: binding.ExecutionID,
			ExecutorID: binding.ExecutorID, ExecutorProfileID: binding.ExecutorProfileID, State: models.TaskSessionStateRunning},
	}
}

func (f *transportFixture) transport(t *testing.T, factory HandlerFactory) *Transport {
	t.Helper()
	authority := fixtureAuthority{fixture: f}
	principal := fixturePrincipal{}
	transport, err := NewTransport(Config{
		Repository: f.repo, Authority: authority, Scope: principal,
		Enabled:        func() bool { return true },
		HandlerFactory: factory,
	})
	if err != nil {
		t.Fatalf("NewTransport() error = %v", err)
	}
	return transport
}

func (f *transportFixture) request(transport *Transport) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	transport.ServeHTTP(response, f.newRequest())
	return response
}

func (f *transportFixture) newRequest() *http.Request {
	request := httptest.NewRequest(http.MethodPost, ManagedCallbackPath+f.grant.ID, strings.NewReader(`{"jsonrpc":"2.0","method":"tools/call"}`))
	request.Header.Set("Authorization", "Bearer "+testGrantToken)
	return request
}

type memoryManagedRepository struct {
	mu        sync.RWMutex
	grant     *models.ManagedAgentToolGrant
	binding   *models.ManagedAgentBinding
	operation *models.ManagedAgentOperation
}

func (r *memoryManagedRepository) CreateManagedAgentToolGrant(_ context.Context, grant *models.ManagedAgentToolGrant) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.grant = grant
	return nil
}

func (r *memoryManagedRepository) GetManagedAgentToolGrantByHash(_ context.Context, tokenHash string) (*models.ManagedAgentToolGrant, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.grant == nil || r.grant.TokenHash != tokenHash {
		return nil, errors.New("grant not found")
	}
	copy := *r.grant
	return &copy, nil
}

func (r *memoryManagedRepository) GetManagedAgentBinding(_ context.Context, bindingID string) (*models.ManagedAgentBinding, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.binding == nil || r.binding.ID != bindingID {
		return nil, errors.New("binding not found")
	}
	copy := *r.binding
	return &copy, nil
}

func (r *memoryManagedRepository) GetManagedAgentOperation(_ context.Context, operationID string) (*models.ManagedAgentOperation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.operation == nil || r.operation.ID != operationID {
		return nil, errors.New("operation not found")
	}
	copy := *r.operation
	return &copy, nil
}

func (r *memoryManagedRepository) revoke() {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	r.grant.RevokedAt = &now
}

type fixtureAuthority struct{ fixture *transportFixture }

func (a fixtureAuthority) GetTask(context.Context, string) (*models.Task, error) {
	copy := *a.fixture.task
	return &copy, nil
}

func (a fixtureAuthority) GetTaskSession(context.Context, string) (*models.TaskSession, error) {
	copy := *a.fixture.session
	return &copy, nil
}

type fixturePrincipal struct{}

func (fixturePrincipal) ScopeOverridingIdentity(ctx context.Context, _ string) (context.Context, error) {
	return authn.WithIdentity(ctx, authn.Identity{UserID: "user-1", Role: authn.RoleMember}), nil
}

func (fixturePrincipal) ScopePrincipal(ctx context.Context, taskID, sessionID string) (context.Context, error) {
	return scope.WithPrincipal(ctx, scope.Principal{
		CallerTaskID: taskID, CallerSessionID: sessionID, WorkspaceID: "workspace-1",
		Surface: profile.SurfaceKanbanTask,
	}), nil
}

func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
