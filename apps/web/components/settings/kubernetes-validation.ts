import {
  getKubernetesProfileValidationError,
  type KubernetesExecutorForm,
  type KubernetesProfileConfigForm,
  type KubernetesProfileValidationError,
} from "./kubernetes-config";

export function kubernetesExecutorInvalidReason(
  form: KubernetesExecutorForm,
  canManage: boolean,
  t: (key: string) => string,
): string | undefined {
  if (!canManage) return t("executors:kubernetesAdminSaveOnly");
  if (!form.name.trim()) return t("executors:kubernetesExecutorNameRequired");
  if (!form.namespace.trim()) return t("executors:kubernetesNamespaceRequired");
  if (form.authMode === "kubeconfig" && !form.kubeconfigPath.trim()) {
    return t("executors:kubernetesKubeconfigPathRequired");
  }
  const timeout = Number(form.requestTimeoutSeconds);
  return Number.isInteger(timeout) && timeout >= 1 && timeout <= 300
    ? undefined
    : t("executors:kubernetesTimeoutInvalid");
}

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
