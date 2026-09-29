import { fetchTask, listTaskSessions } from "@/lib/api";

export async function resolveTaskRoute(taskId: string, requestedSessionId?: string) {
  const [task, allSessionsResponse] = await Promise.all([
    fetchTask(taskId, { cache: "no-store" }),
    listTaskSessions(taskId, { cache: "no-store" }),
  ]);
  const sessions = allSessionsResponse.sessions ?? [];

  const ownedSessions = sessions.filter((session) => session.task_id === taskId);
  const requestedSession = requestedSessionId
    ? ownedSessions.find((session) => session.id === requestedSessionId)
    : undefined;
  const primarySession = task.primary_session_id
    ? ownedSessions.find((session) => session.id === task.primary_session_id)
    : undefined;
  const sessionId = requestedSession?.id ?? primarySession?.id ?? ownedSessions[0]?.id;
  return {
    task,
    sessionId,
    allSessionsResponse: { ...allSessionsResponse, sessions: ownedSessions },
  };
}
