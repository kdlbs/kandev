import type { Message, TaskSession } from "@/lib/types/http";
import {
  type LastAgentError,
  lastAgentErrorStamp,
  readLastAgentError,
} from "@/lib/session-last-agent-error";
import { isTerminalSessionState } from "@/lib/ws/handlers/agent-session";

export type AgentErrorOptions = {
  /** Most recently dismissed `last_agent_error` stamp per sessionId. */
  dismissedAgentErrors?: Record<string, string>;
  /** Sidebar-only acknowledgements for stale errors that should no longer badge task rows. */
  acknowledgedAgentErrors?: Record<string, string>;
  /** Messages keyed by sessionId. Used to auto-hide the icon once the agent
   *  produces a new message after the error timestamp. */
  messagesBySession?: Record<string, readonly Message[] | undefined>;
};

export function agentErrorMessageForTask(
  task: { id: string; primarySessionId?: string | null },
  sessionsById: Record<string, TaskSession>,
  sessionsByTaskId: Record<string, TaskSession[]>,
  options: AgentErrorOptions = {},
): string | null {
  if (task.primarySessionId) {
    const primaryError = readLastAgentError(sessionsById[task.primarySessionId]?.metadata);
    if (primaryError && !shouldHideError(task.primarySessionId, primaryError, options)) {
      return primaryError.message;
    }
  }
  const fallbackSessions = [...(sessionsByTaskId[task.id] ?? [])]
    .filter((session) => !isTerminalSessionState(session.state))
    .sort((a, b) => b.updated_at.localeCompare(a.updated_at));
  for (const session of fallbackSessions) {
    const error = readLastAgentError(session.metadata);
    if (error && !shouldHideError(session.id, error, options)) {
      return error.message;
    }
  }
  return null;
}

function shouldHideError(
  sessionId: string,
  error: LastAgentError,
  options: AgentErrorOptions,
): boolean {
  const stamp = lastAgentErrorStamp(error);
  if (hasStoredStamp(sessionId, stamp, options)) return true;
  return hasAgentMessageAfter(options.messagesBySession?.[sessionId], error.occurredAt);
}

export function resolvedAgentErrorAcknowledgementStamp(
  sessionId: string,
  session: Pick<TaskSession, "metadata"> | null | undefined,
  options: AgentErrorOptions = {},
): string | null {
  const error = readLastAgentError(session?.metadata);
  if (!error) return null;
  const stamp = lastAgentErrorStamp(error);
  if (hasStoredStamp(sessionId, stamp, options)) return null;
  return hasAgentMessageAfter(options.messagesBySession?.[sessionId], error.occurredAt)
    ? stamp
    : null;
}

function hasStoredStamp(sessionId: string, stamp: string, options: AgentErrorOptions): boolean {
  return (
    options.dismissedAgentErrors?.[sessionId] === stamp ||
    options.acknowledgedAgentErrors?.[sessionId] === stamp
  );
}

function hasAgentMessageAfter(
  messages: readonly Message[] | undefined,
  occurredAt?: string,
): boolean {
  if (!messages || !occurredAt) return false;
  const errorMs = Date.parse(occurredAt);
  if (Number.isNaN(errorMs)) return false;
  return messages.some(
    (msg) => msg.author_type === "agent" && Date.parse(msg.created_at) > errorMs,
  );
}
