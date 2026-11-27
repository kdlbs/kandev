package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPRestartConfigChatRejectsTrailingJSON(t *testing.T) {
	for _, suffix := range []string{`{}`, `garbage`, `null`} {
		t.Run(suffix, func(t *testing.T) {
			router, svc, repo, _ := newRestartHTTPFixture(t)
			ticket := restartHTTPTicket(t, svc)
			request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/ws-1/config-chat/restart", strings.NewReader(`{"task_id":"old-task","session_id":"old-session"}`+suffix))
			request.Header.Set(taskDeleteConfirmationHeader, ticket)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, withTaskDeleteTestIdentity(request))
			assert.Equal(t, http.StatusBadRequest, response.Code)
			assert.Empty(t, repo.calls)
		})
	}
}

func TestConfigChatListIgnoresOtherWorkspaceOperations(t *testing.T) {
	_, svc, repo, o := newRestartHTTPFixture(t)
	h := NewTaskHandlers(svc, o, nil, nil, newTestLogger(t))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	repo.listHook = func() {
		calls++
		release, ok := h.configChatAdmission.begin("other-workspace", "other-session")
		require.True(t, ok)
		release()
		if calls == 3 {
			cancel()
		}
	}
	_, _, err := h.listQuickChatsWithRestart(ctx, "ws-1")
	assert.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestHTTPRestartConfigChatRejectsUnavailableProvider(t *testing.T) {
	for _, available := range []bool{false, true} {
		t.Run(map[bool]string{false: "unavailable", true: "available"}[available], func(t *testing.T) {
			router, svc, repo, _ := newRestartHTTPFixture(t)
			ticket := restartHTTPTicket(t, svc)
			repo.executor.Type = models.ExecutorTypePluginRemote
			repo.executor.Provider = &models.ExecutorProvider{Available: available}
			response := restartHTTPRequest(router, ticket)
			if available {
				assert.Equal(t, http.StatusOK, response.Code)
				assert.Equal(t, []string{"stop", "delete", "create"}, repo.calls)
			} else {
				assert.Equal(t, http.StatusConflict, response.Code)
				assert.Empty(t, repo.calls)
			}
		})
	}
}

func TestConfigChatRestartValidationErrorsKeepHTTPMeaning(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		status int
	}{
		{"missing task", repoerrors.ErrTaskNotFound, http.StatusNotFound},
		{"storage failed", errors.New("private database detail"), http.StatusInternalServerError},
		{"stale confirmation", service.ErrTaskDeleteConfirmationStale, http.StatusConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(response)
			h := &TaskHandlers{logger: newTestLogger(t)}
			h.respondConfigChatRestartFailure(ctx, configChatRestartFailure{Stage: configChatRestartStageValidate}, test.err)
			assert.Equal(t, test.status, response.Code)
			assert.NotContains(t, response.Body.String(), "private database detail")
			if test.status != http.StatusConflict {
				assert.NotContains(t, response.Body.String(), "confirmation_invalid")
			}
		})
	}
}

func TestConfigChatRestartPreparationRecoveryOutlivesDeadline(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "retain primary"}[partial], func(t *testing.T) {
			_, svc, repo, o := newRestartHTTPFixture(t)
			ctx, cancel := context.WithTimeout(authn.WithIdentity(context.Background(), authn.Identity{UserID: "http-test-user", Synthetic: true}), 10*time.Millisecond)
			defer cancel()
			o.prepareCancel, o.preparePartial, o.prepareErr = func() { <-ctx.Done() }, partial, context.DeadlineExceeded
			h := &TaskHandlers{service: svc, orchestrator: o, logger: newTestLogger(t)}
			replacement, err := h.createConfigChatReplacement(ctx, "ws-1", &service.ConfigChatRestartSelection{AgentProfileID: "profile-1", ExecutorID: "exec-1", ExecutorProfileID: "exec-profile"})
			require.Error(t, err)
			if partial {
				require.NotNil(t, replacement)
				assert.Equal(t, "new-session", replacement.SessionID)
			} else {
				assert.Nil(t, replacement)
				assert.Len(t, repo.tasks, 1)
			}
		})
	}
}
