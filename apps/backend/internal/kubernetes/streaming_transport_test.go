package kubernetes

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery/fake"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"

	agentkubernetes "github.com/kandev/kandev/internal/agent/kubernetes"
)

func execTransportTestHandler(t *testing.T) (*Handler, *kubernetesfake.Clientset, *streamingPodState) {
	t.Helper()
	clientset := kubernetesfake.NewSimpleClientset()
	clientset.Discovery().(*fake.FakeDiscovery).FakedServerVersion = &version.Info{GitVersion: "v1.30.1"}
	clientset.PrependReactor("create", "selfsubjectaccessreviews", allowAccessReview)
	probeState := installStreamingPodReactors(t, clientset)
	handler := NewHandler(nil, nil, func(agentkubernetes.ExecutorConfig) (*agentkubernetes.Client, error) {
		return &agentkubernetes.Client{Clientset: clientset}, nil
	})
	handler.probeStreaming = func(context.Context, *agentkubernetes.Client, string, string, string) error {
		t.Fatal("exec transport must not run the port-forward probe")
		return nil
	}
	return handler, clientset, probeState
}

func execTransportExecutorConfig() map[string]string {
	config := validHandlerExecutorConfig()
	config["transport"] = "exec"
	return config
}

func TestConnectionExecTransportRunsByteStreamProbe(t *testing.T) {
	handler, clientset, probeState := execTransportTestHandler(t)
	execProbeCalls := 0
	handler.probeExecStreaming = func(_ context.Context, _ *agentkubernetes.Client, _, podName, mainContainer string) error {
		execProbeCalls++
		require.Equal(t, probeState.actualPod.Name, podName)
		require.Equal(t, "kandev-agent", mainContainer)
		return nil
	}

	result := handler.testConnection(context.Background(), TestRequest{
		Config: execTransportExecutorConfig(), ProfileConfig: validHandlerProfileConfig(),
	})

	require.True(t, result.Success, result.Error)
	require.Equal(t, 1, execProbeCalls)
	streaming := result.Steps[len(result.Steps)-1]
	require.Equal(t, "streaming", streaming.Key)
	require.Contains(t, streaming.Detail, "byte round trip")
	permissions := accessReviewPermissions(clientset.Actions())
	require.Contains(t, permissions, "create pods/exec")
	require.NotContains(t, permissions, "create pods/portforward")
	requireStreamingPodCleaned(t, probeState)
}

func TestConnectionExecTransportReportsByteStreamFailure(t *testing.T) {
	handler, _, probeState := execTransportTestHandler(t)
	handler.probeExecStreaming = func(context.Context, *agentkubernetes.Client, string, string, string) error {
		return errors.New("pods/exec byte stream returned 0 of 65536 bytes unchanged")
	}

	result := handler.testConnection(context.Background(), TestRequest{
		Config: execTransportExecutorConfig(), ProfileConfig: validHandlerProfileConfig(),
	})

	require.False(t, result.Success)
	streaming := result.Steps[len(result.Steps)-1]
	require.Equal(t, "streaming", streaming.Key)
	require.False(t, streaming.Success)
	require.Contains(t, streaming.Error, "0 of 65536 bytes")
	requireStreamingPodCleaned(t, probeState)
}

func TestStreamingPermissionsFollowTransport(t *testing.T) {
	subresources := func(permissions []requiredPermission) []string {
		values := make([]string, 0, len(permissions))
		for _, permission := range permissions {
			values = append(values, permission.verb+" "+permission.subresource)
		}
		return values
	}
	require.Equal(t,
		[]string{"get exec", "create exec", "get portforward", "create portforward"},
		subresources(streamingPermissions(agentkubernetes.StreamTransportPortForward)),
	)
	require.Equal(t,
		[]string{"get exec", "create exec"},
		subresources(streamingPermissions(agentkubernetes.StreamTransportExec)),
	)
}

func TestVerifyStreamingProbeEchoRequiresExactBytes(t *testing.T) {
	sent := []byte("kandev-exec-probe")
	require.NoError(t, verifyStreamingProbeEcho(sent, []byte("kandev-exec-probe")))
	require.ErrorContains(t, verifyStreamingProbeEcho(sent, nil), "returned 0 of 17 bytes")
	require.Error(t, verifyStreamingProbeEcho(sent, []byte("kandev-exec-probX")))
}

func TestLimitedProbeWriterBoundsEchoBuffer(t *testing.T) {
	var buffer bytes.Buffer
	writer := &limitedProbeWriter{buffer: &buffer, limit: 4}
	written, err := writer.Write([]byte("abcdef"))
	require.NoError(t, err)
	require.Equal(t, 6, written)
	_, _ = writer.Write([]byte("gh"))
	require.Equal(t, "abcd", buffer.String())
}

func TestExecStreamingProbeRejectsMissingClient(t *testing.T) {
	err := probeExecStreamingTransport(context.Background(), nil, "kandev", "probe", "kandev-agent")
	require.ErrorContains(t, err, "unavailable")
}
