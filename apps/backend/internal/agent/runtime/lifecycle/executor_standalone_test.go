package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

// standaloneControlServer is an in-process stand-in for the host agentctl
// control API the standalone executor talks to.
type standaloneControlServer struct {
	mu             sync.Mutex
	healthy        bool
	healthCalls    int
	createRequests []agentctlclient.CreateInstanceRequest
	deleted        []string
	createStatus   int
	server         *httptest.Server
	// listInstances is what GET /api/v1/instances returns; nil (vs. an empty
	// non-nil slice) tells the fake to answer with a listing failure instead,
	// for exercising AC-EXECUTORS-SURVIVAL-002.12.
	listInstances    []*agentctlclient.InstanceInfo
	listInstancesErr bool
	// listInstancesFailures is the number of remaining GET
	// /api/v1/instances attempts to fail (500) before it starts answering
	// with listInstances -- lets a test pin the exact bounded-retry attempt
	// count (AC-EXECUTORS-SURVIVAL-002.13) for the enumeration read itself.
	listInstancesFailures int
	listInstancesAttempts int
	// deleteFailures, keyed by instance ID, is the number of remaining DELETE
	// attempts to fail (500) for that instance before it starts succeeding --
	// lets a test pin the exact bounded-retry attempt count
	// (AC-EXECUTORS-SURVIVAL-002.15). A count so high it never reaches zero
	// simulates a stop that never succeeds.
	deleteFailures map[string]int
	deleteAttempts map[string]int
	// deleteDelay, keyed by instance ID, makes that instance's DELETE
	// response take at least this long -- used to prove a recovery deadline
	// (AC-EXECUTORS-SURVIVAL-003.7) can elapse while a stop is genuinely
	// still in flight, without the fixture holding s.mu for the whole sleep
	// (which would otherwise serialize every other concurrent stop request).
	deleteDelay map[string]time.Duration

	// turnOutcomes, keyed by instance ID, is what GET
	// .../turn-outcome answers with; an absent key answers "nothing
	// retained" (retained: false), matching AC-EXECUTORS-SURVIVAL-004.5's
	// no-outcome case.
	turnOutcomes map[string]agentctlclient.TurnOutcome
	// turnOutcomeFailures, keyed by instance ID, is the number of remaining
	// GET .../turn-outcome attempts to fail (500) before it starts
	// answering normally -- pins the bounded-retry attempt count
	// (AC-EXECUTORS-SURVIVAL-004.5/002.13).
	turnOutcomeFailures map[string]int
	turnOutcomeAttempts map[string]int
	// ackedTurnOutcomes records every accepted POST .../turn-outcome/ack
	// call, in order.
	ackedTurnOutcomes []ackedTurnOutcome
}

type ackedTurnOutcome struct {
	instanceID string
	turnID     int64
}

func newStandaloneControlServer(t *testing.T, healthy bool) *standaloneControlServer {
	t.Helper()
	s := &standaloneControlServer{
		healthy:             healthy,
		createStatus:        http.StatusOK,
		deleteFailures:      make(map[string]int),
		deleteAttempts:      make(map[string]int),
		deleteDelay:         make(map[string]time.Duration),
		turnOutcomes:        make(map[string]agentctlclient.TurnOutcome),
		turnOutcomeFailures: make(map[string]int),
		turnOutcomeAttempts: make(map[string]int),
	}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		switch {
		case r.URL.Path == "/health":
			s.healthCalls++
			if !s.healthy {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/api/v1/instances" && r.Method == http.MethodPost:
			var req agentctlclient.CreateInstanceRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			s.createRequests = append(s.createRequests, req)
			if s.createStatus != http.StatusOK {
				w.WriteHeader(s.createStatus)
				_, _ = w.Write([]byte(`{"error":"cannot create"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(agentctlclient.CreateInstanceResponse{ID: "std-1", Port: 45678})
		case strings.HasPrefix(r.URL.Path, "/api/v1/instances/") && r.Method == http.MethodDelete:
			id := strings.TrimPrefix(r.URL.Path, "/api/v1/instances/")
			s.deleteAttempts[id]++
			delay := s.deleteDelay[id]
			fail := s.deleteFailures[id] > 0
			if fail {
				s.deleteFailures[id]--
			}
			if delay > 0 {
				s.mu.Unlock()
				time.Sleep(delay)
				s.mu.Lock()
			}
			if fail {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			s.deleted = append(s.deleted, id)
		case r.URL.Path == "/api/v1/instances" && r.Method == http.MethodGet:
			s.listInstancesAttempts++
			if s.listInstancesErr {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if s.listInstancesFailures > 0 {
				s.listInstancesFailures--
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(struct {
				Instances []*agentctlclient.InstanceInfo `json:"instances"`
			}{Instances: s.listInstances})
		case strings.HasPrefix(r.URL.Path, "/api/v1/instances/") && strings.HasSuffix(r.URL.Path, "/turn-outcome") &&
			r.Method == http.MethodGet:
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/instances/"), "/turn-outcome")
			s.turnOutcomeAttempts[id]++
			if s.turnOutcomeFailures[id] > 0 {
				s.turnOutcomeFailures[id]--
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			outcome, retained := s.turnOutcomes[id]
			_ = json.NewEncoder(w).Encode(struct {
				Retained bool               `json:"retained"`
				TurnID   int64              `json:"turn_id"`
				Event    streams.AgentEvent `json:"event"`
			}{Retained: retained, TurnID: outcome.TurnID, Event: outcome.Event})
		case strings.HasPrefix(r.URL.Path, "/api/v1/instances/") && strings.HasSuffix(r.URL.Path, "/turn-outcome/ack") &&
			r.Method == http.MethodPost:
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/instances/"), "/turn-outcome/ack")
			var body struct {
				TurnID int64 `json:"turn_id"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			s.ackedTurnOutcomes = append(s.ackedTurnOutcomes, ackedTurnOutcome{instanceID: id, turnID: body.TurnID})
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(s.server.Close)
	return s
}

func (s *standaloneControlServer) executor(t *testing.T) *StandaloneExecutor {
	t.Helper()
	parsed, err := url.Parse(s.server.URL)
	if err != nil {
		t.Fatalf("parse control URL: %v", err)
	}
	port, _ := strconv.Atoi(parsed.Port())
	ctl := agentctlclient.NewControlClient(parsed.Hostname(), port, newTestLogger())
	return NewStandaloneExecutor(ctl, parsed.Hostname(), port, newTestLogger())
}

func (s *standaloneControlServer) lastCreateRequest(t *testing.T) agentctlclient.CreateInstanceRequest {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.createRequests) == 0 {
		t.Fatal("no create-instance request recorded")
	}
	return s.createRequests[len(s.createRequests)-1]
}

func TestStandaloneExecutorStaticSurface(t *testing.T) {
	exec := newStandaloneControlServer(t, true).executor(t)
	if exec.Name() != executor.NameStandalone {
		t.Fatalf("Name() = %q", exec.Name())
	}
	if exec.RequiresCloneURL() {
		t.Fatal("standalone runs in an existing host workspace, so no clone URL is required")
	}
	if !exec.ShouldApplyPreferredShell() {
		t.Fatal("standalone runs on the host, so the user's preferred shell applies")
	}
	if exec.IsAlwaysResumable() {
		t.Fatal("standalone instances are transient and not always resumable")
	}
	instances, err := exec.RecoverInstances(context.Background(), nil)
	if err != nil || len(instances) != 0 {
		t.Fatalf("RecoverInstances() = %v, %v; want none recovered, no error", instances, err)
	}

	runner := &process.InteractiveRunner{}
	exec.SetInteractiveRunner(runner)
	if exec.GetInteractiveRunner() != runner {
		t.Fatal("interactive runner round-trip failed")
	}
}

func TestStandaloneExecutorHealthCheck(t *testing.T) {
	healthy := newStandaloneControlServer(t, true)
	if err := healthy.executor(t).HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}

	unhealthy := newStandaloneControlServer(t, false)
	if err := unhealthy.executor(t).HealthCheck(context.Background()); err == nil {
		t.Fatal("expected an unhealthy control server to fail the health check")
	}
}

func TestStandaloneExecutorCreateInstance(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	exec := control.executor(t)
	exec.SetAuthToken("launch-token")

	agent := &fakeRuntimeAgent{
		MockAgent:    agents.NewMockAgent(),
		id:           "opencode",
		requiresKill: true,
		stripEnv:     []string{"NODE_OPTIONS"},
	}
	req := &ExecutorCreateRequest{
		InstanceID:           "instance-1",
		TaskID:               "task-1",
		SessionID:            "session-1",
		WorkspacePath:        "/host/workspace",
		WorkspaceSourceRoots: []string{"/host/sources"},
		Protocol:             "acp",
		McpMode:              "task",
		AgentConfig:          agent,
		Env:                  map[string]string{"EXISTING": "value"},
		Metadata: map[string]interface{}{
			MetadataKeyWorktreeID:     "wt-1",
			MetadataKeyWorktreeBranch: "feature/task-1",
			MetadataKeyBaseBranches:   map[string]string{"": "main"},
		},
	}

	instance, err := exec.CreateInstance(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}

	if instance.StandaloneInstanceID != "std-1" || instance.StandalonePort != 45678 {
		t.Fatalf("instance = %+v", instance)
	}
	if instance.RuntimeName != executor.NameStandalone {
		t.Fatalf("RuntimeName = %q", instance.RuntimeName)
	}
	if instance.WorkspacePath != "/host/workspace" {
		t.Fatalf("WorkspacePath = %q, want the host path verbatim", instance.WorkspacePath)
	}
	if instance.Metadata["standalone_port"] != 45678 {
		t.Fatalf("standalone_port metadata = %v", instance.Metadata["standalone_port"])
	}
	if instance.Metadata["worktree_id"] != "wt-1" ||
		instance.Metadata["worktree_path"] != "/host/workspace" ||
		instance.Metadata["worktree_branch"] != "feature/task-1" {
		t.Fatalf("worktree metadata = %+v", instance.Metadata)
	}

	got := control.lastCreateRequest(t)
	if got.ID != "instance-1" || got.SessionID != "session-1" || got.TaskID != "task-1" {
		t.Fatalf("create request identity = %+v", got)
	}
	if got.AgentCommand != "" {
		t.Fatalf("AgentCommand = %q, want empty (the agent starts via a later Configure call)", got.AgentCommand)
	}
	if got.AutoStart {
		t.Fatal("standalone instances must not auto-start the agent")
	}
	if got.AgentType != "opencode" || !got.RequiresProcessKill {
		t.Fatalf("agent runtime fields = %+v", got)
	}
	if !equalStrings(got.StripEnv, []string{"NODE_OPTIONS"}) {
		t.Fatalf("StripEnv = %v", got.StripEnv)
	}
	if got.BaseBranches[""] != "main" {
		t.Fatalf("BaseBranches = %+v", got.BaseBranches)
	}
	if !equalStrings(got.WorkspaceSourceRoots, []string{"/host/sources"}) {
		t.Fatalf("WorkspaceSourceRoots = %v", got.WorkspaceSourceRoots)
	}
	// The executor injects the task/session identity into the child env.
	if got.Env["KANDEV_TASK_ID"] != "task-1" || got.Env["KANDEV_SESSION_ID"] != "session-1" {
		t.Fatalf("Env = %+v, want the task and session IDs injected", got.Env)
	}
	if got.Env["EXISTING"] != "value" {
		t.Fatalf("Env = %+v, want the caller's env preserved", got.Env)
	}
}

func TestStandaloneExecutorCreateInstanceWithoutWorktreeMetadata(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	instance, err := control.executor(t).CreateInstance(context.Background(), &ExecutorCreateRequest{
		InstanceID:    "instance-1",
		TaskID:        "task-1",
		SessionID:     "session-1",
		WorkspacePath: "/host/workspace",
	})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	for _, key := range []string{"worktree_id", "worktree_path", "worktree_branch"} {
		if _, present := instance.Metadata[key]; present {
			t.Fatalf("metadata %q must be absent without a worktree: %+v", key, instance.Metadata)
		}
	}
	if got := control.lastCreateRequest(t); got.AgentType != "" {
		t.Fatalf("AgentType = %q, want empty without an agent config", got.AgentType)
	}
}

func TestStandaloneExecutorCreateInstanceSurfacesControlFailure(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.createStatus = http.StatusInternalServerError
	_, err := control.executor(t).CreateInstance(context.Background(), &ExecutorCreateRequest{
		InstanceID: "instance-1",
	})
	if err == nil || !strings.Contains(err.Error(), "failed to create standalone instance") {
		t.Fatalf("error = %v", err)
	}
}

func TestStandaloneExecutorCreateInstanceFailsWhenAgentctlNeverBecomesReady(t *testing.T) {
	control := newStandaloneControlServer(t, false)
	// waitForReady polls every 500ms; give the deadline a wide enough margin
	// that a loaded CI runner still observes several retries.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := control.executor(t).CreateInstance(ctx, &ExecutorCreateRequest{InstanceID: "instance-1"})
	if err == nil || !strings.Contains(err.Error(), "agentctl not ready") {
		t.Fatalf("error = %v, want the readiness wait to fail", err)
	}
	control.mu.Lock()
	calls := control.healthCalls
	control.mu.Unlock()
	if calls < 2 {
		t.Fatalf("health calls = %d, want the wait loop to retry", calls)
	}
}

func TestStandaloneExecutorStopInstance(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	exec := control.executor(t)

	t.Run("no standalone instance is a no-op", func(t *testing.T) {
		if err := exec.StopInstance(context.Background(), &ExecutorInstance{}, false); err != nil {
			t.Fatalf("StopInstance: %v", err)
		}
		control.mu.Lock()
		defer control.mu.Unlock()
		if len(control.deleted) != 0 {
			t.Fatalf("deleted = %v, want none", control.deleted)
		}
	})

	t.Run("deletes the tracked instance", func(t *testing.T) {
		err := exec.StopInstance(context.Background(),
			&ExecutorInstance{StandaloneInstanceID: "std-1"}, false)
		if err != nil {
			t.Fatalf("StopInstance: %v", err)
		}
		control.mu.Lock()
		defer control.mu.Unlock()
		if len(control.deleted) != 1 || control.deleted[0] != "std-1" {
			t.Fatalf("deleted = %v", control.deleted)
		}
	})
}

// TestStandaloneExecutorRecoverInstancesReTracksWinnerAndStopsOrphan pins
// AC-EXECUTORS-SURVIVAL-002.1/002.2/002.6: a live instance whose session
// matches a recovery-inventory record is re-tracked and returned; a live
// instance with no matching record is stopped as an orphan.
func TestStandaloneExecutorRecoverInstancesReTracksWinnerAndStopsOrphan(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.listInstances = []*agentctlclient.InstanceInfo{
		{
			ID: "instance-1", Port: 5001, SessionID: "session-1", TaskID: "task-1", WorkspacePath: "/ws/1",
			Env:                  map[string]string{"KANDEV_RUN_ID": "run-42", "PATH": "/usr/bin"},
			WorkspaceSourceRoots: []string{"/ws/1", "/ws/1-sibling"},
			ProviderSessionID:    "provider-session-9",
		},
		{ID: "orphan-instance", Port: 5002, SessionID: "session-orphan"},
	}
	exec := control.executor(t)
	exec.SetAuthToken("survival-token")

	records := []*models.ExecutorRunning{
		{SessionID: "session-1", TaskID: "task-1", AgentExecutionID: "instance-1", WorktreePath: "/ws/1-from-record", Metadata: map[string]interface{}{"k": "v"}},
	}

	recovered, err := exec.RecoverInstances(context.Background(), records)
	if err != nil {
		t.Fatalf("RecoverInstances: %v", err)
	}
	if len(recovered) != 1 {
		t.Fatalf("recovered = %+v, want exactly one winner", recovered)
	}
	got := recovered[0]
	if got.SessionID != "session-1" || got.TaskID != "task-1" || got.InstanceID != "instance-1" {
		t.Fatalf("recovered instance identity = %+v", got)
	}
	// AC-EXECUTORS-SURVIVAL-002.14 (Review round 3, finding 3): workspace
	// path's declared source is the recovery-inventory record, never the
	// adopted instance's own report (which the fixture deliberately sets to
	// a different value, "/ws/1", to prove it is ignored).
	if got.WorkspacePath != "/ws/1-from-record" {
		t.Fatalf("WorkspacePath = %q, want the record's WorktreePath, not the instance's self-reported path", got.WorkspacePath)
	}
	if got.Metadata["k"] != "v" {
		t.Fatalf("Metadata = %+v, want the record's persisted metadata", got.Metadata)
	}
	if got.Env["KANDEV_RUN_ID"] != "run-42" || got.Env["PATH"] != "/usr/bin" {
		t.Fatalf("Env = %+v, want the adopted instance's own runtime environment read back (AC-EXECUTORS-SURVIVAL-002.14)", got.Env)
	}
	if len(got.WorkspaceSourceRoots) != 2 || got.WorkspaceSourceRoots[0] != "/ws/1" || got.WorkspaceSourceRoots[1] != "/ws/1-sibling" {
		t.Fatalf("WorkspaceSourceRoots = %+v, want the adopted instance's own live allowlist read back (AC-EXECUTORS-SURVIVAL-002.14)", got.WorkspaceSourceRoots)
	}
	if got.ProviderSessionID != "provider-session-9" {
		t.Fatalf("ProviderSessionID = %q, want the adopted instance's own live provider session read back (AC-EXECUTORS-SURVIVAL-002.14)", got.ProviderSessionID)
	}
	if got.Client == nil {
		t.Fatal("recovered instance must carry a usable agentctl client")
	}

	control.mu.Lock()
	defer control.mu.Unlock()
	if len(control.deleted) != 1 || control.deleted[0] != "orphan-instance" {
		t.Fatalf("deleted = %v, want the orphan instance stopped", control.deleted)
	}
}

// TestStandaloneExecutorRecoverInstancesStopsDuplicateLoser pins
// AC-EXECUTORS-SURVIVAL-002.10: of two live instances for one session, the
// one matching the record's agent execution identifier is re-tracked and the
// other is stopped.
func TestStandaloneExecutorRecoverInstancesStopsDuplicateLoser(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.listInstances = []*agentctlclient.InstanceInfo{
		{ID: "winner", Port: 5001, SessionID: "session-1"},
		{ID: "loser", Port: 5002, SessionID: "session-1"},
	}
	exec := control.executor(t)

	records := []*models.ExecutorRunning{{SessionID: "session-1", AgentExecutionID: "winner"}}

	recovered, err := exec.RecoverInstances(context.Background(), records)
	if err != nil {
		t.Fatalf("RecoverInstances: %v", err)
	}
	if len(recovered) != 1 || recovered[0].InstanceID != "winner" {
		t.Fatalf("recovered = %+v, want only the winner", recovered)
	}

	control.mu.Lock()
	defer control.mu.Unlock()
	if len(control.deleted) != 1 || control.deleted[0] != "loser" {
		t.Fatalf("deleted = %v, want the loser stopped", control.deleted)
	}
}

// TestStandaloneExecutorRecoverInstancesRecordWithNoInstanceIsUntouched pins
// AC-EXECUTORS-SURVIVAL-002.7: a record with no live instance is left alone,
// not stopped and not reported recovered.
func TestStandaloneExecutorRecoverInstancesRecordWithNoInstanceIsUntouched(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.listInstances = nil
	exec := control.executor(t)

	records := []*models.ExecutorRunning{{SessionID: "session-cold", AgentExecutionID: "instance-cold"}}

	recovered, err := exec.RecoverInstances(context.Background(), records)
	if err != nil {
		t.Fatalf("RecoverInstances: %v", err)
	}
	if len(recovered) != 0 {
		t.Fatalf("recovered = %+v, want none", recovered)
	}
	control.mu.Lock()
	defer control.mu.Unlock()
	if len(control.deleted) != 0 {
		t.Fatalf("deleted = %v, want none: a record with no instance is left to the existing repair path", control.deleted)
	}
}

// TestStandaloneExecutorRecoverInstancesEnumerationFailureRecoversNothing
// pins AC-EXECUTORS-SURVIVAL-002.12: when the adopted server cannot be
// enumerated, recovery reports nothing recovered, stops nothing, and does
// not error out (a hard error here would fail backend startup outright).
func TestStandaloneExecutorRecoverInstancesEnumerationFailureRecoversNothing(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.listInstancesErr = true
	exec := control.executor(t)

	records := []*models.ExecutorRunning{{SessionID: "session-1", AgentExecutionID: "instance-1"}}

	recovered, err := exec.RecoverInstances(context.Background(), records)
	if err != nil {
		t.Fatalf("RecoverInstances: %v, want a logged-and-swallowed enumeration failure", err)
	}
	if len(recovered) != 0 {
		t.Fatalf("recovered = %+v, want none", recovered)
	}
	control.mu.Lock()
	defer control.mu.Unlock()
	if len(control.deleted) != 0 {
		t.Fatalf("deleted = %v, want none: an unenumerable server must not have instances stopped blind", control.deleted)
	}
}

// TestStandaloneExecutorRecoverInstancesRetriesTransientEnumerationFailure
// pins AC-EXECUTORS-SURVIVAL-002.13's bounded read retry applied to the
// enumeration call itself: workspace source roots and provider session
// identity are read back only from this response, so a transient failure
// must be retried, not treated as a hard enumeration failure on the first
// error.
func TestStandaloneExecutorRecoverInstancesRetriesTransientEnumerationFailure(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.listInstances = []*agentctlclient.InstanceInfo{
		{ID: "instance-1", Port: 5001, SessionID: "session-1"},
	}
	control.listInstancesFailures = 1 // fails once, succeeds on the retry
	exec := control.executor(t)
	exec.SetRecoveryRetryConfig(50*time.Millisecond, 1)

	records := []*models.ExecutorRunning{{SessionID: "session-1", AgentExecutionID: "instance-1"}}

	recovered, err := exec.RecoverInstances(context.Background(), records)
	if err != nil {
		t.Fatalf("RecoverInstances: %v", err)
	}
	if len(recovered) != 1 {
		t.Fatalf("recovered = %+v, want the one instance recovered after the retry succeeds", recovered)
	}

	control.mu.Lock()
	defer control.mu.Unlock()
	if control.listInstancesAttempts != 2 {
		t.Fatalf("listInstances attempts = %d, want exactly 2 (1 failure plus 1 retry)", control.listInstancesAttempts)
	}
}

// TestStandaloneExecutorRecoverInstancesExhaustingEnumerationRetryRecoversNothing
// pins the exhausted-budget half of AC-EXECUTORS-SURVIVAL-002.13 for the
// enumeration read: once every attempt fails, recovery falls back to
// AC-EXECUTORS-SURVIVAL-002.12's existing repair path rather than retrying
// indefinitely.
func TestStandaloneExecutorRecoverInstancesExhaustingEnumerationRetryRecoversNothing(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.listInstances = []*agentctlclient.InstanceInfo{
		{ID: "instance-1", Port: 5001, SessionID: "session-1"},
	}
	control.listInstancesFailures = 999 // never succeeds within the retry budget
	exec := control.executor(t)
	exec.SetRecoveryRetryConfig(20*time.Millisecond, 1)

	records := []*models.ExecutorRunning{{SessionID: "session-1", AgentExecutionID: "instance-1"}}

	recovered, err := exec.RecoverInstances(context.Background(), records)
	if err != nil {
		t.Fatalf("RecoverInstances: %v, want a logged-and-swallowed exhausted-retry enumeration failure", err)
	}
	if len(recovered) != 0 {
		t.Fatalf("recovered = %+v, want none", recovered)
	}

	control.mu.Lock()
	defer control.mu.Unlock()
	if control.listInstancesAttempts != 2 {
		t.Fatalf("listInstances attempts = %d, want exactly 2 (1 retry configured)", control.listInstancesAttempts)
	}
}

// fakeUnstoppableRecorder captures RetainAsUnstoppable/MarkStopInFlight/
// Release calls without needing a real RecoveryGuard.
type fakeUnstoppableRecorder struct {
	mu             sync.Mutex
	retained       []string
	markedInFlight []string
	released       []string
}

func (f *fakeUnstoppableRecorder) RetainAsUnstoppable(sessionID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.retained = append(f.retained, sessionID)
}

func (f *fakeUnstoppableRecorder) MarkStopInFlight(sessionID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markedInFlight = append(f.markedInFlight, sessionID)
}

func (f *fakeUnstoppableRecorder) Release(sessionID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.released = append(f.released, sessionID)
}

// TestStandaloneExecutorRecoverInstancesRetriesTransientStopFailure pins
// AC-EXECUTORS-SURVIVAL-002.15: a stop that fails once and then succeeds is
// retried within the bounded count rather than being treated as a permanent
// failure.
func TestStandaloneExecutorRecoverInstancesRetriesTransientStopFailure(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.listInstances = []*agentctlclient.InstanceInfo{
		{ID: "orphan-instance", Port: 5002, SessionID: "session-orphan"},
	}
	control.deleteFailures["orphan-instance"] = 1
	exec := control.executor(t)

	recovered, err := exec.RecoverInstances(context.Background(), nil)
	if err != nil {
		t.Fatalf("RecoverInstances: %v", err)
	}
	if len(recovered) != 0 {
		t.Fatalf("recovered = %+v, want none", recovered)
	}

	control.mu.Lock()
	defer control.mu.Unlock()
	if len(control.deleted) != 1 || control.deleted[0] != "orphan-instance" {
		t.Fatalf("deleted = %v, want the orphan eventually stopped after a retry", control.deleted)
	}
	if control.deleteAttempts["orphan-instance"] != 2 {
		t.Fatalf("delete attempts = %d, want exactly 2 (one failure plus one retry)", control.deleteAttempts["orphan-instance"])
	}
}

// TestStandaloneExecutorRecoverInstancesExhaustingLoserStopDropsAndStopsWinner
// pins AC-EXECUTORS-SURVIVAL-002.15: when a losing duplicate's stop exhausts
// its retries, the winning instance for that session is also not re-tracked
// and is itself stopped on the same terms.
func TestStandaloneExecutorRecoverInstancesExhaustingLoserStopDropsAndStopsWinner(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.listInstances = []*agentctlclient.InstanceInfo{
		{ID: "winner", Port: 5001, SessionID: "session-1"},
		{ID: "loser", Port: 5002, SessionID: "session-1"},
	}
	control.deleteFailures["loser"] = 999 // never succeeds within the retry budget
	exec := control.executor(t)
	exec.SetRecoveryRetryConfig(50*time.Millisecond, 1)

	records := []*models.ExecutorRunning{{SessionID: "session-1", AgentExecutionID: "winner"}}

	recovered, err := exec.RecoverInstances(context.Background(), records)
	if err != nil {
		t.Fatalf("RecoverInstances: %v", err)
	}
	if len(recovered) != 0 {
		t.Fatalf("recovered = %+v, want none: the session must not be re-tracked once its loser can't be stopped", recovered)
	}

	control.mu.Lock()
	defer control.mu.Unlock()
	if control.deleteAttempts["loser"] != 2 {
		t.Fatalf("loser delete attempts = %d, want exactly 2 (1 retry configured)", control.deleteAttempts["loser"])
	}
	if len(control.deleted) != 1 || control.deleted[0] != "winner" {
		t.Fatalf("deleted = %v, want the winner also stopped since the session won't be re-tracked", control.deleted)
	}
}

// TestStandaloneExecutorRecoverInstancesJointStopFailureRetainsSessionUnstoppable
// pins AC-EXECUTORS-SURVIVAL-002.16: when both the losing duplicate's stop
// and the winner's own stop exhaust their retries, the session is recorded
// as unstoppable rather than silently left not-re-tracked.
func TestStandaloneExecutorRecoverInstancesJointStopFailureRetainsSessionUnstoppable(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.listInstances = []*agentctlclient.InstanceInfo{
		{ID: "winner", Port: 5001, SessionID: "session-1"},
		{ID: "loser", Port: 5002, SessionID: "session-1"},
	}
	control.deleteFailures["loser"] = 999
	control.deleteFailures["winner"] = 999
	exec := control.executor(t)
	exec.SetRecoveryRetryConfig(50*time.Millisecond, 0)
	recorder := &fakeUnstoppableRecorder{}
	exec.SetUnstoppableSessionRecorder(recorder)

	records := []*models.ExecutorRunning{{SessionID: "session-1", AgentExecutionID: "winner"}}

	recovered, err := exec.RecoverInstances(context.Background(), records)
	if err != nil {
		t.Fatalf("RecoverInstances: %v", err)
	}
	if len(recovered) != 0 {
		t.Fatalf("recovered = %+v, want none", recovered)
	}

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.retained) != 1 || recorder.retained[0] != "session-1" {
		t.Fatalf("retained = %v, want session-1 retained as unstoppable", recorder.retained)
	}
}

// TestStandaloneExecutorRecoverInstancesDeadlineDropsPendingSessionAndFinishesInBackground
// pins AC-EXECUTORS-SURVIVAL-003.7: once ctx's deadline elapses, a session
// whose losing duplicate hasn't finished stopping yet is treated as
// not-re-tracked immediately -- RecoverInstances returns without waiting for
// it -- but the in-flight stop, and the consequent joint winner-stop and
// AC-EXECUTORS-SURVIVAL-002.16 unstoppable report it triggers, are not
// aborted: they run to completion in the background and still get recorded.
func TestStandaloneExecutorRecoverInstancesDeadlineDropsPendingSessionAndFinishesInBackground(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.listInstances = []*agentctlclient.InstanceInfo{
		{ID: "winner", Port: 5001, SessionID: "session-1"},
		{ID: "loser", Port: 5002, SessionID: "session-1"},
	}
	control.deleteFailures["loser"] = 999  // never succeeds within its retry budget
	control.deleteFailures["winner"] = 999 // never succeeds either, once tried
	control.deleteDelay["loser"] = 150 * time.Millisecond
	exec := control.executor(t)
	exec.SetRecoveryRetryConfig(20*time.Millisecond, 0)
	recorder := &fakeUnstoppableRecorder{}
	exec.SetUnstoppableSessionRecorder(recorder)

	records := []*models.ExecutorRunning{{SessionID: "session-1", AgentExecutionID: "winner"}}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	start := time.Now()
	recovered, err := exec.RecoverInstances(ctx, records)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("RecoverInstances: %v", err)
	}
	if len(recovered) != 0 {
		t.Fatalf("recovered = %+v, want none: the session's loser stop was still unresolved at the deadline", recovered)
	}
	if elapsed > 100*time.Millisecond {
		t.Fatalf("RecoverInstances took %s, want it to return promptly at the deadline instead of waiting for the in-flight stop", elapsed)
	}

	pollDeadline := time.Now().Add(2 * time.Second)
	for {
		recorder.mu.Lock()
		retained := len(recorder.retained)
		recorder.mu.Unlock()
		if retained == 1 {
			break
		}
		if time.Now().After(pollDeadline) {
			t.Fatal("timed out waiting for the background drain to finish joint-failure handling")
		}
		time.Sleep(5 * time.Millisecond)
	}

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.retained[0] != "session-1" {
		t.Fatalf("retained = %v, want session-1", recorder.retained)
	}
}

// TestStandaloneExecutorRecoverInstancesDeadlineMarksGuardInFlightAndReleasesOnResolution
// pins Review round 2 finding 3 (AC-EXECUTORS-SURVIVAL-002.8/003.7): a
// session dropped at the recovery deadline because its loser's stop is still
// in flight must have its guard marked stop-in-flight, so a Start() pass's
// ReleaseAllExceptRetained leaves it held rather than releasing it while a
// launch racing the still-resolving stop could still recreate two live
// agents for the session -- and the guard must actually be released once the
// background drain learns the stop resolved, not left held forever. Uses the
// real RecoveryGuard rather than a fake so both state transitions are pinned
// against the type Manager.Start actually wires in.
func TestStandaloneExecutorRecoverInstancesDeadlineMarksGuardInFlightAndReleasesOnResolution(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.listInstances = []*agentctlclient.InstanceInfo{
		{ID: "winner", Port: 5001, SessionID: "session-1"},
		{ID: "loser", Port: 5002, SessionID: "session-1"},
	}
	// Long enough per-attempt timeout to tolerate the delay below, so the
	// loser's stop eventually succeeds instead of failing on a client-side
	// timeout -- this test is about the success/resolution path, not the
	// joint-failure path TestStandaloneExecutorRecoverInstancesJointStopFailureRetainsSessionUnstoppable
	// already covers.
	control.deleteDelay["loser"] = 30 * time.Millisecond
	exec := control.executor(t)
	exec.SetRecoveryRetryConfig(200*time.Millisecond, 0)
	guard := NewRecoveryGuard()
	guard.AcquireOrObserve("session-1")
	exec.SetUnstoppableSessionRecorder(guard)

	records := []*models.ExecutorRunning{{SessionID: "session-1", AgentExecutionID: "winner"}}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	recovered, err := exec.RecoverInstances(ctx, records)
	if err != nil {
		t.Fatalf("RecoverInstances: %v", err)
	}
	if len(recovered) != 0 {
		t.Fatalf("recovered = %+v, want none: the loser stop was still unresolved at the deadline", recovered)
	}

	// Simulate the bulk release a Start() pass performs once every
	// reconstructed session's outcome is published: it must not release
	// this session's guard yet, since its stop is still resolving.
	guard.ReleaseAllExceptRetained()
	if err := guard.CheckLaunchAllowed("session-1"); !errors.Is(err, ErrSessionRecoveryGuarded) {
		t.Fatalf("guard state right after the deadline = %v, want still held (ErrSessionRecoveryGuarded)", err)
	}

	pollDeadline := time.Now().Add(2 * time.Second)
	for guard.CheckLaunchAllowed("session-1") != nil {
		if time.Now().After(pollDeadline) {
			t.Fatal("timed out waiting for the background drain to release the guard")
		}
		time.Sleep(5 * time.Millisecond)
	}

	control.mu.Lock()
	deleted := append([]string(nil), control.deleted...)
	control.mu.Unlock()
	if len(deleted) != 1 || deleted[0] != "loser" {
		t.Fatalf("deleted = %v, want only the loser stopped: the winner must be untouched when the loser eventually succeeds", deleted)
	}
}

// TestStandaloneExecutorRecoverInstancesOrphanStopFailureIsRecordedNotJoint
// pins that an orphan's (AC-EXECUTORS-SURVIVAL-002.6) exhausted stop retry
// has no session to affect: it is only logged, never routed through the
// unstoppable-session recorder.
func TestStandaloneExecutorRecoverInstancesOrphanStopFailureIsRecordedNotJoint(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.listInstances = []*agentctlclient.InstanceInfo{
		{ID: "orphan-instance", Port: 5002, SessionID: "session-orphan"},
	}
	control.deleteFailures["orphan-instance"] = 999
	exec := control.executor(t)
	exec.SetRecoveryRetryConfig(50*time.Millisecond, 0)
	recorder := &fakeUnstoppableRecorder{}
	exec.SetUnstoppableSessionRecorder(recorder)

	recovered, err := exec.RecoverInstances(context.Background(), nil)
	if err != nil {
		t.Fatalf("RecoverInstances: %v", err)
	}
	if len(recovered) != 0 {
		t.Fatalf("recovered = %+v, want none", recovered)
	}

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.retained) != 0 {
		t.Fatalf("retained = %v, want none: an orphan has no session to retain", recorder.retained)
	}
}

func TestBuildStandaloneCreateInstanceRequestMapsEveryField(t *testing.T) {
	autoApprove := true
	req := &ExecutorCreateRequest{
		InstanceID:                     "instance-2",
		TaskID:                         "task-2",
		SessionID:                      "session-2",
		WorkspacePath:                  "/host/workspace",
		WorkspaceSourceRoots:           []string{"/host/sources"},
		Protocol:                       "acp",
		McpMode:                        "office",
		McpProviders:                   []string{"github"},
		AutoApprovePermissionsOverride: &autoApprove,
		Metadata:                       map[string]interface{}{MetadataKeyBaseBranches: map[string]string{"repo": "dev"}},
	}

	got := buildStandaloneCreateInstanceRequest(req, map[string]string{"K": "V"}, "codex",
		true, true, false, true, []string{"STRIP"})

	if got.ID != "instance-2" || got.WorkspacePath != "/host/workspace" || got.AgentType != "codex" {
		t.Fatalf("request = %+v", got)
	}
	if !got.DisableAskQuestion || !got.AssumeMcpSse || got.AssumeMcpHttp || !got.RequiresProcessKill {
		t.Fatalf("capability flags = %+v", got)
	}
	if got.AutoApprovePermissions == nil || !*got.AutoApprovePermissions {
		t.Fatalf("AutoApprovePermissions = %v", got.AutoApprovePermissions)
	}
	if got.Env["K"] != "V" || !equalStrings(got.StripEnv, []string{"STRIP"}) {
		t.Fatalf("env/strip = %+v / %v", got.Env, got.StripEnv)
	}
	if got.BaseBranches["repo"] != "dev" {
		t.Fatalf("BaseBranches = %+v", got.BaseBranches)
	}
	if got.McpMode != "office" || !equalStrings(got.McpProviders, []string{"github"}) {
		t.Fatalf("mcp routing = %q / %v", got.McpMode, got.McpProviders)
	}
	if !equalStrings(got.WorkspaceSourceRoots, []string{"/host/sources"}) {
		t.Fatalf("WorkspaceSourceRoots = %v", got.WorkspaceSourceRoots)
	}
}
