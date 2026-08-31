import {
  getKubernetesProfileValidationError,
  type KubernetesProfileConfigForm,
  type KubernetesProfileValidationError,
} from "./kubernetes-config";

const PROFILE_ERROR_KEYS: Record<KubernetesProfileValidationError, string> = {
  main_container_required: "executors:kubernetesMainContainerRequired",
  pod_template_required: "executors:kubernetesPodTemplateRequired",
  pod_template_too_large: "executors:kubernetesPodTemplateTooLarge",
  workspace_size_required: "executors:kubernetesWorkspaceSizeRequired",
  access_mode_required: "executors:kubernetesAccessModeRequired",
  claim_name_required: "executors:kubernetesClaimNameRequired",
};

export function kubernetesProfileInvalidReason(
  form: KubernetesProfileConfigForm,
  t: (key: string) => string,
): string | undefined {
  const error = getKubernetesProfileValidationError(form);
  return error ? t(PROFILE_ERROR_KEYS[error]) : undefined;
}

export function getKubernetesCreateContributorState(
  canManage: boolean,
  invalidReason?: string,
): { isDirty: boolean; canSave: boolean } {
  return {
    isDirty: canManage,
    canSave: canManage && !invalidReason,
  };
}
