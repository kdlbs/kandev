package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
)

const kubernetesDeletionPollInterval = 100 * time.Millisecond

func verifyCreatedPod(created, desired *corev1.Pod, identity kubeexecutor.ResourceIdentity, mainContainer string) error {
	if created == nil || desired == nil || created.UID == "" {
		return errors.New("kubernetes lifecycle: created Pod identity is incomplete")
	}
	if err := verifyRecordedPod(created, desired.Namespace, desired.Name, created.UID, identity); err != nil {
		return err
	}
	if err := kubeexecutor.ValidateAdmittedPod(created, desired, mainContainer); err != nil {
		return fmt.Errorf("kubernetes lifecycle: %w", err)
	}
	return nil
}

func verifyRecordedPod(
	pod *corev1.Pod,
	namespace, name string,
	uid types.UID,
	identity kubeexecutor.ResourceIdentity,
) error {
	if pod == nil || pod.Namespace != namespace || pod.Name != name || pod.UID != uid || uid == "" {
		return errors.New("kubernetes lifecycle: Pod UID or name does not match recorded inventory")
	}
	want, err := kubeexecutor.OwnershipLabels(identity)
	if err != nil {
		return err
	}
	if err := kubeexecutor.ValidateOwnershipLabels(pod.Labels, want); err != nil {
		return fmt.Errorf("kubernetes lifecycle: Pod ownership labels: %w", err)
	}
	return nil
}

func verifyOwnedPVC(created, desired *corev1.PersistentVolumeClaim, identity kubeexecutor.ResourceIdentity) error {
	if created == nil || desired == nil || created.UID == "" ||
		created.Namespace != desired.Namespace || created.Name != desired.Name {
		return errors.New("kubernetes lifecycle: created PVC identity is incomplete")
	}
	want, err := kubeexecutor.OwnershipLabels(identity)
	if err != nil {
		return err
	}
	if err := kubeexecutor.ValidateOwnershipLabels(created.Labels, want); err != nil {
		return fmt.Errorf("kubernetes lifecycle: PVC ownership labels: %w", err)
	}
	if err := kubeexecutor.ValidateAdmittedPVC(created, desired); err != nil {
		return fmt.Errorf("kubernetes lifecycle: %w", err)
	}
	return nil
}

func deleteKubernetesPodIfExact(
	ctx context.Context,
	resources kubernetesResourceClient,
	recorded *corev1.Pod,
	identity kubeexecutor.ResourceIdentity,
) error {
	current, err := resources.GetPod(ctx, recorded.Namespace, recorded.Name)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("kubernetes lifecycle: inspect Pod before deletion: %w", err)
	}
	if err := verifyRecordedPod(current, recorded.Namespace, recorded.Name, recorded.UID, identity); err != nil {
		return err
	}
	if err := resources.DeletePod(
		ctx, recorded.Namespace, recorded.Name, recorded.UID, current.ResourceVersion,
	); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("kubernetes lifecycle: delete Pod: %w", err)
	}
	return waitForKubernetesPodDeletion(ctx, resources, recorded.Namespace, recorded.Name, recorded.UID)
}

func deleteKubernetesPVCIfExact(
	ctx context.Context,
	resources kubernetesResourceClient,
	recorded *corev1.PersistentVolumeClaim,
	identity kubeexecutor.ResourceIdentity,
) error {
	current, err := resources.GetPersistentVolumeClaim(ctx, recorded.Namespace, recorded.Name)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("kubernetes lifecycle: inspect PVC before deletion: %w", err)
	}
	if current == nil || current.Namespace != recorded.Namespace || current.Name != recorded.Name || current.UID != recorded.UID {
		return errors.New("kubernetes lifecycle: PVC UID does not match recorded inventory")
	}
	labels, err := kubeexecutor.OwnershipLabels(identity)
	if err != nil {
		return err
	}
	if err := kubeexecutor.ValidateOwnershipLabels(current.Labels, labels); err != nil {
		return fmt.Errorf("kubernetes lifecycle: PVC ownership labels: %w", err)
	}
	if err := resources.DeletePersistentVolumeClaim(
		ctx, recorded.Namespace, recorded.Name, recorded.UID, current.ResourceVersion,
	); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("kubernetes lifecycle: delete PVC: %w", err)
	}
	return waitForKubernetesPVCDeletion(ctx, resources, recorded.Namespace, recorded.Name, recorded.UID)
}

// rollbackCreatedKubernetesPod deletes only the UID returned by this launch's
// Create call. Unlike terminal cleanup, it deliberately does not require
// ownership labels: admission may have mutated those labels, and that is one
// of the failures rollback must remove.
func rollbackCreatedKubernetesPod(
	ctx context.Context,
	resources kubernetesResourceClient,
	created *corev1.Pod,
) error {
	return rollbackCreatedKubernetesResource(ctx, kubernetesPodDeletionTarget(resources, created))
}

func rollbackCreatedKubernetesPVC(
	ctx context.Context,
	resources kubernetesResourceClient,
	created *corev1.PersistentVolumeClaim,
) error {
	return rollbackCreatedKubernetesResource(ctx, kubernetesPVCDeletionTarget(resources, created))
}

type kubernetesCurrentResource struct {
	namespace       string
	name            string
	uid             types.UID
	resourceVersion string
}

type kubernetesDeletionTarget struct {
	kind      string
	namespace string
	name      string
	uid       types.UID
	get       func(context.Context) (kubernetesCurrentResource, error)
	delete    func(context.Context, types.UID, string) error
	wait      func(context.Context, types.UID) error
}

func kubernetesPodDeletionTarget(
	resources kubernetesResourceClient,
	pod *corev1.Pod,
) kubernetesDeletionTarget {
	if pod == nil {
		return kubernetesDeletionTarget{kind: "Pod"}
	}
	return newKubernetesDeletionTarget("Pod", pod,
		func(ctx context.Context) (metav1.Object, error) {
			return resources.GetPod(ctx, pod.Namespace, pod.Name)
		},
		func(ctx context.Context, uid types.UID, rv string) error {
			return resources.DeletePod(ctx, pod.Namespace, pod.Name, uid, rv)
		},
		func(ctx context.Context, uid types.UID) error {
			return waitForKubernetesPodDeletion(ctx, resources, pod.Namespace, pod.Name, uid)
		},
	)
}

func kubernetesPVCDeletionTarget(
	resources kubernetesResourceClient,
	pvc *corev1.PersistentVolumeClaim,
) kubernetesDeletionTarget {
	if pvc == nil {
		return kubernetesDeletionTarget{kind: "PVC"}
	}
	return newKubernetesDeletionTarget("PVC", pvc,
		func(ctx context.Context) (metav1.Object, error) {
			return resources.GetPersistentVolumeClaim(ctx, pvc.Namespace, pvc.Name)
		},
		func(ctx context.Context, uid types.UID, rv string) error {
			return resources.DeletePersistentVolumeClaim(ctx, pvc.Namespace, pvc.Name, uid, rv)
		},
		func(ctx context.Context, uid types.UID) error {
			return waitForKubernetesPVCDeletion(ctx, resources, pvc.Namespace, pvc.Name, uid)
		},
	)
}

func newKubernetesDeletionTarget(
	kind string,
	recorded metav1.Object,
	get func(context.Context) (metav1.Object, error),
	deleteResource func(context.Context, types.UID, string) error,
	wait func(context.Context, types.UID) error,
) kubernetesDeletionTarget {
	return kubernetesDeletionTarget{
		kind: kind, namespace: recorded.GetNamespace(), name: recorded.GetName(), uid: recorded.GetUID(),
		get: func(ctx context.Context) (kubernetesCurrentResource, error) {
			current, err := get(ctx)
			if current == nil {
				return kubernetesCurrentResource{}, err
			}
			return kubernetesCurrentResource{
				namespace: current.GetNamespace(), name: current.GetName(), uid: current.GetUID(),
				resourceVersion: current.GetResourceVersion(),
			}, err
		},
		delete: deleteResource,
		wait:   wait,
	}
}

func rollbackCreatedKubernetesResource(ctx context.Context, target kubernetesDeletionTarget) error {
	if target.namespace == "" || target.name == "" || target.uid == "" || target.get == nil {
		return fmt.Errorf(
			"kubernetes lifecycle: cannot roll back %s with incomplete created identity", target.kind,
		)
	}
	current, err := target.get(ctx)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("kubernetes lifecycle: inspect created %s for rollback: %w", target.kind, err)
	}
	if current.namespace != target.namespace || current.name != target.name {
		return fmt.Errorf(
			"kubernetes lifecycle: cannot roll back %s with inconsistent current identity", target.kind,
		)
	}
	if current.uid != target.uid {
		return nil
	}
	if err := target.delete(ctx, target.uid, current.resourceVersion); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("kubernetes lifecycle: roll back created %s: %w", target.kind, err)
	}
	return target.wait(ctx, target.uid)
}

func waitForKubernetesPodDeletion(
	ctx context.Context,
	resources kubernetesResourceClient,
	namespace, name string,
	uid types.UID,
) error {
	return waitForKubernetesResourceDeletion(ctx, "Pod", uid, func(ctx context.Context) (types.UID, error) {
		pod, err := resources.GetPod(ctx, namespace, name)
		if err != nil {
			return "", err
		}
		if pod == nil {
			return "", errors.New("kubernetes API returned an empty Pod")
		}
		return pod.UID, nil
	})
}

func waitForKubernetesPVCDeletion(
	ctx context.Context,
	resources kubernetesResourceClient,
	namespace, name string,
	uid types.UID,
) error {
	return waitForKubernetesResourceDeletion(ctx, "PVC", uid, func(ctx context.Context) (types.UID, error) {
		pvc, err := resources.GetPersistentVolumeClaim(ctx, namespace, name)
		if err != nil {
			return "", err
		}
		if pvc == nil {
			return "", errors.New("kubernetes API returned an empty PVC")
		}
		return pvc.UID, nil
	})
}

func waitForKubernetesResourceDeletion(
	ctx context.Context,
	kind string,
	recordedUID types.UID,
	getUID func(context.Context) (types.UID, error),
) error {
	ticker := time.NewTicker(kubernetesDeletionPollInterval)
	defer ticker.Stop()
	for {
		currentUID, err := getUID(ctx)
		if apierrors.IsNotFound(err) || (err == nil && currentUID != recordedUID) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("kubernetes lifecycle: wait for %s deletion: %w", kind, err)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("kubernetes lifecycle: wait for %s deletion: %w", kind, ctx.Err())
		case <-ticker.C:
		}
	}
}

func findKubernetesContainer(pod *corev1.Pod, name string) *corev1.Container {
	for index := range pod.Spec.Containers {
		if pod.Spec.Containers[index].Name == name {
			return &pod.Spec.Containers[index]
		}
	}
	return nil
}
