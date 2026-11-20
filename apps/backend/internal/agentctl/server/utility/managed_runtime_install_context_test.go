package utility

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

func TestHandleProbeRequiresAuthenticatedInstallContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	valid := &agentctltypes.ManagedRuntimeInstallContext{
		InstallerEnvDigest: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		OwnerScope:         "agent-wide:owner-a",
	}
	for name, test := range map[string]struct {
		authenticated bool
		context       *agentctltypes.ManagedRuntimeInstallContext
		forbidden     bool
	}{
		"unauthenticated": {false, valid, true},
		"invalid":         {true, &agentctltypes.ManagedRuntimeInstallContext{InstallerEnvDigest: "bad", OwnerScope: "task:owner"}, true},
		"missing owner":   {true, &agentctltypes.ManagedRuntimeInstallContext{InstallerEnvDigest: valid.InstallerEnvDigest}, true},
		"authenticated":   {true, valid, false},
	} {
		t.Run(name, func(t *testing.T) {
			handler := NewHandler("", nil, zap.NewNop())
			handler.SetControlAuthorization(func(*gin.Context) bool { return test.authenticated })
			router := gin.New()
			handler.RegisterRoutes(router.Group("/api/v1"))
			body, err := json.Marshal(ProbeRequest{
				AgentID:         "claude-acp",
				InstallContext:  test.context,
				InferenceConfig: &InferenceConfigDTO{Command: []string{"npx", "--yes"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/v1/inference/probe", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if test.forbidden && recorder.Code != http.StatusForbidden {
				t.Fatalf("probe status = %d (body %s), want forbidden", recorder.Code, recorder.Body.String())
			}
			if !test.forbidden && recorder.Code == http.StatusForbidden {
				t.Fatalf("authenticated probe was forbidden: %s", recorder.Body.String())
			}
		})
	}
}
