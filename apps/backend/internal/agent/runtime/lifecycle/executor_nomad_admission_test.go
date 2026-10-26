package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/nomad"
)

// nomadAdmissionFake serves the node and quarantine-variable endpoints and
// keeps one ordered call log shared with the durable checkpoint callback.
type nomadAdmissionFake struct {
	mu              sync.Mutex
	calls           []string
	bodies          []string
	eligibility     string
	status          string
	datacenter      string
	failNodeGet     bool
	failEligibility bool
	failMarkerRead  bool
	failMarkerWrite bool
	marker          string
}

func newNomadAdmissionFake() *nomadAdmissionFake {
	return &nomadAdmissionFake{eligibility: "eligible", status: "ready", datacenter: "dc1"}
}

func (f *nomadAdmissionFake) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

func (f *nomadAdmissionFake) callLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *nomadAdmissionFake) variableBody() string {
	body, _ := json.Marshal(map[string]any{"Path": "kandev/quarantine/node-1", "ModifyIndex": 4, "Items": map[string]string{"marker": f.marker}})
	return string(body)
}

func (f *nomadAdmissionFake) serve(w http.ResponseWriter, r *http.Request) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/node/node-1":
		f.calls = append(f.calls, "node:get")
		if f.failNodeGet {
			http.Error(w, "down", http.StatusInternalServerError)
			return true
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ID": "node-1", "Datacenter": f.datacenter, "Status": f.status, "SchedulingEligibility": f.eligibility,
			"Attributes": map[string]string{"kernel.name": "darwin", "cpu.arch": "arm64"},
		})
	case r.Method == http.MethodPost && r.URL.Path == "/v1/node/node-1/eligibility":
		body, _ := io.ReadAll(r.Body)
		f.calls = append(f.calls, "node:eligibility")
		f.bodies = append(f.bodies, string(body))
		if f.failEligibility {
			http.Error(w, "down", http.StatusInternalServerError)
			return true
		}
		f.eligibility = "ineligible"
		_, _ = io.WriteString(w, `{}`)
	case r.URL.Path == "/v1/var/kandev/quarantine/node-1" && r.Method == http.MethodGet:
		f.calls = append(f.calls, "marker:get")
		switch {
		case f.failMarkerRead:
			http.Error(w, "down", http.StatusInternalServerError)
		case f.marker == "":
			http.NotFound(w, r)
		default:
			_, _ = io.WriteString(w, f.variableBody())
		}
	case r.URL.Path == "/v1/var/kandev/quarantine/node-1" && r.Method == http.MethodPut:
		f.calls = append(f.calls, "marker:put")
		var variable struct{ Items map[string]string }
		_ = json.NewDecoder(r.Body).Decode(&variable)
		switch {
		case f.failMarkerWrite:
			http.Error(w, "down", http.StatusInternalServerError)
		case f.marker != "":
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, f.variableBody())
		default:
			f.marker = variable.Items["marker"]
			_, _ = io.WriteString(w, `{}`)
		}
	default:
		return false
	}
	return true
}

func admissionHarness(t *testing.T) (*nomadCreateFixture, *nomad.Client, nomad.ProfileConfig) {
	t.Helper()
	fixture := newNomadCreateFixture(t, false)
	client, err := nomad.NewClient(fixture.profile.Address, "fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	return fixture, client, fixture.profile
}

func otherNomadEnvState(phase, quarantine string) nomad.EnvironmentState {
	return nomad.EnvironmentState{
		Version: 1, EnvironmentID: "environment-other", ExecutionID: "execution-other", OperationID: "execution-other",
		NodeID: "node-1", Phase: phase, QuarantineState: quarantine,
	}
}

func assertQuarantined(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ErrNomadWorkerQuarantined) {
		t.Fatalf("admission error = %v, want ErrNomadWorkerQuarantined", err)
	}
	if strings.Contains(err.Error(), "fixture-token") {
		t.Fatalf("admission error leaks a credential: %v", err)
	}
}

func TestNomadAdmissionFenceAdmitsCleanWorker(t *testing.T) {
	fixture, client, profile := admissionHarness(t)
	fixture.inventory = []nomad.EnvironmentState{otherNomadEnvState("ready", ""), otherNomadEnvState("absent", "")}
	if err := fixture.runtime.admitNewNomadOperation(context.Background(), fixture.request, profile, client); err != nil {
		t.Fatalf("clean worker rejected: %v", err)
	}
}

func TestNomadAdmissionFenceRejectsEachRule(t *testing.T) {
	cases := map[string]func(*nomadCreateFixture){
		"pending quarantine": func(f *nomadCreateFixture) {
			f.inventory = []nomad.EnvironmentState{otherNomadEnvState("ready", "pending")}
		},
		"marked quarantine": func(f *nomadCreateFixture) {
			f.inventory = []nomad.EnvironmentState{otherNomadEnvState("ready", "marked")}
		},
		"cleanup pending phase": func(f *nomadCreateFixture) {
			f.inventory = []nomad.EnvironmentState{otherNomadEnvState("cleanup_pending", "")}
		},
		"nil inventory lister": func(f *nomadCreateFixture) { f.request.ListNomadInventory = nil },
		"inventory error": func(f *nomadCreateFixture) {
			f.request.ListNomadInventory = func(context.Context) ([]nomad.EnvironmentState, error) { return nil, errors.New("db down") }
		},
		"marker exists":           func(f *nomadCreateFixture) { f.fake.marker = `{"version":1}` },
		"node ineligible":         func(f *nomadCreateFixture) { f.fake.eligibility = "ineligible" },
		"node not ready":          func(f *nomadCreateFixture) { f.fake.status = "down" },
		"node read unavailable":   func(f *nomadCreateFixture) { f.fake.failNodeGet = true },
		"marker read unavailable": func(f *nomadCreateFixture) { f.fake.failMarkerRead = true },
		"non-dedicated node":      func(f *nomadCreateFixture) { f.fake.datacenter = "elsewhere" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			fixture, client, profile := admissionHarness(t)
			mutate(fixture)
			assertQuarantined(t, fixture.runtime.admitNewNomadOperation(context.Background(), fixture.request, profile, client))
			for _, call := range fixture.fake.callLog() {
				if call == "node:eligibility" {
					t.Fatal("admission attempted to change node eligibility")
				}
			}
		})
	}
}

func TestNomadAdmissionFenceIgnoresOwnOperationAndOtherNodes(t *testing.T) {
	fixture, client, profile := admissionHarness(t)
	own := otherNomadEnvState("cleanup_pending", "pending")
	own.EnvironmentID, own.ExecutionID = fixture.request.TaskEnvironmentID, fixture.request.InstanceID
	elsewhere := otherNomadEnvState("cleanup_pending", "marked")
	elsewhere.NodeID = "node-2"
	fixture.inventory = []nomad.EnvironmentState{own, elsewhere}
	if err := fixture.runtime.admitNewNomadOperation(context.Background(), fixture.request, profile, client); err != nil {
		t.Fatalf("own or other-node state rejected: %v", err)
	}
}

func assertNoLaunchSideEffects(t *testing.T, fixture *nomadCreateFixture) {
	t.Helper()
	if fixture.registerCalls != 0 || fixture.createCalls != 0 || len(fixture.checkpoints) != 0 {
		t.Fatalf("rejected admission had side effects: registrations=%d instances=%d checkpoints=%d", fixture.registerCalls, fixture.createCalls, len(fixture.checkpoints))
	}
}

func TestNomadAdmissionFenceRejectsAcrossOperationsBeforeAnySideEffect(t *testing.T) {
	fixture := newNomadCreateFixture(t, false)
	fixture.inventory = []nomad.EnvironmentState{otherNomadEnvState("ready", "pending")}
	_, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	assertQuarantined(t, err)
	assertNoLaunchSideEffects(t, fixture)
}

func TestNomadAdmissionFenceMarkerSurvivesInventoryAndJobHistoryLoss(t *testing.T) {
	fixture := newNomadCreateFixture(t, false)
	fixture.inventory = nil
	fixture.jobHistoryMissing = true
	fixture.fake.marker = `{"version":1,"node_id":"node-1"}`
	_, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	assertQuarantined(t, err)
	assertNoLaunchSideEffects(t, fixture)
}

func TestNomadAdmissionFenceFailsClosedWhenInventoryUnavailable(t *testing.T) {
	fixture := newNomadCreateFixture(t, false)
	fixture.request.ListNomadInventory = nil
	_, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	assertQuarantined(t, err)
	assertNoLaunchSideEffects(t, fixture)
}

func quarantineTestState(t *testing.T, fixture *nomadCreateFixture) nomad.EnvironmentState {
	t.Helper()
	id, err := resolveNomadLaunchIdentity(fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	state := newNomadIntentState(fixture.request, id)
	state.Phase = nomadPhaseCleanupPending
	return state
}

func indexOf(calls []string, want string) int {
	for i, call := range calls {
		if call == want {
			return i
		}
	}
	return -1
}

func TestNomadWorkerQuarantinePersistsPendingBeforeNodeCallThenMarks(t *testing.T) {
	fixture, client, profile := admissionHarness(t)
	state := quarantineTestState(t, fixture)
	if err := fixture.runtime.quarantineWorker(context.Background(), fixture.request, profile, client, &state, "bootstrap_failed"); err != nil {
		t.Fatal(err)
	}
	calls := fixture.fake.callLog()
	pending, node, marked := indexOf(calls, "checkpoint:cleanup_pending:pending"), indexOf(calls, "node:get"), indexOf(calls, "checkpoint:cleanup_pending:marked")
	if pending != 0 || node < pending || marked != len(calls)-1 || indexOf(calls, "node:eligibility") < node || indexOf(calls, "marker:put") < node {
		t.Fatalf("quarantine order = %v", calls)
	}
	if state.QuarantineState != nomad.QuarantineMarked || fixture.fake.eligibility != "ineligible" || !strings.Contains(fixture.fake.marker, "bootstrap_failed") {
		t.Fatalf("state=%+v eligibility=%s marker=%q", state, fixture.fake.eligibility, fixture.fake.marker)
	}
	assertEligibilityBodiesIneligible(t, fixture)
}

func assertEligibilityBodiesIneligible(t *testing.T, fixture *nomadCreateFixture) {
	t.Helper()
	for _, body := range fixture.fake.bodies {
		if !strings.Contains(body, `"ineligible"`) || strings.Contains(strings.ReplaceAll(body, "ineligible", ""), "ligible") {
			t.Fatalf("eligibility request body enables placement: %s", body)
		}
	}
}

func TestNomadWorkerQuarantineRetainsPendingWhenControlAPIUnavailable(t *testing.T) {
	for name, mutate := range map[string]func(*nomadAdmissionFake){
		"node read":    func(f *nomadAdmissionFake) { f.failNodeGet = true },
		"eligibility":  func(f *nomadAdmissionFake) { f.failEligibility = true },
		"marker write": func(f *nomadAdmissionFake) { f.failMarkerWrite = true },
	} {
		t.Run(name, func(t *testing.T) {
			fixture, client, profile := admissionHarness(t)
			mutate(fixture.fake)
			state := quarantineTestState(t, fixture)
			if err := fixture.runtime.quarantineWorker(context.Background(), fixture.request, profile, client, &state, "bootstrap_failed"); err == nil {
				t.Fatal("quarantine succeeded despite an unavailable control API")
			}
			if state.QuarantineState != nomad.QuarantinePending {
				t.Fatalf("quarantine state = %q, want pending retained", state.QuarantineState)
			}
			last := fixture.checkpoints[len(fixture.checkpoints)-1]
			if last.QuarantineState != nomad.QuarantinePending {
				t.Fatalf("last durable state = %q, want pending", last.QuarantineState)
			}
			fixture.fake.failNodeGet, fixture.fake.failMarkerRead = false, false
			fixture.inventory = []nomad.EnvironmentState{last}
			other := newNomadCreateFixture(t, false)
			other.request.TaskEnvironmentID, other.request.InstanceID = "environment-b", "execution-b"
			other.inventory = fixture.inventory
			otherClient, _ := nomad.NewClient(other.profile.Address, "fixture-token")
			assertQuarantined(t, other.runtime.admitNewNomadOperation(context.Background(), other.request, other.profile, otherClient))
		})
	}
}

func TestNomadWorkerQuarantineDoesNotOverwriteExistingMarker(t *testing.T) {
	fixture, client, profile := admissionHarness(t)
	fixture.fake.marker = `{"version":1,"reason":"operator"}`
	state := quarantineTestState(t, fixture)
	if err := fixture.runtime.quarantineWorker(context.Background(), fixture.request, profile, client, &state, "bootstrap_failed"); err != nil {
		t.Fatal(err)
	}
	if fixture.fake.marker != `{"version":1,"reason":"operator"}` || state.QuarantineState != nomad.QuarantineMarked {
		t.Fatalf("existing marker overwritten or state not marked: marker=%q state=%q", fixture.fake.marker, state.QuarantineState)
	}
}

func TestNomadWorkerQuarantineRefusesNonDedicatedNode(t *testing.T) {
	fixture, client, profile := admissionHarness(t)
	fixture.fake.datacenter = "elsewhere"
	state := quarantineTestState(t, fixture)
	if err := fixture.runtime.quarantineWorker(context.Background(), fixture.request, profile, client, &state, "bootstrap_failed"); err == nil {
		t.Fatal("non-dedicated node was quarantined")
	}
	calls := fixture.fake.callLog()
	if indexOf(calls, "node:eligibility") >= 0 || indexOf(calls, "marker:put") >= 0 || state.QuarantineState != nomad.QuarantinePending {
		t.Fatalf("non-dedicated node mutated or pending lost: calls=%v state=%q", calls, state.QuarantineState)
	}
}

func TestNomadWorkerQuarantineReconcileLeavesIneligibleNodeAlone(t *testing.T) {
	fixture, client, profile := admissionHarness(t)
	fixture.fake.eligibility = "ineligible"
	state := quarantineTestState(t, fixture)
	state.QuarantineState = nomad.QuarantinePending
	if err := fixture.runtime.reconcilePendingNomadQuarantine(context.Background(), fixture.request, profile, client, &state); err != nil {
		t.Fatal(err)
	}
	if indexOf(fixture.fake.callLog(), "node:eligibility") >= 0 || state.QuarantineState != nomad.QuarantineMarked || fixture.fake.marker == "" {
		t.Fatalf("reconcile touched eligibility or failed to mark: calls=%v state=%q", fixture.fake.callLog(), state.QuarantineState)
	}
	assertEligibilityBodiesIneligible(t, fixture)
}

func TestNomadWorkerQuarantineReconcileCompletesPendingFromIneligibleAfterOutage(t *testing.T) {
	fixture, client, profile := admissionHarness(t)
	fixture.fake.failMarkerWrite = true
	state := quarantineTestState(t, fixture)
	if err := fixture.runtime.quarantineWorker(context.Background(), fixture.request, profile, client, &state, "bootstrap_failed"); err == nil {
		t.Fatal("expected marker write failure")
	}
	fixture.fake.failMarkerWrite = false
	if err := fixture.runtime.reconcilePendingNomadQuarantine(context.Background(), fixture.request, profile, client, &state); err != nil {
		t.Fatal(err)
	}
	eligibilityCalls := 0
	for _, call := range fixture.fake.callLog() {
		if call == "node:eligibility" {
			eligibilityCalls++
		}
	}
	if eligibilityCalls != 1 || state.QuarantineState != nomad.QuarantineMarked {
		t.Fatalf("eligibility calls=%d state=%q", eligibilityCalls, state.QuarantineState)
	}
}

func TestNomadCrashWithQueuedEnvironmentQuarantinesWithoutSecondLaunch(t *testing.T) {
	fixture := newNomadCreateFixture(t, false)
	fixture.rejectBootstrap = true
	envA := otherNomadEnvState("ready", "")
	fixture.inventory = []nomad.EnvironmentState{envA}
	_, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	if err == nil || !strings.Contains(err.Error(), "bootstrap credential rejected") {
		t.Fatalf("CreateInstance() = %v, want the original bootstrap classification", err)
	}
	state, err := nomad.EnvironmentStateFromMetadata(fixture.request.Metadata)
	if err != nil || state.Phase != "cleanup_pending" || state.QuarantineState != "" {
		t.Fatalf("env B state = %+v, err=%v", state, err)
	}
	if fixture.registerCalls != 1 || fixture.createCalls != 0 || fixture.fake.eligibility == "ineligible" || fixture.fake.marker != "" {
		t.Fatalf("registrations=%d instances=%d eligibility=%s marker=%q", fixture.registerCalls, fixture.createCalls, fixture.fake.eligibility, fixture.fake.marker)
	}
	for _, checkpoint := range fixture.checkpoints {
		if checkpoint.EnvironmentID != fixture.request.TaskEnvironmentID {
			t.Fatalf("a checkpoint touched another environment: %+v", checkpoint)
		}
	}
	if fixture.inventory[0] != envA {
		t.Fatal("env A inventory was modified")
	}

	client, err := nomad.NewClient(fixture.profile.Address, "fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	profile := fixture.profile
	if err := fixture.runtime.quarantineWorker(context.Background(), fixture.request, profile, client, &state, "bootstrap_failed"); err != nil {
		t.Fatalf("quarantineWorker() = %v", err)
	}
	if fixture.fake.eligibility != "ineligible" || fixture.fake.marker == "" {
		t.Fatalf("eligibility=%s marker=%q", fixture.fake.eligibility, fixture.fake.marker)
	}
	fixture.inventory = append(fixture.inventory, state)

	fixture.request.TaskEnvironmentID, fixture.request.InstanceID = "environment-c", "execution-c"
	fixture.request.Metadata["nomad_environment_generation"] = "1"
	_, err = fixture.runtime.CreateInstance(context.Background(), fixture.request)
	assertQuarantined(t, err)
	if fixture.registerCalls != 1 {
		t.Fatalf("a second job was registered on a quarantined worker: %d", fixture.registerCalls)
	}
}

type nomadNonListingWriter struct{ ExecutorRunningWriter }

func TestNomadAdmissionFenceInventoryListerFailsClosedWithoutNomadLister(t *testing.T) {
	manager := &Manager{runningWriter: nomadNonListingWriter{}}
	states, err := manager.listNomadInventoryStates(context.Background())
	if err == nil || states != nil {
		t.Fatalf("listNomadInventoryStates() = %v, %v; want fail-closed error", states, err)
	}
}

func TestNomadIntentStateSnapshotsDisconnectWindowFromProfile(t *testing.T) {
	fixture := newNomadCreateFixture(t, false)
	id, err := resolveNomadLaunchIdentity(fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	id.profile.DisconnectLostAfter = 90 * time.Minute
	state := newNomadIntentState(fixture.request, id)
	if state.DisconnectLostAfterSeconds != 5400 {
		t.Fatalf("DisconnectLostAfterSeconds = %d, want 5400", state.DisconnectLostAfterSeconds)
	}
	if err := state.Validate(); err != nil {
		t.Fatalf("intent state with a configured window is invalid: %v", err)
	}
}

func TestNomadCanonicalProfileMetadataCarriesDisconnectWindow(t *testing.T) {
	profile := nomad.ProfileConfig{DisconnectLostAfter: 2 * time.Hour}
	if got := nomadCanonicalProfileMetadata(profile)[nomadMetadataKeyLostAfter]; got != "2h0m0s" {
		t.Fatalf("canonical metadata disconnect_lost_after = %q, want 2h0m0s", got)
	}
	if !isTrustedExecutorConfigKey(nomadMetadataKeyLostAfter) {
		t.Fatal("disconnect_lost_after must be a trusted profile-only key")
	}
}
