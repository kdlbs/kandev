package journal

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/common/processidentity"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// RetainedRecoveryRequest is accepted only by the authenticated runtime control server.
// It reads an existing owner journal without allocating or starting an instance.
type RetainedRecoveryRequest struct {
	Root              string                   `json:"root"`
	SessionID         string                   `json:"session_id"`
	ExecutionID       string                   `json:"execution_id"`
	IncarnationID     string                   `json:"incarnation_id"`
	HarnessGeneration uint64                   `json:"harness_generation"`
	StreamID          string                   `json:"stream_id"`
	SubmissionID      string                   `json:"submission_id"`
	OriginalRuntime   processidentity.Identity `json:"original_runtime"`
	After             uint64                   `json:"after"`
	Limit             int                      `json:"limit"`
	Acknowledge       uint64                   `json:"acknowledge,omitempty"`
}

type RetainedRecoveryResponse struct {
	Descriptor        RecoveryDescriptor `json:"descriptor"`
	Submission        Submission         `json:"submission"`
	Events            []Event            `json:"events"`
	ProcessTerminated bool               `json:"process_terminated"`
}

// RetainedReconstructionEvidenceRequest reads the complete bounded candidate
// set for one retained session. Process identity is optional because older
// records do not always contain enough information to prove termination.
type RetainedReconstructionEvidenceRequest struct {
	Root              string                    `json:"root"`
	SessionID         string                    `json:"session_id"`
	IncarnationID     string                    `json:"incarnation_id"`
	HarnessGeneration uint64                    `json:"harness_generation"`
	OriginalRuntime   *processidentity.Identity `json:"original_runtime,omitempty"`
}

// RetainedReconstructionEvidence keeps process proof tri-state: nil means
// that termination could not be established from retained evidence.
type RetainedReconstructionEvidence struct {
	Descriptor        RecoveryDescriptor `json:"descriptor"`
	ProcessTerminated *bool              `json:"process_terminated,omitempty"`
}

// RetainedReconstructionSubmissionRequest selects one exact descriptor entry.
type RetainedReconstructionSubmissionRequest struct {
	Root              string            `json:"root"`
	SessionID         string            `json:"session_id"`
	IncarnationID     string            `json:"incarnation_id"`
	HarnessGeneration uint64            `json:"harness_generation"`
	Candidate         SubmissionSummary `json:"candidate"`
}

// ReadRetainedReconstructionEvidence returns candidate metadata without
// changing acknowledgements, submissions, or journal ownership.
func ReadRetainedReconstructionEvidence(
	ctx context.Context,
	request RetainedReconstructionEvidenceRequest,
) (*RetainedReconstructionEvidence, error) {
	if request.SessionID == "" || request.IncarnationID == "" || request.HarnessGeneration == 0 {
		return nil, ErrOwnerMismatch
	}
	journal, err := openRetainedInspectionJournal(RetainedRecoveryRequest{
		Root: request.Root, SessionID: request.SessionID,
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = journal.Close() }()

	descriptor, err := journal.RecoveryDescriptor(
		ctx, request.SessionID, request.IncarnationID, request.HarnessGeneration, "",
	)
	if err != nil {
		return nil, err
	}
	result := &RetainedReconstructionEvidence{Descriptor: descriptor}
	if request.OriginalRuntime != nil {
		if err := request.OriginalRuntime.Validate(); err == nil {
			if terminated, err := processidentity.OwnedSessionTerminated(*request.OriginalRuntime); err == nil {
				result.ProcessTerminated = &terminated
			}
		}
	}
	return result, nil
}

// ReadRetainedReconstructionSubmission returns the payload only when the
// caller's summary still names the sole unresolved candidate.
func ReadRetainedReconstructionSubmission(
	ctx context.Context,
	request RetainedReconstructionSubmissionRequest,
) (*Submission, error) {
	if !validRetainedReconstructionSelection(request) {
		return nil, ErrOwnerMismatch
	}

	journal, err := openRetainedInspectionJournal(RetainedRecoveryRequest{
		Root: request.Root, SessionID: request.SessionID,
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = journal.Close() }()

	descriptor, err := journal.RecoveryDescriptor(
		ctx, request.SessionID, request.IncarnationID, request.HarnessGeneration, "",
	)
	if err != nil {
		return nil, err
	}
	if descriptor.SubmissionsTruncated || descriptor.SubmissionCount != 1 ||
		len(descriptor.Submissions) != 1 || !SubmissionSummariesEqual(descriptor.Submissions[0], request.Candidate) {
		return nil, ErrSubmissionConflict
	}

	submission, err := journal.GetSubmission(ctx, request.Candidate.ID)
	if err != nil {
		return nil, err
	}
	if err := VerifyReconstructionSubmission(submission, request.Candidate); err != nil {
		return nil, err
	}
	return &submission, nil
}

func validRetainedReconstructionSelection(request RetainedReconstructionSubmissionRequest) bool {
	return request.SessionID != "" && request.IncarnationID != "" && request.HarnessGeneration != 0 && request.Candidate.ID != "" &&
		request.Candidate.SessionID == request.SessionID && request.Candidate.IncarnationID != "" &&
		request.Candidate.HarnessGeneration == request.HarnessGeneration && request.Candidate.IncarnationID == request.IncarnationID &&
		request.Candidate.StreamID != ""
}

// SubmissionSummariesEqual compares every persisted identity and state field
// that can change between discovery and selection.
func SubmissionSummariesEqual(left, right SubmissionSummary) bool {
	return left.ID == right.ID && left.SessionID == right.SessionID &&
		left.IncarnationID == right.IncarnationID && left.HarnessGeneration == right.HarnessGeneration &&
		left.StreamID == right.StreamID && left.Hash == right.Hash && left.State == right.State &&
		left.TerminalEventRetained == right.TerminalEventRetained && left.TerminalSequence == right.TerminalSequence &&
		left.CreatedAt.Equal(right.CreatedAt) && left.UpdatedAt.Equal(right.UpdatedAt)
}

// VerifyReconstructionSubmission checks that one immutable payload still
// matches its summary and the prompt-submission envelope stored by agentctl.
func VerifyReconstructionSubmission(submission Submission, expected SubmissionSummary) error {
	if !SubmissionSummariesEqual(submissionSummary(submission), expected) {
		return ErrSubmissionConflict
	}
	if !submissionNeedsRecovery(submission) || submission.Retired {
		return ErrSubmissionState
	}
	if len(submission.Payload) == 0 || int64(len(submission.Payload)) > DefaultMaxEventBytes {
		return ErrStreamFull
	}
	if !validReconstructionPayload(submission.Payload, expected.Hash) {
		return ErrSubmissionConflict
	}
	return nil
}

func validReconstructionPayload(payload []byte, expectedHash string) bool {
	digest := sha256.Sum256(payload)
	if expectedHash == "" || hex.EncodeToString(digest[:]) != expectedHash {
		return false
	}
	if !validReconstructionEnvelope(payload) {
		return false
	}
	var prompt struct {
		Text        *string                `json:"text"`
		Attachments []v1.MessageAttachment `json:"attachments,omitempty"`
		Steer       *bool                  `json:"steer,omitempty"`
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&prompt) != nil || prompt.Text == nil {
		return false
	}
	return validReconstructionAttachments(prompt.Attachments)
}

func validReconstructionEnvelope(payload []byte) bool {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(payload, &envelope); err != nil || envelope == nil {
		return false
	}
	for key := range envelope {
		if key != "text" && key != "attachments" && key != "steer" {
			return false
		}
	}
	if _, ok := envelope["text"]; !ok {
		return false
	}
	if raw, ok := envelope["attachments"]; ok {
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 || trimmed[0] != '[' {
			return false
		}
	}
	if raw, ok := envelope["steer"]; ok {
		trimmed := bytes.TrimSpace(raw)
		if !bytes.Equal(trimmed, []byte("true")) && !bytes.Equal(trimmed, []byte("false")) {
			return false
		}
	}
	return true
}

func validReconstructionAttachments(attachments []v1.MessageAttachment) bool {
	for _, attachment := range attachments {
		if attachment.Type != "image" && attachment.Type != "audio" && attachment.Type != "resource" {
			return false
		}
		if !attachment.HasValidDeliveryMode() || attachment.SizeBytes < 0 {
			return false
		}
	}
	return true
}

func ReadRetainedRecovery(ctx context.Context, request RetainedRecoveryRequest) (*RetainedRecoveryResponse, error) {
	if request.Limit < 1 || request.Limit > 4 || request.Acknowledge > request.After {
		return nil, ErrSequenceConflict
	}
	j, err := openRetainedRecoveryJournal(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = j.Close() }()
	descriptor, err := j.RecoveryDescriptor(ctx, request.SessionID, request.IncarnationID, request.HarnessGeneration, request.StreamID)
	if err != nil {
		return nil, err
	}
	submission, err := j.GetSubmission(ctx, request.SubmissionID)
	if err != nil {
		return nil, err
	}
	if !retainedSubmissionMatches(submission, request) {
		return nil, ErrOwnerMismatch
	}
	events, _, err := j.Replay(ctx, request.StreamID, request.After, request.Limit)
	if err != nil && (descriptor.Stream != nil || request.After != 0) {
		return nil, err
	}
	if request.Acknowledge > 0 {
		if err = j.Acknowledge(ctx, request.StreamID, request.Acknowledge); err != nil {
			return nil, err
		}
	}
	terminated, _ := processidentity.OwnedSessionTerminated(request.OriginalRuntime)
	submission.Payload = nil
	return &RetainedRecoveryResponse{Descriptor: descriptor, Submission: submission, Events: events, ProcessTerminated: terminated}, nil
}

func openRetainedRecoveryJournal(request RetainedRecoveryRequest) (*Journal, error) {
	return openRetainedJournal(request, false)
}

func openRetainedInspectionJournal(request RetainedRecoveryRequest) (*Journal, error) {
	return openRetainedJournal(request, true)
}

func openRetainedJournal(request RetainedRecoveryRequest, readOnly bool) (*Journal, error) {
	location, err := ResolveLocation(request.Root, request.SessionID)
	if err != nil {
		return nil, ErrOwnerMismatch
	}
	if _, err := os.Lstat(filepath.Join(filepath.Dir(location.Path), lostMarkerName)); !os.IsNotExist(err) {
		return nil, ErrJournalCorrupt
	}
	owner, err := os.ReadFile(filepath.Join(filepath.Dir(location.Path), ownerMarkerName))
	if err != nil || strings.TrimSpace(string(owner)) != request.SessionID {
		return nil, ErrOwnerMismatch
	}
	before, err := os.Lstat(location.Path)
	if err != nil || !before.Mode().IsRegular() {
		return nil, ErrJournalCorrupt
	}
	j, err := Open(Config{Path: location.Path, ExistingOnly: true, ReadOnly: readOnly})
	if err != nil {
		return nil, err
	}
	after, err := os.Lstat(location.Path)
	if err != nil || !os.SameFile(before, after) {
		_ = j.Close()
		return nil, ErrOwnerMismatch
	}
	return j, nil
}

func retainedSubmissionMatches(submission Submission, request RetainedRecoveryRequest) bool {
	return submission.SessionID == request.SessionID && submission.IncarnationID == request.IncarnationID && submission.HarnessGeneration == request.HarnessGeneration && submission.StreamID == request.StreamID
}
