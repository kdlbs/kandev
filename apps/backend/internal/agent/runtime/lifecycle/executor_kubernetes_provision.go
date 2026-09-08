package lifecycle

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
)

type kubernetesWorkspaceProvision struct {
	claimName    string
	claim        *corev1.PersistentVolumeClaim
	createdClaim *corev1.PersistentVolumeClaim
}

func provisionKubernetesWorkspace(
	ctx context.Context,
	resources kubernetesResourceClient,
	executorConfig kubeexecutor.ExecutorConfig,
	profile kubeexecutor.ProfileConfig,
	identity kubeexecutor.ResourceIdentity,
	managedClaimName string,
	onManagedCreated func(*corev1.PersistentVolumeClaim) error,
) (kubernetesWorkspaceProvision, error) {
	switch profile.Workspace.Mode {
	case kubeexecutor.WorkspaceModeManagedPVC:
		return createManagedKubernetesWorkspacePVC(
			ctx, resources, executorConfig.Namespace, profile.Workspace, identity, managedClaimName,
			onManagedCreated,
		)
	case kubeexecutor.WorkspaceModeExistingClaim:
		return getExistingKubernetesWorkspacePVC(
			ctx, resources, executorConfig.Namespace, profile.Workspace.ClaimName,
		)
	case kubeexecutor.WorkspaceModeEmptyDir:
		return kubernetesWorkspaceProvision{}, nil
	default:
		return kubernetesWorkspaceProvision{}, errors.New("kubernetes lifecycle: unsupported workspace mode")
	}
}

func createManagedKubernetesWorkspacePVC(
	ctx context.Context,
	resources kubernetesResourceClient,
	namespace string,
	workspace kubeexecutor.WorkspaceConfig,
	identity kubeexecutor.ResourceIdentity,
	claimName string,
	onCreated func(*corev1.PersistentVolumeClaim) error,
) (kubernetesWorkspaceProvision, error) {
	desired, err := kubeexecutor.BuildPersistentVolumeClaim(workspace, kubeexecutor.PVCOptions{
		Name: claimName, Namespace: namespace, Identity: identity,
	})
	if err != nil {
		return kubernetesWorkspaceProvision{}, err
	}
	if err := kubeexecutor.StampCreateNonce(desired); err != nil {
		return kubernetesWorkspaceProvision{}, fmt.Errorf(
			"kubernetes lifecycle: generate workspace PVC create nonce: %w", err,
		)
	}
	created, err := resources.CreatePersistentVolumeClaim(ctx, namespace, desired)
	if err != nil {
		createErr := fmt.Errorf("kubernetes lifecycle: create workspace PVC: %w", err)
		if !kubeexecutor.IsAmbiguousCreateError(err) {
			return kubernetesWorkspaceProvision{claimName: claimName}, createErr
		}
		reconcileCtx, cancel := kubernetesAmbiguousCreateContext(ctx)
		created, err = resources.GetPersistentVolumeClaim(reconcileCtx, namespace, desired.Name)
		cancel()
		if apierrors.IsNotFound(err) {
			return kubernetesWorkspaceProvision{claimName: claimName}, createErr
		}
		if err != nil {
			return kubernetesWorkspaceProvision{claimName: claimName}, errors.Join(
				createErr, fmt.Errorf("kubernetes lifecycle: reconcile ambiguous workspace PVC create: %w", err),
			)
		}
		if verifyErr := verifyOwnedPVC(created, desired, identity); verifyErr != nil {
			return kubernetesWorkspaceProvision{claimName: claimName}, errors.Join(
				createErr, fmt.Errorf("kubernetes lifecycle: reconcile ambiguous workspace PVC create: %w", verifyErr),
			)
		}
	}
	provision := kubernetesWorkspaceProvision{claimName: claimName, claim: created, createdClaim: created}
	if onCreated != nil {
		if err := onCreated(created); err != nil {
			return provision, err
		}
	}
	if err := verifyOwnedPVC(created, desired, identity); err != nil {
		return provision, err
	}
	return provision, nil
}

func getExistingKubernetesWorkspacePVC(
	ctx context.Context,
	resources kubernetesResourceClient,
	namespace, claimName string,
) (kubernetesWorkspaceProvision, error) {
	claim, err := resources.GetPersistentVolumeClaim(ctx, namespace, claimName)
	provision := kubernetesWorkspaceProvision{claimName: claimName, claim: claim}
	if err != nil {
		return provision, fmt.Errorf("kubernetes lifecycle: verify existing workspace PVC: %w", err)
	}
	if claim == nil || claim.UID == "" || claim.Name != claimName || claim.Namespace != namespace {
		return provision, errors.New("kubernetes lifecycle: existing workspace PVC identity is incomplete")
	}
	return provision, nil
}

func composeKubernetesLifecyclePod(
	profile kubeexecutor.ProfileConfig,
	identity kubeexecutor.ResourceIdentity,
	name, namespace, claimName string,
) (*corev1.Pod, error) {
	template, err := kubeexecutor.ParsePodTemplate(profile.PodTemplateYAML)
	if err != nil {
		return nil, err
	}
	pod, _, err := kubeexecutor.ComposePod(template, profile, kubeexecutor.PodOptions{
		Name: name, Namespace: namespace, Identity: identity,
		Command: []string{"sh", "-c"}, Args: []string{kubernetesBootstrapCommand()},
		WorkingDir: kubernetesWorkspacePath, AgentctlPort: kubeexecutor.DefaultAgentctlPort,
		ManagedPVCName: claimName,
	})
	return pod, err
}

func createAndWaitKubernetesPod(
	ctx context.Context,
	resources kubernetesResourceClient,
	desired *corev1.Pod,
	identity kubeexecutor.ResourceIdentity,
	mainContainer, description string,
	onCheckpoint func(string, *corev1.Pod) error,
) (*corev1.Pod, *corev1.Pod, error) {
	if err := kubeexecutor.StampCreateNonce(desired); err != nil {
		return nil, nil, fmt.Errorf("kubernetes lifecycle: generate %s create nonce: %w", description, err)
	}
	created, err := resources.CreatePod(ctx, desired.Namespace, desired)
	if err != nil {
		createErr := fmt.Errorf("kubernetes lifecycle: create %s: %w", description, err)
		if !kubeexecutor.IsAmbiguousCreateError(err) {
			return nil, nil, createErr
		}
		reconcileCtx, cancel := kubernetesAmbiguousCreateContext(ctx)
		created, err = resources.GetPod(reconcileCtx, desired.Namespace, desired.Name)
		cancel()
		if apierrors.IsNotFound(err) {
			return nil, nil, createErr
		}
		if err != nil {
			return nil, nil, errors.Join(
				createErr, fmt.Errorf("kubernetes lifecycle: reconcile ambiguous %s create: %w", description, err),
			)
		}
		if verifyErr := verifyCreatedPod(created, desired, identity, mainContainer); verifyErr != nil {
			return nil, nil, errors.Join(
				createErr, fmt.Errorf("kubernetes lifecycle: reconcile ambiguous %s create: %w", description, verifyErr),
			)
		}
	}
	if onCheckpoint != nil {
		if err := onCheckpoint(KubernetesInventoryStatePodCreated, created); err != nil {
			return created, nil, err
		}
	}
	if err := verifyCreatedPod(created, desired, identity, mainContainer); err != nil {
		return created, nil, err
	}
	if onCheckpoint != nil {
		if err := onCheckpoint(KubernetesInventoryStatePodAdmitted, created); err != nil {
			return created, nil, err
		}
	}
	running, err := resources.WaitForPodRunning(ctx, desired.Namespace, desired.Name, mainContainer)
	if err != nil {
		return created, nil, fmt.Errorf("kubernetes lifecycle: wait for %s: %w", description, err)
	}
	if err := verifyRecordedPod(running, desired.Namespace, desired.Name, created.UID, identity); err != nil {
		return created, nil, err
	}
	return created, running, nil
}
