package backendapp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	officeagents "github.com/kandev/kandev/internal/office/agents"
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceAdoptionProductionCallerGuard(t *testing.T) {
	cases := []struct {
		name                            string
		wired, agent, bearer, synthetic bool
		want                            int
	}{
		{"unwired", false, false, false, false, http.StatusForbidden},
		{"verified operator reaches request validation", true, false, false, false, http.StatusBadRequest},
		{"known agent refused", true, true, false, false, http.StatusForbidden},
		{"single user no-header caller refused", true, false, false, true, http.StatusForbidden},
		{"single user unknown bearer refused", true, false, true, true, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(func(c *gin.Context) {
				authn.SetOnGin(c, authn.Identity{Role: authn.RoleAdmin, Synthetic: tc.synthetic})
				if tc.agent {
					c.Set("agent_claims", &officeagents.AgentClaims{})
				}
			})
			log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
			require.NoError(t, err)
			h := taskhandlers.RegisterTaskRoutes(r, ws.NewDispatcher(), nil, nil, nil, nil, log)
			if tc.wired {
				configureWorkspaceAdoptionCaller(h)
			}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task/workspace-adoption/preview", strings.NewReader(`{"unknown":true}`))
			if tc.bearer {
				req.Header.Set("Authorization", "Bearer unknown-agent-token")
			}
			r.ServeHTTP(rec, req)
			require.Equal(t, tc.want, rec.Code, rec.Body.String())
		})
	}
}
