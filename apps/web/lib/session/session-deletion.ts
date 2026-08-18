import type { QuickChatSession } from "@/lib/state/slices/ui/types";

export type SessionDeletionTarget =
  | { kind: "quick-chat-task"; taskId: string }
  | { kind: "session"; sessionId: string };

/** Routes Quick Chat members through task deletion and ordinary sessions through session deletion. */
export function resolveSessionDeletionTarget(
  sessionId: string,
  taskId: string,
  quickChatSessions: readonly QuickChatSession[],
): SessionDeletionTarget {
  const quickChatSession = quickChatSessions.find((session) => session.sessionId === sessionId);
  if (quickChatSession) {
    return { kind: "quick-chat-task", taskId: quickChatSession.taskId ?? taskId };
  }
  return { kind: "session", sessionId };
}
