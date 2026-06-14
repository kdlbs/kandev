import type { TaskSession } from "@/lib/types/http";
import { readLastAgentError } from "@/lib/session-last-agent-error";
import { isTerminalSessionState } from "@/lib/ws/handlers/agent-session";

export function agentErrorMessageForTask(
  task: { id: string; primarySessionId?: string | null },
  sessionsById: Record<string, TaskSession>,
  sessionsByTaskId: Record<string, TaskSession[]>,
): string | null {
  if (task.primarySessionId) {
    const primaryError = readLastAgentError(sessionsById[task.primarySessionId]?.metadata);
    if (primaryError) return primaryError.message;
  }
  const fallbackSessions = [...(sessionsByTaskId[task.id] ?? [])]
    .filter((session) => !isTerminalSessionState(session.state))
    .sort((a, b) => b.updated_at.localeCompare(a.updated_at));
  for (const session of fallbackSessions) {
    const error = readLastAgentError(session.metadata);
    if (error) return error.message;
  }
  return null;
}
