package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/system/storage"
	"github.com/kandev/kandev/internal/system/storage/gocache"
)

type managedGoCacheSettingsStub struct {
	settings storage.StorageMaintenanceSettings
	err      error
}

func (s managedGoCacheSettingsStub) GetSettings(context.Context) (storage.StorageMaintenanceSettings, error) {
	return s.settings, s.err
}

type managedGoCacheProviderFunc func(context.Context) (map[string]string, error)

func (f managedGoCacheProviderFunc) ExecutionEnvironment(ctx context.Context) (map[string]string, error) {
	return f(ctx)
}

func TestManagedGoCacheFallbackAdoptedRootSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "user-cache")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("create cache target: %v", err)
	}
	sentinel := filepath.Join(target, "keep.txt")
	if err := os.WriteFile(sentinel, []byte("user cache data"), 0o600); err != nil {
		t.Fatalf("write cache sentinel: %v", err)
	}
	cachePath := filepath.Join(root, "adopted-cache")
	if err := os.Symlink(target, cachePath); err != nil {
		t.Fatalf("symlink adopted cache: %v", err)
	}

	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	settings.GoCache.AdoptedPath = cachePath
	provider := gocache.New(gocache.Config{
		HomeDir:  filepath.Join(root, "home"),
		TrashDir: filepath.Join(root, "trash"),
		Settings: managedGoCacheSettingsStub{settings: settings},
	})

	mgr, backend := newEnvironmentExecutionTestManager(t, nil)
	mgr.dataDir = t.TempDir()
	mgr.SetManagedGoCacheEnvironmentProvider(provider)
	request := &LaunchRequest{
		TaskID:         "task-1",
		SessionID:      "session-1",
		AgentProfileID: "profile-1",
		ExecutorType:   "local",
		IsEphemeral:    true,
		Env:            map[string]string{"GOCACHE": "/independent/cache"},
	}

	execution, err := mgr.Launch(context.Background(), request)
	if err != nil {
		t.Fatalf("Launch() error = %v, want cache fallback", err)
	}
	if execution == nil {
		t.Fatal("Launch() returned no execution")
	}
	if got := backend.createCount.Load(); got != 1 {
		t.Fatalf("CreateInstance count = %d, want one execution", got)
	}
	if got := request.Env["GOCACHE"]; got != "/independent/cache" {
		t.Fatalf("request GOCACHE = %q, want independently configured value", got)
	}
	if got := backend.lastRequest.Env["GOCACHE"]; got != "/independent/cache" {
		t.Fatalf("runtime GOCACHE = %q, want independently configured value", got)
	}
	if got := execution.metadataString(managedGoCacheMetadataKey); got != "" {
		t.Fatalf("managed cache metadata = %q, want no managed override", got)
	}
	if data, err := os.ReadFile(sentinel); err != nil || string(data) != "user cache data" {
		t.Fatalf("adopted cache sentinel = %q, err = %v, want unchanged data", data, err)
	}
}

func TestManagedGoCacheStrictEnvironmentComposesManagedOverride(t *testing.T) {
	mgr := newTestManager(t)
	request := &LaunchRequest{
		TaskID:                        "task-1",
		SessionID:                     "session-1",
		EnvironmentResolutionRequired: true,
		Env:                           map[string]string{"GOCACHE": "/independent/cache"},
		managedGoCachePath:            "/managed/cache",
	}

	env, err := mgr.buildEnvForExecution(context.Background(), "exec-1", request, nil, nil)
	if err != nil {
		t.Fatalf("buildEnvForExecution() error = %v", err)
	}
	if got := env["GOCACHE"]; got != "/managed/cache" {
		t.Fatalf("resolved GOCACHE = %q, want managed path", got)
	}
	if got := request.Env["GOCACHE"]; got != "/independent/cache" {
		t.Fatalf("request GOCACHE = %q, want preserved input", got)
	}
}

func TestManagedGoCacheFallbackAncestorSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("create cache target: %v", err)
	}
	sentinel := filepath.Join(target, "keep.txt")
	if err := os.WriteFile(sentinel, []byte("keep ancestor target"), 0o600); err != nil {
		t.Fatalf("write cache sentinel: %v", err)
	}
	ancestor := filepath.Join(root, "cache-root")
	if err := os.Symlink(target, ancestor); err != nil {
		t.Fatalf("symlink cache ancestor: %v", err)
	}

	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	settings.GoCache.AdoptedPath = filepath.Join(ancestor, "go-build")
	provider := gocache.New(gocache.Config{
		HomeDir:  filepath.Join(root, "home"),
		TrashDir: filepath.Join(root, "trash"),
		Settings: managedGoCacheSettingsStub{settings: settings},
	})
	mgr := newTestManager(t)
	mgr.SetManagedGoCacheEnvironmentProvider(provider)
	request := &LaunchRequest{ExecutorType: "worktree", Env: map[string]string{"GOCACHE": "/independent/cache"}}

	if err := mgr.prepareManagedGoCacheEnvironment(context.Background(), request); err != nil {
		t.Fatalf("prepareManagedGoCacheEnvironment() error = %v, want fallback", err)
	}
	if got := request.Env["GOCACHE"]; got != "/independent/cache" {
		t.Fatalf("request GOCACHE = %q, want independent value", got)
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "keep ancestor target" {
		t.Fatalf("ancestor target sentinel = %q, err = %v", got, err)
	}
}

func TestManagedGoCacheFallbackSettingsAndFilesystemErrors(t *testing.T) {
	tests := []struct {
		name     string
		provider ManagedGoCacheEnvironmentProvider
	}{
		{
			name: "settings failure",
			provider: managedGoCacheProviderFromSettings(t, storage.DefaultSettings(),
				errors.New("settings database unavailable")),
		},
		{
			name:     "filesystem failure",
			provider: managedGoCacheFilesystemFailureProvider(t),
		},
		{
			name: "relative provider output",
			provider: managedGoCacheProviderFunc(func(context.Context) (map[string]string, error) {
				return map[string]string{"GOCACHE": "relative/cache"}, nil
			}),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mgr := newTestManager(t)
			mgr.SetManagedGoCacheEnvironmentProvider(test.provider)
			originalMetadata := map[string]interface{}{
				managedGoCacheMetadataKey: "/stale/managed/cache",
				"other":                   "retained",
			}
			request := &LaunchRequest{
				ExecutorType:       "local",
				Env:                map[string]string{"GOCACHE": "/independent/cache"},
				Metadata:           originalMetadata,
				managedGoCachePath: "/stale/managed/cache",
			}

			if err := mgr.prepareManagedGoCacheEnvironment(context.Background(), request); err != nil {
				t.Fatalf("prepareManagedGoCacheEnvironment() error = %v, want fallback", err)
			}
			if request.managedGoCachePath != "" {
				t.Fatalf("managedGoCachePath = %q, want empty", request.managedGoCachePath)
			}
			if _, exists := request.Metadata[managedGoCacheMetadataKey]; exists {
				t.Fatalf("metadata retained rejected cache path: %#v", request.Metadata)
			}
			if request.Metadata["other"] != "retained" {
				t.Fatalf("unrelated metadata = %#v, want retained", request.Metadata)
			}
			if originalMetadata[managedGoCacheMetadataKey] != "/stale/managed/cache" {
				t.Fatalf("caller metadata was changed: %#v", originalMetadata)
			}
			if got := request.Env["GOCACHE"]; got != "/independent/cache" {
				t.Fatalf("request GOCACHE = %q, want independent value", got)
			}
		})
	}
}

func TestManagedGoCacheFallbackWarningIsBounded(t *testing.T) {
	tests := []struct {
		name     string
		provider ManagedGoCacheEnvironmentProvider
		reason   string
	}{
		{
			name: "provider error",
			provider: managedGoCacheProviderFunc(func(context.Context) (map[string]string, error) {
				return nil, errors.New(strings.Repeat("secret-token https://private.example /private/path\n", 100))
			}),
			reason: "preparation_failed",
		},
		{
			name: "invalid output",
			provider: managedGoCacheProviderFunc(func(context.Context) (map[string]string, error) {
				return map[string]string{"GOCACHE": "relative/private-path"}, nil
			}),
			reason: "invalid_output",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			core, logs := observer.New(zapcore.WarnLevel)
			log, err := logger.NewFromZap(zap.New(core))
			if err != nil {
				t.Fatalf("create observer logger: %v", err)
			}
			mgr := newTestManager(t)
			mgr.logger = log
			mgr.SetManagedGoCacheEnvironmentProvider(test.provider)
			request := &LaunchRequest{
				TaskID: "task-1", SessionID: "session-1", ExecutorType: "local",
			}
			if err := mgr.prepareManagedGoCacheEnvironment(context.Background(), request); err != nil {
				t.Fatalf("prepareManagedGoCacheEnvironment() error = %v, want fallback", err)
			}
			entries := logs.All()
			if len(entries) != 1 {
				t.Fatalf("warning count = %d, want 1", len(entries))
			}
			entry := entries[0]
			if entry.Message != "managed Go cache preparation skipped; continuing without managed override" {
				t.Fatalf("warning message = %q", entry.Message)
			}
			fields := entry.ContextMap()
			if len(fields) != 3 || fields["reason"] != test.reason || fields["task_id"] != request.TaskID || fields["session_id"] != request.SessionID {
				t.Fatalf("warning fields = %#v, want reason %q and task/session IDs", fields, test.reason)
			}
			if strings.Contains(entry.Message, "secret-token") || strings.Contains(fmt.Sprint(fields), "private") {
				t.Fatalf("warning exposed provider output: %q %#v", entry.Message, fields)
			}
		})
	}
}

func TestManagedGoCacheDecisionResetsAndPreservesRequestInput(t *testing.T) {
	managedPath := filepath.Join(t.TempDir(), "managed", "cache")
	var calls atomic.Int32
	provider := managedGoCacheProviderFunc(func(context.Context) (map[string]string, error) {
		if calls.Add(1) == 1 {
			return map[string]string{"GOCACHE": managedPath}, nil
		}
		return nil, errors.New("cache unavailable")
	})
	mgr := newTestManager(t)
	mgr.SetManagedGoCacheEnvironmentProvider(provider)
	callerMetadata := map[string]interface{}{
		managedGoCacheMetadataKey: "/old/managed/cache",
		"other":                   "keep",
	}
	request := &LaunchRequest{
		ExecutorType: "local",
		Env:          map[string]string{"GOCACHE": managedPath},
		Metadata:     callerMetadata,
	}

	if err := mgr.prepareManagedGoCacheEnvironment(context.Background(), request); err != nil {
		t.Fatalf("first prepareManagedGoCacheEnvironment() error = %v", err)
	}
	if got := request.managedGoCachePath; got != managedPath {
		t.Fatalf("first managedGoCachePath = %q, want %q", got, managedPath)
	}
	if err := mgr.prepareManagedGoCacheEnvironment(context.Background(), request); err != nil {
		t.Fatalf("second prepareManagedGoCacheEnvironment() error = %v, want fallback", err)
	}
	if got := request.managedGoCachePath; got != "" {
		t.Fatalf("second managedGoCachePath = %q, want reset decision", got)
	}
	if _, exists := request.Metadata[managedGoCacheMetadataKey]; exists {
		t.Fatalf("second metadata retained prior decision: %#v", request.Metadata)
	}
	if got := request.Env["GOCACHE"]; got != managedPath {
		t.Fatalf("request GOCACHE = %q, want preserved caller input", got)
	}
	if callerMetadata[managedGoCacheMetadataKey] != "/old/managed/cache" {
		t.Fatalf("caller metadata was changed: %#v", callerMetadata)
	}
}

func TestManagedGoCacheDisabledAbsentAndRemote(t *testing.T) {
	root := t.TempDir()
	settings := storage.DefaultSettings()
	provider := gocache.New(gocache.Config{
		HomeDir:  filepath.Join(root, "home"),
		TrashDir: filepath.Join(root, "trash"),
		Settings: managedGoCacheSettingsStub{settings: settings},
	})
	var calls atomic.Int32
	disabledProvider := managedGoCacheProviderFunc(func(ctx context.Context) (map[string]string, error) {
		calls.Add(1)
		return provider.ExecutionEnvironment(ctx)
	})
	counted := managedGoCacheProviderFunc(func(context.Context) (map[string]string, error) {
		calls.Add(1)
		return map[string]string{"GOCACHE": "/managed/cache"}, nil
	})
	tests := []struct {
		name     string
		executor string
		provider ManagedGoCacheEnvironmentProvider
		wantCall int32
	}{
		{name: "disabled setting", executor: "local", provider: disabledProvider, wantCall: 1},
		{name: "absent provider", executor: "local", wantCall: 0},
		{name: "remote executor", executor: "local_docker", provider: counted, wantCall: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			callsBefore := calls.Load()
			mgr := newTestManager(t)
			if test.provider != nil {
				mgr.SetManagedGoCacheEnvironmentProvider(test.provider)
			}
			request := &LaunchRequest{
				ExecutorType:       test.executor,
				Env:                map[string]string{"GOCACHE": "/independent/cache"},
				Metadata:           map[string]interface{}{managedGoCacheMetadataKey: "/stale/managed/cache"},
				managedGoCachePath: "/stale/managed/cache",
			}
			if err := mgr.prepareManagedGoCacheEnvironment(context.Background(), request); err != nil {
				t.Fatalf("prepareManagedGoCacheEnvironment() error = %v", err)
			}
			if request.managedGoCachePath != "" {
				t.Fatalf("managedGoCachePath = %q, want no managed override", request.managedGoCachePath)
			}
			if _, exists := request.Metadata[managedGoCacheMetadataKey]; exists {
				t.Fatalf("metadata retained host cache path: %#v", request.Metadata)
			}
			if got := request.Env["GOCACHE"]; got != "/independent/cache" {
				t.Fatalf("request GOCACHE = %q, want independent value", got)
			}
			if got := calls.Load() - callsBefore; got != test.wantCall {
				t.Fatalf("provider calls = %d, want %d", got, test.wantCall)
			}
		})
	}
}

func TestManagedGoCacheCancellationPreserved(t *testing.T) {
	t.Run("caller already canceled", func(t *testing.T) {
		var calls atomic.Int32
		mgr := newTestManager(t)
		mgr.SetManagedGoCacheEnvironmentProvider(managedGoCacheProviderFunc(func(context.Context) (map[string]string, error) {
			calls.Add(1)
			return nil, errors.New("unexpected provider call")
		}))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := mgr.prepareManagedGoCacheEnvironment(ctx, &LaunchRequest{ExecutorType: "local"})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("prepare error = %v, want context cancellation", err)
		}
		if calls.Load() != 0 {
			t.Fatalf("provider calls = %d, want 0", calls.Load())
		}
	})
	t.Run("caller deadline already expired", func(t *testing.T) {
		var calls atomic.Int32
		mgr := newTestManager(t)
		mgr.SetManagedGoCacheEnvironmentProvider(managedGoCacheProviderFunc(func(context.Context) (map[string]string, error) {
			calls.Add(1)
			return nil, errors.New("unexpected provider call")
		}))
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()
		err := mgr.prepareManagedGoCacheEnvironment(ctx, &LaunchRequest{ExecutorType: "local"})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("prepare error = %v, want expired deadline", err)
		}
		if calls.Load() != 0 {
			t.Fatalf("provider calls = %d, want 0", calls.Load())
		}
	})

	for _, providerErr := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(providerErr.Error(), func(t *testing.T) {
			mgr := newTestManager(t)
			wrapped := fmt.Errorf("provider operation: %w", providerErr)
			mgr.SetManagedGoCacheEnvironmentProvider(managedGoCacheProviderFunc(func(context.Context) (map[string]string, error) {
				return nil, wrapped
			}))
			err := mgr.prepareManagedGoCacheEnvironment(context.Background(), &LaunchRequest{ExecutorType: "local"})
			if !errors.Is(err, providerErr) {
				t.Fatalf("prepare error = %v, want wrapped %v", err, providerErr)
			}
		})
	}

	for _, result := range []map[string]string{nil, {"GOCACHE": "/managed/cache"}} {
		t.Run(fmt.Sprintf("canceled with result %v", result != nil), func(t *testing.T) {
			mgr := newTestManager(t)
			entered := make(chan struct{})
			release := make(chan struct{})
			mgr.SetManagedGoCacheEnvironmentProvider(managedGoCacheProviderFunc(func(context.Context) (map[string]string, error) {
				close(entered)
				<-release
				return result, nil
			}))
			ctx, cancel := context.WithCancel(context.Background())
			resultErr := make(chan error, 1)
			go func() {
				resultErr <- mgr.prepareManagedGoCacheEnvironment(ctx, &LaunchRequest{ExecutorType: "local"})
			}()
			<-entered
			cancel()
			close(release)
			if err := <-resultErr; !errors.Is(err, context.Canceled) {
				t.Fatalf("prepare error = %v, want cancellation", err)
			}
		})
	}
}

func TestManagedGoCacheCancellationPreventsLaunch(t *testing.T) {
	mgr, backend := newEnvironmentExecutionTestManager(t, nil)
	mgr.dataDir = t.TempDir()
	entered := make(chan struct{})
	release := make(chan struct{})
	mgr.SetManagedGoCacheEnvironmentProvider(managedGoCacheProviderFunc(func(context.Context) (map[string]string, error) {
		close(entered)
		<-release
		return map[string]string{"GOCACHE": "/managed/cache"}, nil
	}))
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := mgr.Launch(ctx, &LaunchRequest{
			TaskID: "task-1", AgentProfileID: "profile-1", IsEphemeral: true,
		})
		result <- err
	}()
	<-entered
	cancel()
	close(release)
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("Launch() error = %v, want context cancellation", err)
	}
	if got := backend.createCount.Load(); got != 0 {
		t.Fatalf("CreateInstance count = %d, want 0", got)
	}
}

func TestManagedGoCachePromotionKeepsExecutionDecision(t *testing.T) {
	var calls atomic.Int32
	provider := managedGoCacheProviderFunc(func(context.Context) (map[string]string, error) {
		calls.Add(1)
		return map[string]string{"GOCACHE": "/new/managed/cache"}, nil
	})
	mgr, _ := newEnvironmentExecutionTestManager(t, nil)
	mgr.dataDir = t.TempDir()
	mgr.SetManagedGoCacheEnvironmentProvider(provider)

	path := t.TempDir()
	execution := &AgentExecution{
		ID:            "exec-existing",
		TaskID:        "task-1",
		SessionID:     "session-promote",
		RuntimeName:   executor.NameStandalone,
		ExecutorType:  "local",
		WorkspacePath: path,
		IsPassthrough: true,
		metadata:      map[string]interface{}{managedGoCacheMetadataKey: "/established/cache"},
		agentctl:      newReadyAgentctlClient(t, mgr.logger),
	}
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add workspace execution: %v", err)
	}

	_, err := mgr.Launch(context.Background(), &LaunchRequest{
		TaskID: "task-1", SessionID: execution.SessionID, AgentProfileID: "profile-1",
		ExecutorType: "local", WorkspacePath: path, IsPassthrough: true,
	})
	if err != nil {
		t.Fatalf("Launch() promotion error = %v", err)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("cache provider calls = %d, want 0 during promotion", got)
	}
	if got := execution.metadataString(managedGoCacheMetadataKey); got != "/established/cache" {
		t.Fatalf("promoted execution cache decision = %q, want established path", got)
	}
}

func TestManagedGoCacheCoalescedLaunchUsesOneDecision(t *testing.T) {
	mgr, backend := newEnvironmentExecutionTestManager(t, nil)
	mgr.dataDir = t.TempDir()
	var calls atomic.Int32
	mgr.SetManagedGoCacheEnvironmentProvider(managedGoCacheProviderFunc(func(context.Context) (map[string]string, error) {
		calls.Add(1)
		return map[string]string{"GOCACHE": "/managed/cache"}, nil
	}))
	backend.entered = make(chan struct{}, 1)
	backend.barrier = make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(backend.barrier)
		}
	}()

	type launchResult struct {
		execution *AgentExecution
		err       error
	}
	first := make(chan launchResult, 1)
	go func() {
		execution, err := mgr.Launch(context.Background(), managedCacheLaunchRequest("session-coalesced"))
		first <- launchResult{execution: execution, err: err}
	}()
	select {
	case <-backend.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("first launch did not reach CreateInstance")
	}

	followerCtx := &doneObservedContext{Context: context.Background(), doneRead: make(chan struct{})}
	second := make(chan launchResult, 1)
	go func() {
		execution, err := mgr.Launch(followerCtx, managedCacheLaunchRequest("session-coalesced"))
		second <- launchResult{execution: execution, err: err}
	}()
	select {
	case <-followerCtx.doneRead:
	case <-time.After(2 * time.Second):
		t.Fatal("second launch did not join the coalesced execution")
	}
	close(backend.barrier)
	released = true

	firstResult := <-first
	secondResult := <-second
	if firstResult.err != nil || secondResult.err != nil {
		t.Fatalf("coalesced launch errors = %v / %v", firstResult.err, secondResult.err)
	}
	if firstResult.execution.ID != secondResult.execution.ID {
		t.Fatalf("execution IDs = %q / %q, want shared execution", firstResult.execution.ID, secondResult.execution.ID)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("cache provider calls = %d, want one execution decision", got)
	}
	if got := backend.createCount.Load(); got != 1 {
		t.Fatalf("CreateInstance count = %d, want one", got)
	}
}

func TestManagedGoCacheRecoveryClearsStaleMetadata(t *testing.T) {
	mgr := newTestManager(t)
	mgr.SetManagedGoCacheEnvironmentProvider(managedGoCacheProviderFunc(func(context.Context) (map[string]string, error) {
		return nil, errors.New("cache unavailable")
	}))
	originalMetadata := map[string]interface{}{
		managedGoCacheMetadataKey: "/old/managed/cache",
		"other":                   "retained",
	}
	prepared, err := mgr.prepareExecutionCreateRequest(context.Background(), "task-1", &WorkspaceInfo{
		TaskID:        "task-1",
		SessionID:     "session-recovery",
		WorkspacePath: t.TempDir(),
		AgentID:       "auggie",
		ExecutorType:  "local",
		Metadata:      originalMetadata,
	}, "execution-recovery")
	if err != nil {
		t.Fatalf("prepareExecutionCreateRequest() error = %v", err)
	}
	if _, exists := prepared.request.Metadata[managedGoCacheMetadataKey]; exists {
		t.Fatalf("recovered execution retained stale cache metadata: %#v", prepared.request.Metadata)
	}
	if prepared.request.Metadata["other"] != "retained" {
		t.Fatalf("recovered unrelated metadata = %#v", prepared.request.Metadata)
	}
	if got := processEnvironment(&AgentExecution{metadata: prepared.request.Metadata}, map[string]string{"GOCACHE": "/independent/cache"})["GOCACHE"]; got != "/independent/cache" {
		t.Fatalf("recovered process GOCACHE = %q, want independent value", got)
	}
	if originalMetadata[managedGoCacheMetadataKey] != "/old/managed/cache" {
		t.Fatalf("original recovery metadata was changed: %#v", originalMetadata)
	}
}

func managedCacheLaunchRequest(sessionID string) *LaunchRequest {
	return &LaunchRequest{
		TaskID: "task-1", SessionID: sessionID, AgentProfileID: "profile-1", IsEphemeral: true,
	}
}

func managedGoCacheProviderFromSettings(
	t *testing.T,
	settings storage.StorageMaintenanceSettings,
	settingsErr error,
) ManagedGoCacheEnvironmentProvider {
	t.Helper()
	root := t.TempDir()
	return gocache.New(gocache.Config{
		HomeDir:  filepath.Join(root, "home"),
		TrashDir: filepath.Join(root, "trash"),
		Settings: managedGoCacheSettingsStub{settings: settings, err: settingsErr},
	})
}

func managedGoCacheFilesystemFailureProvider(t *testing.T) ManagedGoCacheEnvironmentProvider {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("create cache home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "cache"), []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("create cache parent blocker: %v", err)
	}
	settings := storage.DefaultSettings()
	settings.GoCache.Enabled = true
	return gocache.New(gocache.Config{
		HomeDir:  home,
		TrashDir: filepath.Join(root, "trash"),
		Settings: managedGoCacheSettingsStub{settings: settings},
	})
}
