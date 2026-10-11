package journal

import (
	"context"

	"github.com/kandev/kandev/internal/common/processidentity"
)

func readRetainedHistoryGap(ctx context.Context, j *Journal, request RetainedRecoveryRequest, descriptor RecoveryDescriptor, submission Submission) (*RetainedRecoveryResponse, error) {
	if !validRetainedGapSubmission(request, descriptor, submission) || !validRetainedGapStream(request, descriptor.Stream) {
		return nil, ErrOwnerMismatch
	}
	if err := VerifyReconstructionSubmission(submission, descriptor.Submissions[0]); err != nil {
		return nil, err
	}
	terminated, err := processidentity.OwnedSessionTerminated(request.OriginalRuntime)
	if err != nil || !terminated {
		return nil, ErrOwnerMismatch
	}
	if err := j.ClassifyAbandonedDispatches(ctx, request.SessionID, request.IncarnationID, request.StreamID, request.HarnessGeneration); err != nil {
		return nil, err
	}
	submission, err = j.GetSubmission(ctx, request.SubmissionID)
	if err != nil {
		return nil, err
	}
	descriptor, err = j.RecoveryDescriptor(ctx, request.SessionID, request.IncarnationID, request.HarnessGeneration, request.StreamID)
	if err != nil {
		return nil, err
	}
	submission.Payload = nil
	return &RetainedRecoveryResponse{Descriptor: descriptor, Submission: submission,
		ProcessTerminated: true, HistoryGapVerified: true}, nil
}

func validRetainedGapSubmission(request RetainedRecoveryRequest, descriptor RecoveryDescriptor, submission Submission) bool {
	return request.Acknowledge == 0 && request.ExpectedHash != "" && request.ExpectedHash == submission.Hash &&
		!descriptor.SubmissionsTruncated && descriptor.SubmissionCount == 1 && len(descriptor.Submissions) == 1 &&
		descriptor.Submissions[0].ID == request.SubmissionID &&
		(submission.State == SubmissionDispatching || submission.State == SubmissionInterruptedUnknown) &&
		submission.TerminalSequence == 0 && !submission.Retired
}

func validRetainedGapStream(request RetainedRecoveryRequest, stream *Stream) bool {
	return stream != nil && stream.HighWater != ^uint64(0) && request.After < stream.HighWater &&
		stream.HighWater == request.PrunedThrough && stream.Acknowledged == stream.HighWater && stream.FirstRetained == stream.HighWater+1
}
