package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/stretchr/testify/require"
)

func TestDeliveryJournalFixtureReadsRetirementWhileOwnerIsOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	submission := journal.Submission{ID: "interrupted", SessionID: "session", IncarnationID: "incarnation",
		HarnessGeneration: 1, Hash: "hash", Payload: []byte("prompt"), State: journal.SubmissionInterruptedUnknown}
	input, err := json.Marshal(map[string]any{"path": path, "submission": submission})
	require.NoError(t, err)
	require.NoError(t, runDeliveryJournalFixture("seed", bytes.NewReader(input), &bytes.Buffer{}))
	owner, err := journal.Open(journal.Config{Path: path})
	require.NoError(t, err)
	t.Cleanup(func() { _ = owner.Close() })
	_, err = owner.RetireSubmission(context.Background(), submission.ID, 1)
	require.NoError(t, err)
	var output bytes.Buffer
	require.NoError(t, runDeliveryJournalFixture("read", bytes.NewReader(input), &output))
	var retained journal.Submission
	require.NoError(t, json.Unmarshal(output.Bytes(), &retained))
	require.True(t, retained.Retired)
	require.Equal(t, submission.State, retained.State)
	require.Equal(t, submission.Payload, retained.Payload)
}
