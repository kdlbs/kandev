export function linkToTask(taskId: string, layout?: string): string {
  const base = `/t/${taskId}`;
  return layout ? `${base}?layout=${encodeURIComponent(layout)}` : base;
}

/** Replace the browser URL to reflect the active task (no navigation). */
export function replaceTaskUrl(taskId: string): void {
  if (typeof window === "undefined") return;
  window.history.replaceState({}, "", linkToTask(taskId));
}

export function linkToTasks(workspaceId?: string): string {
  return workspaceId ? `/tasks?workspace=${workspaceId}` : "/tasks";
}
