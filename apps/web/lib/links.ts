export function linkToSession(sessionId: string, layout?: string): string {
  const base = `/s/${sessionId}`;
  return layout ? `${base}?layout=${encodeURIComponent(layout)}` : base;
}

/** Replace the browser URL to reflect the active session (no navigation). */
export function replaceSessionUrl(sessionId: string): void {
  if (typeof window === "undefined") return;
  window.history.replaceState({}, "", linkToSession(sessionId));
}

export function linkToTasks(workspaceId?: string): string {
  return workspaceId ? `/tasks?workspace=${workspaceId}` : "/tasks";
}
