import { getLocalStorage, setLocalStorage } from "@/lib/local-storage";
import {
  normalizeTaskLaunchRecoveryActions,
  type TaskLaunchRecoveryAction,
} from "@/lib/types/task-launch-error";

export type LastAgentError = {
  message: string;
  occurredAt?: string;
  agentExecutionId?: string;
  /** Adapter-validated provider remediation URL; never derived from prose. */
  remediationUrl?: string;
  code?: string;
  details?: string;
  recoveryActions?: TaskLaunchRecoveryAction[];
  taskRepositoryId?: string;
  stamp?: string;
  dismissedAt?: string;
};

// --- Agent error visibility state (localStorage, global) ---
//
// `dismissedAgentErrors` tracks explicit chat-banner dismissals and hides both
// the banner and task-row badge. `acknowledgedAgentErrors` tracks sidebar-only
// stale-error acknowledgements and hides task-row badges without hiding chat.
// Bounded growth: one entry per session that ever had an error.

const DISMISSED_AGENT_ERRORS_KEY = "kandev.dismissedAgentErrors";
const ACKNOWLEDGED_AGENT_ERRORS_KEY = "kandev.acknowledgedAgentErrors";

export function getStoredDismissedAgentErrors(): Record<string, string> {
  return getLocalStorage<Record<string, string>>(DISMISSED_AGENT_ERRORS_KEY, {});
}

export function getStoredAcknowledgedAgentErrors(): Record<string, string> {
  return getLocalStorage<Record<string, string>>(ACKNOWLEDGED_AGENT_ERRORS_KEY, {});
}

/**
 * Merge `map` into whatever is currently in localStorage so concurrent writes
 * from other tabs (or older versions of this tab's state) are not clobbered.
 * Entries in `map` win over the on-disk values for the same session.
 */
export function setStoredDismissedAgentErrors(map: Record<string, string>): void {
  const current = getStoredDismissedAgentErrors();
  setLocalStorage(DISMISSED_AGENT_ERRORS_KEY, { ...current, ...map });
}

export function setStoredAcknowledgedAgentErrors(map: Record<string, string>): void {
  const current = getStoredAcknowledgedAgentErrors();
  setLocalStorage(ACKNOWLEDGED_AGENT_ERRORS_KEY, { ...current, ...map });
}

export function readLastAgentError(metadata: Record<string, unknown> | null | undefined) {
  if (!metadata) return null;
  const raw = metadata.last_agent_error;
  if (!raw || typeof raw !== "object") return null;
  const record = raw as Record<string, unknown>;
  const message = typeof record.message === "string" ? record.message : "";
  if (!message) return null;
  const dismissedAt = readFirstOptionalString(record, ["dismissed_at", "dismissedAt"]);
  if (dismissedAt) return null;
  return { message, ...readOptionalAgentErrorFields(record) };
}

function readOptionalAgentErrorFields(
  record: Record<string, unknown>,
): Omit<LastAgentError, "message"> {
  const result: Omit<LastAgentError, "message"> = {};
  const occurredAt = readFirstOptionalString(record, ["occurred_at", "occurredAt"]);
  const agentExecutionId = readFirstOptionalString(record, [
    "agent_execution_id",
    "agentExecutionId",
  ]);
  const remediationUrl = readFirstOptionalString(record, ["remediation_url", "remediationUrl"]);
  const recoveryActions = normalizeTaskLaunchRecoveryActions(
    record.recovery_actions ?? record.recoveryActions,
  );
  const taskRepositoryId = readFirstOptionalString(record, [
    "task_repository_id",
    "taskRepositoryId",
  ]);
  const stamp = readFirstOptionalString(record, ["stamp"]);
  const structured = readStructuredFailureMetadata(record);
  if (occurredAt) result.occurredAt = occurredAt;
  if (agentExecutionId) result.agentExecutionId = agentExecutionId;
  if (remediationUrl) result.remediationUrl = remediationUrl;
  if (recoveryActions.length > 0) result.recoveryActions = recoveryActions;
  if (taskRepositoryId) result.taskRepositoryId = taskRepositoryId;
  if (stamp) result.stamp = stamp;
  if (structured.code) result.code = structured.code;
  if (structured.details) result.details = structured.details;
  return result;
}

function readStructuredFailureMetadata(record: Record<string, unknown>) {
  return {
    code: readFirstOptionalString(record, ["code", "failure_code", "failureCode"]),
    details: readFirstOptionalString(record, [
      "details",
      "failure_details",
      "failureDetails",
      "error_output",
    ]),
  };
}

function readFirstOptionalString(record: Record<string, unknown>, keys: string[]) {
  for (const key of keys) {
    const value = readOptionalString(record[key]);
    if (value) return value;
  }
  return undefined;
}

/**
 * Stable identifier for a specific error event. Two errors share a stamp iff
 * they have the same occurredAt timestamp and message. Used to decide whether
 * a prior dismissal still applies after a fresh failure replaces the
 * `last_agent_error` metadata.
 */
export function lastAgentErrorStamp(error: LastAgentError) {
  return error.stamp ?? `${error.occurredAt ?? ""}:${error.message}`;
}

function readOptionalString(value: unknown) {
  return typeof value === "string" && value !== "" ? value : undefined;
}
