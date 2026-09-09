package kubernetes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"

	agentkubernetes "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestKubernetesSessionRetentionProjectionTable(t *testing.T) {
	tests := []struct {
		name           string
		sessionState   models.TaskSessionState
		podPhase       corev1.PodPhase
		deleting       bool
		retentionState string
	}{
		{name: "created pending", sessionState: models.TaskSessionStateCreated, podPhase: corev1.PodPending, retentionState: "active"},
		{name: "waiting running", sessionState: models.TaskSessionStateWaitingForInput, podPhase: corev1.PodRunning, retentionState: "active"},
		{name: "cancelled running", sessionState: models.TaskSessionStateCancelled, podPhase: corev1.PodRunning, retentionState: "retained"},
		{name: "idle running", sessionState: models.TaskSessionStateIdle, podPhase: corev1.PodRunning, retentionState: "retained"},
		{name: "deleting active", sessionState: models.TaskSessionStateRunning, podPhase: corev1.PodRunning, deleting: true, retentionState: "terminating"},
		{name: "succeeded retained state", sessionState: models.TaskSessionStateCancelled, podPhase: corev1.PodSucceeded, retentionState: "terminal"},
		{name: "failed retained state", sessionState: models.TaskSessionStateFailed, podPhase: corev1.PodFailed, retentionState: "terminal"},
		{name: "unknown session", sessionState: models.TaskSessionState("future-state"), podPhase: corev1.PodRunning, retentionState: "unknown"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run := kubernetesRunningRow("instance-1", "session-1", "task-1", "pod-1", "pod-uid-1", time.Now())
			session := kubernetesTaskSession("session-1", "task-1", "executor-1", "profile-1")
			session.State = test.sessionState
			pod := kubernetesOwnedPod("pod-1", "pod-uid-1", "instance-1", session)
			pod.Status.Phase = test.podPhase
			if test.deleting {
				deletionTime := metav1.Now()
				pod.DeletionTimestamp = &deletionTime
			}
			pod.Spec.Containers = []corev1.Container{{
				Name: "kandev-agent",
				Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("250m"),
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				}},
			}}
			repo := &fakeResourceRepository{
				executor: &models.Executor{ID: "executor-1", Type: models.ExecutorTypeKubernetes, Config: validHandlerExecutorConfig()},
				runs:     []*models.ExecutorRunning{run}, sessions: map[string]*models.TaskSession{"session-1": session},
			}
			clientset := kubernetesfake.NewSimpleClientset(pod)
			handler := retentionProjectionHandler(repo, clientset)

			row, visible, err := handler.sessionRow(context.Background(), clientset, "executor-1", run)

			require.NoError(t, err)
			require.True(t, visible)
			wantSessionState := string(test.sessionState)
			if !isKnownTaskSessionState(test.sessionState) {
				wantSessionState = "unknown"
			}
			require.Equal(t, wantSessionState, row.SessionState)
			require.Equal(t, test.retentionState, row.RetentionState)
			require.Equal(t, &MainContainerRequests{CPU: "250m", Memory: "512Mi"}, row.MainContainerRequests)
		})
	}
}

func TestKubernetesRetentionProjectionOmitsUnverifiedRequests(t *testing.T) {
	tests := []struct {
		name           string
		pod            *corev1.Pod
		mainContainer  string
		retentionState string
		failureReason  string
	}{
		{name: "missing pod", retentionState: "missing", failureReason: "Pod not found"},
		{name: "identity mismatch", pod: kubernetesOwnedPod("pod-1", "foreign-uid", "instance-1", kubernetesTaskSession("session-1", "task-1", "executor-1", "profile-1")), retentionState: "unknown", failureReason: "Pod identity does not match runtime inventory"},
		{name: "main container missing", pod: kubernetesOwnedPod("pod-1", "pod-uid-1", "instance-1", kubernetesTaskSession("session-1", "task-1", "executor-1", "profile-1")), mainContainer: "missing-container", retentionState: "unknown", failureReason: "Main container status is unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run := kubernetesRunningRow("instance-1", "session-1", "task-1", "pod-1", "pod-uid-1", time.Now())
			session := kubernetesTaskSession("session-1", "task-1", "executor-1", "profile-1")
			session.State = models.TaskSessionStateCancelled
			if test.mainContainer != "" {
				run.Metadata[metadataMainContainer] = test.mainContainer
			}
			repo := &fakeResourceRepository{
				executor: &models.Executor{ID: "executor-1", Type: models.ExecutorTypeKubernetes, Config: validHandlerExecutorConfig()},
				runs:     []*models.ExecutorRunning{run}, sessions: map[string]*models.TaskSession{"session-1": session},
			}
			var clientset *kubernetesfake.Clientset
			if test.pod == nil {
				clientset = kubernetesfake.NewSimpleClientset()
			} else {
				test.pod.Spec.Containers = []corev1.Container{{
					Name: "kandev-agent",
					Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					}},
				}}
				clientset = kubernetesfake.NewSimpleClientset(test.pod)
			}
			handler := retentionProjectionHandler(repo, clientset)

			row, visible, err := handler.sessionRow(context.Background(), clientset, "executor-1", run)

			require.NoError(t, err)
			require.True(t, visible)
			require.Equal(t, string(models.TaskSessionStateCancelled), row.SessionState)
			require.Equal(t, test.retentionState, row.RetentionState)
			require.Nil(t, row.MainContainerRequests)
			require.Equal(t, test.failureReason, row.FailureReason)
		})
	}
}

func TestKubernetesMainContainerRequestsPreserveExplicitZero(t *testing.T) {
	run := kubernetesRunningRow("instance-1", "session-1", "task-1", "pod-1", "pod-uid-1", time.Now())
	session := kubernetesTaskSession("session-1", "task-1", "executor-1", "profile-1")
	session.State = models.TaskSessionStateRunning
	pod := kubernetesOwnedPod("pod-1", "pod-uid-1", "instance-1", session)
	pod.Spec.Containers = []corev1.Container{{
		Name: "sidecar", Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("9"), corev1.ResourceMemory: resource.MustParse("9Gi"),
		}},
	}, {
		Name: "kandev-agent", Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("0"), corev1.ResourceMemory: resource.MustParse("512Mi"),
		}},
	}}
	repo := &fakeResourceRepository{
		executor: &models.Executor{ID: "executor-1", Type: models.ExecutorTypeKubernetes, Config: validHandlerExecutorConfig()},
		runs:     []*models.ExecutorRunning{run}, sessions: map[string]*models.TaskSession{"session-1": session},
	}
	clientset := kubernetesfake.NewSimpleClientset(pod)
	handler := retentionProjectionHandler(repo, clientset)

	row, visible, err := handler.sessionRow(context.Background(), clientset, "executor-1", run)

	require.NoError(t, err)
	require.True(t, visible)
	require.Equal(t, &MainContainerRequests{CPU: "0", Memory: "512Mi"}, row.MainContainerRequests)
}

func TestKubernetesRetentionProjectionSerializesOverHTTPAndWebSocket(t *testing.T) {
	run := kubernetesRunningRow("instance-1", "session-1", "task-1", "pod-1", "pod-uid-1", time.Now())
	session := kubernetesTaskSession("session-1", "task-1", "executor-1", "profile-1")
	session.State = models.TaskSessionStateCancelled
	pod := kubernetesOwnedPod("pod-1", "pod-uid-1", "instance-1", session)
	pod.Spec.Containers = []corev1.Container{{Name: "kandev-agent", Resources: corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("250m")},
	}}}
	repo := &fakeResourceRepository{
		executor: &models.Executor{ID: "executor-1", Type: models.ExecutorTypeKubernetes, Config: validHandlerExecutorConfig()},
		runs:     []*models.ExecutorRunning{run}, sessions: map[string]*models.TaskSession{"session-1": session},
	}
	clientset := kubernetesfake.NewSimpleClientset(pod)
	handler := retentionProjectionHandler(repo, clientset)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, authn.Identity{UserID: "member-1", Role: authn.RoleMember})
		c.Next()
	})
	handler.registerHTTP(router)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/kubernetes/executors/executor-1/sessions", nil))

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var rows []SessionRow
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &rows))
	require.Len(t, rows, 1)
	require.Equal(t, "CANCELLED", rows[0].SessionState)
	require.Equal(t, "retained", rows[0].RetentionState)
	require.Equal(t, &MainContainerRequests{CPU: "250m"}, rows[0].MainContainerRequests)

	dispatcher := ws.NewDispatcher()
	handler.registerWS(dispatcher)
	message, err := ws.NewRequest("req-1", "kubernetes.sessions.list", map[string]string{"executor_id": "executor-1"})
	require.NoError(t, err)
	response, err := dispatcher.Dispatch(authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "member-1", Role: authn.RoleMember,
	}), message)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Contains(t, string(response.Payload), `"retention_state":"retained"`)
}

func retentionProjectionHandler(repo *fakeResourceRepository, clientset *kubernetesfake.Clientset) *Handler {
	return NewHandler(repo, &fakeAccessChecker{}, func(agentkubernetes.ExecutorConfig) (*agentkubernetes.Client, error) {
		return &agentkubernetes.Client{Clientset: clientset}, nil
	})
}
