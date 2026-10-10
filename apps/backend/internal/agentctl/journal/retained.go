package journal

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/common/processidentity"
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
	j, err := Open(Config{Path: location.Path, ExistingOnly: true})
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
