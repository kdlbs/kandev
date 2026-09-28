import type { TaskStatusSummaryActiveError } from "./types/task-status-summary";
import {
  hasSessionRecoveryResolutionAfter,
  isSuccessfulScriptExecutionMetadata,
} from "@/hooks/processed-message-filtering";
import { lastAgentErrorStamp, readLastAgentError } from "./session-last-agent-error";
import { legacyRecoveryMessageMatchesError } from "./session-recovery-presentation";
import type { ActionMeta } from "@/components/task/chat/messages/action-message-details";

type RecoverySession = {
  id: string;
  state: string;
  error_message?: string | null;
  metadata?: Record<string, unknown> | null;
};
type RecoveryMessage = {
  type?: string;
  id: string;
  session_id?: string;
  created_at?: string;
  content?: string;
  metadata?: Record<string, unknown> | null;
};
export type ActiveSessionRecovery = {
  sessionId: string;
  stamp?: string;
  error?: import("./session-last-agent-error").LastAgentError | null;
  kind: string;
  loading?: boolean;
  summary?: string;
  details?: string;
  metadata?: ActionMeta;
};

/** Select current recovery data without letting retained history block a usable session. */
export function selectActiveSessionRecovery(
  session: RecoverySession | null | undefined,
  messages: readonly RecoveryMessage[],
  currentError?: TaskStatusSummaryActiveError | null,
): ActiveSessionRecovery | null {
  if (!session || !["FAILED", "WAITING_FOR_INPUT", "STARTING"].includes(session.state)) return null;
  const error = currentSessionError(session, currentError);
  if (error?.scope === "task") return null;
  const stamp = error ? lastAgentErrorStamp(error) : undefined;
  const candidates = messages.filter((message) =>
    matchesRecoveryMessage(session.id, error, message),
  );
  const message = candidates
    .toSorted((a, b) => (a.created_at ?? "").localeCompare(b.created_at ?? ""))
    .at(-1);
  if (isResolvedFailure(session, error, message, messages)) return null;
  if (session.state === "STARTING" && !hasUnresolvedFailure(session, error, message, messages))
    return null;
  if (
    session.state === "WAITING_FOR_INPUT" &&
    !session.error_message &&
    !hasUnresolvedFailure(session, error, message, messages)
  )
    return null;
  return recoveryModel(session, error, stamp, message);
}

function recoveryModel(
  session: RecoverySession,
  error: ReturnType<typeof readLastAgentError>,
  stamp: string | undefined,
  message?: RecoveryMessage,
): ActiveSessionRecovery {
  const metadata = message?.metadata as ActionMeta | undefined;
  return {
    sessionId: session.id,
    stamp,
    error,
    kind: metadata?.failure_kind ?? error?.code ?? "generic",
    summary: error?.message ?? session.error_message ?? message?.content,
    details: metadata?.error_output ?? error?.details ?? session.error_message ?? undefined,
    metadata,
  };
}

function matchesRecoveryMessage(
  sessionId: string,
  error: ReturnType<typeof readLastAgentError>,
  message: RecoveryMessage,
) {
  const meta = message.metadata;
  if (message.session_id !== sessionId || meta?.scope === "task" || meta?.recovery_actions !== true)
    return false;
  if (!error) return true;
  const stamp = meta.error_stamp ?? meta.recovery_stamp;
  if (stamp) return stamp === lastAgentErrorStamp(error);
  return legacyRecoveryMessageMatchesError(message.content ?? "", message.created_at, error);
}

function hasUnresolvedFailure(
  session: RecoverySession,
  error: ReturnType<typeof readLastAgentError>,
  message: RecoveryMessage | undefined,
  messages: readonly RecoveryMessage[],
) {
  if (!error || (!message && session.state !== "STARTING")) return false;
  const occurredAt = error.occurredAt ?? message?.created_at;
  if (!occurredAt) return false;
  const failedAt = Date.parse(occurredAt);
  return !Number.isNaN(failedAt) && !isResolvedFailure(session, error, message, messages);
}

function isResolvedFailure(
  session: RecoverySession,
  error: ReturnType<typeof readLastAgentError>,
  message: RecoveryMessage | undefined,
  messages: readonly RecoveryMessage[],
) {
  const occurredAt = error?.occurredAt ?? message?.created_at;
  if (hasSessionRecoveryResolutionAfter(session.metadata, occurredAt)) return true;
  const failedAt = Date.parse(occurredAt ?? "");
  return messages.some((candidate) => successfulBootAfter(candidate, session.id, failedAt));
}

function successfulBootAfter(message: RecoveryMessage, sessionId: string, failedAt: number) {
  return (
    message.session_id === sessionId &&
    message.type === "script_execution" &&
    message.metadata?.script_type === "agent_boot" &&
    isSuccessfulScriptExecutionMetadata(message.metadata) &&
    Date.parse(message.created_at ?? "") > failedAt
  );
}

function currentSessionError(
  session: RecoverySession,
  current?: TaskStatusSummaryActiveError | null,
): ReturnType<typeof readLastAgentError> {
  if (current?.scope !== "session" || current.session_id !== session.id)
    return readLastAgentError(session.metadata);
  return {
    message: current.preview,
    stamp: current.stamp,
    scope: "session",
    occurredAt: current.occurred_at,
    phase: current.phase,
    code: current.category,
    details: current.details,
    causes: current.causes,
  };
}
