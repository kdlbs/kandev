package handlers

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestHTTPKubernetesExecutorMutationsReturnForbiddenWithoutInternalLog(t *testing.T) {
	tests := []struct {
		name   string
		method string
		target string
		body   string
		params gin.Params
		invoke func(*ExecutorHandlers, *gin.Context)
	}{
		{
			name: "create", method: http.MethodPost, target: "/api/v1/executors",
			body:   `{"name":"Kubernetes","type":"k8s","config":{"auth_mode":"kubeconfig","kubeconfig_path":"/etc/kandev/kubeconfig","namespace":"kandev","request_timeout_seconds":"30"}}`,
			invoke: (*ExecutorHandlers).httpCreateExecutor,
		},
		{
			name: "update", method: http.MethodPatch, target: "/api/v1/executors/ex-kubernetes",
			body: `{"name":"Renamed"}`, params: gin.Params{{Key: "id", Value: "ex-kubernetes"}},
			invoke: (*ExecutorHandlers).httpUpdateExecutor,
		},
		{
			name: "delete", method: http.MethodDelete, target: "/api/v1/executors/ex-kubernetes",
			params: gin.Params{{Key: "id", Value: "ex-kubernetes"}},
			invoke: (*ExecutorHandlers).httpDeleteExecutor,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := kubernetesHandlerFixture()
			log, observed := observedErrorLogger(t)
			handler := NewExecutorHandlers(newExecutorService(t, repo), log)
			ctx, recorder := newWorkflowRequest(t, test.method, test.target, test.body)
			ctx.Params = test.params
			ctx.Request = ctx.Request.WithContext(kubernetesMemberContext())

			test.invoke(handler, ctx)

			require.Equal(t, http.StatusForbidden, recorder.Code)
			require.JSONEq(t, `{"error":"administrator identity required for Kubernetes settings"}`, recorder.Body.String())
			require.Empty(t, repo.created)
			require.Empty(t, repo.updated)
			require.Empty(t, repo.deleted)
			require.Zero(t, observed.Len(), "expected authorization denial must not be logged as internal")
		})
	}
}

func TestWSKubernetesExecutorMutationsReturnForbiddenWithoutInternalLog(t *testing.T) {
	tests := []struct {
		name    string
		action  string
		payload map[string]any
		invoke  func(*ExecutorHandlers, context.Context, *ws.Message) (*ws.Message, error)
	}{
		{
			name: "create", action: ws.ActionExecutorCreate,
			payload: map[string]any{
				"name": "Kubernetes", "type": string(models.ExecutorTypeKubernetes),
				"config": validHandlerKubernetesExecutorConfig(),
			},
			invoke: (*ExecutorHandlers).wsCreateExecutor,
		},
		{
			name: "update", action: ws.ActionExecutorUpdate,
			payload: map[string]any{"id": "ex-kubernetes", "name": "Renamed"},
			invoke:  (*ExecutorHandlers).wsUpdateExecutor,
		},
		{
			name: "delete", action: ws.ActionExecutorDelete,
			payload: map[string]any{"id": "ex-kubernetes"},
			invoke:  (*ExecutorHandlers).wsDeleteExecutor,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := kubernetesHandlerFixture()
			log, observed := observedErrorLogger(t)
			handler := NewExecutorHandlers(newExecutorService(t, repo), log)

			response, err := test.invoke(
				handler, kubernetesMemberContext(), wsWorkflowRequest(t, test.action, test.payload),
			)

			require.NoError(t, err)
			payload := wsWorkflowError(t, response)
			require.Equal(t, string(ws.ErrorCodeForbidden), payload.Code)
			require.Equal(t, service.ErrKubernetesAdminRequired.Error(), payload.Message)
			require.Empty(t, repo.created)
			require.Empty(t, repo.updated)
			require.Empty(t, repo.deleted)
			require.Zero(t, observed.Len(), "expected authorization denial must not be logged as internal")
		})
	}
}

func TestHTTPKubernetesProfileMutationsReturnForbiddenWithoutInternalLog(t *testing.T) {
	tests := []struct {
		name   string
		method string
		target string
		body   string
		params gin.Params
		invoke func(*ExecutorProfileHandlers, *gin.Context)
	}{
		{
			name: "create", method: http.MethodPost,
			target: "/api/v1/executors/ex-kubernetes/profiles",
			body:   `{"name":"Default"}`,
			params: gin.Params{{Key: "id", Value: "ex-kubernetes"}},
			invoke: (*ExecutorProfileHandlers).httpCreateProfile,
		},
		{
			name: "update", method: http.MethodPatch,
			target: "/api/v1/executors/ex-kubernetes/profiles/pr-kubernetes",
			body:   `{"name":"Renamed"}`,
			params: gin.Params{{Key: "id", Value: "ex-kubernetes"}, {Key: "profileId", Value: "pr-kubernetes"}},
			invoke: (*ExecutorProfileHandlers).httpUpdateProfile,
		},
		{
			name: "delete", method: http.MethodDelete,
			target: "/api/v1/executor-profiles/pr-kubernetes",
			params: gin.Params{{Key: "profileId", Value: "pr-kubernetes"}},
			invoke: (*ExecutorProfileHandlers).httpDeleteProfile,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := kubernetesHandlerFixture()
			log, observed := observedErrorLogger(t)
			handler := NewExecutorProfileHandlers(newExecutorService(t, repo), nil, log)
			ctx, recorder := newWorkflowRequest(t, test.method, test.target, test.body)
			ctx.Params = test.params
			ctx.Request = ctx.Request.WithContext(kubernetesMemberContext())

			test.invoke(handler, ctx)

			require.Equal(t, http.StatusForbidden, recorder.Code)
			require.JSONEq(t, `{"error":"administrator identity required for Kubernetes settings"}`, recorder.Body.String())
			require.Empty(t, repo.createdProfile)
			require.Empty(t, repo.updatedProfile)
			require.Empty(t, repo.deletedProfile)
			require.Zero(t, observed.Len(), "expected authorization denial must not be logged as internal")
		})
	}
}

func TestWSKubernetesProfileMutationsReturnForbiddenWithoutInternalLog(t *testing.T) {
	tests := []struct {
		name    string
		action  string
		payload map[string]any
		invoke  func(*ExecutorProfileHandlers, context.Context, *ws.Message) (*ws.Message, error)
	}{
		{
			name: "create", action: ws.ActionExecutorProfileCreate,
			payload: map[string]any{"executor_id": "ex-kubernetes", "name": "Default"},
			invoke:  (*ExecutorProfileHandlers).wsCreateProfile,
		},
		{
			name: "update", action: ws.ActionExecutorProfileUpdate,
			payload: map[string]any{"id": "pr-kubernetes", "name": "Renamed"},
			invoke:  (*ExecutorProfileHandlers).wsUpdateProfile,
		},
		{
			name: "delete", action: ws.ActionExecutorProfileDelete,
			payload: map[string]any{"id": "pr-kubernetes"},
			invoke:  (*ExecutorProfileHandlers).wsDeleteProfile,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := kubernetesHandlerFixture()
			log, observed := observedErrorLogger(t)
			handler := NewExecutorProfileHandlers(newExecutorService(t, repo), nil, log)

			response, err := test.invoke(
				handler, kubernetesMemberContext(), wsWorkflowRequest(t, test.action, test.payload),
			)

			require.NoError(t, err)
			payload := wsWorkflowError(t, response)
			require.Equal(t, string(ws.ErrorCodeForbidden), payload.Code)
			require.Equal(t, service.ErrKubernetesAdminRequired.Error(), payload.Message)
			require.Empty(t, repo.createdProfile)
			require.Empty(t, repo.updatedProfile)
			require.Empty(t, repo.deletedProfile)
			require.Zero(t, observed.Len(), "expected authorization denial must not be logged as internal")
		})
	}
}

func TestHTTPKubernetesProfileValidationReturnsBadRequestWithoutInternalLog(t *testing.T) {
	tests := []struct {
		name   string
		method string
		target string
		params gin.Params
		invoke func(*ExecutorProfileHandlers, *gin.Context)
	}{
		{
			name: "create", method: http.MethodPost,
			target: "/api/v1/executors/ex-kubernetes/profiles",
			params: gin.Params{{Key: "id", Value: "ex-kubernetes"}},
			invoke: (*ExecutorProfileHandlers).httpCreateProfile,
		},
		{
			name: "update", method: http.MethodPatch,
			target: "/api/v1/executors/ex-kubernetes/profiles/pr-kubernetes",
			params: gin.Params{{Key: "id", Value: "ex-kubernetes"}, {Key: "profileId", Value: "pr-kubernetes"}},
			invoke: (*ExecutorProfileHandlers).httpUpdateProfile,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := kubernetesHandlerFixture()
			log, observed := observedErrorLogger(t)
			handler := NewExecutorProfileHandlers(newExecutorService(t, repo), nil, log)
			ctx, recorder := newWorkflowRequest(
				t, test.method, test.target,
				`{"name":"Invalid","config":{"platform":"windows/amd64"}}`,
			)
			ctx.Params = test.params
			ctx.Request = ctx.Request.WithContext(kubernetesAdminContext())

			test.invoke(handler, ctx)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Contains(t, recorder.Body.String(), service.ErrInvalidExecutorConfig.Error())
			require.Contains(t, recorder.Body.String(), "config.platform")
			require.Empty(t, repo.createdProfile)
			require.Empty(t, repo.updatedProfile)
			require.Zero(t, observed.Len(), "expected validation denial must not be logged as internal")
		})
	}
}

func TestWSKubernetesProfileValidationReturnsValidationWithoutInternalLog(t *testing.T) {
	tests := []struct {
		name    string
		action  string
		payload map[string]any
		invoke  func(*ExecutorProfileHandlers, context.Context, *ws.Message) (*ws.Message, error)
	}{
		{
			name: "create", action: ws.ActionExecutorProfileCreate,
			payload: map[string]any{
				"executor_id": "ex-kubernetes", "name": "Invalid",
				"config": map[string]string{"platform": "windows/amd64"},
			},
			invoke: (*ExecutorProfileHandlers).wsCreateProfile,
		},
		{
			name: "update", action: ws.ActionExecutorProfileUpdate,
			payload: map[string]any{
				"id": "pr-kubernetes", "config": map[string]string{"platform": "windows/amd64"},
			},
			invoke: (*ExecutorProfileHandlers).wsUpdateProfile,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := kubernetesHandlerFixture()
			log, observed := observedErrorLogger(t)
			handler := NewExecutorProfileHandlers(newExecutorService(t, repo), nil, log)

			response, err := test.invoke(
				handler, kubernetesAdminContext(), wsWorkflowRequest(t, test.action, test.payload),
			)

			require.NoError(t, err)
			payload := wsWorkflowError(t, response)
			require.Equal(t, string(ws.ErrorCodeValidation), payload.Code)
			require.Equal(t, service.ErrInvalidExecutorConfig.Error(), payload.Message)
			require.Empty(t, repo.createdProfile)
			require.Empty(t, repo.updatedProfile)
			require.Zero(t, observed.Len(), "expected validation denial must not be logged as internal")
		})
	}
}

func TestKubernetesExecutorAndProfileReadsRemainAvailableToMemberHandlers(t *testing.T) {
	repo := kubernetesHandlerFixture()
	log, observed := observedErrorLogger(t)
	executorHandler := NewExecutorHandlers(newExecutorService(t, repo), log)
	profileHandler := NewExecutorProfileHandlers(newExecutorService(t, repo), nil, log)

	executorCtx, executorRecorder := newWorkflowRequest(
		t, http.MethodGet, "/api/v1/executors/ex-kubernetes", "",
	)
	executorCtx.Params = gin.Params{{Key: "id", Value: "ex-kubernetes"}}
	executorCtx.Request = executorCtx.Request.WithContext(kubernetesMemberContext())
	executorHandler.httpGetExecutor(executorCtx)

	profileCtx, profileRecorder := newWorkflowRequest(
		t, http.MethodGet, "/api/v1/executors/ex-kubernetes/profiles/pr-kubernetes", "",
	)
	profileCtx.Params = gin.Params{{Key: "profileId", Value: "pr-kubernetes"}}
	profileCtx.Request = profileCtx.Request.WithContext(kubernetesMemberContext())
	profileHandler.httpGetProfile(profileCtx)

	require.Equal(t, http.StatusOK, executorRecorder.Code)
	require.Equal(t, http.StatusOK, profileRecorder.Code)
	require.Zero(t, observed.Len())
}

func kubernetesHandlerFixture() *executorRepo {
	repo := executorFixture()
	executor := &models.Executor{
		ID: "ex-kubernetes", Name: "Kubernetes", Type: models.ExecutorTypeKubernetes,
		Status: models.ExecutorStatusActive, Config: validHandlerKubernetesExecutorConfig(),
	}
	profile := &models.ExecutorProfile{
		ID: "pr-kubernetes", ExecutorID: executor.ID, Name: "Default",
		Config: validHandlerKubernetesProfileConfig(),
	}
	repo.executors = append(repo.executors, executor)
	repo.byID[executor.ID] = executor
	repo.profiles = append(repo.profiles, profile)
	repo.profilesByExecutor[executor.ID] = []*models.ExecutorProfile{profile}
	repo.profileByID[profile.ID] = profile
	return repo
}

func validHandlerKubernetesExecutorConfig() map[string]string {
	return map[string]string{
		"auth_mode": "kubeconfig", "kubeconfig_path": "/etc/kandev/kubeconfig",
		"namespace": "kandev", "request_timeout_seconds": "30",
	}
}

func validHandlerKubernetesProfileConfig() map[string]string {
	return map[string]string{
		"platform":       "linux/amd64",
		"main_container": "kandev-agent",
		"pod_template_yaml": `apiVersion: v1
kind: PodTemplate
template:
  spec:
    containers:
      - name: kandev-agent
        image: alpine:3.20
`,
		"workspace.mode": "empty_dir",
	}
}

func kubernetesMemberContext() context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "member-1", Role: authn.RoleMember,
	})
}

func kubernetesAdminContext() context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "admin-1", Role: authn.RoleAdmin,
	})
}

func observedErrorLogger(t *testing.T) (*logger.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, observed := observer.New(zap.ErrorLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	return log, observed
}
