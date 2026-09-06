import type { ContainerLiveStatus, TaskEnvironment } from "@/lib/api/domains/task-environment-api";
import type { KubernetesSession } from "@/lib/types/http-kubernetes";
import { t } from "@/lib/i18n";

export type StatusTone = "running" | "stopped" | "warn" | "error" | "neutral";

export type ExecutorEnvironmentStatus = {
  label: string;
  tone: StatusTone;
};

export type EnvironmentStatusSnapshot = ExecutorEnvironmentStatus & {
  key: string;
};

export type KubernetesEnvironmentStatus = {
  session: KubernetesSession | null;
  loaded: boolean;
  error: string | null;
};

export function getEnvironmentStatusSnapshot(
  env: TaskEnvironment | null,
  container: ContainerLiveStatus | null,
  kubernetes?: KubernetesEnvironmentStatus,
): EnvironmentStatusSnapshot {
  if (!env) {
    return { key: "none", label: t("task:environmentNotCreated"), tone: "neutral" };
  }
  const status = resolveExecutorEnvironmentStatus(env, container, kubernetes);
  return { ...status, key: `${status.tone}:${status.label}` };
}

export function resolveExecutorEnvironmentStatus(
  env: TaskEnvironment,
  container: ContainerLiveStatus | null,
  kubernetes?: KubernetesEnvironmentStatus,
): ExecutorEnvironmentStatus {
  if (env.executor_type === "k8s" && kubernetes) {
    return resolveKubernetesStatus(kubernetes);
  }
  if (container) {
    return resolveContainerStatus(container);
  }
  return resolveEnvStatus(env.status);
}

function resolveKubernetesStatus(status: KubernetesEnvironmentStatus): ExecutorEnvironmentStatus {
  if (!status.loaded) return { label: t("common:loading"), tone: "neutral" };
  if (status.error) return { label: t("common:error"), tone: "error" };
  if (!status.session) return { label: t("common:unavailable"), tone: "warn" };
  if (status.session.failure_reason) {
    return { label: status.session.failure_reason, tone: "error" };
  }
  switch (status.session.container_state?.toLowerCase()) {
    case "running":
      return { label: t("executors:kubernetesStatusRunning"), tone: "running" };
    case "waiting":
      return { label: t("executors:kubernetesStatusWaiting"), tone: "warn" };
    case "terminated":
      return status.session.pod_phase === "Succeeded"
        ? { label: t("executors:kubernetesStatusSucceeded"), tone: "stopped" }
        : { label: t("executors:kubernetesStatusTerminated"), tone: "error" };
  }
  switch (status.session.pod_phase) {
    case "Running":
      return { label: t("executors:kubernetesStatusRunning"), tone: "running" };
    case "Pending":
      return { label: t("executors:kubernetesStatusPending"), tone: "warn" };
    case "Failed":
      return { label: t("executors:kubernetesStatusFailed"), tone: "error" };
    case "Succeeded":
      return { label: t("executors:kubernetesStatusSucceeded"), tone: "stopped" };
    default:
      return { label: t("executors:kubernetesStatusUnknown"), tone: "neutral" };
  }
}

const CONTAINER_STATUS_TONES: Record<string, StatusTone> = {
  paused: "warn",
  restarting: "warn",
  dead: "error",
};

function resolveContainerStatus(container: ContainerLiveStatus): ExecutorEnvironmentStatus {
  if (container.missing) return { label: "missing", tone: "warn" };
  if (container.state === "running") {
    return { label: container.status || "running", tone: "running" };
  }
  if (container.state === "exited") {
    return {
      label: container.exit_code ? `exited (${container.exit_code})` : "exited",
      tone: container.exit_code ? "error" : "stopped",
    };
  }
  const tone = CONTAINER_STATUS_TONES[container.state] ?? "neutral";
  return { label: container.state || "unknown", tone };
}

const ENV_STATUS_MAP: Record<string, ExecutorEnvironmentStatus> = {
  ready: { label: "ready", tone: "running" },
  creating: { label: "starting", tone: "warn" },
  stopped: { label: "stopped", tone: "stopped" },
  failed: { label: "failed", tone: "error" },
};

function resolveEnvStatus(status: string): ExecutorEnvironmentStatus {
  return ENV_STATUS_MAP[status] ?? { label: status || "unknown", tone: "neutral" };
}
