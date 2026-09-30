package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/worktree"
)

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.6
func TestHTTPTaskDeletePreflightLogsInspectionFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, observed := observer.New(zap.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	inspection := &worktree.CleanupInspectionError{
		Stage: "working_tree_status", Reason: worktree.CleanupInspectionReasonRepoUnavailable,
		Err: errors.New("fatal: not a git repository: /private/source/secret"),
	}
	router := newTaskDeletePreflightRouter(t, taskDeletePreflightHTTPCleanup{inspectErr: inspection}, log)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/delete-preflight",
		strings.NewReader(`{"task_ids":["private-task-sentinel"],"cascade":false}`))
	router.ServeHTTP(recorder, withTaskDeleteTestIdentity(request))
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	require.JSONEq(t, `{"error":"task delete preflight unavailable"}`, recorder.Body.String())
	entries := observed.All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, "working_tree_status", fields["stage"])
	require.Equal(t, worktree.CleanupInspectionReasonRepoUnavailable, fields["reason"])
	require.EqualValues(t, 1, fields["task_count"])
	logged := entries[0].Message + fmt.Sprint(fields)
	for _, private := range []string{"/private/source/secret", "fatal:", "private-task-sentinel"} {
		require.NotContains(t, logged, private)
	}
}

// @covers AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.6
func TestHandleNotFoundLogsCleanupInspectionFailure(t *testing.T) {
	log, observed := observedErrorLogger(t)
	inspection := &worktree.CleanupInspectionError{
		Stage: worktree.CleanupInspectionStageCommit, Reason: worktree.CleanupInspectionReasonRepoUnavailable,
		Err: errors.New("fatal: not a git repository: /private/source/secret"),
	}
	err := fmt.Errorf("prepare cleanup task-1: capture worktree cleanup identities: %w", inspection)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	handleNotFound(c, log, err, "task not found")
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.JSONEq(t, `{"error":"request failed"}`, recorder.Body.String())
	entries := observed.All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, worktree.CleanupInspectionStageCommit, fields["stage"])
	require.Equal(t, worktree.CleanupInspectionReasonRepoUnavailable, fields["reason"])
	require.NotContains(t, entries[0].Message+fmt.Sprint(fields), "/private/source/secret")
	require.NotContains(t, entries[0].Message+fmt.Sprint(fields), "fatal:")
}
