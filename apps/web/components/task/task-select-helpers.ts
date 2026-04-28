/**
 * Helpers for task selection in the sidebar. Extracted as pure functions so
 * the no-session fallback path can be unit-tested without standing up the
 * dockview runtime.
 */

export type FinalizeNoSessionSelectDeps = {
  /** Set the new active task in the kanban store (also clears activeSessionId). */
  setActiveTask: (taskId: string) => void;
  /** Save the outgoing session's layout, release its portals, then build the default layout. */
  releaseLayoutToDefault: (oldSessionId: string | null) => void;
  /** Push the new task id into the URL without reloading. */
  replaceTaskUrl: (taskId: string) => void;
};

/**
 * Finalize a sidebar task selection when no session could be resolved or
 * launched for the new task. Releasing the dockview to default first ensures
 * portal cleanup targets the still-active outgoing session before
 * `setActiveTask` clears `activeSessionId` to null.
 */
export function finalizeNoSessionSelect(
  taskId: string,
  oldSessionId: string | null,
  deps: FinalizeNoSessionSelectDeps,
): void {
  deps.releaseLayoutToDefault(oldSessionId);
  deps.setActiveTask(taskId);
  deps.replaceTaskUrl(taskId);
}
