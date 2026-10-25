package kubernetes

import (
	"context"
	"errors"
	"net"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	agentkubernetes "github.com/kandev/kandev/internal/agent/kubernetes"
)

func TestLiveStreamingProbeReconcilesCommittedPodAfterCreateError(t *testing.T) {
	createErr := ambiguousStreamingCreateError("create response was lost")
	clientset, state := createErrorStreamingClient(t, createErr, true)
	profile := streamingProbeProfile(t)
	probeCalls := 0
	handler := &Handler{probeStreaming: func(
		context.Context, *agentkubernetes.Client, string, string, string,
	) error {
		probeCalls++
		return nil
	}}

	err := handler.runLiveStreamingProbe(context.Background(), &agentkubernetes.Client{
		Clientset: clientset,
	}, "kandev", profile)

	require.ErrorIs(t, err, createErr)
	require.EqualError(t, err, createErr.Error())
	require.Zero(t, probeCalls)
	require.Regexp(t, regexp.MustCompile(`^[0-9a-f]{64}$`),
		state.requestedPod.Annotations[agentkubernetes.CreateNonceAnnotation])
	require.Equal(t, state.requestedPod.Annotations[agentkubernetes.CreateNonceAnnotation],
		state.actualPod.Annotations[agentkubernetes.CreateNonceAnnotation])
	requireStreamingPodCleaned(t, state)
	requireOnlyExactProbePodActions(t, clientset.Actions(), state.requestedPod.Name)
}

func TestLiveStreamingProbeCreateErrorNotFoundPreservesOriginalError(t *testing.T) {
	createErr := ambiguousStreamingCreateError("create connection reset")
	clientset, state := createErrorStreamingClient(t, createErr, false)
	profile := streamingProbeProfile(t)
	handler := &Handler{probeStreaming: func(
		context.Context, *agentkubernetes.Client, string, string, string,
	) error {
		return nil
	}}

	err := handler.runLiveStreamingProbe(context.Background(), &agentkubernetes.Client{
		Clientset: clientset,
	}, "kandev", profile)

	require.EqualError(t, err, createErr.Error())
	require.False(t, state.deleteIssued)
	requireOnlyExactProbePodActions(t, clientset.Actions(), state.requestedPod.Name)
	require.Equal(t, []string{"create", "get"}, podActionVerbs(clientset.Actions()))
}

func TestLiveStreamingProbeCreateErrorFailsClosedOnAmbiguousPod(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*streamingPodState)
		want      string
	}{
		{
			name: "read error",
			configure: func(state *streamingPodState) {
				state.getErr = errors.New("read denied: Bearer abcdefghijklmnopqrstuvwxyz")
			},
			want: "inspect exact Pod",
		},
		{
			name: "missing uid",
			configure: func(state *streamingPodState) {
				state.mutateLivePod = func(pod *corev1.Pod) { pod.UID = "" }
			},
			want: "identity is incomplete",
		},
		{
			name: "ownership mismatch",
			configure: func(state *streamingPodState) {
				state.mutateLivePod = func(pod *corev1.Pod) {
					pod.Labels["kandev.ai/session-id"] = "foreign"
				}
			},
			want: "ownership label",
		},
		{
			name: "missing create nonce",
			configure: func(state *streamingPodState) {
				state.mutateLivePod = func(pod *corev1.Pod) {
					delete(pod.Annotations, agentkubernetes.CreateNonceAnnotation)
				}
			},
			want: "owned annotation",
		},
		{
			name: "different create nonce",
			configure: func(state *streamingPodState) {
				state.mutateLivePod = func(pod *corev1.Pod) {
					if pod.Annotations == nil {
						pod.Annotations = make(map[string]string)
					}
					pod.Annotations[agentkubernetes.CreateNonceAnnotation] = "foreign"
				}
			},
			want: "owned annotation",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			createErr := ambiguousStreamingCreateError("create result is ambiguous")
			clientset, state := createErrorStreamingClient(t, createErr, true)
			test.configure(state)
			profile := streamingProbeProfile(t)
			handler := &Handler{probeStreaming: func(
				context.Context, *agentkubernetes.Client, string, string, string,
			) error {
				return nil
			}}

			err := handler.runLiveStreamingProbe(context.Background(), &agentkubernetes.Client{
				Clientset: clientset,
			}, "kandev", profile)

			require.ErrorIs(t, err, createErr)
			require.ErrorContains(t, err, test.want)
			require.False(t, state.deleteIssued, "ambiguous identity must never authorize deletion")
			requireOnlyExactProbePodActions(t, clientset.Actions(), state.requestedPod.Name)
			if test.name == "read error" {
				require.NotContains(t, sanitizeError(err), "abcdefghijklmnopqrstuvwxyz")
			}
		})
	}
}

func TestLiveStreamingProbeCreateErrorJoinsCleanupFailure(t *testing.T) {
	createErr := ambiguousStreamingCreateError("create response was lost")
	clientset, state := createErrorStreamingClient(t, createErr, true)
	state.deleteErr = errors.New("cleanup denied")
	profile := streamingProbeProfile(t)
	handler := &Handler{probeStreaming: func(
		context.Context, *agentkubernetes.Client, string, string, string,
	) error {
		return nil
	}}

	err := handler.runLiveStreamingProbe(context.Background(), &agentkubernetes.Client{
		Clientset: clientset,
	}, "kandev", profile)

	require.ErrorIs(t, err, createErr)
	require.ErrorContains(t, err, "cleanup denied")
	require.ErrorIs(t, err, errStreamingProbeCleanup)
	requireOnlyExactProbePodActions(t, clientset.Actions(), state.requestedPod.Name)
}

func TestLiveStreamingProbeDefiniteCreateErrorsNeverAdoptVisiblePod(t *testing.T) {
	pods := schema.GroupResource{Resource: "pods"}
	tests := []struct {
		name      string
		createErr error
	}{
		{name: "already exists", createErr: apierrors.NewAlreadyExists(pods, "probe-pod")},
		{name: "conflict", createErr: apierrors.NewConflict(pods, "probe-pod", errors.New("conflict"))},
		{name: "too many requests", createErr: apierrors.NewTooManyRequests("rate limited", 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clientset, state := createErrorStreamingClient(t, test.createErr, true)
			handler := &Handler{probeStreaming: func(
				context.Context, *agentkubernetes.Client, string, string, string,
			) error {
				return nil
			}}

			err := handler.runLiveStreamingProbe(context.Background(), &agentkubernetes.Client{
				Clientset: clientset,
			}, "kandev", streamingProbeProfile(t))

			require.ErrorIs(t, err, test.createErr)
			require.False(t, state.deleteIssued)
			require.True(t, state.live, "a visible same-name Pod must not be adopted after a definite rejection")
			require.Equal(t, []string{"create"}, podActionVerbs(clientset.Actions()))
		})
	}
}

func createErrorStreamingClient(
	t *testing.T,
	createErr error,
	committed bool,
) (*kubernetesfake.Clientset, *streamingPodState) {
	t.Helper()
	clientset := kubernetesfake.NewSimpleClientset()
	state := installStreamingPodReactors(t, clientset)
	state.createErr = createErr
	state.commitOnCreateError = committed
	return clientset, state
}

func ambiguousStreamingCreateError(message string) error {
	return &net.OpError{Op: "create", Net: "tcp", Err: errors.New(message)}
}

func streamingProbeProfile(t *testing.T) agentkubernetes.ProfileConfig {
	t.Helper()
	profile, err := agentkubernetes.ParseProfileConfig(validHandlerProfileConfig())
	require.NoError(t, err)
	return profile
}

func requireOnlyExactProbePodActions(t *testing.T, actions []k8stesting.Action, name string) {
	t.Helper()
	for _, action := range actions {
		if action.GetResource().Resource != "pods" {
			continue
		}
		require.Equal(t, "kandev", action.GetNamespace())
		require.NotEqual(t, "list", action.GetVerb())
		switch typed := action.(type) {
		case k8stesting.DeleteAction:
			require.Equal(t, name, typed.GetName())
		case k8stesting.GetAction:
			require.Equal(t, name, typed.GetName())
		}
	}
}

func podActionVerbs(actions []k8stesting.Action) []string {
	verbs := make([]string, 0, len(actions))
	for _, action := range actions {
		if action.GetResource().Resource == "pods" {
			verbs = append(verbs, action.GetVerb())
		}
	}
	return verbs
}
