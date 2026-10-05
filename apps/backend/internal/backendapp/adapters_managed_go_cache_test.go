package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agent/agents"
	agentexecutor "github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/registry"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	agentruntime "github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	orchestratorexecutor "github.com/kandev/kandev/internal/orchestrator/executor"
	storagepkg "github.com/kandev/kandev/internal/system/storage"
	"github.com/kandev/kandev/internal/system/storage/gocache"
	"github.com/kandev/kandev/internal/task/models"
	sqlitetaskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestManagedGoCacheRecoveryFlows(t *testing.T) {
	ctx := context.Background()
	logCore, observedLogs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(logCore))
	require.NoError(t, err)
	harness := newBootStateTestHarness(t)
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	server := newManagedGoCacheAgentCtlServer(t)
	backend := &managedGoCacheExecutorBackend{serverURL: server.URL(), log: log}
	executorRegistry := lifecycle.NewExecutorRegistry(log)
	executorRegistry.Register(backend)
	agentRegistry := registry.NewRegistry(log)
	agent := agents.NewMockAgentWithID("mock-agent", "Mock", "Mock")
	agent.SetEnabled(true)
	agent.SetSupportsMCP(false)
	require.NoError(t, agentRegistry.Register(agent))
	independentCache := filepath.Join(t.TempDir(), "independent-go-cache")
	manager := lifecycle.NewManager(
		agentRegistry, eventBus, executorRegistry, nil,
		managedGoCacheProfileResolver{env: settingsmodels.ProfileEnvVar{Key: "GOCACHE", Value: independentCache}},
		nil, lifecycle.ExecutorFallbackWarn, t.TempDir(), log,
	)
	manager.SetExecutorProfileReader(harness.taskRepo)
	manager.SetExecutorRunningWriter(harness.taskRepo)
	manager.SetWorkspaceInfoProvider(harness.taskSvc)
	cacheHome := t.TempDir()
	trashRoot := filepath.Join(cacheHome, "trash")
	settings, _ := newStorageMaintenanceStores(t)
	managedPath := filepath.Join(cacheHome, "adopted-cache")
	require.NoError(t, os.MkdirAll(managedPath, 0o700))
	_, err = settings.AdoptGoCachePath(ctx, managedPath)
	require.NoError(t, err)
	currentSettings, err := settings.GetSettings(ctx)
	require.NoError(t, err)
	currentSettings.GoCache.Enabled = true
	_, err = settings.SaveSettings(ctx, currentSettings)
	require.NoError(t, err)
	externalCache := filepath.Join(t.TempDir(), "external-cache")
	require.NoError(t, os.MkdirAll(externalCache, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(externalCache, "sentinel"), []byte("keep adopted cache"), 0o600))
	require.NoError(t, os.RemoveAll(managedPath))
	if err := os.Symlink(externalCache, managedPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	cacheProvider := gocache.New(gocache.Config{
		HomeDir: cacheHome, TrashDir: trashRoot, Settings: settings,
	})
	manager.SetManagedGoCacheEnvironmentProvider(cacheProvider)
	t.Cleanup(func() {
		require.NoError(t, manager.StopAllAgents(ctx))
		require.NoError(t, manager.Stop())
	})
	adapter := newLifecycleAdapter(manager, agentRegistry, log)
	repoAdapter := &taskRepositoryAdapter{repo: harness.taskRepo, svc: harness.taskSvc}
	orchestratorSvc := orchestrator.NewService(
		orchestrator.DefaultServiceConfig(), eventBus, adapter,
		repoAdapter, harness.taskRepo, nil, nil, nil, log,
	)
	orchestratorSvc.SetTurnService(newTurnServiceAdapter(harness.taskSvc))
	orchestratorSvc.SetTaskEventPublisher(harness.taskSvc)
	require.NoError(t, orchestratorSvc.Start(ctx))
	t.Cleanup(func() { require.NoError(t, orchestratorSvc.Stop()) })

	workspaces, err := harness.taskSvc.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, workspaces)
	workspacePath := t.TempDir()
	err = harness.taskRepo.CreateExecutor(ctx, &models.Executor{
		ID: "managed-cache-local", Name: "Local", Type: models.ExecutorTypeLocal,
		Status: models.ExecutorStatusActive,
	})
	require.NoError(t, err)
	require.NoError(t, harness.taskRepo.CreateExecutorProfile(ctx, &models.ExecutorProfile{
		ID: "managed-cache-profile", ExecutorID: "managed-cache-local", Name: "Local profile",
	}))
	task := &models.Task{
		ID: "managed-cache-task", WorkspaceID: workspaces[0].ID,
		Title: "Managed cache recovery", State: v1.TaskStateInProgress,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, harness.taskRepo.CreateTask(ctx, task))
	initialSession := createManagedGoCacheSession(t, harness.taskRepo, task.ID, "managed-cache-initial", workspacePath, models.TaskSessionStateCreated, "")
	orchestratorExecutor := orchestratorexecutor.NewExecutor(
		adapter, harness.taskRepo, log, orchestratorexecutor.ExecutorConfig{},
	)
	_, err = orchestratorExecutor.LaunchPreparedSession(ctx, task.ToAPI(), initialSession.ID,
		orchestratorexecutor.LaunchOptions{AgentProfileID: "mock-profile", StartAgent: true})
	require.NoError(t, err)
	waitManagedGoCacheSessionReady(t, harness.taskRepo, initialSession.ID)
	assertManagedGoCacheRuntime(t, backend, server, initialSession.ID, independentCache)

	resumeSession := createManagedGoCacheSession(t, harness.taskRepo, task.ID, "managed-cache-resume", workspacePath, models.TaskSessionStateFailed, "/stale/managed-cache")
	createManagedGoCacheFailedRuntime(t, harness.taskRepo, resumeSession, "provider-resume-token")
	response, err := orchestratorSvc.RecoverSession(ctx, task.ID, resumeSession.ID, "resume")
	require.NoError(t, err)
	require.NotNil(t, response)
	require.True(t, response.Success)
	waitManagedGoCacheSessionReady(t, harness.taskRepo, resumeSession.ID)
	assertManagedGoCacheRuntime(t, backend, server, resumeSession.ID, independentCache)
	require.Contains(t, server.LoadedTokens(), "provider-resume-token")
	assertManagedGoCacheSessionHasNoFailure(t, harness.taskRepo, resumeSession.ID)

	freshSession := createManagedGoCacheSession(t, harness.taskRepo, task.ID, "managed-cache-fresh", workspacePath, models.TaskSessionStateFailed, "/stale/managed-cache")
	createManagedGoCacheFailedRuntime(t, harness.taskRepo, freshSession, "provider-fresh-token")
	newSessionsBeforeFresh := len(server.NewSessionIDs())
	response, err = orchestratorSvc.RecoverSession(ctx, task.ID, freshSession.ID, "fresh_start")
	require.NoError(t, err)
	require.NotNil(t, response)
	require.True(t, response.Success)
	waitManagedGoCacheSessionReady(t, harness.taskRepo, freshSession.ID)
	assertManagedGoCacheRuntime(t, backend, server, freshSession.ID, independentCache)
	require.NotContains(t, server.LoadedTokens(), "provider-fresh-token")
	require.Len(t, server.NewSessionIDs(), newSessionsBeforeFresh+1, "Start fresh creates a new provider conversation")
	assertManagedGoCacheSessionHasNoFailure(t, harness.taskRepo, freshSession.ID)

	failureSession := createManagedGoCacheSession(t, harness.taskRepo, task.ID, "managed-cache-non-cache-failure", workspacePath, models.TaskSessionStateFailed, "/stale/managed-cache")
	createManagedGoCacheFailedRuntime(t, harness.taskRepo, failureSession, "provider-failure-token")
	configureFailuresBefore := server.ConfigureFailures()
	server.FailNextConfigure()
	_, err = orchestratorSvc.RecoverSession(ctx, task.ID, failureSession.ID, "resume")
	require.Error(t, err)
	require.NotContains(t, err.Error(), "managed Go cache")
	require.Equal(t, configureFailuresBefore+1, server.ConfigureFailures(), "unrelated configure failure was masked")
	failureAfterLaunch, err := harness.taskRepo.GetTaskSession(ctx, failureSession.ID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateFailed, failureAfterLaunch.State)
	require.NotContains(t, strings.ToLower(failureAfterLaunch.ErrorMessage), "go cache")

	cancelledSession := createManagedGoCacheSession(t, harness.taskRepo, task.ID, "managed-cache-cancelled", workspacePath, models.TaskSessionStateFailed, "/stale/managed-cache")
	createManagedGoCacheFailedRuntime(t, harness.taskRepo, cancelledSession, "provider-cancel-token")
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()
	createdBeforeCancel := backend.CreateCount()
	_, err = orchestratorSvc.RecoverSession(cancelledCtx, task.ID, cancelledSession.ID, "resume")
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, createdBeforeCancel, backend.CreateCount(), "cancelled recovery created an executor")

	cacheSettings, err := settings.GetSettings(ctx)
	require.NoError(t, err)
	require.True(t, cacheSettings.GoCache.Enabled)
	require.Equal(t, managedPath, cacheSettings.GoCache.AdoptedPath)
	cacheInfo, err := os.Lstat(managedPath)
	require.NoError(t, err)
	require.True(t, cacheInfo.Mode()&os.ModeSymlink != 0)
	cacheBytes, err := os.ReadFile(filepath.Join(externalCache, "sentinel"))
	require.NoError(t, err)
	require.Equal(t, "keep adopted cache", string(cacheBytes))
	entries := observedLogs.FilterMessage("managed Go cache preparation skipped; continuing without managed override").All()
	require.Len(t, entries, 4, "one bounded fallback warning per cache preparation decision")
	for _, entry := range entries {
		require.Equal(t, zapcore.WarnLevel, entry.Level)
		fields := entry.ContextMap()
		require.Equal(t, "preparation_failed", fields["reason"])
		for key := range fields {
			require.Contains(t, []string{"component", "reason", "task_id", "session_id"}, key,
				"unexpected fallback warning field %q", key)
		}
		require.Equal(t, task.ID, fields["task_id"])
		require.NotEmpty(t, fields["session_id"])
		require.NotContains(t, entry.Message, managedPath)
		require.NotContains(t, entry.Message, externalCache)
	}
}

type managedGoCacheProfileResolver struct {
	env settingsmodels.ProfileEnvVar
}

func (r managedGoCacheProfileResolver) ResolveProfile(_ context.Context, profileID string) (*lifecycle.AgentProfileInfo, error) {
	return &lifecycle.AgentProfileInfo{
		ProfileID: profileID, ProfileName: "Mock profile", AgentID: "mock-agent", AgentName: "mock-agent",
		NativeSessionResume: true, EnvVars: []settingsmodels.ProfileEnvVar{r.env},
	}, nil
}

type managedGoCacheExecutorBackend struct {
	serverURL string
	log       *logger.Logger
	mu        sync.Mutex
	created   []managedGoCacheCreateSnapshot
}

type managedGoCacheCreateSnapshot struct {
	sessionID string
	env       map[string]string
	metadata  map[string]interface{}
}

func (b *managedGoCacheExecutorBackend) Name() agentexecutor.Name {
	return agentexecutor.NameStandalone
}
func (b *managedGoCacheExecutorBackend) HealthCheck(context.Context) error { return nil }
func (b *managedGoCacheExecutorBackend) CreateInstance(_ context.Context, req *lifecycle.ExecutorCreateRequest) (*lifecycle.ExecutorInstance, error) {
	b.mu.Lock()
	b.created = append(b.created, managedGoCacheCreateSnapshot{
		sessionID: req.SessionID, env: cloneManagedGoCacheEnv(req.Env), metadata: cloneManagedGoCacheMetadata(req.Metadata),
	})
	b.mu.Unlock()
	parsed, err := url.Parse(b.serverURL)
	if err != nil {
		return nil, err
	}
	host, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return nil, err
	}
	client := agentctl.NewClient(host, port, b.log)
	return &lifecycle.ExecutorInstance{
		InstanceID: req.InstanceID, TaskID: req.TaskID, SessionID: req.SessionID,
		RuntimeName: agentruntime.RuntimeStandalone, Client: client,
		StandaloneInstanceID: req.InstanceID, StandalonePort: port,
		WorkspacePath: req.WorkspacePath, Metadata: cloneManagedGoCacheMetadata(req.Metadata),
	}, nil
}
func (b *managedGoCacheExecutorBackend) StopInstance(_ context.Context, instance *lifecycle.ExecutorInstance, _ bool) error {
	if instance != nil && instance.Client != nil {
		instance.Client.Close()
	}
	return nil
}
func (b *managedGoCacheExecutorBackend) RecoverInstances(context.Context, []*models.ExecutorRunning) ([]*lifecycle.ExecutorInstance, error) {
	return nil, nil
}
func (b *managedGoCacheExecutorBackend) GetInteractiveRunner() *process.InteractiveRunner { return nil }
func (b *managedGoCacheExecutorBackend) RequiresCloneURL() bool                           { return false }
func (b *managedGoCacheExecutorBackend) ShouldApplyPreferredShell() bool                  { return false }
func (b *managedGoCacheExecutorBackend) IsAlwaysResumable() bool                          { return true }

func (b *managedGoCacheExecutorBackend) CreateCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.created)
}

func (b *managedGoCacheExecutorBackend) SnapshotForSession(sessionID string) (managedGoCacheCreateSnapshot, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := len(b.created) - 1; i >= 0; i-- {
		if b.created[i].sessionID == sessionID {
			return b.created[i], true
		}
	}
	return managedGoCacheCreateSnapshot{}, false
}

type managedGoCacheAgentCtlServer struct {
	server            *httptest.Server
	mu                sync.Mutex
	configured        []map[string]string
	loadedTokens      []string
	newSessions       []string
	failConfigure     int
	configureFailures int
	nextSession       int
	upgrader          websocket.Upgrader
}

func newManagedGoCacheAgentCtlServer(t *testing.T) *managedGoCacheAgentCtlServer {
	t.Helper()
	s := &managedGoCacheAgentCtlServer{upgrader: websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/api/v1/agent/configure", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Env map[string]string `json:"env"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		s.configured = append(s.configured, cloneManagedGoCacheEnv(request.Env))
		fail := s.failConfigure > 0
		if fail {
			s.failConfigure--
			s.configureFailures++
		}
		s.mu.Unlock()
		if fail {
			http.Error(w, `{"error":"injected configure failure"}`, http.StatusInternalServerError)
			return
		}
		writeManagedGoCacheJSON(w, map[string]any{"success": true})
	})
	mux.HandleFunc("/api/v1/start", func(w http.ResponseWriter, _ *http.Request) {
		writeManagedGoCacheJSON(w, map[string]any{"success": true, "command": "mock-agent"})
	})
	mux.HandleFunc("/api/v1/agent/stream", s.handleAgentStream)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/stream") {
			s.handleGenericStream(w, r)
			return
		}
		writeManagedGoCacheJSON(w, map[string]any{"success": true})
	})
	s.server = httptest.NewServer(mux)
	t.Cleanup(s.server.Close)
	return s
}

func (s *managedGoCacheAgentCtlServer) URL() string { return s.server.URL }

func (s *managedGoCacheAgentCtlServer) FailNextConfigure() {
	s.mu.Lock()
	s.failConfigure++
	s.mu.Unlock()
}

func (s *managedGoCacheAgentCtlServer) ConfigureFailures() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.configureFailures
}

func (s *managedGoCacheAgentCtlServer) handleAgentStream(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	for {
		var request ws.Message
		if err := conn.ReadJSON(&request); err != nil {
			return
		}
		payload := map[string]any{"success": true}
		s.mu.Lock()
		switch request.Action {
		case "agent.initialize":
			payload["agent_info"] = map[string]string{"name": "mock-agent", "version": "test"}
		case "agent.session.new":
			s.nextSession++
			id := fmt.Sprintf("provider-session-%d", s.nextSession)
			s.newSessions = append(s.newSessions, id)
			payload["session_id"] = id
		case "agent.session.load":
			var load struct {
				SessionID string `json:"session_id"`
			}
			_ = json.Unmarshal(request.Payload, &load)
			s.loadedTokens = append(s.loadedTokens, load.SessionID)
			payload["session_id"] = load.SessionID
		}
		s.mu.Unlock()
		response, err := ws.NewResponse(request.ID, request.Action, payload)
		if err != nil || conn.WriteJSON(response) != nil {
			return
		}
	}
}

func (s *managedGoCacheAgentCtlServer) handleGenericStream(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (s *managedGoCacheAgentCtlServer) LoadedTokens() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.loadedTokens...)
}

func (s *managedGoCacheAgentCtlServer) NewSessionIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.newSessions...)
}

func writeManagedGoCacheJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func cloneManagedGoCacheEnv(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneManagedGoCacheMetadata(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func createManagedGoCacheSession(
	t *testing.T,
	repo *sqlitetaskrepo.Repository,
	taskID, sessionID, workspacePath string,
	state models.TaskSessionState,
	staleManagedCache string,
) *models.TaskSession {
	t.Helper()
	metadata := map[string]interface{}{}
	if staleManagedCache != "" {
		metadata["managed_go_cache_path"] = staleManagedCache
	}
	session := &models.TaskSession{
		ID: sessionID, TaskID: taskID, AgentProfileID: "mock-profile", ExecutionProfileID: "mock-profile",
		ExecutorID: "managed-cache-local", ExecutorProfileID: "managed-cache-profile",
		WorkspacePath: workspacePath, State: state, Metadata: metadata,
		StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, repo.CreateTaskSession(context.Background(), session))
	return session
}

func createManagedGoCacheFailedRuntime(
	t *testing.T,
	repo *sqlitetaskrepo.Repository,
	session *models.TaskSession,
	resumeToken string,
) {
	t.Helper()
	now := time.Now().UTC()
	require.NoError(t, repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		ID: "running-" + session.ID, SessionID: session.ID, TaskID: session.TaskID,
		ExecutionProfileID: "mock-profile", ExecutorID: "managed-cache-local",
		Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusFailed,
		Resumable: true, ResumeToken: resumeToken, AgentExecutionID: "previous-" + session.ID,
		CreatedAt: now, UpdatedAt: now,
	}))
}

func waitManagedGoCacheSessionReady(t *testing.T, repo *sqlitetaskrepo.Repository, sessionID string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		session, err := repo.GetTaskSession(context.Background(), sessionID)
		require.NoError(t, err)
		if session.State == models.TaskSessionStateWaitingForInput {
			return
		}
		if session.State == models.TaskSessionStateFailed {
			t.Fatalf("session %s failed during launch: %s", sessionID, session.ErrorMessage)
		}
		time.Sleep(20 * time.Millisecond)
	}
	session, err := repo.GetTaskSession(context.Background(), sessionID)
	require.NoError(t, err)
	t.Fatalf("session %s did not reach WAITING_FOR_INPUT, state=%s error=%q", sessionID, session.State, session.ErrorMessage)
}

func assertManagedGoCacheRuntime(t *testing.T, backend *managedGoCacheExecutorBackend, server *managedGoCacheAgentCtlServer, sessionID, independentCache string) {
	t.Helper()
	snapshot, found := backend.SnapshotForSession(sessionID)
	require.True(t, found, "lifecycle did not create an executor for %s", sessionID)
	require.Equal(t, independentCache, snapshot.env["GOCACHE"])
	require.NotContains(t, snapshot.metadata, "managed_go_cache_path")
	server.mu.Lock()
	defer server.mu.Unlock()
	require.NotEmpty(t, server.configured)
	require.Equal(t, independentCache, server.configured[len(server.configured)-1]["GOCACHE"])
}

func assertManagedGoCacheSessionHasNoFailure(t *testing.T, repo *sqlitetaskrepo.Repository, sessionID string) {
	t.Helper()
	session, err := repo.GetTaskSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.NotEqual(t, models.TaskSessionStateFailed, session.State)
	require.Empty(t, session.ErrorMessage)
	if failure, ok := models.LoadLastAgentError(session.Metadata); ok {
		require.NotContains(t, strings.ToLower(failure.Message), "go cache")
	}
}

func TestManagedGoCacheQuarantineRejectsSymlinks(t *testing.T) {
	for _, test := range []struct {
		name       string
		operation  string
		unsafePath string
	}{
		{name: "restore rejects linked original", operation: "restore", unsafePath: "original"},
		{name: "restore rejects linked payload", operation: "restore", unsafePath: "payload"},
		{name: "delete rejects linked payload", operation: "delete", unsafePath: "payload"},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			settings, store := newStorageMaintenanceStores(t)
			entry := createGoCacheQuarantineEntryWithID(
				t, store, home, "managed-cache", time.Now().UTC().Add(-time.Hour),
			)
			target := filepath.Join(t.TempDir(), "external-cache")
			require.NoError(t, os.MkdirAll(target, 0o700))
			sentinel := filepath.Join(target, "sentinel")
			require.NoError(t, os.WriteFile(sentinel, []byte("retain target"), 0o600))

			linkPath := entry.OriginalPath
			if test.unsafePath == "payload" {
				linkPath = entry.QuarantinePath
				require.NoError(t, os.RemoveAll(linkPath))
			} else {
				require.NoError(t, os.MkdirAll(filepath.Dir(linkPath), 0o700))
			}
			require.NoError(t, os.Symlink(target, linkPath))
			linkTargetBefore, err := os.Readlink(linkPath)
			require.NoError(t, err)
			linkInfoBefore, err := os.Lstat(linkPath)
			require.NoError(t, err)
			require.True(t, linkInfoBefore.Mode()&os.ModeSymlink != 0)

			controller := &workspaceQuarantineController{settings: settings, store: store, homeDir: home}
			if test.operation == "restore" {
				_, err = controller.Restore(context.Background(), entry.ID)
			} else {
				_, err = controller.PermanentDelete(
					context.Background(), entry.ID, storagepkg.QuarantineConfirmationDelete,
				)
			}
			require.ErrorIs(t, err, storagepkg.ErrValidation)

			stored, getErr := store.GetQuarantineEntry(context.Background(), entry.ID)
			require.NoError(t, getErr)
			require.Equal(t, storagepkg.QuarantineStateQuarantined, stored.State)
			linkInfoAfter, statErr := os.Lstat(linkPath)
			require.NoError(t, statErr)
			require.True(t, linkInfoAfter.Mode()&os.ModeSymlink != 0)
			linkTargetAfter, readErr := os.Readlink(linkPath)
			require.NoError(t, readErr)
			require.Equal(t, linkTargetBefore, linkTargetAfter)
			require.True(t, os.SameFile(linkInfoBefore, linkInfoAfter), "unsafe link identity changed")
			data, readErr := os.ReadFile(sentinel)
			require.NoError(t, readErr)
			require.Equal(t, "retain target", string(data))
			if test.unsafePath == "original" {
				_, statErr = os.Stat(filepath.Join(entry.QuarantinePath, "artifact"))
				require.NoError(t, statErr, "quarantine payload changed")
			} else {
				_, statErr = os.Stat(filepath.Join(entry.QuarantinePath, "artifact"))
				require.Error(t, statErr, "symlinked payload unexpectedly exposes the prior cache")
			}
		})
	}
}

func TestManagedGoCacheQuarantineKeepsSafePathControls(t *testing.T) {
	home := t.TempDir()
	settings, store := newStorageMaintenanceStores(t)
	entry := createGoCacheQuarantineEntryWithID(
		t, store, home, "safe-cache", time.Now().UTC().Add(-time.Hour),
	)
	controller := &workspaceQuarantineController{settings: settings, store: store, homeDir: home}

	deleted, err := controller.PermanentDelete(
		context.Background(), entry.ID, storagepkg.QuarantineConfirmationDelete,
	)
	require.NoError(t, err)
	require.Equal(t, storagepkg.QuarantineStateDeleted, deleted.State)
	_, err = os.Lstat(entry.QuarantinePath)
	require.True(t, errors.Is(err, os.ErrNotExist), "safe quarantine payload still exists: %v", err)
}
