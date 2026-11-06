package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/hostutility"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	settings "github.com/kandev/kandev/internal/agent/settings/models"
	utility "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/utility/controller"
	"github.com/kandev/kandev/internal/utility/models"
	"github.com/kandev/kandev/internal/utility/service"
	"github.com/stretchr/testify/require"
)

type originRouteResolver struct{ typedProfileFailureResolver }

func (originRouteResolver) ResolveExecutionDetails(context.Context, string, string) (agentruntime.ProfileExecution, error) {
	return agentruntime.ProfileExecution{ExecutionProfileID: "first", RouteSessionID: "isolated", Generation: 1, Profile: &settings.AgentProfile{ID: "first", AgentID: "codex-acp", Model: "selected"}}, nil
}

type originRouteExecutor struct {
	*stubInferenceExecutor
	*statefulHostStub
	identities chan agentruntime.InferenceIdentity
}

func (e *originRouteExecutor) ExecuteInferenceProfilePrompt(ctx context.Context, session, id, prompt string) (*utility.PromptResponse, error) {
	e.identities <- agentruntime.InferenceIdentityFromContext(ctx)
	if id == "first" {
		return nil, errors.New("too many requests")
	}
	return &utility.PromptResponse{Success: true, Response: "answer"}, nil
}
func (e *originRouteExecutor) ExecuteProfilePrompt(ctx context.Context, id, prompt string) (*hostutility.PromptResult, error) {
	e.identities <- agentruntime.InferenceIdentityFromContext(ctx)
	if id == "first" {
		return nil, errors.New("too many requests")
	}
	return &hostutility.PromptResult{Response: "answer"}, nil
}
func TestUtilityHTTPRouteUsesServerCallIdentityAndPersistsAttribution(t *testing.T) {
	for _, sessionID := range []string{"session", ""} {
		t.Run("session="+sessionID, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			repo := newFakeUtilityRepo()
			svc := service.NewService(repo)
			svc.SetProfileResolver(stubProfileResolver{profile: &settings.AgentProfile{ID: "logical", AgentID: "codex-acp", Model: "selected"}})
			svc.SetExecutionProfileResolver(originRouteResolver{})
			executor := &originRouteExecutor{stubInferenceExecutor: &stubInferenceExecutor{}, statefulHostStub: newStatefulHostStub(), identities: make(chan agentruntime.InferenceIdentity, 4)}
			log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
			require.NoError(t, err)
			router := gin.New()
			RegisterRoutes(router, controller.NewController(svc), executor, executor, nil, log)
			server := httptest.NewServer(router)
			t.Cleanup(server.Close)
			fixture := &utilityFixture{server: server, repo: repo, service: svc}
			seedUtilityAgent(fixture, &models.UtilityAgent{ID: "utility", Name: "Reviewer", Prompt: "Review", Enabled: true, AgentProfileID: "logical", ProfileBindingState: models.ProfileBindingExplicit})
			body := `{"utility_agent_id":"utility","call_id":"forged","execution_profile_id":"forged","owner_kind":"forged"}`
			if sessionID != "" {
				body = `{"utility_agent_id":"utility","session_id":"session","call_id":"forged"}`
			}
			status, response := executePrompt(t, fixture, body)
			require.Equal(t, http.StatusOK, status)
			require.True(t, response.Success)
			require.NotEmpty(t, response.CallID)
			require.NotEqual(t, "forged", response.CallID)
			record := repo.call(response.CallID)
			require.NotNil(t, record)
			require.Equal(t, "completed", record.Status)
			require.Equal(t, "logical", record.AgentProfileID)
			require.Equal(t, "second", record.ExecutionProfileID)
			require.Len(t, executor.identities, 2)
			for _, id := range []string{"first", "second"} {
				require.Equal(t, agentruntime.InferenceIdentity{SessionID: sessionID, LogicalProfileID: "logical", ExecutionProfileID: id, CallID: response.CallID, OwnerKind: "utility"}, <-executor.identities)
			}
		})
	}
}
