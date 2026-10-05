import { getLocalStorage, setLocalStorage } from "@/lib/local-storage";
import {
  normalizeTaskLaunchRecoveryActions,
  type TaskLaunchRecoveryAction,
} from "@/lib/types/task-launch-error";
import type { AgentErrorCause } from "@/lib/types/task-status-summary";

export type LastAgentError = {
  message: string;
  scope?: "session" | "task";
  occurredAt?: string;
  agentExecutionId?: string;
  executionId?: string;
  phase?: string;
  attemptId?: string;
  causes?: AgentErrorCause[];
  /** Adapter-validated provider remediation URL; never derived from prose. */
  remediationUrl?: string;
  code?: string;
  details?: string;
  startupReason?: string;
  startupAttempts?: number;
  startupNpmCode?: string;
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
  return readLastAgentErrorValue(metadata, false);
}

/** Reads the durable error breadcrumb even after recovery retired its controls. */
export function readLastAgentErrorIncludingDismissed(
  metadata: Record<string, unknown> | null | undefined,
) {
  return readLastAgentErrorValue(metadata, true);
}

function readLastAgentErrorValue(
  metadata: Record<string, unknown> | null | undefined,
  includeDismissed: boolean,
): LastAgentError | null {
  if (!metadata) return null;
  const raw = metadata.last_agent_error;
  if (!raw || typeof raw !== "object") return null;
  const record = raw as Record<string, unknown>;
  const message = typeof record.message === "string" ? record.message : "";
  if (!message) return null;
  const dismissedAt = readFirstOptionalString(record, ["dismissed_at", "dismissedAt"]);
  if (dismissedAt && !includeDismissed) return null;
  return {
    message,
    ...readOptionalAgentErrorFields(record),
    ...(dismissedAt ? { dismissedAt } : {}),
  };
}

function readOptionalAgentErrorFields(
  record: Record<string, unknown>,
): Omit<LastAgentError, "message"> {
  return {
    ...readOptionalAgentErrorIdentity(record),
    ...readOptionalAgentErrorRecovery(record),
    ...readStructuredFailureMetadata(record),
  };
}

function readOptionalAgentErrorIdentity(
  record: Record<string, unknown>,
): Pick<
  LastAgentError,
  "occurredAt" | "scope" | "agentExecutionId" | "executionId" | "phase" | "attemptId"
> {
  const result: Pick<
    LastAgentError,
    "occurredAt" | "scope" | "agentExecutionId" | "executionId" | "phase" | "attemptId"
  > = {};
  const occurredAt = readFirstOptionalString(record, ["occurred_at", "occurredAt"]);
  const scope = readFirstOptionalString(record, ["scope"]);
  const agentExecutionId = readFirstOptionalString(record, [
    "agent_execution_id",
    "agentExecutionId",
  ]);
  const executionId = readFirstOptionalString(record, ["execution_id", "executionId"]);
  const phase = readFirstOptionalString(record, ["phase"]);
  const attemptId = readFirstOptionalString(record, ["attempt_id", "attemptId"]);
  if (occurredAt) result.occurredAt = occurredAt;
  if (scope === "session" || scope === "task") result.scope = scope;
  if (agentExecutionId) result.agentExecutionId = agentExecutionId;
  if (executionId) result.executionId = executionId;
  if (phase) result.phase = phase;
  if (attemptId) result.attemptId = attemptId;
  return result;
}

function readOptionalAgentErrorRecovery(
  record: Record<string, unknown>,
): Pick<
  LastAgentError,
  "causes" | "remediationUrl" | "recoveryActions" | "taskRepositoryId" | "stamp"
> {
  const result: Pick<
    LastAgentError,
    "causes" | "remediationUrl" | "recoveryActions" | "taskRepositoryId" | "stamp"
  > = {};
  const causes = readAgentErrorCauses(record.causes);
  const remediationUrl = readFirstOptionalString(record, ["remediation_url", "remediationUrl"]);
  const recoveryActions = normalizeTaskLaunchRecoveryActions(
    record.recovery_actions ?? record.recoveryActions,
  );
  const taskRepositoryId = readFirstOptionalString(record, [
    "task_repository_id",
    "taskRepositoryId",
  ]);
  const stamp = readFirstOptionalString(record, ["stamp"]);
  if (causes.length > 0) result.causes = causes;
  if (remediationUrl) result.remediationUrl = remediationUrl;
  if (recoveryActions.length > 0) result.recoveryActions = recoveryActions;
  if (taskRepositoryId) result.taskRepositoryId = taskRepositoryId;
  if (stamp) result.stamp = stamp;
  return result;
}

function readAgentErrorCauses(value: unknown): AgentErrorCause[] {
  return normalizeAgentErrorCauses(value);
}

export function normalizeAgentErrorCauses(value: unknown): AgentErrorCause[] {
  if (!Array.isArray(value)) return [];
  return value
    .filter((cause): cause is Record<string, unknown> =>
      Boolean(cause && typeof cause === "object" && !Array.isArray(cause)),
    )
    .slice(0, 2)
    .map(readAgentErrorCause)
    .filter((cause) => Boolean(cause.operation || cause.code || cause.detail));
}

function readAgentErrorCause(record: Record<string, unknown>): AgentErrorCause {
  const cause: AgentErrorCause = {
    ...(boundedString(record.operation, 64)
      ? { operation: boundedString(record.operation, 64) }
      : {}),
    ...(boundedString(record.code, 64) ? { code: boundedString(record.code, 64) } : {}),
    ...(boundedString(record.detail, 1024) ? { detail: boundedString(record.detail, 1024) } : {}),
  };
  const reason = typeof record.reason === "string" ? record.reason : "";
  if (!isKnownSelectionReason(cause.code, reason)) return cause;
  cause.reason = reason;
  if (cause.code === "model_unavailable" || cause.code === "model_selection_failed") {
    const requested = safeAgentErrorSelector(record.requested_model);
    const effective = safeAgentErrorSelector(record.effective_model);
    const attempted = safeAgentErrorSelector(record.attempted_model);
    if (requested) cause.requested_model = requested;
    if (effective) cause.effective_model = effective;
    if (attempted) cause.attempted_model = attempted;
  } else {
    const requested = safeAgentErrorSelector(record.requested_mode);
    const effective = safeAgentErrorSelector(record.effective_mode);
    if (requested) cause.requested_mode = requested;
    if (effective) cause.effective_mode = effective;
  }
  if (typeof record.prompt_not_sent === "boolean") {
    cause.prompt_not_sent = record.prompt_not_sent;
  }
  return cause;
}

function boundedString(value: unknown, maxLength: number): string | undefined {
  return typeof value === "string" && value.length > 0 ? value.slice(0, maxLength) : undefined;
}

function isKnownSelectionReason(code: string | undefined, reason: string): boolean {
  switch (code) {
    case "model_unavailable":
      return reason === "requested_not_advertised";
    case "model_selection_failed":
      return [
        "catalog_empty",
        "selection_unsupported",
        "application_failed",
        "selection_missing",
      ].includes(reason);
    case "permission_mode_failed":
      return reason === "client_unavailable" || reason === "application_failed";
    case "permission_mode_unconfirmed":
      return reason === "confirmation_missing";
    case "permission_mode_mismatch":
      return reason === "effective_mismatch";
    default:
      return false;
  }
}

function safeAgentErrorSelector(value: unknown): string | undefined {
  if (typeof value !== "string" || value.length === 0 || value.length > 256) return undefined;
  if (new TextEncoder().encode(value).length > 256 || value.trim() !== value) return undefined;
  const lower = value.toLowerCase();
  if (
    /[\\=@\s\u0000-\u001f\u007f]/u.test(value) ||
    value.includes("://") ||
    value.startsWith("/") ||
    value.startsWith("~") ||
    /^(?:[a-z]:[\\/])/iu.test(value) ||
    ["token", "secret", "password", "api_key", "apikey", "bearer "].some((part) =>
      lower.includes(part),
    ) ||
    /^(?:sk-|ghp_|github_pat_|kandev_pat_)/iu.test(value)
  ) {
    return undefined;
  }
  const segments = value.split("/");
  if (
    segments.length > 3 ||
    segments.some(
      (segment) =>
        segment === "." ||
        segment === ".." ||
        segment.startsWith(".") ||
        (segment.length >= 32 && /^[A-Za-z0-9+/=_-]+$/u.test(segment)),
    )
  ) {
    return undefined;
  }
  return value;
}

function readStructuredFailureMetadata(record: Record<string, unknown>) {
  const startupAttempts = record.startup_attempts ?? record.startupAttempts;
  return {
    code: readFirstOptionalString(record, ["code", "failure_code", "failureCode"]),
    details: readFirstOptionalString(record, [
      "details",
      "failure_details",
      "failureDetails",
      "error_output",
    ]),
    startupReason: readFirstOptionalString(record, ["startup_reason", "startupReason"]),
    ...(typeof startupAttempts === "number" &&
    Number.isInteger(startupAttempts) &&
    startupAttempts >= 0
      ? { startupAttempts }
      : {}),
    startupNpmCode: readFirstOptionalString(record, ["startup_npm_code", "startupNpmCode"]),
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
