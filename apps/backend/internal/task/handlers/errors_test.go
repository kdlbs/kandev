package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestErrorsAreClassifiable verifies that the package's error-classification
// helpers detect typed sentinels through any depth of wrapping, so HTTP
// status mapping survives downstream wrap/unwrap changes.
func TestErrorsAreClassifiable(t *testing.T) {
	notFoundSentinels := map[string]error{
		"taskrepo.ErrTaskNotFound":      taskrepo.ErrTaskNotFound,
		"taskrepo.ErrWorkspaceNotFound": taskrepo.ErrWorkspaceNotFound,
		"taskrepo.ErrNoPrimarySession":  taskrepo.ErrNoPrimarySession,
		"service.ErrDocumentNotFound":   service.ErrDocumentNotFound,
		"service.ErrTaskPlanNotFound":   service.ErrTaskPlanNotFound,
		"service.ErrRevisionNotFound":   service.ErrRevisionNotFound,
	}
	for name, sentinel := range notFoundSentinels {
		t.Run("isNotFound recognizes "+name, func(t *testing.T) {
			wrapped := fmt.Errorf("look up failed: %w", sentinel)
			if !isNotFound(wrapped) {
				t.Errorf("expected wrapped %s to classify as not-found", name)
			}
		})
	}

	t.Run("isAgentReportedError uses lifecycle.ErrAgentReported", func(t *testing.T) {
		wrapped := fmt.Errorf("waitForPromptDone: %w", lifecycle.ErrAgentReported)
		if !isAgentReportedError(wrapped) {
			t.Errorf("expected wrapped ErrAgentReported to classify")
		}
		if isAgentReportedError(errors.New("agent error: not the sentinel")) {
			t.Errorf("untyped lookalike must no longer classify")
		}
	})

	// A browser that navigates away or unmounts a component aborts its
	// in-flight fetches. That reaches handlers as a cancelled request context,
	// which used to fall through to the 500 branch and log an ERROR with a
	// full stacktrace for a response nobody was waiting on any more.
	t.Run("client disconnect is not a server error", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		handlers := map[string]func(*gin.Context, error){
			"handleNotFound": func(ctx *gin.Context, err error) {
				handleNotFound(ctx, newTestLogger(t), err, "task sessions not found")
			},
			"handleSelectedMoveError": func(ctx *gin.Context, err error) {
				handleSelectedMoveError(ctx, newTestLogger(t), err)
			},
		}
		for name, handle := range handlers {
			t.Run(name, func(t *testing.T) {
				rec := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(rec)
				handle(ctx, fmt.Errorf("list task sessions: %w", context.Canceled))
				if rec.Code != statusClientClosedRequest {
					t.Fatalf("status=%d, want %d", rec.Code, statusClientClosedRequest)
				}
			})
		}
	})

	// Our own deadline firing is a real failure and must keep its 500.
	t.Run("server timeout stays a server error", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		handleNotFound(ctx, newTestLogger(t), fmt.Errorf("query: %w", context.DeadlineExceeded), "task not found")
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d, want %d", rec.Code, http.StatusInternalServerError)
		}
	})

	t.Run("WIP limit is a conflict for task creation", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		handleNotFound(ctx, newTestLogger(t), fmt.Errorf("create task: %w", wfmodels.NewWIPLimitError("review", 2, 2)), "task not created")
		if rec.Code != http.StatusConflict {
			t.Fatalf("status=%d, want %d", rec.Code, http.StatusConflict)
		}
	})

	t.Run("stale branch policies include a stable error code", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		err := fmt.Errorf("%w: %w", service.ErrInvalidRepositoryBranchPolicy, service.ErrRepositoryBranchPolicyStale)
		handleNotFound(ctx, newTestLogger(t), err, "task not created")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status=%d, want %d", rec.Code, http.StatusBadRequest)
		}
		var payload map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if payload["error_code"] != service.BranchPolicyStaleErrorCode {
			t.Fatalf("error_code=%q, want %q", payload["error_code"], service.BranchPolicyStaleErrorCode)
		}
	})
}

func TestTaskCreateRepositorySelectionErrorsMapToTransports(t *testing.T) {
	tests := []struct {
		name       string
		errorCode  service.RepositorySelectionErrorCode
		httpStatus int
		wsCode     string
	}{
		{
			name:       "invalid",
			errorCode:  service.RepositorySelectionErrorInvalid,
			httpStatus: http.StatusBadRequest,
			wsCode:     ws.ErrorCodeValidation,
		},
		{
			name:       "not found",
			errorCode:  service.RepositorySelectionErrorNotFound,
			httpStatus: http.StatusNotFound,
			wsCode:     ws.ErrorCodeNotFound,
		},
		{
			name:       "unavailable",
			errorCode:  service.RepositorySelectionErrorUnavailable,
			httpStatus: http.StatusServiceUnavailable,
			wsCode:     ws.ErrorCodeUnavailable,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := service.NewRepositorySelectionError(test.errorCode, errors.New("plugin response secret"))

			status, ok := repositorySelectionHTTPStatus(err)
			if !ok || status != test.httpStatus {
				t.Fatalf("repositorySelectionHTTPStatus() = (%d, %t), want (%d, true)", status, ok, test.httpStatus)
			}
			code, ok := repositorySelectionWSCode(err)
			if !ok || code != test.wsCode {
				t.Fatalf("repositorySelectionWSCode() = (%q, %t), want (%q, true)", code, ok, test.wsCode)
			}

			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rec)
			handleNotFound(ctx, newTestLogger(t), err, "task not created")
			if rec.Code != test.httpStatus {
				t.Fatalf("HTTP status = %d, want %d", rec.Code, test.httpStatus)
			}
			var payload map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode HTTP response: %v", err)
			}
			if payload["error_code"] != string(test.errorCode) {
				t.Fatalf("error_code = %q, want %q", payload["error_code"], test.errorCode)
			}
			if strings.Contains(rec.Body.String(), "plugin response secret") {
				t.Fatal("HTTP response leaked the wrapped provider error")
			}
		})
	}
}

func TestMoveConflictCode(t *testing.T) {
	cases := []struct {
		name string
		err  string
		want string
	}{
		{"active session", "task has an active session (running)", moveConflictCodeActiveSession},
		{"archived task", "archived tasks cannot be moved", moveConflictCodeArchived},
		{"different workspace", "target workflow is in a different workspace", moveConflictCodeDifferentWorkspace},
		{"workflow step", "target workflow step does not belong to target workflow", moveConflictCodeWorkflowStep},
		{"WIP limit", "WIP limit exceeded for workflow step", moveConflictCodeWIPLimit},
		{"unrelated", "database is locked", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := moveConflictCode(errors.New(tc.err)); got != tc.want {
				t.Fatalf("moveConflictCode(%q) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

// TestIsTimeoutError pins the UX-classification contract for the prompt
// error-message renderer. The pre-refactor substring check on "timeout"
// covered three classes of producer; the typed-sentinel rewrite must keep
// all three classified or the user-facing "Request timed out…" message
// silently downgrades to the generic "Failed to send message to agent".
func TestIsTimeoutError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"unrelated", errors.New("something else"), false},
		// net.Error with Timeout()==true (typed) — e.g. http.Client deadline.
		{"net error timeout", &timeoutNetErr{}, true},
		// Plain fmt.Errorf("timeout …") from waitForSessionReady, agentctl
		// health waits, agent-stream connect waits. No typed Timeout() method.
		{"session-ready timeout (substring fallback)", errors.New("timeout waiting for session to become ready after resume"), true},
		{"agent-stream timeout (substring fallback)", errors.New("timeout waiting for agent stream to connect after restart"), true},
		// context.DeadlineExceeded is itself a net.Error with Timeout()==true,
		// so it matches via the typed path — no separate errors.Is needed in
		// createPromptErrorMessage.
		{"context.DeadlineExceeded", context.DeadlineExceeded, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTimeoutError(tc.err); got != tc.want {
				t.Errorf("isTimeoutError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}

}

type timeoutNetErr struct{}

func (timeoutNetErr) Error() string   { return "i/o timeout" }
func (timeoutNetErr) Timeout() bool   { return true }
func (timeoutNetErr) Temporary() bool { return false }

var _ net.Error = timeoutNetErr{}
