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
  if (form.authMode === "kubeconfig" && !form.kubeconfigPath.trim().startsWith("/")) {
    return t("executors:kubernetesKubeconfigPathRequired");
  }
  const timeout = Number(form.requestTimeoutSeconds);
  return /^(?:[1-9]|[1-9][0-9]|[12][0-9]{2}|300)$/.test(form.requestTimeoutSeconds) &&
    timeout >= 1 &&
    timeout <= 300
    ? undefined
    : t("executors:kubernetesTimeoutInvalid");
}

const PROFILE_ERROR_KEYS: Record<KubernetesProfileValidationError, string> = {
  main_container_required: "executors:kubernetesMainContainerRequired",
  main_container_invalid: "executors:kubernetesMainContainerInvalid",
  pod_template_required: "executors:kubernetesPodTemplateRequired",
  pod_template_too_large: "executors:kubernetesPodTemplateTooLarge",
  workspace_size_required: "executors:kubernetesWorkspaceSizeRequired",
  access_mode_required: "executors:kubernetesAccessModeRequired",
  access_mode_invalid: "executors:kubernetesAccessModeInvalid",
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
