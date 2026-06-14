export type LastAgentError = {
  message: string;
  occurredAt?: string;
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
    occurredAt: readOptionalString(record.occurred_at) ?? readOptionalString(record.occurredAt),
    agentExecutionId:
      readOptionalString(record.agent_execution_id) ?? readOptionalString(record.agentExecutionId),
  } satisfies LastAgentError;
}

export function lastAgentErrorDismissKey(sessionId: string, error: LastAgentError) {
  const stamp = `${error.occurredAt ?? ""}:${error.message}`;
  return `kandev:last-agent-error-dismissed:${sessionId}:${stamp}`;
}

function readOptionalString(value: unknown) {
  return typeof value === "string" && value !== "" ? value : undefined;
}
