package kubernetes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	agentkubernetes "github.com/kandev/kandev/internal/agent/kubernetes"
)

func TestRunPodDryRunRejectsAdmissionMutationOfOwnedInvariants(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*corev1.Pod)
	}{
		{name: "restart policy", mutate: func(pod *corev1.Pod) {
			pod.Spec.RestartPolicy = corev1.RestartPolicyNever
		}},
		{name: "operating system", mutate: func(pod *corev1.Pod) {
			pod.Spec.OS = &corev1.PodOS{Name: corev1.Windows}
		}},
		{name: "operating system selector", mutate: func(pod *corev1.Pod) {
			pod.Spec.NodeSelector[corev1.LabelOSStable] = "windows"
		}},
		{name: "architecture selector", mutate: func(pod *corev1.Pod) {
			pod.Spec.NodeSelector[corev1.LabelArchStable] = "arm64"
		}},
		{name: "node name", mutate: func(pod *corev1.Pod) {
			pod.Spec.NodeName = "kind-worker"
		}},
		{name: "required operating system affinity", mutate: func(pod *corev1.Pod) {
			pod.Spec.Affinity = reservedNodeAffinity(corev1.LabelOSStable, false)
		}},
		{name: "preferred architecture affinity", mutate: func(pod *corev1.Pod) {
			pod.Spec.Affinity = reservedNodeAffinity(corev1.LabelArchStable, true)
		}},
		{name: "service account token automount", mutate: func(pod *corev1.Pod) {
			value := true
			pod.Spec.AutomountServiceAccountToken = &value
		}},
		{name: "main command", mutate: func(pod *corev1.Pod) {
			pod.Spec.Containers[0].Command = []string{"false"}
		}},
		{name: "main arguments", mutate: func(pod *corev1.Pod) {
			pod.Spec.Containers[0].Args = []string{"changed"}
		}},
		{name: "main working directory", mutate: func(pod *corev1.Pod) {
			pod.Spec.Containers[0].WorkingDir = "/tmp"
		}},
		{name: "agentctl port", mutate: func(pod *corev1.Pod) {
			pod.Spec.Containers[0].Ports[0].ContainerPort++
		}},
		{name: "reserved mount", mutate: func(pod *corev1.Pod) {
			pod.Spec.Containers[0].VolumeMounts[0].MountPath = "/tmp/runtime"
		}},
		{name: "reserved volume", mutate: func(pod *corev1.Pod) {
			pod.Spec.Volumes[0].EmptyDir = nil
			pod.Spec.Volumes[0].ConfigMap = &corev1.ConfigMapVolumeSource{}
		}},
		{name: "AGENTCTL prefix environment", mutate: func(pod *corev1.Pod) {
			pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, corev1.EnvVar{Name: "AGENTCTL_FUTURE"})
		}},
		{name: "KANDEV prefix environment", mutate: func(pod *corev1.Pod) {
			pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, corev1.EnvVar{Name: "KANDEV_FUTURE"})
		}},
		{name: "HOME environment", mutate: func(pod *corev1.Pod) {
			pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, corev1.EnvVar{Name: "HOME"})
		}},
		{name: "owner reference", mutate: func(pod *corev1.Pod) {
			pod.OwnerReferences = []metav1.OwnerReference{{APIVersion: "v1", Kind: "ConfigMap", Name: "foreign"}}
		}},
		{name: "finalizer", mutate: func(pod *corev1.Pod) {
			pod.Finalizers = []string{"example.com/hold"}
		}},
		{name: "extra Kandev label", mutate: func(pod *corev1.Pod) {
			pod.Labels["kandev.ai/foreign"] = "mutated"
		}},
		{name: "extra Kandev annotation", mutate: func(pod *corev1.Pod) {
			pod.Annotations = map[string]string{"kandev.ai/foreign": "mutated"}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			step := runMutatedPodDryRun(t, test.mutate)

			require.False(t, step.Success)
			require.Equal(t, "Dry-run admission failed", step.Detail)
			require.Contains(t, step.Error, "admitted Pod")
		})
	}
}

func TestRunPodDryRunAllowsUnrelatedAdmissionAdditions(t *testing.T) {
	step := runMutatedPodDryRun(t, func(pod *corev1.Pod) {
		pod.Labels["example.com/injected"] = "true"
		pod.Annotations = map[string]string{"example.com/injected": "true"}
		pod.Spec.NodeSelector["example.com/pool"] = "agents"
		pod.Spec.Affinity = reservedNodeAffinity("example.com/pool", false)
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: "injected", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		})
		pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{
			Name: "injected", Image: "example.test/sidecar:latest",
			Env: []corev1.EnvVar{{Name: "EXAMPLE", Value: "true"}},
		})
	})

	require.True(t, step.Success, step.Error)
}

func TestRunPodDryRunRejectsMissingServerUID(t *testing.T) {
	step := runMutatedPodDryRun(t, func(pod *corev1.Pod) {
		pod.UID = ""
	})

	require.False(t, step.Success)
	require.Contains(t, step.Error, "UID")
}

func TestRunPVCDryRunRejectsAdmissionMutationOfStorageSemantics(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*corev1.PersistentVolumeClaim)
	}{
		{name: "explicit storage class", mutate: func(claim *corev1.PersistentVolumeClaim) {
			value := "slow"
			claim.Spec.StorageClassName = &value
		}},
		{name: "access modes", mutate: func(claim *corev1.PersistentVolumeClaim) {
			claim.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
		}},
		{name: "volume mode", mutate: func(claim *corev1.PersistentVolumeClaim) {
			value := corev1.PersistentVolumeBlock
			claim.Spec.VolumeMode = &value
		}},
		{name: "storage request", mutate: func(claim *corev1.PersistentVolumeClaim) {
			claim.Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse("20Gi")
		}},
		{name: "selector", mutate: func(claim *corev1.PersistentVolumeClaim) {
			claim.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"foreign": "true"}}
		}},
		{name: "data source", mutate: func(claim *corev1.PersistentVolumeClaim) {
			claim.Spec.DataSource = &corev1.TypedLocalObjectReference{Kind: "PersistentVolumeClaim", Name: "foreign"}
		}},
		{name: "data source ref", mutate: func(claim *corev1.PersistentVolumeClaim) {
			claim.Spec.DataSourceRef = &corev1.TypedObjectReference{Kind: "PersistentVolumeClaim", Name: "foreign"}
		}},
		{name: "volume name", mutate: func(claim *corev1.PersistentVolumeClaim) {
			claim.Spec.VolumeName = "foreign-volume"
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			step := runMutatedPVCDryRun(t, "fast", test.mutate)

			require.False(t, step.Success)
			require.Equal(t, "Dry-run admission failed", step.Detail)
			require.Contains(t, step.Error, "admitted PVC")
		})
	}
}

func TestRunPVCDryRunAllowsStorageClassDefaultingWhenNotRequested(t *testing.T) {
	step := runMutatedPVCDryRun(t, "", func(claim *corev1.PersistentVolumeClaim) {
		value := "cluster-default"
		claim.Spec.StorageClassName = &value
		claim.Finalizers = []string{"kubernetes.io/pvc-protection"}
		claim.Annotations = map[string]string{"volume.kubernetes.io/storage-provisioner": "example.test"}
	})

	require.True(t, step.Success, step.Error)
}

func TestRunPVCDryRunRejectsMissingServerUID(t *testing.T) {
	step := runMutatedPVCDryRun(t, "fast", func(claim *corev1.PersistentVolumeClaim) {
		claim.UID = ""
	})

	require.False(t, step.Success)
	require.Contains(t, step.Error, "UID")
}

func TestLiveStreamingProbeRejectsAdmissionMutationAndCleansExactPod(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*corev1.Pod)
	}{
		{name: "operating system", mutate: func(pod *corev1.Pod) {
			pod.Spec.OS = &corev1.PodOS{Name: corev1.Windows}
		}},
		{name: "node name", mutate: func(pod *corev1.Pod) {
			pod.Spec.NodeName = "kind-worker"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clientset := kubernetesfake.NewSimpleClientset()
			state := installStreamingPodReactors(t, clientset)
			state.mutateLivePod = test.mutate
			profile, err := agentkubernetes.ParseProfileConfig(validHandlerProfileConfig())
			require.NoError(t, err)
			probeCalls := 0
			handler := &Handler{probeStreaming: func(
				context.Context, *agentkubernetes.Client, string, string, string,
			) error {
				probeCalls++
				return nil
			}}

			err = handler.runLiveStreamingProbe(context.Background(), &agentkubernetes.Client{
				Clientset: clientset,
			}, "kandev", profile)

			require.ErrorContains(t, err, "admitted Pod")
			require.Zero(t, probeCalls)
			requireStreamingPodCleaned(t, state)
		})
	}
}

func TestLiveStreamingProbeCleansReturnedIdentityAfterAdmissionMismatch(t *testing.T) {
	clientset := kubernetesfake.NewSimpleClientset()
	state := installStreamingPodReactors(t, clientset)
	state.mutateLivePod = func(pod *corev1.Pod) {
		pod.Name = "admitted-probe-name"
		pod.Namespace = "admitted-probe-namespace"
	}
	profile, err := agentkubernetes.ParseProfileConfig(validHandlerProfileConfig())
	require.NoError(t, err)
	handler := &Handler{probeStreaming: func(
		context.Context, *agentkubernetes.Client, string, string, string,
	) error {
		return nil
	}}

	err = handler.runLiveStreamingProbe(context.Background(), &agentkubernetes.Client{
		Clientset: clientset,
	}, "kandev", profile)

	require.ErrorContains(t, err, "name or namespace")
	require.Equal(t, state.actualPod.Namespace, state.deletedNamespace)
	require.Equal(t, state.actualPod.Name, state.deletedName)
	requireStreamingPodCleaned(t, state)
}

func runMutatedPodDryRun(t *testing.T, mutate func(*corev1.Pod)) TestStep {
	t.Helper()
	clientset := kubernetesfake.NewSimpleClientset()
	clientset.PrependReactor("create", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		pod := action.(k8stesting.CreateAction).GetObject().(*corev1.Pod).DeepCopy()
		pod.UID = "dry-run-pod-uid"
		mutate(pod)
		return true, pod, nil
	})
	profile, err := agentkubernetes.ParseProfileConfig(validHandlerProfileConfig())
	require.NoError(t, err)
	result := &TestResult{}

	runPodDryRun(
		context.Background(), clientset, "kandev", "admission-probe",
		admissionTestIdentity(), profile, result,
	)

	require.Len(t, result.Steps, 1)
	return result.Steps[0]
}

func runMutatedPVCDryRun(
	t *testing.T,
	storageClass string,
	mutate func(*corev1.PersistentVolumeClaim),
) TestStep {
	t.Helper()
	clientset := kubernetesfake.NewSimpleClientset()
	clientset.PrependReactor("create", "persistentvolumeclaims", func(action k8stesting.Action) (bool, runtime.Object, error) {
		claim := action.(k8stesting.CreateAction).GetObject().(*corev1.PersistentVolumeClaim).DeepCopy()
		claim.UID = "dry-run-pvc-uid"
		mutate(claim)
		return true, claim, nil
	})
	values := validHandlerProfileConfig()
	values["workspace.mode"] = "managed_pvc"
	values["workspace.size"] = "10Gi"
	values["workspace.storage_class"] = storageClass
	profile, err := agentkubernetes.ParseProfileConfig(values)
	require.NoError(t, err)
	result := &TestResult{}

	runPVCDryRun(
		context.Background(), clientset, "kandev", "admission-probe",
		admissionTestIdentity(), profile, result,
	)

	require.Len(t, result.Steps, 1)
	return result.Steps[0]
}

func admissionTestIdentity() agentkubernetes.ResourceIdentity {
	return agentkubernetes.ResourceIdentity{
		ExecutorID: connectionTestIdentity, ProfileID: connectionTestIdentity,
		InstanceID: connectionTestIdentity, TaskID: connectionTestIdentity,
		SessionID: connectionTestIdentity, EnvironmentID: connectionTestIdentity,
	}
}

func reservedNodeAffinity(key string, preferred bool) *corev1.Affinity {
	requirement := corev1.NodeSelectorRequirement{
		Key: key, Operator: corev1.NodeSelectorOpIn, Values: []string{"foreign"},
	}
	term := corev1.NodeSelectorTerm{MatchExpressions: []corev1.NodeSelectorRequirement{requirement}}
	nodeAffinity := &corev1.NodeAffinity{}
	if preferred {
		nodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution = []corev1.PreferredSchedulingTerm{{
			Weight: 1, Preference: term,
		}}
	} else {
		nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{term},
		}
	}
	return &corev1.Affinity{NodeAffinity: nodeAffinity}
}
