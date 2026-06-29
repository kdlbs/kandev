export function linkToTask(taskId: string, layout?: string): string {
  const base = `/t/${taskId}`;
  return layout ? `${base}?layout=${encodeURIComponent(layout)}` : base;
}

/** Task-detail route prefixes the SPA serves: canonical and compatibility. */
const TASK_DETAIL_PREFIXES = ["/t/", "/tasks/"];

/**
 * True when `pathname` is a task-detail route for `taskId`. Matches both the
 * canonical `/t/:id` and the compatibility `/tasks/:id` paths.
 */
export function isTaskDetailPath(pathname: string, taskId: string): boolean {
  return TASK_DETAIL_PREFIXES.some((prefix) => pathname === `${prefix}${taskId}`);
}

/** Replace the browser URL to reflect the active task (no navigation). */
export function replaceTaskUrl(taskId: string): void {
  if (typeof window === "undefined") return;
  window.history.replaceState({}, "", linkToTask(taskId));
}

export function linkToTasks(workspaceId?: string): string {
  return workspaceId ? `/tasks?workspace=${workspaceId}` : "/tasks";
}
