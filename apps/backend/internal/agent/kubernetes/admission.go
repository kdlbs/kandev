package kubernetes

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ValidateAdmittedPod verifies the fields Kandev owns after API admission.
// Unrelated labels, annotations, scheduling selectors, and server metadata are
// intentionally allowed so normal admission/defaulting can still operate.
func ValidateAdmittedPod(admitted, desired *corev1.Pod, mainContainer string) error {
	if admitted == nil || desired == nil {
		return errors.New("admitted Pod validation requires both objects")
	}
	if err := validateAdmittedMetadata("Pod", admitted.ObjectMeta, desired.ObjectMeta); err != nil {
		return err
	}
	if err := validateAdmittedPodSpec(admitted, desired); err != nil {
		return err
	}
	desiredMain := admittedContainerByName(desired.Spec.Containers, mainContainer)
	admittedMain := admittedContainerByName(admitted.Spec.Containers, mainContainer)
	if desiredMain == nil || admittedMain == nil {
		return errors.New("admitted Pod is missing the owned main container")
	}
	if !reflect.DeepEqual(admittedMain.Command, desiredMain.Command) ||
		!reflect.DeepEqual(admittedMain.Args, desiredMain.Args) ||
		admittedMain.WorkingDir != desiredMain.WorkingDir {
		return errors.New("admitted Pod mutated the main-container bootstrap")
	}
	if err := validateAdmittedContainers(admitted, desiredMain, mainContainer); err != nil {
		return err
	}
	return validateAdmittedVolumes(admitted.Spec.Volumes, desired.Spec.Volumes)
}

// ValidateAdmittedPVC verifies the Kandev-owned shape of a managed claim while
// allowing ordinary API binding fields and default storage-class selection.
func ValidateAdmittedPVC(admitted, desired *corev1.PersistentVolumeClaim) error {
	if admitted == nil || desired == nil {
		return errors.New("admitted PVC validation requires both objects")
	}
	if err := validateAdmittedMetadata("PVC", admitted.ObjectMeta, desired.ObjectMeta); err != nil {
		return err
	}
	if !samePersistentVolumeAccessModes(admitted.Spec.AccessModes, desired.Spec.AccessModes) ||
		!reflect.DeepEqual(admitted.Spec.VolumeMode, desired.Spec.VolumeMode) ||
		!sameVolumeResourceRequirements(admitted.Spec.Resources, desired.Spec.Resources) {
		return errors.New("admitted PVC mutated owned storage requirements")
	}
	if desired.Spec.StorageClassName != nil &&
		!reflect.DeepEqual(admitted.Spec.StorageClassName, desired.Spec.StorageClassName) {
		return errors.New("admitted PVC mutated the explicit storage class")
	}
	if !reflect.DeepEqual(admitted.Spec.VolumeAttributesClassName, desired.Spec.VolumeAttributesClassName) {
		return errors.New("admitted PVC mutated the volume attributes class")
	}
	if !reflect.DeepEqual(admitted.Spec.Selector, desired.Spec.Selector) ||
		!reflect.DeepEqual(admitted.Spec.DataSource, desired.Spec.DataSource) ||
		!reflect.DeepEqual(admitted.Spec.DataSourceRef, desired.Spec.DataSourceRef) {
		return errors.New("admitted PVC mutated selector or data-source semantics")
	}
	return nil
}

// ValidateOwnershipLabels verifies all Kandev-owned standard and custom
// ownership labels while allowing unrelated labels added by the API server or
// cluster policy.
func ValidateOwnershipLabels(admitted, desired map[string]string) error {
	return validateAdmittedOwnedLabels("resource", admitted, desired)
}

func validateAdmittedMetadata(kind string, admitted, desired metav1.ObjectMeta) error {
	if admitted.Name != desired.Name || admitted.Namespace != desired.Namespace {
		return fmt.Errorf("admitted %s name or namespace changed", kind)
	}
	if len(admitted.OwnerReferences) != 0 {
		return fmt.Errorf("admitted %s added an owner reference", kind)
	}
	if !admittedFinalizersAllowed(kind, admitted.Finalizers) {
		return fmt.Errorf("admitted %s added a finalizer", kind)
	}
	if err := validateAdmittedOwnedLabels(kind, admitted.Labels, desired.Labels); err != nil {
		return err
	}
	return validateAdmittedOwnedAnnotations(kind, admitted.Annotations, desired.Annotations)
}

func validateAdmittedOwnedLabels(kind string, admitted, desired map[string]string) error {
	for key, desiredValue := range desired {
		if !isOwnedMetadataLabel(key) {
			continue
		}
		if admitted[key] != desiredValue {
			return fmt.Errorf("admitted %s ownership label %q changed", kind, key)
		}
	}
	for key, admittedValue := range admitted {
		if !isOwnedMetadataLabel(key) {
			continue
		}
		desiredValue, exists := desired[key]
		if !exists || admittedValue != desiredValue {
			return fmt.Errorf("admitted %s added or changed ownership label %q", kind, key)
		}
	}
	return nil
}

func validateAdmittedOwnedAnnotations(kind string, admitted, desired map[string]string) error {
	for key, desiredValue := range desired {
		if !strings.HasPrefix(key, "kandev.ai/") {
			continue
		}
		if admitted[key] != desiredValue {
			return fmt.Errorf("admitted %s owned annotation %q changed", kind, key)
		}
	}
	for key, admittedValue := range admitted {
		if !strings.HasPrefix(key, "kandev.ai/") {
			continue
		}
		desiredValue, exists := desired[key]
		if !exists || admittedValue != desiredValue {
			return fmt.Errorf("admitted %s added or changed owned annotation %q", kind, key)
		}
	}
	return nil
}

func isOwnedMetadataLabel(key string) bool {
	return reservedLabelKeys[key] || strings.HasPrefix(key, "kandev.ai/")
}

func admittedFinalizersAllowed(kind string, finalizers []string) bool {
	if len(finalizers) == 0 {
		return true
	}
	return kind == "PVC" && len(finalizers) == 1 && finalizers[0] == "kubernetes.io/pvc-protection"
}

func validateAdmittedPodSpec(admitted, desired *corev1.Pod) error {
	if admitted.Spec.RestartPolicy != desired.Spec.RestartPolicy ||
		admitted.Spec.NodeName != desired.Spec.NodeName ||
		!reflect.DeepEqual(admitted.Spec.OS, desired.Spec.OS) ||
		!reflect.DeepEqual(admitted.Spec.AutomountServiceAccountToken, desired.Spec.AutomountServiceAccountToken) {
		return errors.New("admitted Pod mutated an owned Pod invariant")
	}
	for _, key := range []string{corev1.LabelOSStable, corev1.LabelArchStable} {
		if admitted.Spec.NodeSelector[key] != desired.Spec.NodeSelector[key] {
			return fmt.Errorf("admitted Pod mutated owned node selector %q", key)
		}
	}
	return validateAdmittedNodeAffinity(admitted.Spec.Affinity, desired.Spec.Affinity, desired.Spec.NodeSelector)
}

type admittedNodeRequirement struct {
	requirement corev1.NodeSelectorRequirement
	path        string
	preferred   bool
	matchField  bool
}

func validateAdmittedNodeAffinity(
	admitted, desired *corev1.Affinity,
	nodeSelector map[string]string,
) error {
	desiredRequirements := platformNodeRequirements(desired)
	matchedDesired := make([]bool, len(desiredRequirements))
	for _, admittedRequirement := range platformNodeRequirements(admitted) {
		if matchDesiredNodeRequirement(admittedRequirement, desiredRequirements, matchedDesired) {
			continue
		}
		if admittedRequirement.matchField ||
			!platformRequirementAllows(admittedRequirement.requirement, nodeSelector) {
			return fmt.Errorf("admitted Pod added conflicting platform node affinity at %s", admittedRequirement.path)
		}
	}
	for _, matched := range matchedDesired {
		if !matched {
			return errors.New("admitted Pod mutated desired platform node affinity")
		}
	}
	return nil
}

func matchDesiredNodeRequirement(
	admitted admittedNodeRequirement,
	desired []admittedNodeRequirement,
	matched []bool,
) bool {
	for index, candidate := range desired {
		if matched[index] || admitted.preferred != candidate.preferred || admitted.matchField != candidate.matchField {
			continue
		}
		if reflect.DeepEqual(admitted.requirement, candidate.requirement) {
			matched[index] = true
			return true
		}
	}
	return false
}

func platformNodeRequirements(affinity *corev1.Affinity) []admittedNodeRequirement {
	if affinity == nil || affinity.NodeAffinity == nil {
		return nil
	}
	var requirements []admittedNodeRequirement
	nodeAffinity := affinity.NodeAffinity
	if required := nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution; required != nil {
		for termIndex, term := range required.NodeSelectorTerms {
			requirements = appendPlatformNodeRequirements(requirements, term, false, termIndex)
		}
	}
	for termIndex, term := range nodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution {
		requirements = appendPlatformNodeRequirements(requirements, term.Preference, true, termIndex)
	}
	return requirements
}

func appendPlatformNodeRequirements(
	result []admittedNodeRequirement,
	term corev1.NodeSelectorTerm,
	preferred bool,
	termIndex int,
) []admittedNodeRequirement {
	prefix := "required"
	if preferred {
		prefix = "preferred"
	}
	for index, requirement := range term.MatchExpressions {
		if isPlatformNodeKey(requirement.Key) {
			result = append(result, admittedNodeRequirement{
				requirement: requirement,
				path:        fmt.Sprintf("%s[%d].matchExpressions[%d]", prefix, termIndex, index),
				preferred:   preferred,
			})
		}
	}
	for index, requirement := range term.MatchFields {
		if isPlatformNodeKey(requirement.Key) {
			result = append(result, admittedNodeRequirement{
				requirement: requirement,
				path:        fmt.Sprintf("%s[%d].matchFields[%d]", prefix, termIndex, index),
				preferred:   preferred,
				matchField:  true,
			})
		}
	}
	return result
}

func platformRequirementAllows(requirement corev1.NodeSelectorRequirement, nodeSelector map[string]string) bool {
	want, ok := nodeSelector[requirement.Key]
	if !ok || want == "" {
		return false
	}
	switch requirement.Operator {
	case corev1.NodeSelectorOpIn:
		return stringSliceContains(requirement.Values, want)
	case corev1.NodeSelectorOpNotIn:
		return !stringSliceContains(requirement.Values, want)
	case corev1.NodeSelectorOpExists:
		return true
	default:
		return false
	}
}

func isPlatformNodeKey(key string) bool {
	return key == corev1.LabelOSStable || key == corev1.LabelArchStable
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func validateAdmittedContainers(
	admitted *corev1.Pod,
	desiredMain *corev1.Container,
	mainContainer string,
) error {
	for index := range admitted.Spec.Containers {
		container := &admitted.Spec.Containers[index]
		if err := validateAdmittedContainer(container, desiredMain, container.Name == mainContainer); err != nil {
			return err
		}
	}
	for index := range admitted.Spec.InitContainers {
		if err := validateAdmittedContainer(&admitted.Spec.InitContainers[index], desiredMain, false); err != nil {
			return err
		}
	}
	for index := range admitted.Spec.EphemeralContainers {
		container := corev1.Container(admitted.Spec.EphemeralContainers[index].EphemeralContainerCommon)
		if err := validateAdmittedContainer(&container, desiredMain, false); err != nil {
			return err
		}
	}
	main := admittedContainerByName(admitted.Spec.Containers, mainContainer)
	return validateRequiredMainContainerFields(main, desiredMain)
}

func validateAdmittedContainer(container, desiredMain *corev1.Container, isMain bool) error {
	if err := validateAdmittedContainerEnvironment(container); err != nil {
		return err
	}
	if err := validateAdmittedContainerPorts(container, desiredMain, isMain); err != nil {
		return err
	}
	if err := validateAdmittedContainerMounts(container, desiredMain, isMain); err != nil {
		return err
	}
	return validateAdmittedContainerDevices(container)
}

func validateAdmittedContainerEnvironment(container *corev1.Container) error {
	if len(container.EnvFrom) > 0 {
		return errors.New("admitted Pod uses envFrom, which can inject reserved environment keys")
	}
	for _, env := range container.Env {
		if isReservedEnvironmentKey(env.Name) {
			return errors.New("admitted Pod added a reserved environment key")
		}
	}
	return nil
}

func validateAdmittedContainerPorts(container, desiredMain *corev1.Container, isMain bool) error {
	for _, port := range container.Ports {
		if port.Name != AgentctlContainerPort && port.ContainerPort != DefaultAgentctlPort {
			continue
		}
		if !isMain || countContainerPorts(desiredMain.Ports, port) != 1 {
			return errors.New("admitted Pod mutated the agentctl port")
		}
	}
	return nil
}

func validateAdmittedContainerMounts(container, desiredMain *corev1.Container, isMain bool) error {
	for _, mount := range container.VolumeMounts {
		if !isReservedVolumeName(mount.Name) && !isReservedMountPath(mount.MountPath) {
			continue
		}
		if !isMain || countVolumeMounts(desiredMain.VolumeMounts, mount) != 1 {
			return errors.New("admitted Pod mutated a reserved volume mount")
		}
	}
	return nil
}

func validateAdmittedContainerDevices(container *corev1.Container) error {
	for _, device := range container.VolumeDevices {
		if isReservedVolumeName(device.Name) || isReservedMountPath(device.DevicePath) {
			return errors.New("admitted Pod added a reserved volume device")
		}
	}
	return nil
}

func validateRequiredMainContainerFields(admitted, desired *corev1.Container) error {
	if admitted == nil || desired == nil {
		return errors.New("admitted Pod is missing the owned main container")
	}
	for _, required := range desired.Ports {
		if required.Name == AgentctlContainerPort && countContainerPorts(admitted.Ports, required) != 1 {
			return errors.New("admitted Pod mutated the agentctl port")
		}
	}
	for _, required := range desired.VolumeMounts {
		if isReservedVolumeName(required.Name) && countVolumeMounts(admitted.VolumeMounts, required) != 1 {
			return errors.New("admitted Pod mutated a reserved volume mount")
		}
	}
	return nil
}

func validateAdmittedVolumes(admitted, desired []corev1.Volume) error {
	for _, required := range desired {
		if isReservedVolumeName(required.Name) && countVolumes(admitted, required) != 1 {
			return errors.New("admitted Pod mutated a reserved volume")
		}
	}
	for _, volume := range admitted {
		if !isReservedVolumeName(volume.Name) {
			continue
		}
		if countVolumes(desired, volume) != 1 {
			return errors.New("admitted Pod added or mutated a reserved volume")
		}
	}
	return nil
}

func admittedContainerByName(containers []corev1.Container, name string) *corev1.Container {
	for index := range containers {
		if containers[index].Name == name {
			return &containers[index]
		}
	}
	return nil
}

func countContainerPorts(values []corev1.ContainerPort, want corev1.ContainerPort) int {
	count := 0
	for _, value := range values {
		if reflect.DeepEqual(value, want) {
			count++
		}
	}
	return count
}

func countVolumeMounts(values []corev1.VolumeMount, want corev1.VolumeMount) int {
	count := 0
	for _, value := range values {
		if reflect.DeepEqual(value, want) {
			count++
		}
	}
	return count
}

func countVolumes(values []corev1.Volume, want corev1.Volume) int {
	count := 0
	for _, value := range values {
		if reflect.DeepEqual(value, want) {
			count++
		}
	}
	return count
}

func samePersistentVolumeAccessModes(left, right []corev1.PersistentVolumeAccessMode) bool {
	leftSet := make(map[corev1.PersistentVolumeAccessMode]struct{}, len(left))
	for _, mode := range left {
		leftSet[mode] = struct{}{}
	}
	rightSet := make(map[corev1.PersistentVolumeAccessMode]struct{}, len(right))
	for _, mode := range right {
		rightSet[mode] = struct{}{}
	}
	return reflect.DeepEqual(leftSet, rightSet)
}

func sameVolumeResourceRequirements(left, right corev1.VolumeResourceRequirements) bool {
	return sameResourceList(left.Requests, right.Requests) && sameResourceList(left.Limits, right.Limits)
}

func sameResourceList(left, right corev1.ResourceList) bool {
	if len(left) != len(right) {
		return false
	}
	for name, leftQuantity := range left {
		rightQuantity, exists := right[name]
		if !exists || leftQuantity.Cmp(rightQuantity) != 0 {
			return false
		}
	}
	return true
}
