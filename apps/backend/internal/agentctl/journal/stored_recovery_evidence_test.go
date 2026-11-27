package journal

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStoredRecoveryEvidencePreservesHistoricalLabels(t *testing.T) {
	for _, state := range []SubmissionState{SubmissionFailed, SubmissionCompleted, SubmissionCancelled, SubmissionInterruptedUnknown} {
		for _, retired := range []bool{false, true} {
			name := string(state)
			if retired {
				name += " retired"
			}
			t.Run(name, func(t *testing.T) {
				ctx := context.Background()
				root := t.TempDir()
				path := CheckStorage(root, "session").Path
				store, err := Open(Config{Path: path})
				require.NoError(t, err)
				original := Submission{ID: "history", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream", Hash: "hash", Payload: []byte("must not replay"), State: state}
				_, err = store.PutSubmission(ctx, original)
				require.NoError(t, err)
				if retired {
					_, err = store.RetireSubmission(ctx, original.ID, 1)
					require.NoError(t, err)
				}
				require.NoError(t, store.Close())
				before, err := os.ReadFile(path)
				require.NoError(t, err)
				request := RetainedReconstructionEvidenceRequest{Root: root, SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StoredSubmissionID: original.ID}
				evidence, err := ReadRetainedReconstructionEvidence(ctx, request)
				require.NoError(t, err)
				require.NotNil(t, evidence.StoredSubmission)
				require.Equal(t, state, evidence.StoredSubmission.State)
				require.Equal(t, retired, evidence.StoredSubmission.Retired)
				require.Empty(t, evidence.StoredSubmission.Payload)
				require.Equal(t, original.StreamID, evidence.Descriptor.StreamID)
				request.StoredSubmissionID = ""
				request.RetiredSubmissionID = original.ID
				proof, err := ReadRetainedReconstructionEvidence(ctx, request)
				if retired {
					require.NoError(t, err)
					require.True(t, proof.RetiredSubmission.Retired)
				} else {
					require.ErrorIs(t, err, ErrOwnerMismatch)
				}
				request.RetiredSubmissionID = ""
				request.StoredSubmissionID = original.ID
				request.IncarnationID = "foreign"
				_, err = ReadRetainedReconstructionEvidence(ctx, request)
				require.ErrorIs(t, err, ErrOwnerMismatch)
				after, err := os.ReadFile(path)
				require.NoError(t, err)
				require.Equal(t, before, after)
			})
		}
	}
}
