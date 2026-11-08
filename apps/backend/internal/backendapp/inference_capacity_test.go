package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/system/sessioncapacity"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type nativeInferenceHarness struct {
	capacity      *orchestrator.Service
	store         *sessioncapacity.Store
	raw           *systemsettings.Store
	client        *agentruntime.InferenceClient
	identity      agentruntime.InferenceIdentity
	mu            sync.Mutex
	status        utility.AttemptStatus
	prompts, acks int
	terminal      bool
	notStarted    bool
	unsupported   bool
	t             *testing.T
}

func newNativeInferenceHarness(t *testing.T, gradeMaximum int, terminal bool) *nativeInferenceHarness {
	t.Helper()
	pool := newMessageQueueSettingsTestPool(t)
	repo, err := tasksqlite.NewWithDB(pool.Writer(), pool.Reader(), testLogger(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &taskmodels.Task{ID: "parent", Title: "Existing parent", ExecutionGrade: "A"}); err != nil {
		t.Fatal(err)
	}
	capacity := orchestrator.NewService(orchestrator.ServiceConfig{QueueSize: 8, SessionCapacity: 2}, nil, nil, nil, repo, nil, nil, nil, testLogger(t))
	capacity.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(nil, nil, false))
	capacity.SetExecutionCapacity(sessioncapacity.Effective{Enabled: true, MaxSessions: 2, Policy: sessioncapacity.PolicyAll, GradeLimits: map[string]int{"A": gradeMaximum}})
	raw, err := systemsettings.NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	h := &nativeInferenceHarness{raw: raw, capacity: capacity, store: sessioncapacity.NewStore(raw), identity: agentruntime.InferenceIdentity{TaskID: "parent", OwnerKind: "session", ParentExecutionID: "instance", CallID: uuid.NewString()}, terminal: terminal, t: t}
	capacity.SetInferenceCapacityOwner(h.store, nil)
	if err := capacity.SyncInferenceCapacity(ctx, true, func(map[string]sessioncapacity.InferenceBinding) error { return nil }); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(h.serve))
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}
	h.client = agentctlclient.NewClient(parsed.Hostname(), port, testLogger(t), agentruntime.WithInferenceExecutionID("instance"))
	t.Cleanup(h.client.Close)
	return h
}

func (h *nativeInferenceHarness) serve(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/inference/attempts":
		if h.unsupported {
			http.NotFound(w, r)
			return
		}
		var req utility.PrepareAttemptRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.t.Error(err)
		}
		h.status = utility.AttemptStatus{PreparationID: req.PreparationID, AttemptID: uuid.NewString(), CallID: req.CallID, InstanceID: "instance", OwnerID: "owner", TaskID: "parent", State: utility.AttemptPrepared, PromptDispatch: utility.PromptNotSent, Known: true, Retained: req.Retain}
		_ = json.NewEncoder(w).Encode(h.status)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/inference/attempts":
		attempts := []utility.AttemptStatus{}
		if h.status.AttemptID != "" {
			attempts = append(attempts, h.status)
		}
		_ = json.NewEncoder(w).Encode(utility.AttemptSnapshot{OwnerID: "owner", InstanceID: "instance", Known: true, Attempts: attempts})
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/inference/prompt":
		h.prompts++
		if h.unsupported {
			_ = json.NewEncoder(w).Encode(utility.PromptResponse{Success: true, Response: "legacy"})
			return
		}
		var req utility.PromptRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.t.Error(err)
		}
		if req.AttemptID != h.status.AttemptID {
			h.t.Error("actual prompt did not use prepared token")
		}
		h.status.State = utility.AttemptUnknown
		h.status.Known = false
		h.status.PromptDispatch = utility.PromptPossiblySent
		if h.terminal {
			h.status.State = utility.AttemptTerminated
			h.status.Known = true
			h.status.Settled = true
		}
		if h.notStarted {
			h.status.State = utility.AttemptNotStarted
			h.status.PromptDispatch = utility.PromptNotSent
		}
		status := h.status
		_ = json.NewEncoder(w).Encode(utility.PromptResponse{Success: false, Error: "fixture failure", Attempt: &status})
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/inference/attempts/"+h.status.AttemptID+"/cancel":
		if h.status.State == utility.AttemptPrepared {
			h.status.State = utility.AttemptNotStarted
			h.status.Known = true
			h.status.Settled = true
		} else if h.status.State != utility.AttemptNotStarted {
			w.WriteHeader(http.StatusConflict)
		}
		_ = json.NewEncoder(w).Encode(h.status)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/inference/attempts/"+h.status.AttemptID:
		_ = json.NewEncoder(w).Encode(h.status)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/inference/attempts/"+h.status.AttemptID+"/release":
		held, err := h.store.LoadInferenceBindings(r.Context())
		if err != nil || len(held) != 0 {
			h.t.Errorf("ACK before durable binding removal: %+v %v", held, err)
		}
		h.acks++
		h.status.Retained = false
		_ = json.NewEncoder(w).Encode(h.status)
	default:
		http.NotFound(w, r)
	}
}

func (h *nativeInferenceHarness) InferenceClients(context.Context) ([]agentruntime.InferenceClientRef, error) {
	return []agentruntime.InferenceClientRef{{Client: h.client, Identity: h.identity, Release: func() {}}}, nil
}

func TestNativeInferenceRootGradeRefusalPreventsActualSend(t *testing.T) {
	h := newNativeInferenceHarness(t, 0, true)
	b := &sharedInferenceConsumer{capacity: h.capacity, sources: []inferenceClientSource{h}}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, err := b.Execute(ctx, h.client, &utility.PromptRequest{Prompt: "fixture", AgentID: "fake"}, h.identity)
	if !agentruntime.IsInferenceCapacityError(err) {
		t.Fatalf("grade-zero did not remain a capacity refusal: %v", err)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.prompts != 0 {
		t.Fatalf("grade-zero sent %d prompts", h.prompts)
	}
}

func TestNativeInferenceRootTerminalAndUnknownKeepDifferentOwnership(t *testing.T) {
	for _, terminal := range []bool{false, true} {
		t.Run(map[bool]string{false: "unknown", true: "terminal"}[terminal], func(t *testing.T) {
			h := newNativeInferenceHarness(t, 1, terminal)
			b := &sharedInferenceConsumer{capacity: h.capacity, sources: []inferenceClientSource{h}}
			_, err := b.Execute(context.Background(), h.client, &utility.PromptRequest{Prompt: "fixture", AgentID: "fake"}, h.identity)
			held, loadErr := h.store.LoadInferenceBindings(context.Background())
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			h.mu.Lock()
			prompts, acks := h.prompts, h.acks
			h.mu.Unlock()
			if prompts != 1 {
				t.Fatalf("positive actual send count=%d", prompts)
			}
			if terminal {
				if len(held) != 0 || acks != 1 || agentruntime.IsInferenceCapacityError(err) {
					t.Fatalf("exact settled child not returned/acked: held=%+v acks=%d err=%v", held, acks, err)
				}
			} else if len(held) != 1 || acks != 0 || !agentruntime.IsInferenceCapacityError(err) {
				t.Fatalf("unknown child released or permitted fallback: held=%+v acks=%d err=%v", held, acks, err)
			}
		})
	}
}

type failingInferenceSettlement struct {
	store *sessioncapacity.Store
	calls int
	fail  bool
}

func (f *failingInferenceSettlement) FinishProviderTurn(ctx context.Context, key string, _ uint64, _ bool) error {
	f.calls++
	held, err := f.store.LoadInferenceBindings(ctx)
	if err != nil {
		return err
	}
	if held[key].TerminalState == "" {
		return errors.New("settlement preceded durable terminal proof")
	}
	if f.fail {
		return errors.New("fixture settlement unavailable")
	}
	return nil
}

func TestNativeInferenceRootStopProofRetainedBeforeSettlementAndDuplicateNoOp(t *testing.T) {
	h := newNativeInferenceHarness(t, 1, true)
	key := "inference:owner:attempt"
	binding := sessioncapacity.InferenceBinding{Key: key, InstanceID: "instance", OwnerID: "owner", AttemptID: "attempt", PreparationID: "preparation", Grade: "A", GradeKnown: true, ProviderKey: "existing-claim", ProviderKnown: true, State: "active"}
	if err := h.capacity.UpdateInferenceCapacity(context.Background(), func(active map[string]sessioncapacity.InferenceBinding) error { active[key] = binding; return nil }); err != nil {
		t.Fatal(err)
	}
	settler := &failingInferenceSettlement{store: h.store, fail: true}
	b := &sharedInferenceConsumer{capacity: h.capacity, settler: settler}
	snapshot := &utility.AttemptSnapshot{OwnerID: "owner", InstanceID: "instance", Attempts: []utility.AttemptStatus{{OwnerID: "owner", InstanceID: "instance", AttemptID: "attempt", PreparationID: "preparation", Known: true, Settled: true, State: utility.AttemptTerminated}}}
	if err := b.ObserveInferenceStop(context.Background(), snapshot); err == nil {
		t.Fatal("failed settlement accepted")
	}
	held, err := h.store.LoadInferenceBindings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(held) != 1 || held[key].TerminalState != utility.AttemptTerminated || settler.calls != 1 {
		t.Fatalf("proof/owner lost on failure: %+v calls=%d", held, settler.calls)
	}
	settler.fail = false
	if err := b.ObserveInferenceStop(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := b.ObserveInferenceStop(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	held, err = h.store.LoadInferenceBindings(context.Background())
	if err != nil || len(held) != 0 || settler.calls != 2 {
		t.Fatalf("duplicate proof re-settled: %+v %v calls=%d", held, err, settler.calls)
	}
}

func TestNativeInferenceRootNotStartedAcknowledgedOnce(t *testing.T) {
	h := newNativeInferenceHarness(t, 1, true)
	h.notStarted = true
	b := &sharedInferenceConsumer{capacity: h.capacity, sources: []inferenceClientSource{h}}
	_, err := b.Execute(context.Background(), h.client, &utility.PromptRequest{Prompt: "fixture", AgentID: "fake"}, h.identity)
	if agentruntime.IsInferenceCapacityError(err) {
		t.Fatal(err)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.acks != 1 {
		t.Fatalf("not-started retention ACK count=%d", h.acks)
	}
}

func TestNativeInferenceRootLegacyOffPreservesUnsupportedClient(t *testing.T) {
	h := newNativeInferenceHarness(t, 1, true)
	h.unsupported = true
	h.capacity.SetExecutionCapacity(sessioncapacity.Effective{})
	b := &sharedInferenceConsumer{capacity: h.capacity, sources: []inferenceClientSource{h}}
	response, err := b.Execute(context.Background(), h.client, &utility.PromptRequest{Prompt: "fixture", AgentID: "fake"}, h.identity)
	if err != nil || response == nil || !response.Success {
		t.Fatalf("default-off legacy changed: response=%+v err=%v", response, err)
	}
	if h.prompts != 1 {
		t.Fatalf("legacy actual sends=%d", h.prompts)
	}
}

func TestNativeInferenceRootRefreshPreservesFailedSettlementProofAcrossRestart(t *testing.T) {
	h := newNativeInferenceHarness(t, 1, true)
	ctx := context.Background()
	key := "inference:owner:attempt"
	binding := sessioncapacity.InferenceBinding{Key: key, InstanceID: "instance", OwnerID: "owner", AttemptID: "attempt", PreparationID: "preparation", Grade: "A", GradeKnown: true, ProviderKey: "existing-claim", ProviderKnown: true, State: "active"}
	if err := h.capacity.UpdateInferenceCapacity(ctx, func(active map[string]sessioncapacity.InferenceBinding) error { active[key] = binding; return nil }); err != nil {
		t.Fatal(err)
	}
	h.status = utility.AttemptStatus{OwnerID: "owner", InstanceID: "instance", AttemptID: "attempt", PreparationID: "preparation", Known: true, Settled: true, Retained: true, State: utility.AttemptTerminated}
	settler := &failingInferenceSettlement{store: h.store, fail: true}
	b := &sharedInferenceConsumer{capacity: h.capacity, settler: settler, sources: []inferenceClientSource{h}}
	b.Refresh(ctx)
	held, err := h.store.LoadInferenceBindings(ctx)
	if err != nil || held[key].TerminalState != utility.AttemptTerminated || settler.calls != 1 || h.acks != 0 {
		t.Fatalf("refresh erased failed settlement proof: %+v err=%v calls=%d ACKs=%d", held, err, settler.calls, h.acks)
	}
	settler.fail = false
	restarted := &sharedInferenceConsumer{capacity: h.capacity, settler: settler}
	restarted.Refresh(ctx)
	held, err = h.store.LoadInferenceBindings(ctx)
	if err != nil || len(held) != 0 || settler.calls != 2 {
		t.Fatalf("retained proof did not settle without live client: %+v err=%v calls=%d", held, err, settler.calls)
	}
}

type rootInferenceEndpoint struct {
	*nativeInferenceHarness
	consumer agentruntime.InferenceConsumer
}

func (e *rootInferenceEndpoint) SetInferenceConsumer(consumer agentruntime.InferenceConsumer) {
	e.consumer = consumer
}

func TestNativeInferenceRootProductionWiringUsesSameAdmissionAndPreservesSettings(t *testing.T) {
	h := newNativeInferenceHarness(t, 0, true)
	first, second := &rootInferenceEndpoint{nativeInferenceHarness: h}, &rootInferenceEndpoint{nativeInferenceHarness: h}
	wireSharedInferenceCapacity(h.capacity, h.raw, nil, first, second)
	if first.consumer == nil || first.consumer != second.consumer {
		t.Fatal("actual consumers did not share installed owner")
	}
	for _, endpoint := range []*rootInferenceEndpoint{first, second} {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
		_, err := endpoint.consumer.Execute(ctx, h.client, &utility.PromptRequest{Prompt: "fixture", AgentID: "fake"}, h.identity)
		cancel()
		if !agentruntime.IsInferenceCapacityError(err) {
			t.Fatalf("wired entry escaped grade refusal: %v", err)
		}
	}
	if h.prompts != 0 {
		t.Fatalf("wired entries sent %d blocked prompts", h.prompts)
	}
}

func TestNativeInferenceRootRefreshAcknowledgesOnlySettledOwnedBinding(t *testing.T) {
	h := newNativeInferenceHarness(t, 1, true)
	ctx := context.Background()
	key := "inference:owner:attempt"
	binding := sessioncapacity.InferenceBinding{Key: key, InstanceID: "instance", OwnerID: "owner", AttemptID: "attempt", PreparationID: "preparation", Grade: "A", GradeKnown: true, State: "active", SettlementKind: inferenceSettlementNone}
	if err := h.capacity.UpdateInferenceCapacity(ctx, func(active map[string]sessioncapacity.InferenceBinding) error { active[key] = binding; return nil }); err != nil {
		t.Fatal(err)
	}
	h.status = utility.AttemptStatus{OwnerID: "owner", InstanceID: "instance", AttemptID: "attempt", PreparationID: "preparation", Known: true, Settled: true, Retained: true, State: utility.AttemptTerminated}
	b := &sharedInferenceConsumer{capacity: h.capacity, sources: []inferenceClientSource{h}}
	b.Refresh(ctx)
	b.Refresh(ctx)
	held, err := h.store.LoadInferenceBindings(ctx)
	if err != nil || len(held) != 0 || h.acks != 1 {
		t.Fatalf("recovered settled owner retention not acknowledged once: %+v err=%v ACKs=%d", held, err, h.acks)
	}
}

func TestNativeInferenceRootUnknownLegacyClaimNeverReleased(t *testing.T) {
	h := newNativeInferenceHarness(t, 1, true)
	ctx := context.Background()
	key := "inference:owner:attempt"
	binding := sessioncapacity.InferenceBinding{Key: key, InstanceID: "instance", OwnerID: "owner", AttemptID: "attempt", PreparationID: "preparation", Grade: "A", GradeKnown: true, State: "active"}
	if err := h.capacity.UpdateInferenceCapacity(ctx, func(active map[string]sessioncapacity.InferenceBinding) error { active[key] = binding; return nil }); err != nil {
		t.Fatal(err)
	}
	h.status = utility.AttemptStatus{OwnerID: "owner", InstanceID: "instance", AttemptID: "attempt", PreparationID: "preparation", Known: true, Settled: true, Retained: true, State: utility.AttemptTerminated}
	b := &sharedInferenceConsumer{capacity: h.capacity, sources: []inferenceClientSource{h}}
	b.Refresh(ctx)
	held, err := h.store.LoadInferenceBindings(ctx)
	if err != nil || len(held) != 1 || held[key].TerminalState == "" || h.acks != 0 {
		t.Fatalf("unknown legacy claim released: %+v err=%v ACKs=%d", held, err, h.acks)
	}
}
