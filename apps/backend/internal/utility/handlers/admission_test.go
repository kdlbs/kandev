package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/hostutility"
	settings "github.com/kandev/kandev/internal/agent/settings/models"
	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/utility/controller"
	"github.com/kandev/kandev/internal/utility/dto"
	"github.com/kandev/kandev/internal/utility/models"
	"github.com/kandev/kandev/internal/utility/profilebinding"
	"github.com/kandev/kandev/internal/utility/service"
	"github.com/kandev/kandev/internal/utility/store"
	"github.com/stretchr/testify/require"
)

type admissionProfiles struct {
	reads   atomic.Int32
	enabled atomic.Bool
}

func (p *admissionProfiles) GetAgentProfileIncludingDeleted(_ context.Context, id string) (*settings.AgentProfile, error) {
	p.reads.Add(1)
	if id != "profile" {
		return nil, nil
	}
	return &settings.AgentProfile{ID: id, AgentID: "agent", Model: "model", Enabled: p.enabled.Load()}, nil
}
func (*admissionProfiles) GetAgent(context.Context, string) (*settings.Agent, error) {
	return &settings.Agent{ID: "agent", Name: "codex-acp"}, nil
}
func (*admissionProfiles) ListAgents(context.Context) ([]*settings.Agent, error) { return nil, nil }
func (*admissionProfiles) ListAgentProfiles(context.Context, string) ([]*settings.AgentProfile, error) {
	return nil, nil
}

type admissionExecutor struct {
	*stubInferenceExecutor
	*statefulHostStub
	calls   atomic.Int32
	entered chan struct{}
	release chan struct{}
}

func (e *admissionExecutor) execute() {
	e.calls.Add(1)
	if e.entered != nil {
		close(e.entered)
		<-e.release
	}
}
func (e *admissionExecutor) ExecuteInferenceProfilePrompt(context.Context, string, string, string) (*agentctlutil.PromptResponse, error) {
	e.execute()
	return &agentctlutil.PromptResponse{Success: true, Response: "done"}, nil
}
func (e *admissionExecutor) ExecuteProfilePrompt(context.Context, string, string) (*hostutility.PromptResult, error) {
	e.execute()
	return &hostutility.PromptResult{Response: "done"}, nil
}

type admissionSettings struct{}

func (admissionSettings) GetDefaultUtilitySettings(context.Context) (string, string, error) {
	return "", "", nil
}
func (admissionSettings) GetDefaultUtilityAgentProfileID(context.Context) (string, error) {
	return "profile", nil
}

func admissionRequest(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	result := httptest.NewRecorder()
	router.ServeHTTP(result, req)
	return result
}

// @covers AC-AGENTS-UTILITY-AGENT-PROFILES-001.9
func TestExecuteDisabledCustomAdmission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, session := range []string{"session", ""} {
		t.Run("session="+session, func(t *testing.T) {
			raw, err := db.OpenSQLite(filepath.Join(t.TempDir(), "utility.db"))
			require.NoError(t, err)
			database := sqlx.NewDb(raw, "sqlite3")
			t.Cleanup(func() { require.NoError(t, database.Close()) })
			repo, _, err := store.Provide(database, database)
			require.NoError(t, err)
			profiles := &admissionProfiles{}
			profiles.enabled.Store(true)
			svc := service.NewService(repo)
			svc.SetProfileResolver(profilebinding.New(profiles, func(id string) bool { return id == "codex-acp" }))
			executor := &admissionExecutor{stubInferenceExecutor: &stubInferenceExecutor{}, statefulHostStub: newStatefulHostStub()}
			log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
			require.NoError(t, err)
			router := gin.New()
			RegisterRoutes(router, controller.NewController(svc), executor, executor, admissionSettings{}, log)
			created := admissionRequest(router, http.MethodPost, "/api/v1/utility/agents", `{"name":"Reviewer","prompt":"Review","agent_profile_id":"profile"}`)
			require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
			var agent dto.UtilityAgentDTO
			require.NoError(t, json.Unmarshal(created.Body.Bytes(), &agent))
			path := "/api/v1/utility/agents/" + agent.ID
			bodyBytes, err := json.Marshal(map[string]string{"utility_agent_id": agent.ID, "session_id": session})
			require.NoError(t, err)
			execute := func() *httptest.ResponseRecorder {
				return admissionRequest(router, http.MethodPost, "/api/v1/utility/execute", string(bodyBytes))
			}
			callCount := func() int {
				var n int
				require.NoError(t, database.Get(&n, `SELECT COUNT(*) FROM utility_agent_calls`))
				return n
			}
			positive := execute()
			require.Equal(t, http.StatusOK, positive.Code, positive.Body.String())
			var positiveResponse dto.ExecutePromptResponse
			require.NoError(t, json.Unmarshal(positive.Body.Bytes(), &positiveResponse))
			require.True(t, positiveResponse.Success)
			require.NotEmpty(t, positiveResponse.CallID)
			var status string
			require.NoError(t, database.Get(&status, `SELECT status FROM utility_agent_calls WHERE id = ?`, positiveResponse.CallID))
			require.Equal(t, "completed", status)
			require.Equal(t, int32(1), executor.calls.Load())
			require.Equal(t, 1, callCount())
			disabled := admissionRequest(router, http.MethodPatch, path, `{"enabled":false}`)
			require.Equal(t, http.StatusOK, disabled.Code, disabled.Body.String())
			saved, err := repo.GetAgentByID(context.Background(), agent.ID)
			require.NoError(t, err)
			require.False(t, saved.Enabled)
			before := profiles.reads.Load()
			rejected := execute()
			var response map[string]any
			require.NoError(t, json.Unmarshal(rejected.Body.Bytes(), &response))
			require.Equal(t, http.StatusBadRequest, rejected.Code, rejected.Body.String())
			require.Equal(t, "utility agent is disabled", response["error"])
			require.NotContains(t, response, "call_id")
			require.Equal(t, before, profiles.reads.Load())
			require.Equal(t, int32(1), executor.calls.Load())
			require.Equal(t, 1, callCount())
			enabled := admissionRequest(router, http.MethodPatch, path, `{"enabled":true}`)
			require.Equal(t, http.StatusOK, enabled.Code, enabled.Body.String())
			require.Equal(t, http.StatusOK, execute().Code)
			require.Equal(t, int32(2), executor.calls.Load())
			require.Equal(t, 2, callCount())
			missingBody, err := json.Marshal(map[string]string{"utility_agent_id": "missing", "session_id": session})
			require.NoError(t, err)
			missing := admissionRequest(router, http.MethodPost, "/api/v1/utility/execute", string(missingBody))
			require.Equal(t, http.StatusNotFound, missing.Code, missing.Body.String())
			profiles.enabled.Store(false)
			ineligible := execute()
			require.Equal(t, http.StatusInternalServerError, ineligible.Code, ineligible.Body.String())
			require.Equal(t, int32(2), executor.calls.Load())
			require.Equal(t, 2, callCount())
			profiles.enabled.Store(true)
			builtin := &models.UtilityAgent{ID: "builtin-test", Name: "Built-in", Prompt: "Review", Builtin: true, Enabled: false, ProfileBindingState: models.ProfileBindingInherit}
			require.NoError(t, repo.CreateAgent(context.Background(), builtin))
			builtinBody, err := json.Marshal(map[string]string{"utility_agent_id": builtin.ID, "session_id": session})
			require.NoError(t, err)
			result := admissionRequest(router, http.MethodPost, "/api/v1/utility/execute", string(builtinBody))
			require.Equal(t, http.StatusOK, result.Code, result.Body.String())
			require.Equal(t, int32(3), executor.calls.Load())

			executor.entered = make(chan struct{})
			executor.release = make(chan struct{})
			done := make(chan *httptest.ResponseRecorder, 1)
			go func() { done <- execute() }()
			select {
			case <-executor.entered:
			case <-time.After(5 * time.Second):
				close(executor.release)
				t.Fatal("admitted call did not reach executor")
			}
			changed := admissionRequest(router, http.MethodPatch, path, `{"enabled":false}`)
			close(executor.release)
			admitted := <-done
			require.Equal(t, http.StatusOK, changed.Code, changed.Body.String())
			require.Equal(t, http.StatusOK, admitted.Code, admitted.Body.String())
			require.Equal(t, http.StatusBadRequest, execute().Code)
			require.Equal(t, int32(4), executor.calls.Load())
			require.Equal(t, 4, callCount())
		})
	}
}
