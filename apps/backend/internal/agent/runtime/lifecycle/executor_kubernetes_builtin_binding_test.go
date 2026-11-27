package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestKubernetesReconnectKeepsBindingOfRetainedHelper(t *testing.T) {
	for _, mode := range []string{"session", "task"} {
		t.Run(mode, func(t *testing.T) {
			for _, tc := range []struct {
				name              string
				changed, replaced bool
			}{
				{name: "unchanged retained"},
				{name: "changed retained", changed: true},
				{name: "changed recreated", changed: true, replaced: true},
			} {
				t.Run(tc.name, func(t *testing.T) { testKubernetesReconnectBinding(t, mode == "task", tc.changed, tc.replaced) })
			}
		})
	}
}

func testKubernetesReconnectBinding(t *testing.T, taskOwned, changed, replaced bool) {
	t.Helper()
	bound := builtinDispatchProfile(p.PackageCore)
	bound.BuiltinPackages.TaskID = "task-1"
	bound.Providers = []string{"github"}
	desired := builtinDispatchProfile(p.PackageCore)
	desired.BuiltinPackages.TaskID = "task-1"
	if !changed {
		desired.Providers = []string{"github"}
	}
	retained, err := p.MarshalBuiltinBinding(bound)
	require.NoError(t, err)
	current, err := p.MarshalBuiltinBinding(desired)
	require.NoError(t, err)
	expected := bound
	if replaced {
		expected = desired
	}
	helper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/mcp/builtin-binding":
			require.Equal(t, http.MethodGet, r.Method)
			_ = json.NewEncoder(w).Encode(expected)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(helper.Close)
	backend, resources, created, task := newKubernetesBindingFixture(t, taskOwned, testHTTPPort(t, helper))
	created.Metadata[p.BuiltinBindingMetadataKey] = retained
	if replaced {
		if taskOwned {
			task.control.reboot()
		}
		resources.mu.Lock()
		resources.pod.Status.ContainerStatuses[0].RestartCount++
		resources.mu.Unlock()
	}
	req := kubernetesReconnectRequest(created)
	req.McpProfile = &desired
	req.Metadata[p.BuiltinBindingMetadataKey] = current
	req.RetainedBuiltinBinding = retained
	resumed, err := backend.CreateInstance(context.Background(), req)
	require.NoError(t, err)
	expectedBinding, err := p.MarshalBuiltinBinding(expected)
	require.NoError(t, err)
	require.Equal(t, expectedBinding, resumed.Metadata[p.BuiltinBindingMetadataKey])
	profile, err := p.ParseBuiltinBindingMetadata(resumed.Metadata[p.BuiltinBindingMetadataKey])
	require.NoError(t, err)
	require.NoError(t, resumed.Client.VerifyBuiltinMCP(context.Background(), *profile))
	require.Equal(t, current, req.Metadata[p.BuiltinBindingMetadataKey], "reconnect must not overwrite the caller's desired metadata")
	require.Len(t, resources.createdPods, 1)
	if !taskOwned {
		return
	}
	creates, active, _ := task.control.snapshot()
	require.Len(t, active, 1)
	if !replaced {
		require.Len(t, creates, 1)
		if changed {
			testKubernetesRetainedBindingSurvivesActiveRestart(t, task, resumed, req, retained)
		}
		return
	}
	actual, err := p.MarshalBuiltinBinding(*creates[len(creates)-1].McpProfile)
	require.NoError(t, err)
	require.Equal(t, current, actual)
	if changed {
		testKubernetesNewBindingSurvivesTransportRefresh(t, task, resumed, req, current)
	}

}

func newKubernetesBindingFixture(t *testing.T, taskOwned bool, helperPort uint16) (*KubernetesExecutor, *fakeKubernetesResources, *ExecutorInstance, *taskPodFixture) {
	t.Helper()
	var backend *KubernetesExecutor
	var resources *fakeKubernetesResources
	var created *ExecutorInstance
	var task *taskPodFixture
	if taskOwned {
		task = newTaskPodFixture(t)
		backend, resources = task.runtime, task.resources
		backend.clientFactory = func(kubeexecutor.ExecutorConfig) (*kubernetesRuntimeClient, error) {
			return &kubernetesRuntimeClient{resources: resources, streams: kubeexecutor.NewStreamOperations(task.execs, &recordingKubernetesForwarder{localPorts: map[uint16]uint16{uint16(kubeexecutor.DefaultAgentctlPort): builtinBindingControlPort(t, task.port), 41001: helperPort}})}, nil
		}
		created = task.launch(t, 1)
	} else {
		resources = &fakeKubernetesResources{}
		backend = newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{uint16(kubeexecutor.DefaultAgentctlPort): builtinBindingControlPort(t, startKubernetesAgentctlServer(t, true, 41001)), 41001: helperPort})
		t.Cleanup(func() { require.NoError(t, backend.Close()) })
		var err error
		created, err = backend.CreateInstance(context.Background(), validKubernetesCreateRequest())
		require.NoError(t, err)
	}
	return backend, resources, created, task
}

func builtinBindingControlPort(t *testing.T, port uint16) uint16 {
	t.Helper()
	target, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	require.NoError(t, err)
	proxy := httputil.NewSingleHostReverseProxy(target)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/identity" {
			_ = json.NewEncoder(w).Encode(map[string]any{"capabilities": []string{p.BuiltinPackagesCapability}, "builtin_mcp_catalog_version": 1})
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return testHTTPPort(t, server)
}

func TestKubernetesTaskReconnectRejectsForeignHelper(t *testing.T) {
	for _, field := range []string{"task", "session"} {
		t.Run(field, func(t *testing.T) {
			f := newTaskPodFixture(t)
			created := f.launch(t, 1)
			f.control.mu.Lock()
			foreign := f.control.active["instance-1"]
			if field == "task" {
				foreign.TaskID = "foreign-task"
			} else {
				foreign.SessionID = "foreign-session"
			}
			f.control.active["instance-1"] = foreign
			f.control.mu.Unlock()
			_, err := f.runtime.CreateInstance(context.Background(), kubernetesReconnectRequest(created))
			require.ErrorContains(t, err, "instance owner does not match request")
			creates, _, _ := f.control.snapshot()
			require.Len(t, creates, 1)
		})
	}
}

func TestKubernetesEnvironmentDoesNotSupplySessionBinding(t *testing.T) {
	for _, desired := range []string{"current-session-binding", ""} {
		name := "no binding"
		if desired != "" {
			name = "current binding"
		}
		t.Run(name, func(t *testing.T) {
			f := newTaskPodFixture(t)
			f.launch(t, 1)
			ctx := context.Background()
			record, err := f.repo.GetKubernetesEnvironment(ctx, "environment-1")
			require.NoError(t, err)
			record.Metadata[p.BuiltinBindingMetadataKey] = "prior-session-binding"
			req := taskPodRequest(2)
			if desired != "" {
				req.Metadata[p.BuiltinBindingMetadataKey] = desired
			}
			require.NoError(t, f.runtime.attachKubernetesEnvironmentRequest(ctx, req, record))
			if desired == "" {
				require.NotContains(t, req.Metadata, p.BuiltinBindingMetadataKey)
			} else {
				require.Equal(t, desired, req.Metadata[p.BuiltinBindingMetadataKey])
			}
		})
	}
}

func testKubernetesRetainedBindingSurvivesActiveRestart(t *testing.T, f *taskPodFixture, instance *ExecutorInstance, req *ExecutorCreateRequest, retained string) {
	t.Helper()
	f.control.reboot()
	f.resources.mu.Lock()
	f.resources.pod.Status.ContainerStatuses[0].RestartCount++
	f.resources.mu.Unlock()
	refreshed, err := f.runtime.RefreshRemoteInstance(context.Background(), kubernetesRefreshInstance(instance, req.Metadata))
	require.NoError(t, err)
	require.NotNil(t, refreshed)
	t.Cleanup(refreshed.Abort)
	creates, _, _ := f.control.snapshot()
	actual, err := p.MarshalBuiltinBinding(*creates[len(creates)-1].McpProfile)
	require.NoError(t, err)
	require.Equal(t, retained, actual, "active restart must recreate the admitted authority")
	require.Equal(t, retained, refreshed.Instance.Metadata[p.BuiltinBindingMetadataKey])
	expected, err := p.ParseBuiltinBindingMetadata(actual)
	require.NoError(t, err)
	require.NoError(t, refreshed.Instance.Client.VerifyBuiltinMCP(context.Background(), *expected))
	require.NoError(t, refreshed.Commit(nil))
}

func TestKubernetesReconnectRejectsInvalidRetainedBinding(t *testing.T) {
	for _, mode := range []string{"session", "task"} {
		t.Run(mode, func(t *testing.T) {
			helper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
			t.Cleanup(helper.Close)
			backend, _, created, task := newKubernetesBindingFixture(t, mode == "task", testHTTPPort(t, helper))
			req := kubernetesReconnectRequest(created)
			req.RetainedBuiltinBinding = "{}"
			_, err := backend.CreateInstance(context.Background(), req)
			require.ErrorContains(t, err, "invalid retained built-in MCP binding")
			if task != nil {
				creates, _, _ := task.control.snapshot()
				require.Len(t, creates, 1)
			}
		})
	}
}

func testKubernetesNewBindingSurvivesTransportRefresh(t *testing.T, f *taskPodFixture, instance *ExecutorInstance, req *ExecutorCreateRequest, binding string) {
	t.Helper()
	f.runtime.mu.Lock()
	session := f.runtime.sessions[instance.InstanceID]
	session.client.Close()
	session.client = newKubernetesAgentctlClient(f.runtime.logger, req, "instance-1", instance.AuthToken, 1, false)
	f.runtime.mu.Unlock()
	refreshed, err := f.runtime.RefreshRemoteInstance(context.Background(), kubernetesRefreshInstance(instance, req.Metadata))
	require.NoError(t, err)
	require.NotNil(t, refreshed)
	t.Cleanup(refreshed.Abort)
	require.Equal(t, binding, refreshed.Instance.Metadata[p.BuiltinBindingMetadataKey])
	require.NoError(t, refreshed.Commit(nil))
	testKubernetesRetainedBindingSurvivesActiveRestart(t, f, refreshed.Instance, req, binding)
}
