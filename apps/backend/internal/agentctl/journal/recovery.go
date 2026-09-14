package journal

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	bolt "go.etcd.io/bbolt"
)

// MaxRecoverySubmissionSummaries bounds the submission evidence returned by
// the authenticated recovery descriptor. Submission payloads never cross
// this boundary.
const MaxRecoverySubmissionSummaries = 16

// SubmissionSummary is the non-secret portion of a retained prompt
// submission. It is sufficient to identify work that must be reconciled while
// keeping the immutable prompt payload inside the owner journal.
type SubmissionSummary struct {
	ID                    string          `json:"id"`
	SessionID             string          `json:"session_id"`
	IncarnationID         string          `json:"incarnation_id"`
	HarnessGeneration     uint64          `json:"harness_generation"`
	Hash                  string          `json:"hash"`
	State                 SubmissionState `json:"state"`
	TerminalEventRetained bool            `json:"terminal_event_retained,omitempty"`
	TerminalSequence      uint64          `json:"terminal_sequence,omitempty"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
}

// RecoveryDescriptor is a consistent identity and retained-work snapshot for
// one agentctl owner. The stream watermark and submission list are read from
// the same bbolt view. A later event can advance the watermark immediately
// after this method returns; callers must therefore use the captured
// high-water mark only for the bounded replay it requested.
type RecoveryDescriptor struct {
	StorageCapability
	SessionID            string              `json:"session_id"`
	IncarnationID        string              `json:"incarnation_id"`
	HarnessGeneration    uint64              `json:"harness_generation"`
	StreamID             string              `json:"stream_id"`
	Stream               *Stream             `json:"stream,omitempty"`
	Submissions          []SubmissionSummary `json:"submissions,omitempty"`
	SubmissionCount      int                 `json:"submission_count"`
	SubmissionsTruncated bool                `json:"submissions_truncated,omitempty"`
}

// RecoveryDescriptor returns a bounded, owner-scoped snapshot suitable for a
// backend adopting a surviving process. A stream that has not emitted an
// event yet is represented by a nil Stream; that is a valid empty journal,
// distinct from a journal read failure.
func (j *Journal) RecoveryDescriptor(
	ctx context.Context,
	sessionID, incarnationID string,
	harnessGeneration uint64,
	streamID string,
) (RecoveryDescriptor, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()

	descriptor := RecoveryDescriptor{
		SessionID:         sessionID,
		IncarnationID:     incarnationID,
		HarnessGeneration: harnessGeneration,
		StreamID:          streamID,
		StorageCapability: StorageCapability{Version: CurrentVersion, Durable: true},
	}
	err := j.db.View(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		stream, streamUnresolved, err := recoveryStream(tx, sessionID, incarnationID, harnessGeneration, streamID)
		if err != nil {
			return err
		}
		descriptor.Stream = stream
		descriptor.Unresolved = streamUnresolved

		submissions, submissionsUnresolved, err := recoverySubmissionSummaries(tx, sessionID)
		if err != nil {
			return err
		}
		descriptor.Unresolved = descriptor.Unresolved || submissionsUnresolved
		descriptor.SubmissionCount = len(submissions)
		descriptor.Submissions = boundRecoverySubmissions(submissions, &descriptor.SubmissionsTruncated)
		return nil
	})
	if err != nil {
		return RecoveryDescriptor{}, err
	}
	return descriptor, nil
}

func recoveryStream(
	tx *bolt.Tx,
	sessionID, incarnationID string,
	harnessGeneration uint64,
	streamID string,
) (*Stream, bool, error) {
	stream, err := decodeStream(tx.Bucket(bucketStreamMeta).Get([]byte(streamID)))
	if err != nil {
		return nil, false, err
	}
	if stream.StreamID == "" {
		return nil, false, nil
	}
	if stream.SessionID != sessionID || stream.IncarnationID != incarnationID || stream.HarnessGeneration != harnessGeneration {
		return nil, false, ErrOwnerMismatch
	}
	return &stream, stream.HighWater > stream.Acknowledged, nil
}

func recoverySubmissionSummaries(tx *bolt.Tx, sessionID string) ([]SubmissionSummary, bool, error) {
	summaries := make([]SubmissionSummary, 0)
	unresolved := false
	err := tx.Bucket(bucketSubmissions).ForEach(func(_, raw []byte) error {
		if raw == nil {
			return nil
		}
		var submission Submission
		if err := json.Unmarshal(raw, &submission); err != nil {
			return ErrJournalCorrupt
		}
		if sessionID != "" && submission.SessionID != sessionID {
			return nil
		}
		summaries = append(summaries, submissionSummary(submission))
		unresolved = unresolved || submissionNeedsRecovery(submission)
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	sort.SliceStable(summaries, func(i, k int) bool {
		if summaries[i].UpdatedAt.Equal(summaries[k].UpdatedAt) {
			return summaries[i].ID < summaries[k].ID
		}
		return summaries[i].UpdatedAt.After(summaries[k].UpdatedAt)
	})
	return summaries, unresolved, nil
}

func boundRecoverySubmissions(submissions []SubmissionSummary, truncated *bool) []SubmissionSummary {
	if len(submissions) <= MaxRecoverySubmissionSummaries {
		return submissions
	}
	*truncated = true
	return submissions[:MaxRecoverySubmissionSummaries]
}

func submissionSummary(submission Submission) SubmissionSummary {
	return SubmissionSummary{
		ID:                    submission.ID,
		SessionID:             submission.SessionID,
		IncarnationID:         submission.IncarnationID,
		HarnessGeneration:     submission.HarnessGeneration,
		Hash:                  submission.Hash,
		State:                 submission.State,
		TerminalEventRetained: submission.TerminalEventRetained,
		TerminalSequence:      submission.TerminalSequence,
		CreatedAt:             submission.CreatedAt,
		UpdatedAt:             submission.UpdatedAt,
	}
}

func submissionNeedsRecovery(submission Submission) bool {
	switch submission.State {
	case SubmissionPrepared, SubmissionAccepted, SubmissionDispatching, SubmissionInterruptedUnknown:
		return true
	case SubmissionCompleted:
		return !submission.TerminalEventRetained
	default:
		return false
	}
}
