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
	request.Header.Set(journal.DeliveryProtocolHeader, "2")
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	require.Equal(t, http.StatusConflict, response.Code)
	stored, err := retained.GetSubmission(context.Background(), "old")
	require.NoError(t, err)
	require.False(t, stored.Retired)
	request = httptest.NewRequest(http.MethodPost, "/api/v1/agent/submissions/old/retire", bytes.NewBufferString(`{"stream_id":"old-stream","harness_generation":1}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(journal.DeliveryProtocolHeader, "2")
	response = httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	require.Equal(t, http.StatusNoContent, response.Code)
	stored, err = retained.GetSubmission(context.Background(), "old")
	require.NoError(t, err)
	require.True(t, stored.Retired)
	require.Equal(t, journal.SubmissionInterruptedUnknown, stored.State)
}

func TestNativeResumeCanRetireMissingInterruptedUnknownRecord(t *testing.T) {
	server, _, retained := newDurableDeliveryTestServer(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agent/submissions/interrupted/retire", bytes.NewBufferString(`{"session_id":"session-1","incarnation_id":"session-1","harness_generation":1,"stream_id":"session-1","hash":"prompt-hash"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(journal.DeliveryProtocolHeader, "2")
	response := httptest.NewRecorder()
	server.Router().ServeHTTP(response, request)
	require.Equal(t, http.StatusNoContent, response.Code)

	stored, err := retained.GetSubmission(context.Background(), "interrupted")
	require.NoError(t, err)
	require.Equal(t, journal.SubmissionInterruptedUnknown, stored.State)
	require.True(t, stored.Retired)
	require.Equal(t, "prompt-hash", stored.Hash)
	require.Empty(t, stored.Payload)
}

func TestNativeResumeDoesNotCreateMissingRecordForNewerGeneration(t *testing.T) {
	server, _, retained := newDurableDeliveryTestServer(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agent/submissions/interrupted/retire", bytes.NewBufferString(`{"session_id":"session-1","incarnation_id":"session-1","harness_generation":2,"stream_id":"session-1","hash":"prompt-hash"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(journal.DeliveryProtocolHeader, "2")
	response := httptest.NewRecorder()
	server.Router().ServeHTTP(response, request)
	require.Equal(t, http.StatusConflict, response.Code)

	_, err := retained.GetSubmission(context.Background(), "interrupted")
	require.ErrorIs(t, err, journal.ErrSubmissionNotFound)
}
