package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type deferredLaunchDiscardHandlerProbe struct {
	calls  int
	taskID string
}

func (p *deferredLaunchDiscardHandlerProbe) DiscardDeferredCeilingLaunch(
	_ context.Context, taskID string,
) (models.DeferredLaunchDiscardResult, error) {
	p.calls++
	p.taskID = taskID
	return models.DeferredLaunchDiscardResult{Outcome: models.DeferredLaunchDiscardDiscarded}, nil
}

func TestHTTPDiscardDeferredLaunchMapsSuccessUnavailableAndUnauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name       string
		userID     string
		wire       bool
		wantStatus int
		wantCalls  int
	}{
		{name: "discarded", userID: "user-b", wire: true, wantStatus: http.StatusOK, wantCalls: 1},
		{name: "unavailable", userID: "user-b", wantStatus: http.StatusServiceUnavailable},
		{name: "foreign task", userID: "user-a", wire: true, wantStatus: http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newAuthzTaskHandlers(t, &authzDeleteRepo{})
			probe := &deferredLaunchDiscardHandlerProbe{}
			if tc.wire {
				h.service.SetDeferredLaunchDiscarder(probe)
			}
			router := gin.New()
			h.registerHTTP(router)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-b/deferred-launch/discard", nil)
			req = req.WithContext(authn.WithIdentity(req.Context(), authn.Identity{UserID: tc.userID, Role: authn.RoleMember}))
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
			require.Equal(t, tc.wantCalls, probe.calls)
			if tc.wantStatus == http.StatusOK {
				var body struct {
					Outcome models.DeferredLaunchDiscardOutcome `json:"outcome"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				require.Equal(t, models.DeferredLaunchDiscardDiscarded, body.Outcome)
				require.Equal(t, "task-b", probe.taskID)
			}
		})
	}
}
