import path from "node:path";

import { KANDEV_TASKS_DIR } from "./constants";

/**
 * Returns true when the current process looks like it was spawned inside a
 * kandev-created task workspace. Two signals:
 *   1. The parent kandev backend exports KANDEV_TASK_ID into every task shell.
 *   2. Task worktrees live under ~/.kandev/tasks/ (see KANDEV_TASKS_DIR).
 *
 * Used by dev mode to auto-isolate the backend onto a local dev root so that
 * `make dev` never mutates the user's production state.
 *
 * Note: the path-prefix fallback is a defensive secondary signal for nested
 * shells where KANDEV_TASK_ID was stripped. It is case-sensitive and does not
 * resolve symlinks, so a realpath'd repoRoot may miss a symlinked HOME on
 * macOS / Windows. KANDEV_TASK_ID remains the primary guarantee.
 */
export function isInsideKandevTask(repoRoot: string): boolean {
  if (process.env.KANDEV_TASK_ID) {
    return true;
  }
  return repoRoot.startsWith(KANDEV_TASKS_DIR + path.sep);
}
