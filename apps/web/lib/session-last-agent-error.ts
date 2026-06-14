export type LastAgentError = {
  message: string;
  occurredAt: string;
  agentExecutionId?: string;
};

export function readLastAgentError(metadata: Record<string, unknown> | null | undefined) {
  if (!metadata) return null;
  const raw = metadata.last_agent_error;
  if (!raw || typeof raw !== "object") return null;
  const record = raw as Record<string, unknown>;
  const message = typeof record.message === "string" ? record.message : "";
  if (!message) return null;
  return {
    message,
    occurredAt: readString(record.occurred_at) || readString(record.occurredAt),
    agentExecutionId: readString(record.agent_execution_id) || readString(record.agentExecutionId),
  } satisfies LastAgentError;
}

export function lastAgentErrorDismissKey(sessionId: string, error: LastAgentError) {
  const stamp = error.occurredAt || error.message;
  return `kandev:last-agent-error-dismissed:${sessionId}:${stamp}`;
}

function readString(value: unknown) {
  return typeof value === "string" ? value : "";
}
