import type { TFunction } from "i18next";
import { formatDateTime } from "@/lib/i18n/formats";
import type { KubernetesSession } from "@/lib/types/http-kubernetes";

const POD_STATUS_LABEL_KEYS: Record<string, string> = {
  running: "executors:kubernetesStatusRunning",
  waiting: "executors:kubernetesStatusWaiting",
  terminated: "executors:kubernetesStatusTerminated",
  unknown: "executors:kubernetesStatusUnknown",
  pending: "executors:kubernetesStatusPending",
  succeeded: "executors:kubernetesStatusSucceeded",
  failed: "executors:kubernetesStatusFailed",
};

const RETENTION_LABEL_KEYS: Record<string, string> = {
  active: "executors:kubernetesRetentionActive",
  retained: "executors:kubernetesRetentionRetained",
  terminating: "executors:kubernetesRetentionTerminating",
  terminal: "executors:kubernetesRetentionTerminal",
  missing: "executors:kubernetesRetentionMissing",
  unknown: "executors:kubernetesRetentionUnknown",
};

const SESSION_STATE_LABEL_KEYS: Record<string, string> = {
  created: "executors:kubernetesSessionCreated",
  starting: "executors:kubernetesSessionStarting",
  running: "executors:kubernetesSessionRunning",
  idle: "executors:kubernetesSessionIdle",
  waiting_for_input: "executors:kubernetesSessionWaitingForInput",
  completed: "executors:kubernetesSessionCompleted",
  failed: "executors:kubernetesSessionFailed",
  cancelled: "executors:kubernetesSessionCancelled",
  unknown: "executors:kubernetesStatusUnknown",
};

export function workspaceLabel(value: string | undefined, t: TFunction): string {
  const keys: Record<string, string> = {
    managed_pvc: "executors:kubernetesWorkspaceManagedPvc",
    empty_dir: "executors:kubernetesWorkspaceEmptyDir",
    existing_claim: "executors:kubernetesWorkspaceExistingClaim",
  };
  return value && keys[value] ? t(keys[value]) : value || "-";
}

export function podStatusLabel(value: string, t: TFunction): string {
  const key = POD_STATUS_LABEL_KEYS[value.toLowerCase()];
  return key ? t(key) : t("executors:kubernetesStatusUnknown");
}

export function retentionLabel(value: string, t: TFunction): string {
  return t(RETENTION_LABEL_KEYS[value] ?? RETENTION_LABEL_KEYS.unknown);
}

export function sessionStateLabel(value: string | undefined, t: TFunction): string {
  const state = normalizedState(value);
  return t(SESSION_STATE_LABEL_KEYS[state] ?? SESSION_STATE_LABEL_KEYS.unknown);
}

export function normalizedState(value?: string): string {
  return value?.trim().toLowerCase() || "unknown";
}

export function podStatusValue(session: KubernetesSession): string {
  if (
    session.container_state?.toLowerCase() === "terminated" &&
    session.pod_phase?.toLowerCase() === "succeeded"
  ) {
    return session.pod_phase;
  }
  return session.container_state || session.pod_phase || "unknown";
}

export function translateTaskLink(session: KubernetesSession, t: TFunction): string {
  return t("executors:kubernetesOpenTask", {
    task: session.task_id,
    session: session.session_id,
  });
}

export function taskHref(taskId: string): string {
  return `/t/${encodeURIComponent(taskId)}`;
}

export function shortId(value: string): string {
  return value.slice(0, 8);
}

export function formatCreatedAt(value?: string): string {
  if (!value) return "-";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : formatDateTime(date);
}
