type LayoutSession = { id: string };

export async function resolveLayoutApplySessionIds(args: {
  activeTaskId: string | null;
  activeSessionId: string | null;
  sessionsLoaded: boolean;
  loadSessions: (force?: boolean) => Promise<unknown> | unknown;
  getSessionsForTask: (taskId: string) => readonly LayoutSession[];
}): Promise<string[]> {
  const { activeTaskId, activeSessionId, sessionsLoaded, loadSessions, getSessionsForTask } = args;
  if (activeTaskId && !sessionsLoaded) await loadSessions(true);

  const sessionIds = activeTaskId
    ? getSessionsForTask(activeTaskId).map((session) => session.id)
    : [];
  if (activeSessionId && !sessionIds.includes(activeSessionId)) {
    sessionIds.unshift(activeSessionId);
  }
  return sessionIds;
}
