package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/stretchr/testify/require"
)

func TestInterruptedRetirementBindsObservedStreamAndPredecessor(t *testing.T) {
	server, _, retained := newDurableDeliveryTestServer(t)
	server.cfg.DeliveryHarnessGeneration = 2
	_, err := retained.PutSubmission(context.Background(), journal.Submission{ID: "old", SessionID: "session-1", IncarnationID: "session-1", HarnessGeneration: 1, StreamID: "old-stream", Hash: "hash", Payload: []byte("old instruction"), State: journal.SubmissionInterruptedUnknown})
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agent/submissions/old/retire", bytes.NewBufferString(`{"stream_id":"foreign-stream","harness_generation":1}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	require.Equal(t, http.StatusConflict, response.Code)
	stored, err := retained.GetSubmission(context.Background(), "old")
	require.NoError(t, err)
	require.False(t, stored.Retired)
	request = httptest.NewRequest(http.MethodPost, "/api/v1/agent/submissions/old/retire", bytes.NewBufferString(`{"stream_id":"old-stream","harness_generation":1}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	require.Equal(t, http.StatusNoContent, response.Code)
	stored, err = retained.GetSubmission(context.Background(), "old")
	require.NoError(t, err)
	require.True(t, stored.Retired)
	require.Equal(t, journal.SubmissionInterruptedUnknown, stored.State)
}
