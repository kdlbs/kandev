import type { ContainerLiveStatus, TaskEnvironment } from "@/lib/api/domains/task-environment-api";

export type StatusTone = "running" | "stopped" | "warn" | "error" | "neutral";

export type ExecutorEnvironmentStatus = {
  label: string;
  tone: StatusTone;
};

export type EnvironmentStatusSnapshot = ExecutorEnvironmentStatus & {
  key: string;
};

export function getEnvironmentStatusSnapshot(
  env: TaskEnvironment | null,
  container: ContainerLiveStatus | null,
): EnvironmentStatusSnapshot {
  if (!env) {
    return { key: "none", label: "not created", tone: "neutral" };
  }
  const status = resolveExecutorEnvironmentStatus(env, container);
  return { ...status, key: `${status.tone}:${status.label}` };
}

export function resolveExecutorEnvironmentStatus(
  env: TaskEnvironment,
  container: ContainerLiveStatus | null,
): ExecutorEnvironmentStatus {
  if (container) {
    return resolveContainerStatus(container);
  }
  return resolveEnvStatus(env.status);
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
