package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery/fake"
	kubeclient "k8s.io/client-go/kubernetes"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"

	agentkubernetes "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/auth/authn"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestRegisterRoutesWiresHTTPAndWebSocket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	dispatcher := ws.NewDispatcher()

	RegisterRoutes(router, dispatcher, nil, nil, nil)

	routes := make(map[string]bool)
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	require.True(t, routes["POST /api/v1/kubernetes/test"])
	require.True(t, routes["GET /api/v1/kubernetes/executors/:id/sessions"])
	require.True(t, routes["GET /api/v1/kubernetes/executors/:id/session-impact"])
	require.True(t, dispatcher.HasHandler("kubernetes.test"))
	require.True(t, dispatcher.HasHandler("kubernetes.sessions.list"))
	require.True(t, dispatcher.HasHandler("kubernetes.sessions.impact"))
}

func TestHTTPTestConnectionRejectsMember(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, authn.Identity{UserID: "member-1", Role: authn.RoleMember})
		c.Next()
	})
	RegisterRoutes(router, ws.NewDispatcher(), nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kubernetes/test", strings.NewReader(`{"config":{}}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestWebSocketTestConnectionRejectsMember(t *testing.T) {
	dispatcher := ws.NewDispatcher()
	RegisterRoutes(gin.New(), dispatcher, nil, nil, nil)
	msg, err := ws.NewRequest("req-1", "kubernetes.test", map[string]any{"config": map[string]string{}})
	require.NoError(t, err)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "member-1", Role: authn.RoleMember})

	response, err := dispatcher.Dispatch(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	var payload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(response.Payload, &payload))
	require.Equal(t, ws.ErrorCodeForbidden, payload.Code)
}

func TestHTTPTestConnectionReportsMandatoryChecks(t *testing.T) {
	clientset := kubernetesfake.NewSimpleClientset(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "kandev"},
	})
	discovery := clientset.Discovery().(*fake.FakeDiscovery)
	discovery.FakedServerVersion = &version.Info{GitVersion: "v1.30.1"}
	clientset.PrependReactor("create", "selfsubjectaccessreviews", allowAccessReview)
	handler := NewHandler(nil, nil, func(config agentkubernetes.ExecutorConfig) (*agentkubernetes.Client, error) {
		return &agentkubernetes.Client{Clientset: clientset}, nil
	})
	probeCalls := 0
	handler.probeStreaming = func(
		context.Context, *agentkubernetes.Client, string, string, string,
	) error {
		probeCalls++
		return nil
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, authn.Identity{UserID: "admin-1", Role: authn.RoleAdmin})
		c.Next()
	})
	handler.registerHTTP(router)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kubernetes/test", strings.NewReader(`{
		"config": {
			"auth_mode": "kubeconfig",
			"kubeconfig_path": "/etc/kandev/kubeconfig",
			"namespace": "kandev",
			"request_timeout_seconds": "30"
		}
	}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var result testConnectionResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
	require.True(t, result.Success)
	require.Equal(t, "v1.30.1", result.ServerVersion)
	require.Equal(t, "kandev", result.Namespace)
	require.Equal(t, []string{"discovery", "namespace", "rbac", "streaming"}, testStepKeys(result.Steps))
	require.True(t, result.Steps[0].Success)
	require.True(t, result.Steps[1].Success)
	require.True(t, result.Steps[2].Success)
	require.True(t, result.Steps[3].Success)
	require.Contains(t, result.Steps[3].Detail, "live transport probe was not run")
	require.Zero(t, probeCalls)
	permissions := accessReviewPermissions(clientset.Actions())
	for _, permission := range []string{
		"get pods/exec", "create pods/exec",
		"get pods/portforward", "create pods/portforward",
	} {
		require.Contains(t, permissions, permission)
	}
	for _, action := range clientset.Actions() {
		require.NotEqual(t, "namespaces", action.GetResource().Resource,
			"namespace validation must not require cluster-scoped Namespace RBAC")
		if action.GetResource().Resource == "pods" {
			require.NotEqual(t, "create", action.GetVerb(), "executor-only test must not create a probe Pod")
		}
	}
}

func TestHTTPTestConnectionDryRunsPodAdmission(t *testing.T) {
	clientset := kubernetesfake.NewSimpleClientset(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "kandev"},
	})
	clientset.Discovery().(*fake.FakeDiscovery).FakedServerVersion = &version.Info{GitVersion: "v1.30.1"}
	clientset.PrependReactor("create", "selfsubjectaccessreviews", allowAccessReview)
	probeState := installStreamingPodReactors(t, clientset)
	handler := NewHandler(nil, nil, func(config agentkubernetes.ExecutorConfig) (*agentkubernetes.Client, error) {
		return &agentkubernetes.Client{Clientset: clientset}, nil
	})
	handler.probeStreaming = successfulStreamingProbe(t, probeState)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, authn.Identity{UserID: "admin-1", Role: authn.RoleAdmin})
		c.Next()
	})
	handler.registerHTTP(router)
	body, err := json.Marshal(TestRequest{
		Config: map[string]string{
			"auth_mode": "kubeconfig", "kubeconfig_path": "/etc/kandev/kubeconfig",
			"namespace": "kandev", "request_timeout_seconds": "30",
		},
		ProfileConfig: validHandlerProfileConfig(),
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kubernetes/test", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var result testConnectionResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
	require.True(t, result.Success)
	require.Contains(t, testStepKeys(result.Steps), "admission.pod")
	require.Equal(t, []string{metav1.DryRunAll}, probeState.podDryRun.DryRun)
	requireStreamingPodCleaned(t, probeState)
}

func TestConnectionWaitsForExactProbeUIDToDisappear(t *testing.T) {
	clientset := kubernetesfake.NewSimpleClientset()
	clientset.Discovery().(*fake.FakeDiscovery).FakedServerVersion = &version.Info{GitVersion: "v1.30.1"}
	clientset.PrependReactor("create", "selfsubjectaccessreviews", allowAccessReview)
	probeState := installStreamingPodReactors(t, clientset)
	probeState.deleteVisibilityGets = 2
	handler := NewHandler(nil, nil, func(agentkubernetes.ExecutorConfig) (*agentkubernetes.Client, error) {
		return &agentkubernetes.Client{Clientset: clientset}, nil
	})
	handler.probeStreaming = successfulStreamingProbe(t, probeState)

	result := handler.testConnection(context.Background(), TestRequest{
		Config: validHandlerExecutorConfig(), ProfileConfig: validHandlerProfileConfig(),
	})

	require.True(t, result.Success)
	require.GreaterOrEqual(t, probeState.getsAfterDelete, 3)
	requireStreamingPodCleaned(t, probeState)
}

func TestConnectionStreamingFailureStillDeletesExactProbePod(t *testing.T) {
	clientset := kubernetesfake.NewSimpleClientset()
	clientset.Discovery().(*fake.FakeDiscovery).FakedServerVersion = &version.Info{GitVersion: "v1.30.1"}
	clientset.PrependReactor("create", "selfsubjectaccessreviews", allowAccessReview)
	probeState := installStreamingPodReactors(t, clientset)
	handler := NewHandler(nil, nil, func(config agentkubernetes.ExecutorConfig) (*agentkubernetes.Client, error) {
		return &agentkubernetes.Client{Clientset: clientset}, nil
	})
	probeCalls := 0
	handler.probeStreaming = func(
		_ context.Context,
		_ *agentkubernetes.Client,
		_, podName, mainContainer string,
	) error {
		probeCalls++
		require.Equal(t, probeState.actualPod.Name, podName)
		require.Equal(t, "kandev-agent", mainContainer)
		return errors.New("proxy refused upgrade: token=secret-token")
	}

	result := handler.testConnection(context.Background(), TestRequest{
		Config: validHandlerExecutorConfig(), ProfileConfig: validHandlerProfileConfig(),
	})

	require.Equal(t, 1, probeCalls)
	require.True(t, connectionStep(t, result.Steps, "discovery").Success)
	require.True(t, connectionStep(t, result.Steps, "rbac").Success)
	streaming := connectionStep(t, result.Steps, "streaming")
	require.False(t, streaming.Success)
	require.NotContains(t, streaming.Error, "secret-token")
	requireStreamingPodCleaned(t, probeState)
}

func TestConnectionReportsExactProbePodCleanupFailure(t *testing.T) {
	clientset := kubernetesfake.NewSimpleClientset()
	clientset.Discovery().(*fake.FakeDiscovery).FakedServerVersion = &version.Info{GitVersion: "v1.30.1"}
	clientset.PrependReactor("create", "selfsubjectaccessreviews", allowAccessReview)
	probeState := installStreamingPodReactors(t, clientset)
	probeState.deleteErr = errors.New("delete failed: token=secret-token")
	handler := NewHandler(nil, nil, func(agentkubernetes.ExecutorConfig) (*agentkubernetes.Client, error) {
		return &agentkubernetes.Client{Clientset: clientset}, nil
	})
	handler.probeStreaming = successfulStreamingProbe(t, probeState)

	result := handler.testConnection(context.Background(), TestRequest{
		Config: validHandlerExecutorConfig(), ProfileConfig: validHandlerProfileConfig(),
	})

	streaming := connectionStep(t, result.Steps, "streaming")
	require.False(t, streaming.Success)
	require.Contains(t, streaming.Detail, "cleanup")
	require.NotContains(t, streaming.Error, "secret-token")
	require.True(t, probeState.live)
	require.Equal(t, probeState.actualPod.Name, probeState.deletedName)
	require.Equal(t, probeState.actualPod.UID, *probeState.deleteOptions.Preconditions.UID)
}

func TestSanitizeErrorPreservesCauseAndRedactsSensitiveDetails(t *testing.T) {
	err := errors.New(
		"admission denied: quota exceeded; Bearer abcdefghijklmnopqrstuvwxyz; " +
			"endpoint https://user:password@cluster.example/apis/private?token=secret; " +
			"file /home/alice/.kube/config; " + strings.Repeat("x", routingerr.MaxRawExcerptBytes),
	)

	got := sanitizeError(err)

	require.Contains(t, got, "admission denied: quota exceeded")
	require.Contains(t, got, "https://cluster.example")
	require.NotContains(t, got, "abcdefghijklmnopqrstuvwxyz")
	require.NotContains(t, got, "user:password")
	require.NotContains(t, got, "/apis/private")
	require.NotContains(t, got, "/home/alice/")
	require.LessOrEqual(t, len(got), routingerr.MaxRawExcerptBytes)
}

func TestStreamingRESTConfigClearsTimeoutWithoutMutatingClient(t *testing.T) {
	original := &rest.Config{Host: "https://cluster.example", Timeout: 30 * time.Second}
	client := &agentkubernetes.Client{RESTConfig: original}

	streamingConfig, err := streamingRESTConfig(client)

	require.NoError(t, err)
	require.NotSame(t, original, streamingConfig)
	require.Zero(t, streamingConfig.Timeout)
	require.Equal(t, 30*time.Second, original.Timeout)
}

func TestStreamingTransportsUseWebSocketGETThenSPDYPOSTFallback(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		transport func(context.Context, *agentkubernetes.Client) error
	}{
		{
			name: "exec", path: "/exec",
			transport: func(ctx context.Context, client *agentkubernetes.Client) error {
				return probeExecUpgrade(ctx, client, "kandev", "probe-pod", "kandev-agent")
			},
		},
		{
			name: "port-forward", path: "/portforward",
			transport: func(ctx context.Context, client *agentkubernetes.Client) error {
				return probePortForwardUpgrade(ctx, client, "kandev", "probe-pod")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			methods := make([]string, 0, 2)
			paths := make([]string, 0, 2)
			commands := make([][]string, 0, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				methods = append(methods, request.Method)
				paths = append(paths, request.URL.Path)
				commands = append(commands, request.URL.Query()["command"])
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte("proxy rejected streaming upgrade"))
			}))
			t.Cleanup(server.Close)
			config := &rest.Config{Host: server.URL, Timeout: time.Second}
			clientset, err := kubeclient.NewForConfig(config)
			require.NoError(t, err)
			client := &agentkubernetes.Client{RESTConfig: config, Clientset: clientset}

			err = test.transport(context.Background(), client)

			require.Error(t, err)
			require.Equal(t, []string{http.MethodGet, http.MethodPost}, methods)
			require.Len(t, paths, 2)
			require.Contains(t, paths[0], test.path)
			require.Contains(t, paths[1], test.path)
			if test.name == "exec" {
				require.Equal(t, []string{"sh", "-c", ":"}, commands[0])
				require.Equal(t, commands[0], commands[1])
			}
			require.Equal(t, time.Second, config.Timeout)
		})
	}
}

func TestStreamingTransportDoesNotTreatPodNotFoundAsCompatible(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{
			"kind":"Status","apiVersion":"v1","status":"Failure",
			"message":"pods probe-pod not found","reason":"NotFound","code":404
		}`))
	}))
	t.Cleanup(server.Close)
	config := &rest.Config{Host: server.URL, Timeout: time.Second}
	clientset, err := kubeclient.NewForConfig(config)
	require.NoError(t, err)
	client := &agentkubernetes.Client{RESTConfig: config, Clientset: clientset}

	err = probeExecUpgrade(context.Background(), client, "kandev", "probe-pod", "kandev-agent")

	require.Error(t, err)
	require.True(t, apierrors.IsNotFound(err), err)
}

func TestWaitForStreamingProbePodRejectsReplacementWithSameName(t *testing.T) {
	current := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "probe-pod", Namespace: "kandev", UID: types.UID("original-uid"),
			Labels: map[string]string{"kandev.ai/managed-by": "kandev"},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}
	replacement := current.DeepCopy()
	replacement.UID = types.UID("replacement-uid")
	replacement.Status.Phase = corev1.PodRunning
	replacement.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name: "kandev-agent",
		State: corev1.ContainerState{
			Running: &corev1.ContainerStateRunning{StartedAt: metav1.Now()},
		},
	}}
	clientset := kubernetesfake.NewSimpleClientset(replacement)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := waitForStreamingProbePod(
		ctx, clientset, "kandev", "probe-pod", "kandev-agent", current,
	)

	require.ErrorContains(t, err, "identity changed")
}

func TestHTTPTestConnectionChecksManagedPVCPermissionsAndDryRunsAdmission(t *testing.T) {
	clientset := kubernetesfake.NewSimpleClientset()
	clientset.Discovery().(*fake.FakeDiscovery).FakedServerVersion = &version.Info{GitVersion: "v1.30.1"}
	permissions := make([]string, 0)
	clientset.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		review := action.(k8stesting.CreateAction).GetObject().(*authorizationv1.SelfSubjectAccessReview).DeepCopy()
		attributes := review.Spec.ResourceAttributes
		permissions = append(permissions, attributes.Verb+" "+attributes.Resource+"/"+attributes.Subresource)
		review.Status.Allowed = true
		return true, review, nil
	})
	var pvcCreateOptions metav1.CreateOptions
	clientset.PrependReactor("create", "persistentvolumeclaims", func(action k8stesting.Action) (bool, runtime.Object, error) {
		create := action.(k8stesting.CreateAction)
		pvcCreateOptions = action.(k8stesting.CreateActionImpl).GetCreateOptions()
		claim := create.GetObject().(*corev1.PersistentVolumeClaim).DeepCopy()
		claim.UID = "dry-run-pvc-uid"
		return true, claim, nil
	})
	probeState := installStreamingPodReactors(t, clientset)
	handler := NewHandler(nil, nil, func(agentkubernetes.ExecutorConfig) (*agentkubernetes.Client, error) {
		return &agentkubernetes.Client{Clientset: clientset}, nil
	})
	handler.probeStreaming = successfulStreamingProbe(t, probeState)
	profile := validHandlerProfileConfig()
	profile["workspace.mode"] = "managed_pvc"
	profile["workspace.size"] = "10Gi"

	result := handler.testConnection(context.Background(), TestRequest{
		Config: validHandlerExecutorConfig(), ProfileConfig: profile,
	})

	require.True(t, result.Success)
	require.True(t, connectionStep(t, result.Steps, "admission.pvc").Success)
	require.True(t, connectionStep(t, result.Steps, "admission.pod").Success)
	require.Equal(t, []string{metav1.DryRunAll}, pvcCreateOptions.DryRun)
	for _, permission := range []string{
		"get persistentvolumeclaims/", "create persistentvolumeclaims/",
		"delete persistentvolumeclaims/",
	} {
		require.Contains(t, permissions, permission)
	}
	require.NotContains(t, permissions, "watch persistentvolumeclaims/")
	require.NotNil(t, probeState.actualPod.Spec.Volumes[0].EmptyDir)
	require.Nil(t, probeState.actualPod.Spec.Volumes[0].PersistentVolumeClaim)
	require.Equal(t, []string{"sh"}, probeState.actualPod.Spec.Containers[0].Command)
	requireStreamingPodCleaned(t, probeState)
}

func TestConnectionGetsConfiguredExistingClaimWithoutMutation(t *testing.T) {
	claim := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
		Name: "shared-workspace", Namespace: "kandev", UID: types.UID("shared-workspace-uid"),
	}}
	clientset := kubernetesfake.NewSimpleClientset(claim)
	clientset.Discovery().(*fake.FakeDiscovery).FakedServerVersion = &version.Info{GitVersion: "v1.30.1"}
	clientset.PrependReactor("create", "selfsubjectaccessreviews", allowAccessReview)
	probeState := installStreamingPodReactors(t, clientset)
	handler := NewHandler(nil, nil, func(agentkubernetes.ExecutorConfig) (*agentkubernetes.Client, error) {
		return &agentkubernetes.Client{Clientset: clientset}, nil
	})
	handler.probeStreaming = successfulStreamingProbe(t, probeState)
	profile := validHandlerProfileConfig()
	profile["workspace.mode"] = "existing_claim"
	profile["workspace.claim_name"] = claim.Name

	result := handler.testConnection(context.Background(), TestRequest{
		Config: validHandlerExecutorConfig(), ProfileConfig: profile,
	})

	require.True(t, result.Success)
	require.True(t, connectionStep(t, result.Steps, "storage.existing_claim").Success)
	requireOnlyExactClaimGet(t, clientset.Actions(), claim.Name)
	requireStreamingPodCleaned(t, probeState)
}

func TestConnectionRejectsMismatchedExistingClaimResponseWithoutMutation(t *testing.T) {
	tests := []struct {
		name  string
		claim *corev1.PersistentVolumeClaim
	}{
		{
			name: "name",
			claim: &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
				Name: "other-workspace", Namespace: "kandev", UID: types.UID("claim-uid"),
			}},
		},
		{
			name: "namespace",
			claim: &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
				Name: "shared-workspace", Namespace: "other", UID: types.UID("claim-uid"),
			}},
		},
		{
			name: "uid",
			claim: &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
				Name: "shared-workspace", Namespace: "kandev",
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clientset := kubernetesfake.NewSimpleClientset()
			clientset.Discovery().(*fake.FakeDiscovery).FakedServerVersion = &version.Info{GitVersion: "v1.30.1"}
			clientset.PrependReactor("create", "selfsubjectaccessreviews", allowAccessReview)
			clientset.PrependReactor("get", "persistentvolumeclaims", func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, test.claim.DeepCopy(), nil
			})
			probeState := installStreamingPodReactors(t, clientset)
			handler := NewHandler(nil, nil, func(agentkubernetes.ExecutorConfig) (*agentkubernetes.Client, error) {
				return &agentkubernetes.Client{Clientset: clientset}, nil
			})
			handler.probeStreaming = successfulStreamingProbe(t, probeState)
			profile := validHandlerProfileConfig()
			profile["workspace.mode"] = "existing_claim"
			profile["workspace.claim_name"] = "shared-workspace"

			result := handler.testConnection(context.Background(), TestRequest{
				Config: validHandlerExecutorConfig(), ProfileConfig: profile,
			})

			require.False(t, result.Success)
			storage := connectionStep(t, result.Steps, "storage.existing_claim")
			require.False(t, storage.Success)
			require.Contains(t, storage.Error, "identity")
			requireOnlyExactClaimGet(t, clientset.Actions(), "shared-workspace")
			require.Nil(t, probeState.actualPod, "invalid claim identity must skip live probe creation")
		})
	}
}

func TestConnectionReportsMissingExistingClaimWithoutMutation(t *testing.T) {
	clientset := kubernetesfake.NewSimpleClientset()
	clientset.Discovery().(*fake.FakeDiscovery).FakedServerVersion = &version.Info{GitVersion: "v1.30.1"}
	clientset.PrependReactor("create", "selfsubjectaccessreviews", allowAccessReview)
	probeState := installStreamingPodReactors(t, clientset)
	handler := NewHandler(nil, nil, func(agentkubernetes.ExecutorConfig) (*agentkubernetes.Client, error) {
		return &agentkubernetes.Client{Clientset: clientset}, nil
	})
	handler.probeStreaming = successfulStreamingProbe(t, probeState)
	profile := validHandlerProfileConfig()
	profile["workspace.mode"] = "existing_claim"
	profile["workspace.claim_name"] = "missing-workspace"

	result := handler.testConnection(context.Background(), TestRequest{
		Config: validHandlerExecutorConfig(), ProfileConfig: profile,
	})

	require.False(t, result.Success)
	storage := connectionStep(t, result.Steps, "storage.existing_claim")
	require.False(t, storage.Success)
	require.Contains(t, strings.ToLower(storage.Error), "not found")
	requireOnlyExactClaimGet(t, clientset.Actions(), "missing-workspace")
	require.Nil(t, probeState.actualPod, "failed storage validation must skip live probe creation")
}

type testConnectionResponse struct {
	Success       bool               `json:"success"`
	ServerVersion string             `json:"server_version"`
	Namespace     string             `json:"namespace"`
	Steps         []testStepResponse `json:"steps"`
}

type testStepResponse struct {
	Key     string `json:"key"`
	Success bool   `json:"success"`
	Detail  string `json:"detail"`
}

func allowAccessReview(action k8stesting.Action) (bool, runtime.Object, error) {
	review := action.(k8stesting.CreateAction).GetObject().(*authorizationv1.SelfSubjectAccessReview).DeepCopy()
	review.Status.Allowed = true
	return true, review, nil
}

func accessReviewPermissions(actions []k8stesting.Action) []string {
	permissions := make([]string, 0)
	for _, action := range actions {
		if action.GetResource().Resource != "selfsubjectaccessreviews" {
			continue
		}
		review := action.(k8stesting.CreateAction).GetObject().(*authorizationv1.SelfSubjectAccessReview)
		attributes := review.Spec.ResourceAttributes
		resource := attributes.Resource
		if attributes.Subresource != "" {
			resource += "/" + attributes.Subresource
		}
		permissions = append(permissions, attributes.Verb+" "+resource)
	}
	return permissions
}

func requireOnlyExactClaimGet(t *testing.T, actions []k8stesting.Action, claimName string) {
	t.Helper()
	gets := 0
	for _, action := range actions {
		if action.GetResource().Resource != "persistentvolumeclaims" {
			continue
		}
		require.Equal(t, "get", action.GetVerb(), "existing claims must never be listed or mutated")
		get, ok := action.(k8stesting.GetAction)
		require.True(t, ok)
		require.Equal(t, claimName, get.GetName())
		gets++
	}
	require.Equal(t, 1, gets)
}

func testStepKeys(steps []testStepResponse) []string {
	keys := make([]string, 0, len(steps))
	for _, step := range steps {
		keys = append(keys, step.Key)
	}
	return keys
}

func validHandlerProfileConfig() map[string]string {
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

func validHandlerExecutorConfig() map[string]string {
	return map[string]string{
		"auth_mode": "kubeconfig", "kubeconfig_path": "/etc/kandev/kubeconfig",
		"namespace": "kandev", "request_timeout_seconds": "30",
	}
}

type streamingPodState struct {
	podDryRun            metav1.CreateOptions
	requestedPod         *corev1.Pod
	actualPod            *corev1.Pod
	deletedNamespace     string
	deletedName          string
	deleteOptions        metav1.DeleteOptions
	deleteErr            error
	deleteIssued         bool
	deleteVisibilityGets int
	getsAfterDelete      int
	live                 bool
	mutateLivePod        func(*corev1.Pod)
	createErr            error
	commitOnCreateError  bool
	getErr               error
}

func installStreamingPodReactors(
	t *testing.T,
	clientset *kubernetesfake.Clientset,
) *streamingPodState {
	t.Helper()
	state := &streamingPodState{}
	clientset.PrependReactor("create", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		create := action.(k8stesting.CreateAction)
		options := action.(k8stesting.CreateActionImpl).GetCreateOptions()
		pod := create.GetObject().(*corev1.Pod).DeepCopy()
		if len(options.DryRun) > 0 {
			state.podDryRun = options
			pod.UID = types.UID("dry-run-pod-uid")
			return true, pod, nil
		}
		state.requestedPod = pod.DeepCopy()
		pod.UID = types.UID("probe-uid")
		pod.Status.Phase = corev1.PodRunning
		pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
			Name: "kandev-agent", State: corev1.ContainerState{
				Running: &corev1.ContainerStateRunning{StartedAt: metav1.Now()},
			},
		}}
		if state.mutateLivePod != nil {
			state.mutateLivePod(pod)
		}
		if state.createErr != nil {
			if state.commitOnCreateError {
				state.actualPod = pod
				state.live = true
			}
			return true, nil, state.createErr
		}
		state.actualPod = pod
		state.live = true
		return true, pod.DeepCopy(), nil
	})
	clientset.PrependReactor("get", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		get := action.(k8stesting.GetAction)
		name := get.GetName()
		namespace := get.GetNamespace()
		if state.getErr != nil && !state.deleteIssued {
			return true, nil, state.getErr
		}
		if state.deleteIssued && name == state.deletedName && namespace == state.deletedNamespace {
			state.getsAfterDelete++
			if state.actualPod != nil && state.actualPod.Name == name && state.actualPod.Namespace == namespace &&
				state.deleteVisibilityGets > 0 {
				state.deleteVisibilityGets--
				return true, state.actualPod.DeepCopy(), nil
			}
			if state.actualPod != nil && state.actualPod.Name == name && state.actualPod.Namespace == namespace {
				state.live = false
			}
			return true, nil, apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, name)
		}
		if state.live && state.actualPod != nil && state.actualPod.Name == name && state.actualPod.Namespace == namespace {
			return true, state.actualPod.DeepCopy(), nil
		}
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, name)
	})
	clientset.PrependReactor("delete", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		deletion := action.(k8stesting.DeleteAction)
		state.deletedNamespace = deletion.GetNamespace()
		state.deletedName = deletion.GetName()
		state.deleteOptions = deletion.GetDeleteOptions()
		if state.deleteErr != nil {
			return true, nil, state.deleteErr
		}
		state.deleteIssued = true
		if state.deleteVisibilityGets == 0 && state.actualPod != nil &&
			state.actualPod.Name == state.deletedName && state.actualPod.Namespace == state.deletedNamespace {
			state.live = false
		}
		return true, nil, nil
	})
	return state
}

func successfulStreamingProbe(t *testing.T, state *streamingPodState) StreamingProbe {
	t.Helper()
	return func(
		_ context.Context,
		_ *agentkubernetes.Client,
		_, podName, mainContainer string,
	) error {
		require.NotNil(t, state.actualPod)
		require.True(t, state.live)
		require.Equal(t, state.actualPod.Name, podName)
		require.Equal(t, "kandev-agent", mainContainer)
		return nil
	}
}

func requireStreamingPodCleaned(t *testing.T, state *streamingPodState) {
	t.Helper()
	require.NotNil(t, state.actualPod)
	require.False(t, state.live)
	require.Equal(t, state.actualPod.Namespace, state.deletedNamespace)
	require.Equal(t, state.actualPod.Name, state.deletedName)
	require.NotNil(t, state.deleteOptions.Preconditions)
	require.NotNil(t, state.deleteOptions.Preconditions.UID)
	require.Equal(t, state.actualPod.UID, *state.deleteOptions.Preconditions.UID)
}

func connectionStep(t *testing.T, steps []TestStep, key string) TestStep {
	t.Helper()
	for _, step := range steps {
		if step.Key == key {
			return step
		}
	}
	t.Fatalf("step %q missing from %+v", key, steps)
	return TestStep{}
}
