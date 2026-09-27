package plugins

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

type completionGateRecordingWriter struct {
	*fakeTaskWriter
	criteriaCalls int
	evidenceCalls int
	lastCriteria  ExactTaskCompletionCriteriaInput
	lastEvidence  ExactTaskCompletionEvidenceInput
}

func (w *completionGateRecordingWriter) SetTaskCompletionCriteriaExact(_ context.Context, input ExactTaskCompletionCriteriaInput) (*models.TaskCompletionGateSnapshot, bool, error) {
	w.criteriaCalls++
	w.lastCriteria = input
	criteria := make([]models.TaskCompletionCriterion, 0, len(input.Criteria))
	for _, item := range input.Criteria {
		item.CriterionRevision = 1
		criteria = append(criteria, item)
	}
	return &models.TaskCompletionGateSnapshot{
		TaskID: input.TaskID, WorkspaceID: input.WorkspaceID,
		Revision: input.ExpectedRevision + 1, Criteria: criteria,
		Blockers: []models.TaskCompletionBlocker{{CriterionID: criteria[0].ID, Reason: "unverified"}}, Blocked: true,
	}, w.criteriaCalls > 1, nil
}

func (w *completionGateRecordingWriter) VerifyTaskCompletionCriterionExact(_ context.Context, input ExactTaskCompletionEvidenceInput) (*models.TaskCompletionGateSnapshot, bool, error) {
	w.evidenceCalls++
	w.lastEvidence = input
	return &models.TaskCompletionGateSnapshot{
		TaskID: input.TaskID, WorkspaceID: input.WorkspaceID, Revision: input.ExpectedRevision,
		Criteria: []models.TaskCompletionCriterion{{ID: input.CriterionID, Evidence: &input.Evidence}},
	}, w.evidenceCalls > 1, nil
}

func TestExactTaskCompletionGateCommandsAreApprovedFencedAndReplayable(t *testing.T) {
	host, _ := newExactTaskCommandHost(t)
	writer := &completionGateRecordingWriter{fakeTaskWriter: &fakeTaskWriter{}}
	host.taskWriter = writer
	manager, ok := pluginsdk.HostTaskCompletionGates(host)
	if !ok {
		t.Fatal("plugin Host does not expose task completion gate commands")
	}
	manifestDigest := ManifestCapabilityDigest(host.service.installedRecordByInstallationID(host.installationID).Manifest)
	criteria := pluginsdk.ExactTaskCompletionCriteria{
		RequestID: "request-gate-set", WorkspaceID: "workspace-task-commands", TaskID: "task-gated",
		IdempotencyKey: "gate-set-1", ExpectedTaskResourceVersion: "2026-09-26T12:00:00Z", ExpectedRevision: 0,
		ApprovalRevision: 1, ManifestDigest: manifestDigest,
		ManagementInstanceKey: "manager", ExpectedClaimGeneration: 4,
		Criteria: []pluginsdk.TaskCompletionCriterionInput{{
			ID: "checks", Description: "Checks pass",
			EvidenceSubject: pluginsdk.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidenceArtifact, ID: "artifact-1"},
		}},
	}
	result, gate, err := manager.SetCriteria(context.Background(), criteria)
	if err != nil || result == nil || result.Status != pluginsdk.CommandApplied || gate == nil || !gate.Blocked {
		t.Fatalf("set exact completion criteria = result:%+v gate:%+v err:%v", result, gate, err)
	}
	if writer.criteriaCalls != 1 || writer.lastCriteria.OperationID == "" ||
		writer.lastCriteria.ClaimFence.InstallationID != host.installationID ||
		writer.lastCriteria.ClaimFence.InstanceKey != criteria.ManagementInstanceKey ||
		writer.lastCriteria.ClaimFence.Generation != criteria.ExpectedClaimGeneration {
		t.Fatalf("criteria writer admission = calls:%d input:%+v", writer.criteriaCalls, writer.lastCriteria)
	}

	replayed, replayGate, err := manager.SetCriteria(context.Background(), criteria)
	if err != nil || replayed == nil || replayed.Status != pluginsdk.CommandApplied || replayGate == nil || replayGate.Revision != gate.Revision || writer.criteriaCalls != 2 {
		t.Fatalf("replayed completion criteria = result:%+v gate:%+v calls:%d err:%v", replayed, replayGate, writer.criteriaCalls, err)
	}

	evidence := pluginsdk.ExactTaskCompletionEvidence{
		RequestID: "request-gate-evidence", WorkspaceID: criteria.WorkspaceID, TaskID: criteria.TaskID,
		CriterionID: "checks", IdempotencyKey: "gate-evidence-1", ExpectedTaskResourceVersion: criteria.ExpectedTaskResourceVersion,
		ExpectedRevision: 1, ApprovalRevision: 1, ManifestDigest: manifestDigest,
		ManagementInstanceKey: criteria.ManagementInstanceKey, ExpectedClaimGeneration: criteria.ExpectedClaimGeneration,
		Evidence: pluginsdk.TaskCompletionEvidence{
			Subject: pluginsdk.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidenceArtifact, ID: "artifact-1", Revision: "run-v1"},
			Summary: "Checks passed.", Reference: "run-17",
		},
	}
	verified, verifiedGate, err := manager.Verify(context.Background(), evidence)
	if err != nil || verified == nil || verified.Status != pluginsdk.CommandApplied || verifiedGate == nil || verifiedGate.Criteria[0].Evidence.Subject.Revision != "run-v1" {
		t.Fatalf("verify exact completion evidence = result:%+v gate:%+v err:%v", verified, verifiedGate, err)
	}
	if writer.evidenceCalls != 1 || writer.lastEvidence.OperationID == "" || writer.lastEvidence.Evidence.Reference != "run-17" {
		t.Fatalf("evidence writer admission = calls:%d input:%+v", writer.evidenceCalls, writer.lastEvidence)
	}

	capability, err := host.GetCapabilityContext(context.Background(), criteria.WorkspaceID)
	if err != nil {
		t.Fatalf("get capability context: %v", err)
	}
	for _, operation := range capability.Operations {
		if operation.Method == exactTaskCompletionCriteriaMethod || operation.Method == exactTaskCompletionEvidenceMethod {
			if !operation.Supported || !operation.Authorized || operation.CapabilityID != "host.v2.write:tasks" {
				t.Fatalf("completion gate operation is unavailable: %+v", operation)
			}
		}
	}
}

func TestExactTaskCompletionGateRejectsDuplicateCriteriaBeforeAdmission(t *testing.T) {
	host, _ := newExactTaskCommandHost(t)
	writer := &completionGateRecordingWriter{fakeTaskWriter: &fakeTaskWriter{}}
	host.taskWriter = writer
	manager, _ := pluginsdk.HostTaskCompletionGates(host)
	result, _, err := manager.SetCriteria(context.Background(), pluginsdk.ExactTaskCompletionCriteria{
		RequestID: "request-gate-invalid", WorkspaceID: "workspace-task-commands", TaskID: "task-gated",
		IdempotencyKey: "gate-invalid", ExpectedTaskResourceVersion: "2026-09-26T12:00:00Z",
		ApprovalRevision: 1, ManifestDigest: ManifestCapabilityDigest(host.service.installedRecordByInstallationID(host.installationID).Manifest),
		ManagementInstanceKey: "manager", ExpectedClaimGeneration: 1,
		Criteria: []pluginsdk.TaskCompletionCriterionInput{
			{ID: "duplicate", Description: "First", EvidenceSubject: pluginsdk.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidenceArtifact, ID: "artifact-1"}},
			{ID: "duplicate", Description: "Second", EvidenceSubject: pluginsdk.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidenceArtifact, ID: "artifact-2"}},
		},
	})
	if err != nil || result == nil || result.Status != pluginsdk.CommandInvalid || writer.criteriaCalls != 0 {
		t.Fatalf("duplicate criteria result:%+v calls:%d err:%v", result, writer.criteriaCalls, err)
	}
}
