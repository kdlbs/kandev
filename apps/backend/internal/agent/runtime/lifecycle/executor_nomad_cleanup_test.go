package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/agent/nomad"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

const testCleanupClaim = "claim-1"

func runNomadCleanupFixture(f *nomadCreateFixture) error {
	return f.runtime.destroyNomadOperation(context.Background(), f.request, testCleanupClaim)
}

func TestNomadCleanupEvidenceDrainsThenStopsThenReleases(t *testing.T) {
	f := prepareNomadCleanupFixture(t)
	if err := runNomadCleanupFixture(f); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	calls := f.fake.callLog()
	order := []string{"drain", "job:stop", "ssh:fence-release", "inventory:release"}
	last := -1
	for _, want := range order {
		at := indexOfCall(calls, want)
		if at <= last {
			t.Fatalf("call %q out of order in %v", want, calls)
		}
		last = at
	}
	if got := f.lastCheckpoint(t); got.Phase != nomadPhaseAbsent || len(got.DrainReceiptDigest) != 64 {
		t.Fatalf("final checkpoint phase=%q digest=%q", got.Phase, got.DrainReceiptDigest)
	}
	if f.cleanup.drainCalls != 1 || f.cleanup.stopCalls != 1 {
		t.Fatalf("drain=%d stop=%d, want 1 each", f.cleanup.drainCalls, f.cleanup.stopCalls)
	}
}

func TestNomadCleanupEvidencePersistsReceiptBeforeJobStop(t *testing.T) {
	f := prepareNomadCleanupFixture(t)
	if err := runNomadCleanupFixture(f); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if len(f.cleanup.receiptAtStop) != 64 {
		t.Fatalf("job stop observed receipt digest %q, want it persisted first", f.cleanup.receiptAtStop)
	}
}

func TestNomadCleanupEvidenceRemovesOnlyOwnedBootstrapEnvironment(t *testing.T) {
	f := prepareNomadCleanupFixture(t)
	f.cleanup.failBootstrapRm = true
	if err := runNomadCleanupFixture(f); err != nil {
		t.Fatalf("bootstrap removal failure blocked cleanup: %v", err)
	}
	var removals []string
	for _, command := range f.cleanup.ssh {
		if strings.HasPrefix(command, "rm -f -- ") {
			removals = append(removals, command)
		}
	}
	state := f.lastCheckpoint(t)
	if len(removals) != 1 || !strings.Contains(removals[0], state.BootstrapPath+".env") {
		t.Fatalf("removals = %v, want exactly the owned bootstrap env file", removals)
	}
}

func TestNomadCleanupEvidenceDrainFailureQuarantinesAndNeverStops(t *testing.T) {
	f := prepareNomadCleanupFixture(t)
	f.cleanup.drainFail = true
	err := runNomadCleanupFixture(f)
	if !errors.Is(err, ErrNomadCleanupRetained) {
		t.Fatalf("cleanup = %v, want retained", err)
	}
	if f.cleanup.stopCalls != 0 || f.cleanup.releaseCalls != 0 {
		t.Fatalf("stop=%d release=%d after failed drain", f.cleanup.stopCalls, f.cleanup.releaseCalls)
	}
	state := f.lastCheckpoint(t)
	if state.Phase != nomadPhaseCleanupPending || state.QuarantineState == "" {
		t.Fatalf("phase=%q quarantine=%q, want cleanup_pending with quarantine recorded", state.Phase, state.QuarantineState)
	}
	if indexOfCall(f.fake.callLog(), "node:eligibility") < 0 {
		t.Fatalf("worker was not fenced: %v", f.fake.callLog())
	}
}

func TestNomadCleanupEvidenceServerIdentityMismatchNeverDrains(t *testing.T) {
	f := prepareNomadCleanupFixture(t)
	state, _ := nomad.EnvironmentStateFromMetadata(f.request.Metadata)
	state.ServerIdentity = "some-other-agentctl"
	if err := nomad.PutEnvironmentState(f.request.Metadata, state); err != nil {
		t.Fatal(err)
	}
	if err := runNomadCleanupFixture(f); !errors.Is(err, ErrNomadCleanupRetained) {
		t.Fatalf("cleanup = %v, want retained", err)
	}
	if f.cleanup.drainCalls != 0 || f.cleanup.stopCalls != 0 {
		t.Fatalf("drain=%d stop=%d for a changed runtime identity", f.cleanup.drainCalls, f.cleanup.stopCalls)
	}
}

func TestNomadCleanupEvidenceUnrecordedIdentityQuarantines(t *testing.T) {
	f := prepareNomadCleanupFixture(t)
	state, _ := nomad.EnvironmentStateFromMetadata(f.request.Metadata)
	state.ServerIdentity = ""
	if err := nomad.PutEnvironmentState(f.request.Metadata, state); err != nil {
		t.Fatal(err)
	}
	if err := runNomadCleanupFixture(f); !errors.Is(err, ErrNomadCleanupRetained) {
		t.Fatalf("cleanup = %v, want retained", err)
	}
	if f.cleanup.drainCalls != 0 || f.cleanup.stopCalls != 0 || f.lastCheckpoint(t).Phase != nomadPhaseCleanupPending {
		t.Fatalf("drain=%d stop=%d phase=%q", f.cleanup.drainCalls, f.cleanup.stopCalls, f.lastCheckpoint(t).Phase)
	}
}

func TestNomadCleanupEvidenceStopFailureRetainsPendingThenResumesWithoutDrain(t *testing.T) {
	f := prepareNomadCleanupFixture(t)
	f.cleanup.stopFail = true
	if err := runNomadCleanupFixture(f); !errors.Is(err, ErrNomadCleanupRetained) {
		t.Fatalf("cleanup = %v, want retained", err)
	}
	if got := f.lastCheckpoint(t); got.Phase != nomadPhaseCleanupPending || len(got.DrainReceiptDigest) != 64 {
		t.Fatalf("phase=%q digest=%q", got.Phase, got.DrainReceiptDigest)
	}
	f.cleanup.stopFail = false
	if err := runNomadCleanupFixture(f); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if f.cleanup.drainCalls != 1 {
		t.Fatalf("resume drained again: %d drain calls", f.cleanup.drainCalls)
	}
	if got := f.lastCheckpoint(t); got.Phase != nomadPhaseAbsent {
		t.Fatalf("phase = %q after resume", got.Phase)
	}
	if err := runNomadCleanupFixture(f); err != nil {
		t.Fatalf("repeat after absent: %v", err)
	}
	if f.cleanup.drainCalls != 1 || f.cleanup.stopCalls != 2 {
		t.Fatalf("idempotent repeat drain=%d stop=%d", f.cleanup.drainCalls, f.cleanup.stopCalls)
	}
}

func TestNomadCleanupEvidenceRunningAllocationIsNotAbsence(t *testing.T) {
	f := prepareNomadCleanupFixture(t)
	f.cleanup.allocationStatus = "running"
	f.runtime.stopConfirmTimeout = 50 * time.Millisecond
	if err := runNomadCleanupFixture(f); !errors.Is(err, ErrNomadCleanupRetained) {
		t.Fatalf("cleanup = %v, want retained", err)
	}
	for _, state := range f.checkpoints {
		if state.Phase == nomadPhaseAbsent {
			t.Fatal("absence was recorded without terminal allocation evidence")
		}
	}
	if f.cleanup.releaseCalls != 0 {
		t.Fatal("inventory was released without absence")
	}
}

func TestNomadCleanupEvidenceFenceReleaseFailureKeepsAbsentThenRetries(t *testing.T) {
	f := prepareNomadCleanupFixture(t)
	f.cleanup.failFenceRelease = true
	if err := runNomadCleanupFixture(f); !errors.Is(err, ErrNomadCleanupRetained) {
		t.Fatalf("cleanup = %v, want retained", err)
	}
	if f.lastCheckpoint(t).Phase != nomadPhaseAbsent || f.cleanup.releaseCalls != 0 {
		t.Fatalf("phase=%q release=%d", f.lastCheckpoint(t).Phase, f.cleanup.releaseCalls)
	}
	f.cleanup.failFenceRelease = false
	if err := runNomadCleanupFixture(f); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if f.cleanup.releaseCalls != 1 || f.cleanup.stopCalls != 1 || f.cleanup.drainCalls != 1 {
		t.Fatalf("release=%d stop=%d drain=%d", f.cleanup.releaseCalls, f.cleanup.stopCalls, f.cleanup.drainCalls)
	}
}

func TestNomadCleanupEvidenceResumeNeverDrains(t *testing.T) {
	f := prepareNomadCleanupFixture(t)
	f.cleanup.stopFail = true
	_ = runNomadCleanupFixture(f)
	f.cleanup.stopFail = false
	if err := f.runtime.resumeNomadCleanup(context.Background(), f.request, testCleanupClaim); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if f.cleanup.drainCalls != 1 || f.lastCheckpoint(t).Phase != nomadPhaseAbsent {
		t.Fatalf("drain=%d phase=%q", f.cleanup.drainCalls, f.lastCheckpoint(t).Phase)
	}
}

func TestNomadCleanupEvidenceRequiresClaim(t *testing.T) {
	f := prepareNomadCleanupFixture(t)
	if err := f.runtime.destroyNomadOperation(context.Background(), f.request, ""); err == nil {
		t.Fatal("cleanup ran without a claim")
	}
	if f.cleanup.drainCalls != 0 || len(f.checkpoints) != 0 {
		t.Fatalf("writes without a claim: drain=%d checkpoints=%d", f.cleanup.drainCalls, len(f.checkpoints))
	}
}

func TestNomadCleanupEvidenceLogsSanitized(t *testing.T) {
	f := prepareNomadCleanupFixture(t)
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatal(err)
	}
	f.runtime.logger = log
	f.cleanup.drainFail = true
	_ = runNomadCleanupFixture(f)
	for _, entry := range logs.All() {
		rendered := entry.Message
		for _, field := range entry.Context {
			rendered += " " + field.Key + "=" + field.String
		}
		for _, secret := range []string{"fixture-token", "fixture-instance-token", "worker.test", "drain failed"} {
			if strings.Contains(rendered, secret) {
				t.Fatalf("log %q leaked %q", rendered, secret)
			}
		}
	}
	if logs.FilterMessage("nomad cleanup retained").Len() == 0 {
		t.Fatal("retained cleanup was not logged")
	}
}

type recordingCleanupCoordinator struct {
	instances, launches, resumes int
	err                          error
}

func (c *recordingCleanupCoordinator) DestroyInstance(context.Context, *ExecutorInstance) error {
	c.instances++
	return c.err
}

func (c *recordingCleanupCoordinator) DestroyLaunch(context.Context, *ExecutorCreateRequest) error {
	c.launches++
	return c.err
}

func (c *recordingCleanupCoordinator) ResumeCleanup(context.Context, *models.ExecutorRunning) (bool, error) {
	c.resumes++
	return false, nil
}

func TestNomadCleanupEvidenceStopInstanceRoutesByReason(t *testing.T) {
	instance := nomadStopTestInstance("operation-3", "task-3", "session-3", "allocation-3")
	coordinator := &recordingCleanupCoordinator{}
	runtime := NewNomadExecutor(nil, nil, nil)
	runtime.setCleanupCoordinator(coordinator)
	runtime.sessions[instance.InstanceID] = &nomadAgentctlConnection{instance: instance}

	if err := runtime.StopInstance(context.Background(), instance, false); err != nil || coordinator.instances != 0 {
		t.Fatalf("empty-reason stop = %v, destroy calls %d, want transport-only", err, coordinator.instances)
	}
	instance.StopReason = StopReasonSessionArchived
	if err := runtime.StopInstance(context.Background(), instance, false); err != nil || coordinator.instances != 1 {
		t.Fatalf("archive stop = %v, destroy calls %d, want 1", err, coordinator.instances)
	}
	coordinator.err = ErrNomadCleanupRetained
	if err := runtime.StopInstance(context.Background(), instance, true); !errors.Is(err, ErrNomadCleanupRetained) {
		t.Fatalf("force stop = %v, want retained error surfaced", err)
	}
}

type claimStoreWriter struct {
	ExecutorRunningWriter
	rows     []*models.ExecutorRunning
	acquired []models.TaskEnvironmentRecoveryClaimRequest
	released int
}

func (w *claimStoreWriter) ListExecutorsRunningNomad(context.Context) ([]*models.ExecutorRunning, error) {
	return w.rows, nil
}

func (w *claimStoreWriter) AcquireTaskEnvironmentRecoveryClaim(_ context.Context, req models.TaskEnvironmentRecoveryClaimRequest) (*models.TaskEnvironmentRecoveryClaim, error) {
	w.acquired = append(w.acquired, req)
	return &models.TaskEnvironmentRecoveryClaim{TaskEnvironmentID: req.TaskEnvironmentID, OperationID: req.OperationID}, nil
}

func (w *claimStoreWriter) ReleaseTaskEnvironmentRecoveryClaim(context.Context, *models.TaskEnvironmentRecoveryClaim) error {
	w.released++
	return nil
}

func TestNomadCleanupEvidenceEnvironmentWithoutRetainedRowsIsNoop(t *testing.T) {
	writer := &claimStoreWriter{}
	manager := &Manager{runningWriter: writer, logger: newTestLogger(), executorRegistry: NewExecutorRegistry(newTestRegistryLogger())}
	manager.executorRegistry.Register(NewNomadExecutor(nil, nil, nil))
	if err := manager.DestroyNomadEnvironment(context.Background(), "environment-1"); err != nil {
		t.Fatalf("DestroyNomadEnvironment() = %v, want no-op", err)
	}
	if len(writer.acquired) != 0 {
		t.Fatalf("acquired %d claims for an environment without retained rows", len(writer.acquired))
	}
}

func TestNomadCleanupEvidenceClaimIsDeterministicAndReleased(t *testing.T) {
	f := prepareNomadCleanupFixture(t)
	writer := &claimStoreWriter{}
	manager := &Manager{runningWriter: writer, logger: newTestLogger()}
	var seen string
	run := func() {
		err := manager.withNomadCleanupClaim(context.Background(), f.request, func(ctx context.Context, claimID string) error {
			seen = claimID
			if held := recoveryclaim.ClaimFromContext(ctx); held == nil || held.OperationID != claimID {
				t.Fatal("claim was not carried in the cleanup context")
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	run()
	first := seen
	run()
	if first != seen || !strings.HasSuffix(first, nomadCleanupClaimSuffix) || writer.released != 2 {
		t.Fatalf("claim %q then %q, released %d", first, seen, writer.released)
	}
}

type rereadClaimWriter struct {
	claimStoreWriter
	row *models.ExecutorRunning
}

func (w *rereadClaimWriter) GetExecutorRunningBySessionID(context.Context, string) (*models.ExecutorRunning, error) {
	return w.row, nil
}

func TestNomadCleanupEvidenceClaimedCleanupReReadsCompetingHoldersAbsence(t *testing.T) {
	f := prepareNomadCleanupFixture(t)
	state, err := nomad.EnvironmentStateFromMetadata(f.request.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	state.Phase = nomadPhaseAbsent
	state.DrainReceiptDigest = strings.Repeat("a", 64)
	state.DrainReceiptGeneration = state.Generation
	state.Revision++
	metadata := cloneRemoteRecoveryRecord(&models.ExecutorRunning{Metadata: f.request.Metadata}).Metadata
	if err := nomad.PutEnvironmentState(metadata, state); err != nil {
		t.Fatal(err)
	}
	writer := &rereadClaimWriter{row: &models.ExecutorRunning{
		SessionID: f.request.SessionID, TaskID: f.request.TaskID, AgentExecutionID: f.request.InstanceID,
		Runtime: agentruntime.RuntimeNomad, Metadata: metadata,
	}}
	manager := &Manager{runningWriter: writer, logger: newTestLogger()}
	err = manager.withFreshNomadCleanupClaim(context.Background(), f.request,
		func(_ context.Context, _ string, fresh *ExecutorCreateRequest) error {
			got, err := nomad.EnvironmentStateFromMetadata(fresh.Metadata)
			if err != nil {
				return err
			}
			if got.Phase != nomadPhaseAbsent || got.DrainReceiptDigest != state.DrainReceiptDigest {
				t.Fatalf("claimed request phase=%q receipt=%q, want the competing holder's absence", got.Phase, got.DrainReceiptDigest)
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	writer.row.AgentExecutionID = "another-execution"
	err = manager.withFreshNomadCleanupClaim(context.Background(), f.request,
		func(context.Context, string, *ExecutorCreateRequest) error {
			t.Fatal("rotated execution must not run cleanup")
			return nil
		})
	if !errors.Is(err, models.ErrExecutionRotated) {
		t.Fatalf("rotated execution error = %v", err)
	}
}

func TestNomadCleanupEvidenceAbsentFenceIsClassifiedAndQuarantined(t *testing.T) {
	f := prepareNomadCleanupFixture(t)
	f.fenceOK, f.fenceAbsent = false, true
	err := runNomadCleanupFixture(f)
	if !errors.Is(err, ErrNomadCleanupRetained) || !strings.Contains(err.Error(), nomadCleanupReasonFenceAbsent) {
		t.Fatalf("cleanup = %v, want retained with %s", err, nomadCleanupReasonFenceAbsent)
	}
	if f.cleanup.drainCalls != 0 || f.cleanup.stopCalls != 0 {
		t.Fatalf("drain=%d stop=%d without an owner record", f.cleanup.drainCalls, f.cleanup.stopCalls)
	}
	if state := f.lastCheckpoint(t); state.QuarantineState == "" {
		t.Fatal("absent owner record must leave the worker quarantined")
	}
}

func TestNomadCleanupEvidenceHandoffStopsAreTransportOnly(t *testing.T) {
	for _, reason := range []string{StopReasonCascadeArchive, StopReasonCascadeDelete, StopReasonTaskTreeArchived, StopReasonTaskTreeDeleted, ""} {
		for _, force := range []bool{false, true} {
			instance := nomadStopTestInstance("operation-h", "task-h", "session-h", "allocation-h")
			instance.StopReason = reason
			coordinator := &recordingCleanupCoordinator{}
			runtime := NewNomadExecutor(nil, nil, nil)
			runtime.setCleanupCoordinator(coordinator)
			runtime.sessions[instance.InstanceID] = &nomadAgentctlConnection{instance: instance}
			if err := runtime.StopInstance(context.Background(), instance, force); err != nil || coordinator.instances != 0 {
				t.Fatalf("reason %q force %v: err=%v destroy calls=%d, want transport-only", reason, force, err, coordinator.instances)
			}
			if _, kept := runtime.sessions[instance.InstanceID]; kept {
				t.Fatalf("reason %q force %v: transport was not detached", reason, force)
			}
		}
	}
}

func TestNomadCleanupEvidenceCascadeForceStopLeavesFenceForCleanupJob(t *testing.T) {
	f := prepareNomadCleanupFixture(t)
	instance := nomadStopTestInstance(f.request.InstanceID, f.request.TaskID, f.request.SessionID, "alloc-1")
	instance.StopReason = StopReasonCascadeArchive
	if err := f.runtime.StopInstance(context.Background(), instance, true); err != nil {
		t.Fatalf("cascade force stop: %v", err)
	}
	if calls := f.fake.callLog(); indexOfCall(calls, "ssh:fence-release") >= 0 || indexOfCall(calls, "job:stop") >= 0 || f.cleanup.drainCalls != 0 {
		t.Fatalf("cascade stop touched the runtime outside the claim: %v drain=%d", calls, f.cleanup.drainCalls)
	}
	if err := runNomadCleanupFixture(f); err != nil {
		t.Fatalf("cleanup job after cascade stop: %v", err)
	}
	if got := f.lastCheckpoint(t); got.Phase != nomadPhaseAbsent || len(got.DrainReceiptDigest) != 64 || f.cleanup.drainCalls != 1 {
		t.Fatalf("phase=%q receipt=%q drain=%d, want absent with one drain receipt", got.Phase, got.DrainReceiptDigest, f.cleanup.drainCalls)
	}
}

func TestNomadCleanupEvidenceDuplicateCallerLeavesInFlightClaimHeld(t *testing.T) {
	f := prepareNomadCleanupFixture(t)
	writer := &claimStoreWriter{}
	manager := &Manager{runningWriter: writer, logger: newTestLogger()}
	err := manager.withNomadCleanupClaim(context.Background(), f.request, func(ctx context.Context, _ string) error {
		dupErr := manager.withNomadCleanupClaim(context.Background(), f.request, func(context.Context, string) error {
			t.Fatal("duplicate cleanup must not run")
			return nil
		})
		if dupErr == nil || !strings.Contains(dupErr.Error(), "already running") {
			t.Fatalf("duplicate cleanup = %v, want already running", dupErr)
		}
		if writer.released != 0 || len(writer.acquired) != 1 {
			t.Fatalf("duplicate touched the shared claim: acquired %d, released %d", len(writer.acquired), writer.released)
		}
		return nil
	})
	if err != nil || writer.released != 1 {
		t.Fatalf("in-flight cleanup = %v, released %d, want one release", err, writer.released)
	}
	if err := manager.withNomadCleanupClaim(context.Background(), f.request, func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("cleanup after completion = %v", err)
	}
}
