export type KubernetesAuthMode = "kubeconfig" | "in_cluster";
export type KubernetesPlatform = "linux/amd64" | "linux/arm64";
export type KubernetesWorkspaceMode = "managed_pvc" | "empty_dir" | "existing_claim";
export type KubernetesAccessMode =
  | "ReadWriteOnce"
  | "ReadOnlyMany"
  | "ReadWriteMany"
  | "ReadWriteOncePod";

export type KubernetesExecutorForm = {
  name: string;
  authMode: KubernetesAuthMode;
  kubeconfigPath: string;
  kubeContext: string;
  namespace: string;
  requestTimeoutSeconds: string;
};

export type KubernetesProfileConfigForm = {
  platform: KubernetesPlatform;
  mainContainer: string;
  podTemplateYaml: string;
  workspaceMode: KubernetesWorkspaceMode;
  workspaceSize: string;
  storageClass: string;
  accessModes: string[];
  claimName: string;
};

export type KubernetesProfileValidationError =
  | "main_container_required"
  | "main_container_invalid"
  | "pod_template_required"
  | "pod_template_too_large"
  | "workspace_size_required"
  | "access_mode_required"
  | "access_mode_invalid"
  | "claim_name_required";

const MAX_POD_TEMPLATE_BYTES = 256 * 1024;
const KUBERNETES_CONTAINER_NAME = /^[a-z0-9](?:[-a-z0-9]{0,61}[a-z0-9])?$/;

// i18n-exempt: container image reference persisted in the Kubernetes PodTemplate.
export const DEFAULT_KUBERNETES_IMAGE = "ghcr.io/kdlbs/kandev:latest";

// i18n-exempt: default PodTemplate is persisted Kubernetes YAML, not user-facing copy.
export const DEFAULT_KUBERNETES_POD_TEMPLATE = `apiVersion: v1
kind: PodTemplate
template:
  spec:
    containers:
      - name: kandev-agent
        image: ${DEFAULT_KUBERNETES_IMAGE}
`;

export function createDefaultKubernetesExecutorForm(): KubernetesExecutorForm {
  return {
    name: "",
    authMode: "kubeconfig",
    kubeconfigPath: "",
    kubeContext: "",
    namespace: "default",
    requestTimeoutSeconds: "30",
  };
}

export function createDefaultKubernetesProfileConfig(): KubernetesProfileConfigForm {
  return {
    platform: "linux/amd64",
    mainContainer: "kandev-agent",
    podTemplateYaml: DEFAULT_KUBERNETES_POD_TEMPLATE,
    workspaceMode: "managed_pvc",
    workspaceSize: "10Gi",
    storageClass: "",
    accessModes: ["ReadWriteOnce"],
    claimName: "",
  };
}

export function parseKubernetesExecutorConfig(
  name: string,
  config?: Record<string, string>,
): KubernetesExecutorForm {
  const defaults = createDefaultKubernetesExecutorForm();
  const values = config ?? {};
  const authMode = values.auth_mode === "in_cluster" ? "in_cluster" : "kubeconfig";
  return {
    name,
    authMode,
    kubeconfigPath: authMode === "kubeconfig" ? (values.kubeconfig_path ?? "").trim() : "",
    kubeContext: authMode === "kubeconfig" ? (values.kube_context ?? "").trim() : "",
    namespace: values.namespace?.trim() || defaults.namespace,
    requestTimeoutSeconds: values.request_timeout_seconds?.trim() || defaults.requestTimeoutSeconds,
  };
}

export function serializeKubernetesExecutorConfig(
  form: KubernetesExecutorForm,
): Record<string, string> {
  const config: Record<string, string> = {
    auth_mode: form.authMode,
    namespace: form.namespace.trim(),
    request_timeout_seconds: form.requestTimeoutSeconds.trim(),
  };
  if (form.authMode === "kubeconfig") {
    config.kubeconfig_path = form.kubeconfigPath.trim();
    if (form.kubeContext.trim()) config.kube_context = form.kubeContext.trim();
  }
  return config;
}

export function parseKubernetesProfileConfig(
  config?: Record<string, string>,
): KubernetesProfileConfigForm {
  const defaults = createDefaultKubernetesProfileConfig();
  const values = config ?? {};
  const workspaceMode = parseWorkspaceMode(configValue(values, "workspace.mode", "workspace_mode"));
  const accessModes = parseAccessModes(
    configValue(values, "workspace.access_modes", "workspace_access_modes"),
  );
  const managedAccessModes = accessModes.length > 0 ? accessModes : defaults.accessModes;
  return {
    platform: values.platform === "linux/arm64" ? "linux/arm64" : "linux/amd64",
    mainContainer: values.main_container?.trim() || defaults.mainContainer,
    podTemplateYaml: values.pod_template_yaml ?? defaults.podTemplateYaml,
    workspaceMode,
    workspaceSize:
      workspaceMode === "managed_pvc"
        ? configValue(values, "workspace.size", "workspace_size").trim() || defaults.workspaceSize
        : "",
    storageClass:
      workspaceMode === "managed_pvc"
        ? configValue(values, "workspace.storage_class", "workspace_storage_class").trim()
        : "",
    accessModes: workspaceMode === "managed_pvc" ? managedAccessModes : [],
    claimName:
      workspaceMode === "existing_claim"
        ? configValue(values, "workspace.claim_name", "workspace_claim_name").trim()
        : "",
  };
}

export function serializeKubernetesProfileConfig(
  form: KubernetesProfileConfigForm,
): Record<string, string> {
  const config: Record<string, string> = {
    platform: form.platform,
    main_container: form.mainContainer.trim(),
    pod_template_yaml: form.podTemplateYaml,
    "workspace.mode": form.workspaceMode,
  };
  if (form.workspaceMode === "managed_pvc") {
    config["workspace.size"] = form.workspaceSize.trim();
    if (form.storageClass.trim()) config["workspace.storage_class"] = form.storageClass.trim();
    const accessModes = form.accessModes.length > 0 ? form.accessModes : ["ReadWriteOnce"];
    config["workspace.access_modes"] = JSON.stringify(accessModes);
  }
  if (form.workspaceMode === "existing_claim") {
    config["workspace.claim_name"] = form.claimName.trim();
  }
  return config;
}

export function replaceKubernetesProfileConfig(
  baseConfig: Record<string, string>,
  form: KubernetesProfileConfigForm,
): Record<string, string> {
  const config = { ...baseConfig };
  for (const key of KUBERNETES_PROFILE_CONFIG_KEYS) delete config[key];
  return { ...config, ...serializeKubernetesProfileConfig(form) };
}

export function isKubernetesExecutorDirty(
  form: KubernetesExecutorForm,
  baseline: KubernetesExecutorForm,
): boolean {
  return executorRevision(form) !== executorRevision(baseline);
}

export function isKubernetesProfileDirty(
  form: KubernetesProfileConfigForm,
  baseline: KubernetesProfileConfigForm,
): boolean {
  return (
    JSON.stringify(serializeKubernetesProfileConfig(form)) !==
    JSON.stringify(serializeKubernetesProfileConfig(baseline))
  );
}

export function getKubernetesProfileValidationError(
  form: KubernetesProfileConfigForm,
): KubernetesProfileValidationError | null {
  if (!form.mainContainer.trim()) return "main_container_required";
  if (!KUBERNETES_CONTAINER_NAME.test(form.mainContainer.trim())) {
    return "main_container_invalid";
  }
  if (!form.podTemplateYaml.trim()) return "pod_template_required";
  if (new TextEncoder().encode(form.podTemplateYaml).byteLength > MAX_POD_TEMPLATE_BYTES) {
    return "pod_template_too_large";
  }
  if (form.workspaceMode === "managed_pvc" && !form.workspaceSize.trim()) {
    return "workspace_size_required";
  }
  if (form.workspaceMode === "managed_pvc" && form.accessModes.length === 0) {
    return "access_mode_required";
  }
  if (
    form.workspaceMode === "managed_pvc" &&
    form.accessModes.some((mode) => !ACCESS_MODES.has(mode as KubernetesAccessMode))
  ) {
    return "access_mode_invalid";
  }
  if (form.workspaceMode === "existing_claim" && !form.claimName.trim()) {
    return "claim_name_required";
  }
  return null;
}

const ACCESS_MODES = new Set<KubernetesAccessMode>([
  "ReadWriteOnce",
  "ReadOnlyMany",
  "ReadWriteMany",
  "ReadWriteOncePod",
]);

const KUBERNETES_PROFILE_CONFIG_KEYS = [
  "platform",
  "main_container",
  "pod_template_yaml",
  "workspace.mode",
  "workspace.size",
  "workspace.storage_class",
  "workspace.access_modes",
  "workspace.claim_name",
  "workspace_mode",
  "workspace_size",
  "workspace_storage_class",
  "workspace_access_modes",
  "workspace_claim_name",
];

function parseWorkspaceMode(raw: string): KubernetesWorkspaceMode {
  if (raw === "empty_dir" || raw === "existing_claim") return raw;
  return "managed_pvc";
}

function parseAccessModes(raw: string): string[] {
  if (!raw.trim()) return [];
  try {
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed.filter((value): value is string => typeof value === "string");
  } catch {
    return [];
  }
}

function configValue(config: Record<string, string>, preferred: string, fallback: string): string {
  return config[preferred] ?? config[fallback] ?? "";
}

function executorRevision(form: KubernetesExecutorForm): string {
  return JSON.stringify({
    name: form.name.trim(),
    config: serializeKubernetesExecutorConfig(form),
  });
}
