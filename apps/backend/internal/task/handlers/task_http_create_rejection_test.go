package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/service"
)

// TestHTTPCreateTask_CreationRejectionEvidence pins the handler-level half of
// the rejection proof: a request refused before Service.CreateTask carries
// creation_rejected, and no task row exists.
func TestHTTPCreateTask_CreationRejectionEvidence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newIdempotentCreateTaskRepo()
	h := newIdempotencyTestHandlers(t, repo)

	rec := doCreateTask(h, `{"workspace_id":"ws-1","workflow_id":"wf-1","workflow_step_id":"step-1","title":"Task","start_agent":true}`)
	require.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, true, resp["creation_rejected"], "handler validation must prove rejection")

	repo.mu.Lock()
	defer repo.mu.Unlock()
	assert.Empty(t, repo.tasks, "a rejected request must not allocate a task")
}

// TestHTTPCreateTask_PreAllocationServiceFailureCarriesRejection covers the
// typed service validation boundary, which runs before the task insert.
func TestHTTPCreateTask_PreAllocationServiceFailureCarriesRejection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newIdempotentCreateTaskRepo()
	h := newIdempotencyTestHandlers(t, repo)

	rec := doCreateTask(h, `{"workspace_id":"ws-1","workflow_id":"wf-1","workflow_step_id":"step-1","title":"Task","external_id":"ext-rejected","initial_workspace_layout":"bogus"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, true, resp["creation_rejected"])
	assert.NotEmpty(t, resp["error"])

	repo.mu.Lock()
	defer repo.mu.Unlock()
	assert.Empty(t, repo.tasks)
}

// TestHTTPCreateTask_PostAllocationErrorIsUncertain pins the other side of the
// boundary: an attachment claim failure happens after allocation and must not
// tell the browser that its request was rejected.
func TestHTTPCreateTask_PostAllocationErrorIsUncertain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newIdempotentCreateTaskRepo()
	h := newIdempotencyTestHandlers(t, repo)

	rec := doCreateTask(h, `{"workspace_id":"ws-1","workflow_id":"wf-1","workflow_step_id":"step-1","title":"Task","external_id":"ext-uncertain","attachments":[{"type":"image","attachment_id":"upload-1","mime_type":"image/png","name":"shot.png"}]}`)
	require.Equal(t, http.StatusInternalServerError, rec.Code, "body: %s", rec.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotContains(t, resp, "creation_rejected", "post-allocation failures must stay uncertain")
}

// TestHTTPCreateTask_ExternalIDValidationStaysUncertain covers a
// service-level validation failure that is not wrapped in the typed
// pre-allocation error: absence of the marker keeps it from authorizing a
// fresh create identity.
func TestHTTPCreateTask_ExternalIDValidationStaysUncertain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newIdempotentCreateTaskRepo()
	h := newIdempotencyTestHandlers(t, repo)

	rec := doCreateTask(h, `{"workspace_id":"ws-1","workflow_id":"wf-1","workflow_step_id":"step-1","title":"Task","external_id":"ext-1\n"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotContains(t, resp, "creation_rejected")
}

// TestHandleTaskCreateFailure_StaleBranchPolicyCarriesRejection covers a
// pre-allocation branch-policy failure. The response keeps its machine-readable
// error code and carries the rejection proof so the native dialog can refresh
// policy choices and retire only a definitive first attempt.
func TestHandleTaskCreateFailure_StaleBranchPolicyCarriesRejection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	err := &service.PreAllocationTaskCreateError{
		Cause: fmt.Errorf("%w: policy was removed", service.ErrRepositoryBranchPolicyStale),
	}
	require.True(t, isPreAllocationCreateRejection(err))

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	handleTaskCreateFailure(c, newTestLogger(t), err)

	require.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, true, resp["creation_rejected"])
	assert.Equal(t, service.BranchPolicyStaleErrorCode, resp["error_code"])
}

// TestHandleTaskCreateFailure_RepositoryProviderUnavailableCarriesRejection
// keeps a typed provider-unavailable reference failure correctable: the
// request was rejected before allocation even though its HTTP status is 503.
func TestHandleTaskCreateFailure_RepositoryProviderUnavailableCarriesRejection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	err := &service.PreAllocationTaskCreateError{
		Cause: &service.RepositorySelectionError{Code: service.RepositorySelectionErrorUnavailable},
	}
	require.True(t, isPreAllocationCreateRejection(err))

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	handleTaskCreateFailure(c, newTestLogger(t), err)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code, "body: %s", rec.Body.String())
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, true, resp["creation_rejected"])
	assert.Equal(t, string(service.RepositorySelectionErrorUnavailable), resp["error_code"])
}
