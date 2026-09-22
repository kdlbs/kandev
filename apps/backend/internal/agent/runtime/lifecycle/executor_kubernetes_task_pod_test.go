package lifecycle

import (
	"context"
	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"path"
	"strings"
	"testing"
)

func TestKubernetesTaskPodAdditionalSession(t *testing.T) {
	f := newTaskPodFixture(t)
	a := f.launch(t, 1)
	b := f.launch(t, 2)
	require.Len(t, f.resources.createdPods, 1)
	require.Len(t, f.resources.createdPVCs, 1)
	require.Equal(t, a.Metadata[MetadataKeyKubernetesPodUID], b.Metadata[MetadataKeyKubernetesPodUID])
	instances, active, handshakes := f.control.snapshot()
	require.Len(t, instances, 2)
	require.Len(t, active, 2)
	require.Equal(t, 1, handshakes)
	require.NotEqual(t, instances[0].ID, instances[1].ID)
	require.Equal(t, "session-2", instances[1].SessionID)
	require.NotEmpty(t, instances[0].Env["HOME"])
	require.NotEqual(t, instances[0].Env["HOME"], instances[1].Env["HOME"])
	require.Equal(t, "credential-1", instances[0].Env["SESSION_CREDENTIAL"])
	require.Equal(t, "credential-2", instances[1].Env["SESSION_CREDENTIAL"])
	require.Equal(t, "session-2", instances[1].Env[kubernetesEnvSessionID])
}

func TestKubernetesTaskPodStopPreservesSibling(t *testing.T) {
	f := newTaskPodFixture(t)
	a := f.launch(t, 1)
	b := f.launch(t, 2)
	ctx := context.Background()
	b.StopReason = StopReasonTaskDeleted
	require.NoError(t, f.runtime.StopInstance(ctx, b, true))
	require.Empty(t, f.resources.deletedPods)
	require.Empty(t, f.resources.deletedPVCs)
	require.NoError(t, a.Client.Health(ctx))
	require.NoError(t, f.manager.deleteKubernetesRuntimeSecrets(ctx, b.Metadata))
	_, err := f.secretStore.Reveal(ctx, getMetadataString(a.Metadata, MetadataKeyAuthTokenSecret))
	require.NoError(t, err)
	_, active, _ := f.control.snapshot()
	require.Contains(t, active, "instance-1")
	require.NotContains(t, active, "instance-2")
}

func TestKubernetesTaskPodTerminalCleanup(t *testing.T) {
	f := newTaskPodFixture(t)
	a := f.launch(t, 1)
	b := f.launch(t, 2)
	ctx := context.Background()
	env, err := f.repo.GetTaskEnvironment(ctx, "environment-1")
	require.NoError(t, err)
	require.Error(t, f.manager.DestroyKubernetesEnvironment(ctx, env), "a live sibling blocks physical teardown")
	require.Empty(t, f.resources.deletedPods)
	require.NoError(t, f.runtime.StopInstance(ctx, a, true))
	require.NoError(t, f.runtime.StopInstance(ctx, b, true))
	require.NoError(t, f.manager.DestroyKubernetesEnvironment(ctx, env))
	require.Len(t, f.resources.deletedPods, 1)
	require.Len(t, f.resources.deletedPVCs, 1)
	_, err = f.repo.GetKubernetesEnvironment(ctx, env.ID)
	require.ErrorIs(t, err, models.ErrKubernetesEnvironmentNotFound)
	require.NoError(t, f.manager.DestroyKubernetesEnvironment(ctx, env))
	require.NoError(t, f.runtime.StopInstance(ctx, a, true), "session stop remains idempotent after task cleanup")
	require.Len(t, f.resources.deletedPods, 1)
}

func TestKubernetesTaskPodControlEnvironmentIsTaskScoped(t *testing.T) {
	req := validKubernetesCreateRequest()
	req.Metadata[metadataKubernetesTaskOwned] = true
	env := kubernetesRuntimeEnvironment(req)
	require.Empty(t, env[kubernetesEnvSessionID])
	require.Empty(t, env[kubernetesEnvExecutionProfile])
	require.Equal(t, req.TaskEnvironmentID, env[kubernetesEnvInstanceID])
}

func TestKubernetesTaskPodBootstrapDropsAgentCredentials(t *testing.T) {
	req := validKubernetesCreateRequest()
	req.Metadata[metadataKubernetesTaskOwned] = true
	req.Env = map[string]string{"SESSION_CREDENTIAL": "must-not-be-inherited"}
	execs := &recordingKubernetesExec{}
	runtime := newFakeKubernetesExecutor(t, &fakeKubernetesResources{}, execs, nil)
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "kandev-agents", Name: "task-pod"}}
	require.NoError(t, runtime.bootstrapPod(context.Background(), &kubernetesRuntimeClient{streams: kubeexecutor.NewStreamOperations(execs, nil)}, req, pod, kubeexecutor.ProfileConfig{MainContainer: "kandev-agent"}, "bootstrap-nonce", []byte("agentctl")))
	var controlEnv string
	for _, call := range execs.requests {
		if strings.Contains(strings.Join(call.request.Command, " "), kubernetesAuthEnvPath) && len(call.stdin) > 0 {
			controlEnv = string(call.stdin)
		}
	}
	require.NotEmpty(t, controlEnv)
	require.NotContains(t, controlEnv, "must-not-be-inherited")
	require.Contains(t, controlEnv, "bootstrap-nonce")
}

func TestKubernetesTaskPodResourceIdentityIsEnvironmentScoped(t *testing.T) {
	req := validKubernetesCreateRequest()
	req.Metadata[metadataKubernetesTaskOwned] = true
	identity, err := kubernetesIdentity(req)
	require.NoError(t, err)
	require.Equal(t, req.TaskEnvironmentID, identity.InstanceID)
	labels, err := kubeexecutor.OwnershipLabels(identity)
	require.NoError(t, err)
	require.Equal(t, "task-v1", labels["kandev.ai/ownership-version"])
}

func TestKubernetesTaskPodCleanupAfterTaskDeletion(t *testing.T) {
	f := newTaskPodFixture(t)
	instance := f.launch(t, 1)
	ctx := context.Background()
	env, err := f.repo.GetTaskEnvironment(ctx, "environment-1")
	require.NoError(t, err)
	require.NoError(t, f.repo.DeleteTask(ctx, env.TaskID))
	require.NoError(t, f.runtime.StopInstance(ctx, instance, true))
	require.NoError(t, f.manager.DestroyKubernetesEnvironment(ctx, env))
	require.Len(t, f.resources.deletedPods, 1)
	require.Len(t, f.resources.deletedPVCs, 1)
}

func TestKubernetesTaskPodAuthUploadCreatesSessionHome(t *testing.T) {
	req := validKubernetesCreateRequest()
	req.Metadata[metadataKubernetesTaskOwned] = true
	// The hardened uploader creates destination parents before preparation or
	// agent startup uses HOME, including sessions with no credential files.
	require.Equal(t, kubernetesSessionHome(req), path.Dir(kubernetesSessionAuthPath(req)))
}

func TestKubernetesTaskPodStopAfterExecutionIdentityChanges(t *testing.T) {
	f := newTaskPodFixture(t)
	instance := f.launch(t, 1)
	rebound := *instance
	rebound.InstanceID = "resumed-local-execution"
	require.NoError(t, f.runtime.StopInstance(context.Background(), &rebound, true))
	env, err := f.repo.GetTaskEnvironment(context.Background(), "environment-1")
	require.NoError(t, err)
	require.NoError(t, f.manager.DestroyKubernetesEnvironment(context.Background(), env), "stopping the same remote session must release its original local connection")
	require.Len(t, f.resources.deletedPods, 1)
}
