package kubernetes

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestValidateAdmittedPodAllowsUnrelatedWebhookAdditions(t *testing.T) {
	desired, admitted := admittedPodFixture(t)
	if admitted.Labels == nil {
		admitted.Labels = make(map[string]string)
	}
	if admitted.Annotations == nil {
		admitted.Annotations = make(map[string]string)
	}
	if admitted.Spec.NodeSelector == nil {
		admitted.Spec.NodeSelector = make(map[string]string)
	}
	admitted.Labels["mesh.example/injected"] = "true"
	admitted.Annotations["mesh.example/status"] = "ready"
	admitted.ManagedFields = []metav1.ManagedFieldsEntry{{Manager: "api-server"}}
	admitted.Spec.NodeSelector["topology.kubernetes.io/zone"] = "zone-a"
	admitted.Spec.Affinity = &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{
			MatchExpressions: []corev1.NodeSelectorRequirement{{
				Key: "node.example/class", Operator: corev1.NodeSelectorOpIn, Values: []string{"agents"},
			}},
		}}},
	}}

	if err := ValidateAdmittedPod(admitted, desired, DefaultMainContainerName); err != nil {
		t.Fatalf("ValidateAdmittedPod() error = %v", err)
	}
}

func TestValidateAdmittedPodAllowsCompatiblePlatformAffinity(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		operator  corev1.NodeSelectorOperator
		values    []string
		preferred bool
	}{
		{name: "required in includes linux", key: corev1.LabelOSStable, operator: corev1.NodeSelectorOpIn, values: []string{"linux", "other"}},
		{name: "required exists", key: corev1.LabelArchStable, operator: corev1.NodeSelectorOpExists},
		{name: "preferred not-in keeps amd64", key: corev1.LabelArchStable, operator: corev1.NodeSelectorOpNotIn, values: []string{"arm64"}, preferred: true},
		{name: "preferred in includes amd64", key: corev1.LabelArchStable, operator: corev1.NodeSelectorOpIn, values: []string{"amd64"}, preferred: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			desired, admitted := admittedPodFixture(t)
			admitted.Spec.Affinity = nodeAffinityWithRequirement(corev1.NodeSelectorRequirement{
				Key: test.key, Operator: test.operator, Values: test.values,
			}, test.preferred)

			if err := ValidateAdmittedPod(admitted, desired, DefaultMainContainerName); err != nil {
				t.Fatalf("ValidateAdmittedPod() error = %v", err)
			}
		})
	}
}

func TestValidateAdmittedPodRejectsOwnedMutations(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*corev1.Pod)
	}{
		{name: "name", mutate: func(p *corev1.Pod) { p.Name = "other" }},
		{name: "namespace", mutate: func(p *corev1.Pod) { p.Namespace = "other" }},
		{name: "ownership label", mutate: func(p *corev1.Pod) { p.Labels["kandev.ai/session-id"] = "other" }},
		{name: "standard ownership label", mutate: func(p *corev1.Pod) {
			p.Labels["app.kubernetes.io/instance"] = "other"
		}},
		{name: "extra ownership label", mutate: func(p *corev1.Pod) { p.Labels["kandev.ai/forged"] = "true" }},
		{name: "reserved annotation", mutate: func(p *corev1.Pod) { p.Annotations["kandev.ai/owner"] = "other" }},
		{name: "owner reference", mutate: func(p *corev1.Pod) {
			p.OwnerReferences = []metav1.OwnerReference{{APIVersion: "v1", Kind: "ConfigMap", Name: "owner", UID: types.UID("owner")}}
		}},
		{name: "finalizer", mutate: func(p *corev1.Pod) { p.Finalizers = []string{"example.test/hold"} }},
		{name: "restart policy", mutate: func(p *corev1.Pod) { p.Spec.RestartPolicy = corev1.RestartPolicyNever }},
		{name: "os removed", mutate: func(p *corev1.Pod) { p.Spec.OS = nil }},
		{name: "os changed", mutate: func(p *corev1.Pod) { p.Spec.OS = &corev1.PodOS{Name: "windows"} }},
		{name: "automount", mutate: func(p *corev1.Pod) { value := true; p.Spec.AutomountServiceAccountToken = &value }},
		{name: "os selector", mutate: func(p *corev1.Pod) { p.Spec.NodeSelector[corev1.LabelOSStable] = "windows" }},
		{name: "arch selector", mutate: func(p *corev1.Pod) { p.Spec.NodeSelector[corev1.LabelArchStable] = "arm64" }},
		{name: "node name", mutate: func(p *corev1.Pod) { p.Spec.NodeName = "worker-a" }},
		{name: "required conflicting in affinity", mutate: func(p *corev1.Pod) {
			p.Spec.Affinity = nodeAffinityWithRequirement(corev1.NodeSelectorRequirement{
				Key: corev1.LabelOSStable, Operator: corev1.NodeSelectorOpIn, Values: []string{"windows"},
			}, false)
		}},
		{name: "preferred conflicting not-in affinity", mutate: func(p *corev1.Pod) {
			p.Spec.Affinity = nodeAffinityWithRequirement(corev1.NodeSelectorRequirement{
				Key: corev1.LabelArchStable, Operator: corev1.NodeSelectorOpNotIn, Values: []string{"amd64"},
			}, true)
		}},
		{name: "required does-not-exist affinity", mutate: func(p *corev1.Pod) {
			p.Spec.Affinity = nodeAffinityWithRequirement(corev1.NodeSelectorRequirement{
				Key: corev1.LabelOSStable, Operator: corev1.NodeSelectorOpDoesNotExist,
			}, false)
		}},
		{name: "preferred numeric affinity", mutate: func(p *corev1.Pod) {
			p.Spec.Affinity = nodeAffinityWithRequirement(corev1.NodeSelectorRequirement{
				Key: corev1.LabelArchStable, Operator: corev1.NodeSelectorOpGt, Values: []string{"1"},
			}, true)
		}},
		{name: "main command", mutate: func(p *corev1.Pod) { admittedMainContainer(p).Command = []string{"sleep"} }},
		{name: "main args", mutate: func(p *corev1.Pod) { admittedMainContainer(p).Args = []string{"infinity"} }},
		{name: "main workdir", mutate: func(p *corev1.Pod) { admittedMainContainer(p).WorkingDir = "/tmp" }},
		{name: "agentctl port", mutate: func(p *corev1.Pod) {
			admittedMainContainer(p).Ports[len(admittedMainContainer(p).Ports)-1].ContainerPort = 9999
		}},
		{name: "home env", mutate: func(p *corev1.Pod) {
			admittedMainContainer(p).Env = append(admittedMainContainer(p).Env, corev1.EnvVar{Name: "HOME", Value: "/tmp"})
		}},
		{name: "future agentctl env", mutate: func(p *corev1.Pod) {
			admittedMainContainer(p).Env = append(admittedMainContainer(p).Env, corev1.EnvVar{Name: "AGENTCTL_FUTURE", Value: "x"})
		}},
		{name: "future kandev env", mutate: func(p *corev1.Pod) {
			admittedMainContainer(p).Env = append(admittedMainContainer(p).Env, corev1.EnvVar{Name: "KANDEV_FUTURE", Value: "x"})
		}},
		{name: "main envFrom", mutate: func(p *corev1.Pod) {
			admittedMainContainer(p).EnvFrom = []corev1.EnvFromSource{{
				ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "runtime-env"}},
			}}
		}},
		{name: "reserved mount", mutate: func(p *corev1.Pod) {
			admittedMainContainer(p).VolumeMounts[0].MountPath = "/tmp/runtime"
		}},
		{name: "reserved volume", mutate: func(p *corev1.Pod) { p.Spec.Volumes[0].EmptyDir = nil }},
		{name: "sidecar reserved mount", mutate: func(p *corev1.Pod) {
			p.Spec.Containers = append(p.Spec.Containers, corev1.Container{
				Name: "sidecar", Image: "example/sidecar", VolumeMounts: []corev1.VolumeMount{{Name: AuthVolumeName, MountPath: AuthMountPath}},
			})
		}},
		{name: "init reserved env", mutate: func(p *corev1.Pod) {
			p.Spec.InitContainers = append(p.Spec.InitContainers, corev1.Container{
				Name: "init", Image: "example/init", Env: []corev1.EnvVar{{Name: "KANDEV_TASK_ID", Value: "forged"}},
			})
		}},
		{name: "ephemeral reserved mount", mutate: func(p *corev1.Pod) {
			p.Spec.EphemeralContainers = append(p.Spec.EphemeralContainers, corev1.EphemeralContainer{
				EphemeralContainerCommon: corev1.EphemeralContainerCommon{
					Name: "debug", Image: "example/debug", VolumeMounts: []corev1.VolumeMount{{Name: RuntimeVolumeName, MountPath: RuntimeMountPath}},
				},
			})
		}},
		{name: "reserved volume device", mutate: func(p *corev1.Pod) {
			admittedMainContainer(p).VolumeDevices = []corev1.VolumeDevice{{Name: WorkspaceVolumeName, DevicePath: "/dev/workspace"}}
		}},
		{name: "reserved volume device path", mutate: func(p *corev1.Pod) {
			admittedMainContainer(p).VolumeDevices = []corev1.VolumeDevice{{Name: "foreign", DevicePath: "/opt/kandev/device"}}
		}},
	}

	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			desired, admitted := admittedPodFixture(t)
			mutation.mutate(admitted)
			if err := ValidateAdmittedPod(admitted, desired, DefaultMainContainerName); err == nil {
				t.Fatal("ValidateAdmittedPod() error = nil")
			}
		})
	}
}

func TestValidateAdmittedPVCAllowsBindingAndDefaultStorageClass(t *testing.T) {
	desired, err := BuildPersistentVolumeClaim(
		WorkspaceConfig{Mode: WorkspaceModeManagedPVC, Size: "1Gi", AccessModes: []string{"ReadWriteOnce", "ReadOnlyMany"}},
		PVCOptions{Name: "workspace", Namespace: "agents", Identity: podOptions().Identity},
	)
	if err != nil {
		t.Fatal(err)
	}
	admitted := desired.DeepCopy()
	admitted.UID = "pvc-uid"
	admitted.ResourceVersion = "1"
	admitted.ManagedFields = []metav1.ManagedFieldsEntry{{Manager: "api-server"}}
	admitted.Finalizers = []string{"kubernetes.io/pvc-protection"}
	admitted.Labels["storage.example/defaulted"] = "true"
	admitted.Annotations = map[string]string{"storage.example/status": "bound"}
	defaultClass := "default-class"
	admitted.Spec.StorageClassName = &defaultClass
	admitted.Spec.VolumeName = "pv-1"
	admitted.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{
		corev1.ReadOnlyMany, corev1.ReadWriteOnce,
	}
	admitted.Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse("1024Mi")
	admitted.Status.Phase = corev1.ClaimBound

	if err := ValidateAdmittedPVC(admitted, desired); err != nil {
		t.Fatalf("ValidateAdmittedPVC() error = %v", err)
	}
}

func TestValidateAdmittedPVCRejectsOwnedMutations(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*corev1.PersistentVolumeClaim)
	}{
		{name: "name", mutate: func(p *corev1.PersistentVolumeClaim) { p.Name = "other" }},
		{name: "namespace", mutate: func(p *corev1.PersistentVolumeClaim) { p.Namespace = "other" }},
		{name: "ownership label", mutate: func(p *corev1.PersistentVolumeClaim) { p.Labels["kandev.ai/session-id"] = "other" }},
		{name: "standard ownership label", mutate: func(p *corev1.PersistentVolumeClaim) {
			p.Labels["app.kubernetes.io/managed-by"] = "other"
		}},
		{name: "extra ownership label", mutate: func(p *corev1.PersistentVolumeClaim) { p.Labels["kandev.ai/forged"] = "true" }},
		{name: "reserved annotation", mutate: func(p *corev1.PersistentVolumeClaim) {
			p.Annotations = map[string]string{"kandev.ai/owner": "forged"}
		}},
		{name: "owner reference", mutate: func(p *corev1.PersistentVolumeClaim) {
			p.OwnerReferences = []metav1.OwnerReference{{APIVersion: "v1", Kind: "Pod", Name: "owner", UID: "owner"}}
		}},
		{name: "finalizer", mutate: func(p *corev1.PersistentVolumeClaim) { p.Finalizers = []string{"example.test/hold"} }},
		{name: "extra finalizer", mutate: func(p *corev1.PersistentVolumeClaim) {
			p.Finalizers = []string{"kubernetes.io/pvc-protection", "example.test/hold"}
		}},
		{name: "access modes", mutate: func(p *corev1.PersistentVolumeClaim) {
			p.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
		}},
		{name: "volume mode", mutate: func(p *corev1.PersistentVolumeClaim) { mode := corev1.PersistentVolumeBlock; p.Spec.VolumeMode = &mode }},
		{name: "requests", mutate: func(p *corev1.PersistentVolumeClaim) {
			p.Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse("2Gi")
		}},
		{name: "explicit storage class", mutate: func(p *corev1.PersistentVolumeClaim) { value := "slow"; p.Spec.StorageClassName = &value }},
		{name: "volume attributes class", mutate: func(p *corev1.PersistentVolumeClaim) {
			value := "mutated"
			p.Spec.VolumeAttributesClassName = &value
		}},
		{name: "selector", mutate: func(p *corev1.PersistentVolumeClaim) {
			p.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"disk": "fast"}}
		}},
		{name: "data source", mutate: func(p *corev1.PersistentVolumeClaim) {
			p.Spec.DataSource = &corev1.TypedLocalObjectReference{Kind: "PersistentVolumeClaim", Name: "source"}
		}},
		{name: "data source ref", mutate: func(p *corev1.PersistentVolumeClaim) {
			p.Spec.DataSourceRef = &corev1.TypedObjectReference{Kind: "PersistentVolumeClaim", Name: "source"}
		}},
	}

	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			desired, err := BuildPersistentVolumeClaim(
				WorkspaceConfig{Mode: WorkspaceModeManagedPVC, Size: "1Gi", StorageClass: "fast", AccessModes: []string{"ReadWriteOnce"}},
				PVCOptions{Name: "workspace", Namespace: "agents", Identity: podOptions().Identity},
			)
			if err != nil {
				t.Fatal(err)
			}
			admitted := desired.DeepCopy()
			admitted.UID = "pvc-uid"
			mutation.mutate(admitted)
			if err := ValidateAdmittedPVC(admitted, desired); err == nil {
				t.Fatal("ValidateAdmittedPVC() error = nil")
			}
		})
	}
}

func admittedPodFixture(t *testing.T) (*corev1.Pod, *corev1.Pod) {
	t.Helper()
	template, err := ParsePodTemplate(validPodTemplate(""))
	if err != nil {
		t.Fatal(err)
	}
	desired, _, err := ComposePod(template, validProfile(""), podOptions())
	if err != nil {
		t.Fatal(err)
	}
	admitted := desired.DeepCopy()
	admitted.UID = "pod-uid"
	admitted.ResourceVersion = "1"
	if admitted.Labels == nil {
		admitted.Labels = make(map[string]string)
	}
	if admitted.Annotations == nil {
		admitted.Annotations = make(map[string]string)
	}
	if admitted.Spec.NodeSelector == nil {
		admitted.Spec.NodeSelector = make(map[string]string)
	}
	return desired, admitted
}

func admittedMainContainer(pod *corev1.Pod) *corev1.Container {
	for index := range pod.Spec.Containers {
		if pod.Spec.Containers[index].Name == DefaultMainContainerName {
			return &pod.Spec.Containers[index]
		}
	}
	panic("main container missing")
}

func nodeAffinityWithRequirement(requirement corev1.NodeSelectorRequirement, preferred bool) *corev1.Affinity {
	nodeAffinity := &corev1.NodeAffinity{}
	if preferred {
		nodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution = []corev1.PreferredSchedulingTerm{{
			Weight: 1, Preference: corev1.NodeSelectorTerm{MatchExpressions: []corev1.NodeSelectorRequirement{requirement}},
		}}
	} else {
		nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{
			MatchExpressions: []corev1.NodeSelectorRequirement{requirement},
		}}}
	}
	return &corev1.Affinity{NodeAffinity: nodeAffinity}
}
