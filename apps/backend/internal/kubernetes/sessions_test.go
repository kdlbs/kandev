package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	agentkubernetes "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestHTTPListSessionsAllowsMemberAndGetsOnlyAuthorizedRecordedPods(t *testing.T) {
	createdAt := time.Date(2026, time.August, 24, 9, 30, 0, 0, time.UTC)
	repo := &fakeResourceRepository{
		executor: &models.Executor{
			ID: "executor-1", Type: models.ExecutorTypeKubernetes, Config: validHandlerExecutorConfig(),
		},
		runs: []*models.ExecutorRunning{
			kubernetesRunningRow("instance-1", "session-1", "task-1", "pod-1", "pod-uid-1", createdAt),
			kubernetesRunningRow("instance-2", "session-2", "task-2", "pod-2", "pod-uid-2", createdAt),
			{ID: "ssh-instance", Runtime: agentruntime.RuntimeSSH, SessionID: "ssh-session"},
		},
		sessions: map[string]*models.TaskSession{
			"session-1": kubernetesTaskSession("session-1", "task-1", "executor-1", "profile-1"),
			"session-2": kubernetesTaskSession("session-2", "task-2", "executor-1", "profile-1"),
		},
	}
	clientset := kubernetesfake.NewSimpleClientset(kubernetesOwnedPod(
		"pod-1", "pod-uid-1", "instance-1", repo.sessions["session-1"],
	))
	access := &fakeAccessChecker{denied: map[string]error{"task-2": repoerrors.ErrTaskNotFound}}
	handler := NewHandler(repo, access, func(agentkubernetes.ExecutorConfig) (*agentkubernetes.Client, error) {
		return &agentkubernetes.Client{Clientset: clientset}, nil
	})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, authn.Identity{UserID: "member-1", Role: authn.RoleMember})
		c.Next()
	})
	handler.registerHTTP(router)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/api/v1/kubernetes/executors/executor-1/sessions", nil,
	))

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var rows []SessionRow
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &rows))
	require.Equal(t, []SessionRow{{
		SessionID: "session-1", TaskID: "task-1", PodName: "pod-1",
		PodPhase: "Running", ContainerState: "running", Restarts: 2,
		WorkspaceKind: "empty_dir", CreatedAt: createdAt.Format(time.RFC3339),
	}}, rows)
	require.Equal(t, []string{"task-1", "task-2"}, access.calls)
	require.Len(t, clientset.Actions(), 1)
	getAction, ok := clientset.Actions()[0].(k8stesting.GetAction)
	require.True(t, ok)
	require.Equal(t, "pods", getAction.GetResource().Resource)
	require.Equal(t, "pod-1", getAction.GetName())
}

func TestWebSocketListSessionsAllowsMember(t *testing.T) {
	repo := &fakeResourceRepository{
		executor: &models.Executor{
			ID: "executor-1", Type: models.ExecutorTypeKubernetes, Config: validHandlerExecutorConfig(),
		},
		sessions: map[string]*models.TaskSession{},
	}
	dispatcher := ws.NewDispatcher()
	handler := NewHandler(repo, &fakeAccessChecker{}, func(agentkubernetes.ExecutorConfig) (*agentkubernetes.Client, error) {
		return &agentkubernetes.Client{Clientset: kubernetesfake.NewSimpleClientset()}, nil
	})
	handler.registerWS(dispatcher)
	msg, err := ws.NewRequest("req-1", "kubernetes.sessions.list", map[string]string{
		"executor_id": "executor-1",
	})
	require.NoError(t, err)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "member-1", Role: authn.RoleMember,
	})

	response, err := dispatcher.Dispatch(ctx, msg)

	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.JSONEq(t, `[]`, string(response.Payload))
}

func TestHTTPListSessionsFiltersTaskSessionBeforeAuthorizationAndPodLookup(t *testing.T) {
	createdAt := time.Date(2026, time.August, 25, 9, 30, 0, 0, time.UTC)
	repo := &fakeResourceRepository{
		executor: &models.Executor{
			ID: "executor-1", Type: models.ExecutorTypeKubernetes, Config: validHandlerExecutorConfig(),
		},
		runs: []*models.ExecutorRunning{
			kubernetesRunningRow("instance-1", "session-1", "task-1", "pod-1", "pod-uid-1", createdAt),
			kubernetesRunningRow("instance-2", "session-2", "task-2", "pod-2", "pod-uid-2", createdAt),
		},
		sessions: map[string]*models.TaskSession{
			"session-1": kubernetesTaskSession("session-1", "task-1", "executor-1", "profile-1"),
			"session-2": kubernetesTaskSession("session-2", "task-2", "executor-1", "profile-1"),
		},
	}
	clientset := kubernetesfake.NewSimpleClientset(
		kubernetesOwnedPod("pod-1", "pod-uid-1", "instance-1", repo.sessions["session-1"]),
		kubernetesOwnedPod("pod-2", "pod-uid-2", "instance-2", repo.sessions["session-2"]),
	)
	access := &fakeAccessChecker{}
	handler := NewHandler(repo, access, func(agentkubernetes.ExecutorConfig) (*agentkubernetes.Client, error) {
		return &agentkubernetes.Client{Clientset: clientset}, nil
	})
	router := gin.New()
	handler.registerHTTP(router)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet,
		"/api/v1/kubernetes/executors/executor-1/sessions?task_id=task-1&session_id=session-1",
		nil,
	))

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var rows []SessionRow
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &rows))
	require.Len(t, rows, 1)
	require.Equal(t, "session-1", rows[0].SessionID)
	require.Equal(t, []string{"task-1"}, access.calls)
	require.Len(t, clientset.Actions(), 1)
	getAction, ok := clientset.Actions()[0].(k8stesting.GetAction)
	require.True(t, ok)
	require.Equal(t, "pod-1", getAction.GetName())

	clientset.ClearActions()
	access.calls = nil
	mismatchRecorder := httptest.NewRecorder()
	router.ServeHTTP(mismatchRecorder, httptest.NewRequest(
		http.MethodGet,
		"/api/v1/kubernetes/executors/executor-1/sessions?task_id=task-1&session_id=session-2",
		nil,
	))
	require.Equal(t, http.StatusOK, mismatchRecorder.Code, mismatchRecorder.Body.String())
	require.JSONEq(t, `[]`, mismatchRecorder.Body.String())
	require.Empty(t, access.calls)
	require.Empty(t, clientset.Actions())
}

func TestHTTPListSessionsRejectsSessionFilterWithoutTask(t *testing.T) {
	handler := NewHandler(&fakeResourceRepository{}, &fakeAccessChecker{}, nil)
	router := gin.New()
	handler.registerHTTP(router)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet,
		"/api/v1/kubernetes/executors/executor-1/sessions?session_id=session-1",
		nil,
	))

	require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
}

func TestWebSocketListSessionsFiltersTaskSessionAndRejectsSessionOnly(t *testing.T) {
	createdAt := time.Date(2026, time.August, 25, 9, 30, 0, 0, time.UTC)
	repo := &fakeResourceRepository{
		executor: &models.Executor{
			ID: "executor-1", Type: models.ExecutorTypeKubernetes, Config: validHandlerExecutorConfig(),
		},
		runs: []*models.ExecutorRunning{
			kubernetesRunningRow("instance-1", "session-1", "task-1", "pod-1", "pod-uid-1", createdAt),
			kubernetesRunningRow("instance-2", "session-2", "task-2", "pod-2", "pod-uid-2", createdAt),
		},
		sessions: map[string]*models.TaskSession{
			"session-1": kubernetesTaskSession("session-1", "task-1", "executor-1", "profile-1"),
			"session-2": kubernetesTaskSession("session-2", "task-2", "executor-1", "profile-1"),
		},
	}
	clientset := kubernetesfake.NewSimpleClientset(
		kubernetesOwnedPod("pod-1", "pod-uid-1", "instance-1", repo.sessions["session-1"]),
		kubernetesOwnedPod("pod-2", "pod-uid-2", "instance-2", repo.sessions["session-2"]),
	)
	access := &fakeAccessChecker{}
	dispatcher := ws.NewDispatcher()
	handler := NewHandler(repo, access, func(agentkubernetes.ExecutorConfig) (*agentkubernetes.Client, error) {
		return &agentkubernetes.Client{Clientset: clientset}, nil
	})
	handler.registerWS(dispatcher)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "member-1", Role: authn.RoleMember,
	})
	filtered, err := ws.NewRequest("req-1", "kubernetes.sessions.list", map[string]string{
		"executor_id": "executor-1",
		"task_id":     "task-1",
		"session_id":  "session-1",
	})
	require.NoError(t, err)

	response, err := dispatcher.Dispatch(ctx, filtered)

	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	var rows []SessionRow
	require.NoError(t, json.Unmarshal(response.Payload, &rows))
	require.Len(t, rows, 1)
	require.Equal(t, "session-1", rows[0].SessionID)
	require.Equal(t, []string{"task-1"}, access.calls)
	require.Len(t, clientset.Actions(), 1)

	sessionOnly, err := ws.NewRequest("req-2", "kubernetes.sessions.list", map[string]string{
		"executor_id": "executor-1",
		"session_id":  "session-1",
	})
	require.NoError(t, err)
	response, err = dispatcher.Dispatch(ctx, sessionOnly)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	var wsError ws.ErrorPayload
	require.NoError(t, json.Unmarshal(response.Payload, &wsError))
	require.Equal(t, ws.ErrorCodeBadRequest, wsError.Code)
}

func TestHTTPSessionImpactCountsEveryExactKubernetesInventoryRowForAdmin(t *testing.T) {
	repo := &fakeResourceRepository{
		executor: &models.Executor{ID: "executor-1", Type: models.ExecutorTypeKubernetes},
		runs: []*models.ExecutorRunning{
			{ExecutorID: "executor-1", Runtime: agentruntime.RuntimeKubernetes},
			{ExecutorID: "executor-1", Runtime: agentruntime.RuntimeKubernetes, Metadata: map[string]interface{}{}},
			{ExecutorID: "executor-1", Runtime: agentruntime.RuntimeKubernetes, Status: models.ExecutorRunningStatusFailed},
			{ExecutorID: "executor-1", Runtime: agentruntime.RuntimeKubernetes, Status: models.ExecutorRunningStatusStopped},
			{ExecutorID: "executor-1", Runtime: agentruntime.RuntimeKubernetes, Status: models.ExecutorRunningStatusComplete},
			{ExecutorID: "executor-1", Runtime: agentruntime.RuntimeDocker},
			{ExecutorID: "executor-other", Runtime: agentruntime.RuntimeKubernetes},
			nil,
		},
	}
	handler := NewHandler(repo, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, authn.Identity{UserID: "admin-1", Role: authn.RoleAdmin})
		c.Next()
	})
	handler.registerHTTP(router)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/api/v1/kubernetes/executors/executor-1/session-impact", nil,
	))

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{"active_session_count":2}`, recorder.Body.String())
}

func TestSessionImpactForbidsMembersOverHTTPAndWebSocket(t *testing.T) {
	repo := &fakeResourceRepository{
		executor: &models.Executor{ID: "executor-1", Type: models.ExecutorTypeKubernetes},
	}
	handler := NewHandler(repo, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, authn.Identity{UserID: "member-1", Role: authn.RoleMember})
		c.Next()
	})
	handler.registerHTTP(router)
	httpRecorder := httptest.NewRecorder()
	router.ServeHTTP(httpRecorder, httptest.NewRequest(
		http.MethodGet, "/api/v1/kubernetes/executors/executor-1/session-impact", nil,
	))
	require.Equal(t, http.StatusForbidden, httpRecorder.Code)

	dispatcher := ws.NewDispatcher()
	handler.registerWS(dispatcher)
	msg, err := ws.NewRequest("req-1", "kubernetes.sessions.impact", map[string]string{
		"executor_id": "executor-1",
	})
	require.NoError(t, err)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "member-1", Role: authn.RoleMember,
	})
	response, err := dispatcher.Dispatch(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	var payload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(response.Payload, &payload))
	require.Equal(t, ws.ErrorCodeForbidden, payload.Code)
}

func TestHTTPSessionImpactValidatesExecutorAndPropagatesRepositoryFailures(t *testing.T) {
	repositoryFailure := errors.New("inventory unavailable")
	tests := []struct {
		name       string
		repo       *fakeResourceRepository
		wantStatus int
	}{
		{
			name: "executor not found", repo: &fakeResourceRepository{},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "wrong executor type",
			repo: &fakeResourceRepository{executor: &models.Executor{
				ID: "executor-1", Type: models.ExecutorTypeLocal,
			}},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "executor repository failure",
			repo: &fakeResourceRepository{
				executorErr: repositoryFailure,
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "inventory repository failure",
			repo: &fakeResourceRepository{
				executor: &models.Executor{ID: "executor-1", Type: models.ExecutorTypeKubernetes},
				runsErr:  repositoryFailure,
			},
			wantStatus: http.StatusInternalServerError,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := NewHandler(test.repo, nil, nil)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				authn.SetOnGin(c, authn.Identity{UserID: "admin-1", Role: authn.RoleAdmin})
				c.Next()
			})
			handler.registerHTTP(router)
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, httptest.NewRequest(
				http.MethodGet, "/api/v1/kubernetes/executors/executor-1/session-impact", nil,
			))

			require.Equal(t, test.wantStatus, recorder.Code, recorder.Body.String())
		})
	}
}

func TestHTTPListSessionsReturns500ForUnexpectedAccessError(t *testing.T) {
	run := kubernetesRunningRow(
		"instance-1", "session-1", "task-1", "pod-1", "pod-uid-1", time.Now(),
	)
	repo := &fakeResourceRepository{
		executor: &models.Executor{
			ID: "executor-1", Type: models.ExecutorTypeKubernetes,
			Config: validHandlerExecutorConfig(),
		},
		runs: []*models.ExecutorRunning{run},
		sessions: map[string]*models.TaskSession{
			"session-1": kubernetesTaskSession("session-1", "task-1", "executor-1", "profile-1"),
		},
	}
	handler := NewHandler(
		repo,
		&fakeAccessChecker{denied: map[string]error{
			"task-1": errors.New("workspace database unavailable"),
		}},
		func(agentkubernetes.ExecutorConfig) (*agentkubernetes.Client, error) {
			return &agentkubernetes.Client{Clientset: kubernetesfake.NewSimpleClientset()}, nil
		},
	)
	router := gin.New()
	handler.registerHTTP(router)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/api/v1/kubernetes/executors/executor-1/sessions", nil,
	))

	require.Equal(t, http.StatusInternalServerError, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{"error":"Kubernetes session status is unavailable"}`, recorder.Body.String())
}

func TestListSessionsPropagatesUnexpectedSessionAndAccessErrors(t *testing.T) {
	tests := []struct {
		name       string
		sessionErr error
		accessErr  error
	}{
		{name: "session repository", sessionErr: errors.New("session database unavailable")},
		{name: "access repository", accessErr: errors.New("workspace database unavailable")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run := kubernetesRunningRow(
				"instance-1", "session-1", "task-1", "pod-1", "pod-uid-1", time.Now(),
			)
			repo := &fakeResourceRepository{
				executor: &models.Executor{
					ID: "executor-1", Type: models.ExecutorTypeKubernetes,
					Config: validHandlerExecutorConfig(),
				},
				runs: []*models.ExecutorRunning{run},
				sessions: map[string]*models.TaskSession{
					"session-1": kubernetesTaskSession("session-1", "task-1", "executor-1", "profile-1"),
				},
				sessionErrs: map[string]error{"session-1": test.sessionErr},
			}
			access := &fakeAccessChecker{denied: map[string]error{"task-1": test.accessErr}}
			handler := NewHandler(repo, access, func(agentkubernetes.ExecutorConfig) (*agentkubernetes.Client, error) {
				return &agentkubernetes.Client{Clientset: kubernetesfake.NewSimpleClientset()}, nil
			})

			rows, err := handler.listSessions(context.Background(), "executor-1", SessionFilter{})

			require.Error(t, err)
			require.Empty(t, rows)
			if test.sessionErr != nil {
				require.ErrorIs(t, err, test.sessionErr)
			} else {
				require.ErrorIs(t, err, test.accessErr)
			}
		})
	}
}

func TestListSessionsFailsClosedOnPodIdentityMismatch(t *testing.T) {
	createdAt := time.Date(2026, time.August, 24, 9, 30, 0, 0, time.UTC)
	session := kubernetesTaskSession("session-1", "task-1", "executor-1", "profile-1")
	repo := &fakeResourceRepository{
		executor: &models.Executor{
			ID: "executor-1", Type: models.ExecutorTypeKubernetes, Config: validHandlerExecutorConfig(),
		},
		runs: []*models.ExecutorRunning{
			kubernetesRunningRow("instance-1", "session-1", "task-1", "pod-1", "recorded-uid", createdAt),
		},
		sessions: map[string]*models.TaskSession{"session-1": session},
	}
	pod := kubernetesOwnedPod("pod-1", "different-uid", "instance-1", session)
	handler := NewHandler(repo, &fakeAccessChecker{}, func(agentkubernetes.ExecutorConfig) (*agentkubernetes.Client, error) {
		return &agentkubernetes.Client{Clientset: kubernetesfake.NewSimpleClientset(pod)}, nil
	})

	rows, err := handler.listSessions(context.Background(), "executor-1", SessionFilter{})

	require.NoError(t, err)
	require.Equal(t, []SessionRow{{
		SessionID: "session-1", TaskID: "task-1", PodName: "pod-1",
		WorkspaceKind: "empty_dir", CreatedAt: createdAt.Format(time.RFC3339),
		FailureReason: "Pod identity does not match runtime inventory",
	}}, rows)
}

func TestListSessionsUsesAuthoritativeRunningExecutorAfterSessionRepoint(t *testing.T) {
	createdAt := time.Date(2026, time.August, 24, 9, 30, 0, 0, time.UTC)
	run := kubernetesRunningRow(
		"instance-1", "session-1", "task-1", "pod-1", "pod-uid-1", createdAt,
	)
	session := kubernetesTaskSession("session-1", "task-1", "repointed-executor", "current-profile")
	clientset := kubernetesfake.NewSimpleClientset(kubernetesOwnedPodForIdentity(
		"pod-1", "pod-uid-1", agentkubernetes.ResourceIdentity{
			ExecutorID: "executor-1", ProfileID: "profile-1", InstanceID: "instance-1",
			TaskID: "task-1", SessionID: "session-1", EnvironmentID: "environment-1",
		},
	))
	repo := &fakeResourceRepository{
		executor: &models.Executor{
			ID: "executor-1", Type: models.ExecutorTypeKubernetes, Config: validHandlerExecutorConfig(),
		},
		runs:     []*models.ExecutorRunning{run},
		sessions: map[string]*models.TaskSession{"session-1": session},
	}
	handler := NewHandler(repo, &fakeAccessChecker{}, func(agentkubernetes.ExecutorConfig) (*agentkubernetes.Client, error) {
		return &agentkubernetes.Client{Clientset: clientset}, nil
	})

	rows, err := handler.listSessions(context.Background(), "executor-1", SessionFilter{})

	require.NoError(t, err)
	require.Equal(t, []SessionRow{{
		SessionID: "session-1", TaskID: "task-1", PodName: "pod-1",
		PodPhase: "Running", ContainerState: "running", Restarts: 2,
		WorkspaceKind: "empty_dir", CreatedAt: createdAt.Format(time.RFC3339),
	}}, rows)
}

func TestListSessionsOmitsKubernetesRowWithoutRecordedExecutorBeforeAuthorization(t *testing.T) {
	run := kubernetesRunningRow(
		"instance-1", "session-1", "task-1", "pod-1", "pod-uid-1", time.Now(),
	)
	run.ExecutorID = ""
	repo := &fakeResourceRepository{
		executor: &models.Executor{
			ID: "executor-1", Type: models.ExecutorTypeKubernetes, Config: validHandlerExecutorConfig(),
		},
		runs: []*models.ExecutorRunning{run},
		sessions: map[string]*models.TaskSession{
			"session-1": kubernetesTaskSession("session-1", "task-1", "executor-1", "profile-1"),
		},
	}
	clientset := kubernetesfake.NewSimpleClientset()
	access := &fakeAccessChecker{}
	handler := NewHandler(repo, access, func(agentkubernetes.ExecutorConfig) (*agentkubernetes.Client, error) {
		return &agentkubernetes.Client{Clientset: clientset}, nil
	})

	rows, err := handler.listSessions(context.Background(), "executor-1", SessionFilter{})

	require.NoError(t, err)
	require.Empty(t, rows, "an unassociated row must not appear under every Kubernetes executor")
	require.Empty(t, access.calls, "an unassociated row must stop before authorization")
	require.Empty(t, clientset.Actions(), "an unassociated row must stop before Kubernetes API access")
}

func TestListSessionsRejectsUncorrelatedTaskSessionBeforeAuthorization(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*models.TaskSession)
	}{
		{name: "session id", mutate: func(session *models.TaskSession) { session.ID = "other-session" }},
		{name: "task id", mutate: func(session *models.TaskSession) { session.TaskID = "other-task" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run := kubernetesRunningRow(
				"instance-1", "session-1", "task-1", "pod-1", "pod-uid-1", time.Now(),
			)
			session := kubernetesTaskSession("session-1", "task-1", "executor-1", "profile-1")
			test.mutate(session)
			repo := &fakeResourceRepository{
				executor: &models.Executor{
					ID: "executor-1", Type: models.ExecutorTypeKubernetes,
					Config: validHandlerExecutorConfig(),
				},
				runs: []*models.ExecutorRunning{run},
				sessions: map[string]*models.TaskSession{
					run.SessionID: session,
				},
			}
			clientset := kubernetesfake.NewSimpleClientset()
			access := &fakeAccessChecker{}
			handler := NewHandler(repo, access, func(agentkubernetes.ExecutorConfig) (*agentkubernetes.Client, error) {
				return &agentkubernetes.Client{Clientset: clientset}, nil
			})

			rows, err := handler.listSessions(context.Background(), "executor-1", SessionFilter{})

			require.NoError(t, err)
			require.Empty(t, rows)
			require.Empty(t, access.calls, "uncorrelated row must stop before authorization")
			require.Empty(t, clientset.Actions(), "uncorrelated row must stop before Kubernetes API access")
		})
	}
}

func TestMatchesSessionIdentityRejectsRecordedNameOrNamespaceMismatch(t *testing.T) {
	session := kubernetesTaskSession("session-1", "task-1", "executor-1", "profile-1")
	run := kubernetesRunningRow(
		"instance-1", "session-1", "task-1", "pod-1", "pod-uid-1", time.Now(),
	)
	pod := kubernetesOwnedPod("pod-1", "pod-uid-1", "instance-1", session)
	pod.Name = "different-pod"

	require.False(t, matchesSessionIdentity(pod, run))
	pod.Name = "pod-1"
	pod.Namespace = "different-namespace"
	require.False(t, matchesSessionIdentity(pod, run))
}

func TestMatchesSessionIdentityUsesPersistedResourceInstanceID(t *testing.T) {
	session := kubernetesTaskSession("session-1", "task-1", "executor-1", "profile-1")
	run := kubernetesRunningRow(
		"resource-instance-1", "session-1", "task-1", "pod-1", "pod-uid-1", time.Now(),
	)
	pod := kubernetesOwnedPod("pod-1", "pod-uid-1", "resource-instance-1", session)

	require.Equal(t, "session-1", run.ID)
	require.True(t, matchesSessionIdentity(pod, run))
}

func TestMatchesSessionIdentityUsesEntireRecordedIdentityAfterSessionEdit(t *testing.T) {
	run := kubernetesRunningRow(
		"resource-instance-1", "session-1", "task-1", "pod-1", "pod-uid-1", time.Now(),
	)
	session := kubernetesTaskSession("session-1", "task-1", "executor-1", "current-profile")
	session.TaskEnvironmentID = "current-environment"
	pod := kubernetesOwnedPodForIdentity("pod-1", "pod-uid-1", agentkubernetes.ResourceIdentity{
		ExecutorID: "executor-1", ProfileID: "profile-1", InstanceID: "resource-instance-1",
		TaskID: "task-1", SessionID: "session-1", EnvironmentID: "environment-1",
	})

	require.NotEqual(t, session.ExecutorProfileID, "profile-1")
	require.NotEqual(t, session.TaskEnvironmentID, "environment-1")
	require.True(t, matchesSessionIdentity(pod, run))
}

func TestMatchesSessionIdentityRejectsAdditionalKandevOwnershipLabel(t *testing.T) {
	run := kubernetesRunningRow(
		"resource-instance-1", "session-1", "task-1", "pod-1", "pod-uid-1", time.Now(),
	)
	pod := kubernetesOwnedPodForIdentity("pod-1", "pod-uid-1", agentkubernetes.ResourceIdentity{
		ExecutorID: "executor-1", ProfileID: "profile-1", InstanceID: "resource-instance-1",
		TaskID: "task-1", SessionID: "session-1", EnvironmentID: "environment-1",
	})
	pod.Labels["kandev.ai/forged"] = "true"

	require.False(t, matchesSessionIdentity(pod, run))
	delete(pod.Labels, "kandev.ai/forged")
	pod.Labels["example.com/injected"] = "true"
	require.True(t, matchesSessionIdentity(pod, run), "unrelated admission labels remain allowed")
}

func TestValidateSessionInventoryRequiresEntireRecordedIdentity(t *testing.T) {
	keys := []string{
		metadataResourceExecutor,
		metadataResourceProfile,
		metadataResourceInstance,
		metadataResourceTask,
		metadataResourceSession,
		metadataResourceEnv,
	}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			run := kubernetesRunningRow(
				"resource-instance-1", "session-1", "task-1", "pod-1", "pod-uid-1", time.Now(),
			)
			delete(run.Metadata, key)
			session := kubernetesTaskSession("session-1", "task-1", "executor-1", "profile-1")

			failure := validateSessionInventory(run, session.ExecutorID, newInventorySessionRow(run))

			require.Equal(t, "Kubernetes runtime inventory is incomplete", failure)
		})
	}
}

type fakeResourceRepository struct {
	executor    *models.Executor
	executorErr error
	runs        []*models.ExecutorRunning
	runsErr     error
	sessions    map[string]*models.TaskSession
	sessionErrs map[string]error
}

func (r *fakeResourceRepository) ListExecutorsRunning(context.Context) ([]*models.ExecutorRunning, error) {
	return r.runs, r.runsErr
}

func (r *fakeResourceRepository) GetTaskSession(_ context.Context, id string) (*models.TaskSession, error) {
	if err := r.sessionErrs[id]; err != nil {
		return nil, err
	}
	session := r.sessions[id]
	if session == nil {
		return nil, errors.New("session not found")
	}
	return session, nil
}

func (r *fakeResourceRepository) GetExecutor(context.Context, string) (*models.Executor, error) {
	if r.executorErr != nil {
		return nil, r.executorErr
	}
	if r.executor == nil {
		return nil, models.ErrExecutorNotFound
	}
	return r.executor, nil
}

type fakeAccessChecker struct {
	denied map[string]error
	calls  []string
}

func (c *fakeAccessChecker) AuthorizeTaskAccess(_ context.Context, taskID string) error {
	c.calls = append(c.calls, taskID)
	return c.denied[taskID]
}

func kubernetesRunningRow(
	instanceID, sessionID, taskID, podName, podUID string,
	createdAt time.Time,
) *models.ExecutorRunning {
	return &models.ExecutorRunning{
		ID: sessionID, Runtime: agentruntime.RuntimeKubernetes,
		SessionID: sessionID, TaskID: taskID, ExecutorID: "executor-1", CreatedAt: createdAt,
		Metadata: map[string]interface{}{
			metadataNamespace: "kandev", metadataPodName: podName, metadataPodUID: podUID,
			metadataMainContainer: "kandev-agent", metadataWorkspaceMode: "empty_dir",
			metadataResourceExecutor: "executor-1",
			metadataResourceProfile:  "profile-1",
			metadataResourceInstance: instanceID,
			metadataResourceTask:     taskID,
			metadataResourceSession:  sessionID,
			metadataResourceEnv:      "environment-1",
		},
	}
}

func kubernetesTaskSession(sessionID, taskID, executorID, profileID string) *models.TaskSession {
	return &models.TaskSession{
		ID: sessionID, TaskID: taskID, ExecutorID: executorID,
		ExecutorProfileID: profileID, TaskEnvironmentID: "environment-1",
	}
}

func kubernetesOwnedPod(
	podName, podUID, instanceID string,
	session *models.TaskSession,
) *corev1.Pod {
	return kubernetesOwnedPodForIdentity(podName, podUID, agentkubernetes.ResourceIdentity{
		ExecutorID: session.ExecutorID, ProfileID: session.ExecutorProfileID,
		InstanceID: instanceID, TaskID: session.TaskID, SessionID: session.ID,
		EnvironmentID: session.TaskEnvironmentID,
	})
}

func kubernetesOwnedPodForIdentity(
	podName, podUID string,
	identity agentkubernetes.ResourceIdentity,
) *corev1.Pod {
	labels, err := agentkubernetes.OwnershipLabels(identity)
	if err != nil {
		panic(err)
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: podName, Namespace: "kandev", UID: types.UID(podUID), Labels: labels,
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "kandev-agent", RestartCount: 2,
				State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			}},
		},
	}
}
