package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/nomad"
	"github.com/kandev/kandev/internal/task/models"
)

type readmitStoreFake struct {
	*nomadInventoryStoreFake
	phases       []string
	deletes      int
	claimErr     error
	claimsTaken  int
	claimsClosed int
}

func (f *readmitStoreFake) CheckpointNomadExecutorInventory(ctx context.Context, running *models.ExecutorRunning) error {
	if state, err := nomad.EnvironmentStateFromMetadata(running.Metadata); err == nil {
		f.phases = append(f.phases, state.Phase)
	}
	return f.nomadInventoryStoreFake.CheckpointNomadExecutorInventory(ctx, running)
}

func (f *readmitStoreFake) DeleteExecutorRunningBySessionID(context.Context, string) error {
	f.deletes++
	return nil
}

func (f *readmitStoreFake) ListExecutorsRunningNomad(context.Context) ([]*models.ExecutorRunning, error) {
	if f.row == nil {
		return nil, nil
	}
	return []*models.ExecutorRunning{f.row}, nil
}

func (f *readmitStoreFake) AcquireTaskEnvironmentRecoveryClaim(_ context.Context, req models.TaskEnvironmentRecoveryClaimRequest) (*models.TaskEnvironmentRecoveryClaim, error) {
	if f.claimErr != nil {
		return nil, f.claimErr
	}
	f.claimsTaken++
	return &models.TaskEnvironmentRecoveryClaim{TaskEnvironmentID: req.TaskEnvironmentID, OperationID: req.OperationID}, nil
}

func (f *readmitStoreFake) ReleaseTaskEnvironmentRecoveryClaim(context.Context, *models.TaskEnvironmentRecoveryClaim) error {
	f.claimsClosed++
	return nil
}

func newReadmitHarness(t *testing.T, phase string) (*Manager, *readmitStoreFake) {
	t.Helper()
	state := nomadInventoryState("execution-1", phase)
	state.Revision = 3
	switch phase {
	case "absent":
		state.DrainReceiptDigest, state.DrainReceiptGeneration = strings.Repeat("b", 64), state.Generation
	case "operator_released":
		state.OperatorAttestationDigest = strings.Repeat("c", 64)
	}
	metadata := map[string]interface{}{MetadataKeyExecutorType: string(models.ExecutorTypeNomad)}
	if err := nomad.PutEnvironmentState(metadata, state); err != nil {
		t.Fatal(err)
	}
	store := &readmitStoreFake{nomadInventoryStoreFake: &nomadInventoryStoreFake{row: &models.ExecutorRunning{
		SessionID: "session-1", TaskID: "task-1", AgentExecutionID: "execution-1", Runtime: "nomad", Metadata: metadata,
	}}}
	return &Manager{runningWriter: store}, store
}

func readmitRequest() nomad.NomadReadmitRequest {
	return nomad.NomadReadmitRequest{
		NodeID: "node-1", EnvironmentID: "environment-1", Generation: 1, JobID: "job-1",
		Attestation: nomad.OperatorAttestation{ProcessTreeAbsent: true, JobStoppedPurgeFalse: true, FenceRemoved: true, Operator: "operator-a"},
	}
}

func TestNomadWorkerQuarantineReadmitRequiresAttestation(t *testing.T) {
	cases := map[string]func(*nomad.NomadReadmitRequest){
		"process tree":  func(r *nomad.NomadReadmitRequest) { r.Attestation.ProcessTreeAbsent = false },
		"job stopped":   func(r *nomad.NomadReadmitRequest) { r.Attestation.JobStoppedPurgeFalse = false },
		"fence removed": func(r *nomad.NomadReadmitRequest) { r.Attestation.FenceRemoved = false },
		"operator":      func(r *nomad.NomadReadmitRequest) { r.Attestation.Operator = "  " },
		"generation":    func(r *nomad.NomadReadmitRequest) { r.Generation = 0 },
		"control chars": func(r *nomad.NomadReadmitRequest) { r.Attestation.Operator = "op\nerator" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			manager, store := newReadmitHarness(t, "quarantined")
			req := readmitRequest()
			mutate(&req)
			if err := manager.ReadmitNomadEnvironment(context.Background(), req); !errors.Is(err, nomad.ErrNomadReadmitInvalid) {
				t.Fatalf("err = %v, want nomad.ErrNomadReadmitInvalid", err)
			}
			if store.checkpoints != 0 || store.claimsTaken != 0 {
				t.Fatalf("refused request wrote: checkpoints=%d claims=%d", store.checkpoints, store.claimsTaken)
			}
		})
	}
}

func TestNomadWorkerQuarantineReadmitExactRowOnly(t *testing.T) {
	for name, mutate := range map[string]func(*nomad.NomadReadmitRequest){
		"node":        func(r *nomad.NomadReadmitRequest) { r.NodeID = "node-2" },
		"empty node":  func(r *nomad.NomadReadmitRequest) { r.NodeID = "" },
		"environment": func(r *nomad.NomadReadmitRequest) { r.EnvironmentID = "environment-2" },
		"generation":  func(r *nomad.NomadReadmitRequest) { r.Generation = 2 },
		"job":         func(r *nomad.NomadReadmitRequest) { r.JobID = "job-2" },
	} {
		t.Run(name, func(t *testing.T) {
			manager, store := newReadmitHarness(t, "cleanup_pending")
			req := readmitRequest()
			mutate(&req)
			if err := manager.ReadmitNomadEnvironment(context.Background(), req); !errors.Is(err, nomad.ErrNomadReadmitNotFound) {
				t.Fatalf("err = %v, want nomad.ErrNomadReadmitNotFound", err)
			}
			if store.checkpoints != 0 {
				t.Fatal("mismatched identity checkpointed a row")
			}
		})
	}
	for _, phase := range []string{"intent", "registered", "allocated", "ready", "absent", "operator_released"} {
		t.Run("phase "+phase, func(t *testing.T) {
			manager, store := newReadmitHarness(t, phase)
			if err := manager.ReadmitNomadEnvironment(context.Background(), readmitRequest()); !errors.Is(err, nomad.ErrNomadReadmitPhase) {
				t.Fatalf("err = %v, want nomad.ErrNomadReadmitPhase", err)
			}
			if store.checkpoints != 0 {
				t.Fatal("ineligible phase was checkpointed")
			}
		})
	}
}

func TestNomadWorkerQuarantineReadmitNeverWritesAbsent(t *testing.T) {
	for _, phase := range []string{"cleanup_pending", "quarantined"} {
		manager, store := newReadmitHarness(t, phase)
		if err := manager.ReadmitNomadEnvironment(context.Background(), readmitRequest()); err != nil {
			t.Fatalf("%s: readmit: %v", phase, err)
		}
		if len(store.phases) != 1 || store.phases[0] != "operator_released" {
			t.Fatalf("%s: written phases = %v, want only operator_released", phase, store.phases)
		}
		state, err := nomad.EnvironmentStateFromMetadata(store.row.Metadata)
		if err != nil || state.Phase != "operator_released" || state.DrainReceiptDigest != "" || state.Revision != 4 {
			t.Fatalf("%s: stored state = %+v, %v", phase, state, err)
		}
		want := nomad.OperatorAttestationDigest(state, readmitRequest().Attestation)
		if state.OperatorAttestationDigest != want {
			t.Fatalf("%s: attestation digest does not bind the exact attestation", phase)
		}
		if encoded, _ := json.Marshal(store.row.Metadata); strings.Contains(string(encoded), "operator-a") {
			t.Fatalf("%s: operator name persisted in clear", phase)
		}
	}
}

func TestNomadWorkerQuarantineReadmitKeepsRetainedRow(t *testing.T) {
	manager, store := newReadmitHarness(t, "quarantined")
	if err := manager.ReadmitNomadEnvironment(context.Background(), readmitRequest()); err != nil {
		t.Fatal(err)
	}
	if store.row == nil || store.deletes != 0 {
		t.Fatalf("readmission removed the retained row: row=%v deletes=%d", store.row, store.deletes)
	}
	if store.claimsTaken != 1 || store.claimsClosed != 1 {
		t.Fatalf("claims taken/released = %d/%d, want 1/1", store.claimsTaken, store.claimsClosed)
	}
	if err := manager.releaseNomadRuntimeInventory(context.Background(), &ExecutorCreateRequest{
		InstanceID: "execution-1", SessionID: "session-1", TaskEnvironmentID: "environment-1",
	}); err == nil {
		t.Fatal("an operator release was accepted as proof of absence")
	}
}

func TestNomadWorkerQuarantineReadmitFailsClosedWithoutClaim(t *testing.T) {
	manager, store := newReadmitHarness(t, "quarantined")
	store.claimErr = errors.New("claim held")
	if err := manager.ReadmitNomadEnvironment(context.Background(), readmitRequest()); err == nil {
		t.Fatal("readmit proceeded without the environment claim")
	}
	if store.checkpoints != 0 {
		t.Fatal("readmit checkpointed without the claim")
	}
}

func TestNomadWorkerQuarantineOperatorReleasedUnblocksAdmissionButNotMarker(t *testing.T) {
	released := otherNomadEnvState("operator_released", "marked")
	released.OperatorAttestationDigest = strings.Repeat("c", 64)
	fixture, client, profile := admissionHarness(t)
	fixture.inventory = []nomad.EnvironmentState{released}
	if err := fixture.runtime.admitNewNomadOperation(context.Background(), fixture.request, profile, client); err != nil {
		t.Fatalf("operator_released row blocked admission: %v", err)
	}
	fixture, client, profile = admissionHarness(t)
	fixture.inventory = []nomad.EnvironmentState{released}
	fixture.fake.marker = `{"version":1}`
	assertQuarantined(t, fixture.runtime.admitNewNomadOperation(context.Background(), fixture.request, profile, client))
	fixture, client, profile = admissionHarness(t)
	fixture.inventory = []nomad.EnvironmentState{released}
	fixture.fake.eligibility = "ineligible"
	assertQuarantined(t, fixture.runtime.admitNewNomadOperation(context.Background(), fixture.request, profile, client))
}

func TestNomadWorkerQuarantineOperatorReleasedIsNeitherCleanedNorResumed(t *testing.T) {
	manager, store := newReadmitHarness(t, "cleanup_pending")
	if err := manager.ReadmitNomadEnvironment(context.Background(), readmitRequest()); err != nil {
		t.Fatal(err)
	}
	records, err := manager.nomadRecordsForEnvironment(context.Background(), "environment-1")
	if err != nil || len(records) != 0 {
		t.Fatalf("destroy would target a released row: %d, %v", len(records), err)
	}
	state, _ := nomad.EnvironmentStateFromMetadata(store.row.Metadata)
	state.QuarantineState = nomad.QuarantinePending
	if nomadCleanupResumable(state) {
		t.Fatal("a released row with a stale pending interlock is resumable")
	}
}

// claimHoldingStore models the real claim authority: a different operation id
// is rejected and release deletes only the exact operation.
type claimHoldingStore struct {
	*readmitStoreFake
	held     string
	acquired []string
}

func (f *claimHoldingStore) AcquireTaskEnvironmentRecoveryClaim(_ context.Context, req models.TaskEnvironmentRecoveryClaimRequest) (*models.TaskEnvironmentRecoveryClaim, error) {
	f.acquired = append(f.acquired, req.OperationID)
	if f.held != "" && f.held != req.OperationID {
		return nil, errors.New("environment is claimed by another operation")
	}
	f.held = req.OperationID
	return &models.TaskEnvironmentRecoveryClaim{TaskEnvironmentID: req.TaskEnvironmentID, OperationID: req.OperationID}, nil
}

func (f *claimHoldingStore) ReleaseTaskEnvironmentRecoveryClaim(_ context.Context, claim *models.TaskEnvironmentRecoveryClaim) error {
	if f.held == claim.OperationID {
		f.held = ""
	}
	return nil
}

func TestNomadWorkerQuarantineReadmitDoesNotShareCleanupClaim(t *testing.T) {
	manager, base := newReadmitHarness(t, "quarantined")
	store := &claimHoldingStore{readmitStoreFake: base}
	manager.runningWriter = store
	state, err := nomad.EnvironmentStateFromMetadata(base.row.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	cleanupClaim := state.OperationID + nomadCleanupClaimSuffix
	store.held = cleanupClaim

	if err := manager.ReadmitNomadEnvironment(context.Background(), readmitRequest()); err == nil {
		t.Fatal("readmit proceeded while a cleanup held the environment claim")
	}
	if store.held != cleanupClaim || base.checkpoints != 0 {
		t.Fatalf("cleanup claim = %q checkpoints = %d", store.held, base.checkpoints)
	}
	if len(store.acquired) != 1 || store.acquired[0] == cleanupClaim || !strings.HasSuffix(store.acquired[0], nomadReadmitClaimSuffix) {
		t.Fatalf("readmit claim identity = %v", store.acquired)
	}
}

func TestNomadWorkerQuarantineReadmitPhaseErrorReleasesOnlyItsOwnClaim(t *testing.T) {
	manager, base := newReadmitHarness(t, "operator_released")
	store := &claimHoldingStore{readmitStoreFake: base}
	manager.runningWriter = store
	err := manager.ReadmitNomadEnvironment(context.Background(), readmitRequest())
	if !errors.Is(err, nomad.ErrNomadReadmitPhase) {
		t.Fatalf("err = %v, want phase error", err)
	}
	if store.held != "" || base.checkpoints != 0 {
		t.Fatalf("held = %q checkpoints = %d", store.held, base.checkpoints)
	}
}

func TestNomadReadmitReleasesNeverPlacedPoolRowWithEmptyNode(t *testing.T) {
	manager, store := newReadmitHarness(t, "cleanup_pending")
	state, err := nomad.EnvironmentStateFromMetadata(store.row.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	state.PlacementMode, state.NodePool, state.Placement, state.MemoryMaxMB = nomad.ModePool, "kandev", nomad.PlacementSpread, 2048
	state.NodeID, state.AgentctlPath, state.BootstrapPath, state.WorkspaceRoot, state.AllocationRoot = "", "", "", "", ""
	state.WorkspacePath, state.NativeJobPolicyVersion, state.NativeJobSpecSHA256 = "", 0, ""
	state.SSHHost, state.SSHUser, state.SSHPort, state.SSHHostFingerprint = "", "", 0, ""
	state.StateRoot, state.NodeContractSHA256, state.NodeAttach, state.AllocationID = "", "", "", ""
	if err := nomad.PutEnvironmentState(store.row.Metadata, state); err != nil {
		t.Fatal(err)
	}
	req := readmitRequest()
	req.NodeID = ""
	if err := manager.ReadmitNomadEnvironment(context.Background(), req); err != nil {
		t.Fatalf("readmit never-placed row = %v", err)
	}
	if store.checkpoints != 1 {
		t.Fatalf("checkpoints = %d, want 1", store.checkpoints)
	}
}
