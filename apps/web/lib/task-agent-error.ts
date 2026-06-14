import type { TaskSession } from "@/lib/types/http";
import { readLastAgentError } from "@/lib/session-last-agent-error";

export function agentErrorMessageForTask(
  task: { id: string; primarySessionId?: string | null },
  sessionsById: Record<string, TaskSession>,
  sessionsByTaskId: Record<string, TaskSession[]>,
): string | null {
  if (task.primarySessionId) {
    const primaryError = readLastAgentError(sessionsById[task.primarySessionId]?.metadata);
    if (primaryError) return primaryError.message;
  }
  for (const session of sessionsByTaskId[task.id] ?? []) {
    const error = readLastAgentError(session.metadata);
    if (error) return error.message;
  }
  return null;
}
