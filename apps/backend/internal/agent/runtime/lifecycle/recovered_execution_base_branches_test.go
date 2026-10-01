package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.7, AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.9
func TestPrepareExecutionCreateRequest_HydratesBaseBranches(t *testing.T) {
	want := map[string]string{"alpha": "origin/develop", "beta": "origin/develop", "gamma": "origin/develop"}
	cases := []struct {
		name        string
		metadata    map[string]interface{}
		provided    map[string]string
		providerErr error
		noProvider  bool
		noTask      bool
		want        map[string]string
		calls       int
	}{
		{name: "missing metadata", provided: want, want: want, calls: 1},
		{name: "nil map", metadata: map[string]interface{}{MetadataKeyBaseBranches: nil}, provided: want, want: want, calls: 1},
		{name: "empty map", metadata: map[string]interface{}{MetadataKeyBaseBranches: map[string]string{}}, provided: want, want: want, calls: 1},
		{name: "unusable map", metadata: map[string]interface{}{MetadataKeyBaseBranches: map[string]interface{}{"alpha": 42}}, provided: want, want: want, calls: 1},
		{name: "typed metadata", metadata: map[string]interface{}{MetadataKeyBaseBranches: want}, want: want},
		{name: "JSON metadata", metadata: map[string]interface{}{MetadataKeyBaseBranches: map[string]interface{}{"alpha": "origin/develop"}}, want: map[string]string{"alpha": "origin/develop"}},
		{name: "root key", provided: map[string]string{"": "origin/develop"}, want: map[string]string{"": "origin/develop"}, calls: 1},
		{name: "empty provider", calls: 1},
		{name: "provider error", providerErr: errors.New("database unavailable"), calls: 1},
		{name: "no provider", noProvider: true},
		{name: "no task", provided: want, noTask: true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newTestManager(t)
			tt.provided = maps.Clone(tt.provided)
			tt.metadata = maps.Clone(tt.metadata)
			if raw, ok := tt.metadata[MetadataKeyBaseBranches].(map[string]string); ok {
				tt.metadata[MetadataKeyBaseBranches] = maps.Clone(raw)
			}
			originalBefore, err := json.Marshal(tt.metadata)
			if err != nil {
				t.Fatal(err)
			}
			providedBefore := maps.Clone(tt.provided)
			calls := 0
			if !tt.noProvider {
				mgr.SetBaseBranchProvider(func(_ context.Context, taskID string) (map[string]string, error) {
					calls++
					if taskID != "task-1" {
						t.Errorf("provider task=%q", taskID)
					}
					return tt.provided, tt.providerErr
				})
			}
			info := &WorkspaceInfo{TaskID: "task-1", SessionID: "session-1", AgentID: "auggie", WorkspacePath: t.TempDir(), ExecutorType: string(models.ExecutorTypeWorktree), Metadata: tt.metadata}
			taskID := "task-1"
			if tt.noTask {
				taskID = ""
			}
			prep, err := mgr.prepareExecutionCreateRequest(context.Background(), taskID, info, "execution-1")
			if err != nil {
				t.Fatal(err)
			}
			got := getMetadataStringMap(prep.request.Metadata, MetadataKeyBaseBranches)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("creation bases=%v, want %v", got, tt.want)
			}
			if calls != tt.calls {
				t.Errorf("provider calls=%d, want %d", calls, tt.calls)
			}
			assertCreationBaseMapOwned(t, prep.request.Metadata, info.Metadata, originalBefore, tt.provided, providedBefore)

		})
	}
}

func assertCreationBaseMapOwned(t *testing.T, request, original map[string]interface{}, originalBefore []byte, provided, providedBefore map[string]string) {
	t.Helper()
	got := getMetadataStringMap(request, MetadataKeyBaseBranches)
	if len(got) > 0 {
		got["request-only"] = "feature/private"
	}
	after, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(originalBefore) {
		t.Error("creation or request mutation changed persisted metadata")
	}
	if !reflect.DeepEqual(provided, providedBefore) {
		t.Error("request mutation changed provider map")
	}
}

// @covers AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.1, AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.7
func TestGetOrEnsureExecution_SeedsBaseBranchesAtCreation(t *testing.T) {
	mgr, backend := newTerminalSessionManager(t, models.TaskSessionStateCompleted)
	want := map[string]string{"alpha": "origin/develop", "beta": "origin/develop"}
	mgr.SetBaseBranchProvider(func(context.Context, string) (map[string]string, error) { return want, nil })
	_, err := mgr.GetOrEnsureExecution(context.Background(), terminalSessionID)
	if err != nil {
		t.Fatal(err)
	}
	got := buildReconnectCreateInstanceRequest(backend.lastRequest, "recovered-instance").BaseBranches
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("agentctl creation bases=%v, want %v", got, want)
	}
}

// @covers AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.9
func TestWaitForAgentctlReady_SeedsBeforeReady(t *testing.T) {
	mgr := newTestManager(t)
	execution := &AgentExecution{ID: "execution-1", TaskID: "task-1", SessionID: "session-1", WorkspacePath: t.TempDir()}
	var readyEvents atomic.Int32
	mgr.eventBus.(*MockEventBus).OnPublish = func(subject string, _ *bus.Event) {
		if subject == events.AgentctlReady {
			readyEvents.Add(1)
		}
	}
	entered := make(chan string, 2)
	release := make(chan struct{})
	polled := make(chan bool, 1)
	done := make(chan struct{})
	var releaseOnce sync.Once
	execution.agentctl = processTestClient(t, blockingComparisonSeedHandler(entered, release, polled))
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("startup did not finish")
		}
	})
	// Cache focus before registration, so only the readiness flush can send it.
	mgr.HandleSessionMode(execution.SessionID, WorkspacePollModeFast)
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}
	mgr.SetBaseBranchProvider(func(context.Context, string) (map[string]string, error) {
		return map[string]string{"": "origin/develop"}, nil
	})
	mgr.SetComparisonTargetProvider(func(context.Context, string) (map[string]models.ComparisonTarget, error) { return nil, nil })
	go func() { defer close(done); mgr.waitForAgentctlReady(execution) }()
	for _, path := range []string{"/api/v1/workspace/base-branches", "/api/v1/workspace/comparison-targets"} {
		select {
		case got := <-entered:
			if got != path {
				t.Errorf("push path=%s, want %s", got, path)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("configuration push did not begin")
		}
		if execution.IsAgentctlReady() {
			t.Errorf("readiness exposed while %s is blocked", path)
		}
		if readyEvents.Load() != 0 {
			t.Error("ready event exposed before configuration completed")
		}
		// Release this request alone; the next push remains blocked.
		release <- struct{}{}
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("startup did not finish")
	}
	if !execution.IsAgentctlReady() || readyEvents.Load() != 1 {
		t.Error("successful seeding did not publish readiness")
	}
	select {
	case afterSeeding := <-polled:
		if !afterSeeding {
			t.Error("cached poll mode flushed before seeding")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("cached poll mode not flushed")
	}
}

func blockingComparisonSeedHandler(entered chan<- string, release <-chan struct{}, polled chan<- bool) http.HandlerFunc {
	var seeded atomic.Int32
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/workspace/base-branches", "/api/v1/workspace/comparison-targets":
			entered <- r.URL.Path
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			seeded.Add(1)
			w.WriteHeader(http.StatusOK)
		case "/api/v1/workspace/poll-mode":
			polled <- seeded.Load() == 2
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}
}

func TestWaitForAgentctlReady_SeedingFailuresNonFatal(t *testing.T) {
	for _, failure := range []string{"provider", "base push", "comparison push"} {
		t.Run(failure, func(t *testing.T) {
			mgr := newTestManager(t)
			var baseCalls, comparisonCalls atomic.Int32
			execution := &AgentExecution{ID: "execution-1", TaskID: "task-1", SessionID: "session-1"}
			execution.agentctl = processTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				code := http.StatusOK
				switch r.URL.Path {
				case "/health":
				case "/api/v1/workspace/base-branches":
					baseCalls.Add(1)
					if failure == "base push" {
						code = http.StatusInternalServerError
					}
				case "/api/v1/workspace/comparison-targets":
					comparisonCalls.Add(1)
					if failure == "comparison push" {
						code = http.StatusInternalServerError
					}
				default:
					code = http.StatusNotFound
				}
				w.WriteHeader(code)
			}))
			mgr.SetBaseBranchProvider(func(context.Context, string) (map[string]string, error) {
				if failure == "provider" {
					return nil, errors.New("read failed")
				}
				return map[string]string{"": "origin/develop"}, nil
			})
			mgr.SetComparisonTargetProvider(func(context.Context, string) (map[string]models.ComparisonTarget, error) { return nil, nil })
			mgr.waitForAgentctlReady(execution)
			if !execution.IsAgentctlReady() {
				t.Error("best-effort seeding failure blocked readiness")
			}
			wantBaseCalls := 1
			if failure == "provider" {
				wantBaseCalls = 0
			}
			if baseCalls.Load() != int32(wantBaseCalls) || comparisonCalls.Load() != 1 {
				t.Errorf("base calls=%d, comparison calls=%d", baseCalls.Load(), comparisonCalls.Load())
			}
			assertAgentctlLeaseReleased(t, execution)
		})
	}
}

func TestWaitForAgentctlReady_HealthFailureDoesNotSeed(t *testing.T) {
	mgr := newTestManager(t)
	providerCalls := 0
	execution := &AgentExecution{ID: "execution-1", TaskID: "task-1", SessionID: "session-1"}
	execution.agentctl = processTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Shutdown ends the health retry deterministically, without waiting its timeout.
		mgr.closeStopCh()
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	mgr.SetBaseBranchProvider(func(context.Context, string) (map[string]string, error) {
		providerCalls++
		return map[string]string{"": "origin/develop"}, nil
	})
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}
	mgr.waitForAgentctlReady(execution)
	if execution.IsAgentctlReady() || providerCalls != 0 {
		t.Error("failed health check exposed readiness or seeded configuration")
	}
	assertAgentctlLeaseReleased(t, execution)
}

func assertAgentctlLeaseReleased(t *testing.T, execution *AgentExecution) {
	t.Helper()
	if !execution.agentctlLifecycleMu.TryLock() {
		t.Fatal("startup retained the agentctl client lease")
	}
	execution.agentctlLifecycleMu.Unlock()
}

// @covers AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.11
func TestPrepareExecutionCreateRequest_BaseBranchLookupDeadline(t *testing.T) {
	for _, tt := range []struct {
		name          string
		callerTimeout time.Duration
		cancelled     bool
		wantWait      time.Duration
	}{
		{name: "slow lookup", callerTimeout: time.Minute, wantWait: 5 * time.Second},
		{name: "earlier caller deadline", callerTimeout: time.Second, wantWait: time.Second},
		{name: "cancelled caller", callerTimeout: time.Minute, cancelled: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				mgr := newTestManager(t)
				ctx, cancel := context.WithTimeout(context.Background(), tt.callerTimeout)
				defer cancel()
				if tt.cancelled {
					cancel()
				}
				mgr.SetBaseBranchProvider(func(ctx context.Context, _ string) (map[string]string, error) {
					<-ctx.Done()
					return nil, ctx.Err()
				})
				info := &WorkspaceInfo{TaskID: "task-1", SessionID: "session-1", AgentID: "auggie", WorkspacePath: t.TempDir(), ExecutorType: string(models.ExecutorTypeWorktree)}
				started := time.Now()
				prep, err := mgr.prepareExecutionCreateRequest(ctx, info.TaskID, info, "execution-1")
				if err != nil {
					t.Fatalf("best-effort lookup failed request preparation: %v", err)
				}
				if waited := time.Since(started); waited != tt.wantWait {
					t.Errorf("lookup waited %s, want %s", waited, tt.wantWait)
				}
				if bases := getMetadataStringMap(prep.request.Metadata, MetadataKeyBaseBranches); len(bases) != 0 {
					t.Errorf("failed lookup seeded bases: %v", bases)
				}
			})
		})
	}
}
