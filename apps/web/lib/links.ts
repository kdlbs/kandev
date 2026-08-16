export function linkToTask(taskId: string, layout?: string): string {
  const base = `/t/${taskId}`;
  return layout ? `${base}?layout=${encodeURIComponent(layout)}` : base;
}

export function linkToTaskOverview({
  workspaceId,
  workflowId,
}: {
  workspaceId?: string;
  workflowId?: string;
} = {}): string {
  const params = new URLSearchParams({ home: "overview" });
  if (workspaceId) params.set("workspaceId", workspaceId);
  if (workflowId) params.set("workflowId", workflowId);
  return `/?${params.toString()}`;
}

/**
 * The Office home for a workspace. The one builder for this URL: the sidebar
 * brand link, the Home row, and the topbar's home crumb render side by side,
 * so their hrefs must be byte-identical for the same workspace.
 */
export function linkToOfficeHome({ workspaceId }: { workspaceId?: string } = {}): string {
  if (!workspaceId) return "/office";
  return `/office?${new URLSearchParams({ workspaceId }).toString()}`;
}

/** Task-detail route prefixes the SPA serves: canonical and compatibility. */
const TASK_DETAIL_PREFIXES = ["/t/", "/tasks/"];

export function normalizePathname(pathname: string): string {
  return pathname.length > 1 && pathname.endsWith("/") ? pathname.slice(0, -1) : pathname;
}

/**
 * True when `pathname` is a task-detail route for `taskId`. Matches both the
 * canonical `/t/:id` and the compatibility `/tasks/:id` paths.
 */
export function isTaskDetailPath(pathname: string, taskId: string): boolean {
  const normalized = normalizePathname(pathname);
  return TASK_DETAIL_PREFIXES.some((prefix) => normalized === `${prefix}${taskId}`);
}

/** Replace the browser URL to reflect the active task (no navigation). */
export function replaceTaskUrl(taskId: string): void {
  if (typeof window === "undefined") return;
  window.history.replaceState({}, "", linkToTask(taskId));
}

export function linkToTasks(workspaceId?: string): string {
  return workspaceId ? `/tasks?workspace=${workspaceId}` : "/tasks";
}
