export function linkToSession(sessionId: string): string {
  return `/s/${sessionId}`;
}

export function linkToTasks(workspaceId?: string): string {
  return workspaceId ? `/tasks?workspace=${workspaceId}` : "/tasks";
}
