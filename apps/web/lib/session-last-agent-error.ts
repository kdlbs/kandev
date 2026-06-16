import { getLocalStorage, setLocalStorage } from "@/lib/local-storage";

export type LastAgentError = {
  message: string;
  occurredAt?: string;
  agentExecutionId?: string;
};

// --- Dismissed agent errors (localStorage, global) ---
//
// Tracks the most recent dismissed `last_agent_error` stamp per session so the
// red error icon in the sidebar and the chat banner can share dismissal state
// across components and reloads. Bounded growth: one entry per session that
// ever had an error.

const DISMISSED_AGENT_ERRORS_KEY = "kandev.dismissedAgentErrors";

export function getStoredDismissedAgentErrors(): Record<string, string> {
  return getLocalStorage<Record<string, string>>(DISMISSED_AGENT_ERRORS_KEY, {}) ?? {};
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

export function readLastAgentError(metadata: Record<string, unknown> | null | undefined) {
  if (!metadata) return null;
  const raw = metadata.last_agent_error;
  if (!raw || typeof raw !== "object") return null;
  const record = raw as Record<string, unknown>;
  const message = typeof record.message === "string" ? record.message : "";
  if (!message) return null;
  return {
    message,
    occurredAt: readOptionalString(record.occurred_at) ?? readOptionalString(record.occurredAt),
    agentExecutionId:
      readOptionalString(record.agent_execution_id) ?? readOptionalString(record.agentExecutionId),
  } satisfies LastAgentError;
}

/**
 * Stable identifier for a specific error event. Two errors share a stamp iff
 * they have the same occurredAt timestamp and message. Used to decide whether
 * a prior dismissal still applies after a fresh failure replaces the
 * `last_agent_error` metadata.
 */
export function lastAgentErrorStamp(error: LastAgentError) {
  return `${error.occurredAt ?? ""}:${error.message}`;
}

function readOptionalString(value: unknown) {
  return typeof value === "string" && value !== "" ? value : undefined;
}
