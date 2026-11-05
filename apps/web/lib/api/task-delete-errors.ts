import { ApiError } from "./client";

export const TASK_DELETE_DIRTY_WORKTREE_ERROR_CODE = "task_delete_dirty_worktree";

export function isTaskDeleteDirtyWorktreeError(error: unknown): error is ApiError {
  return error instanceof ApiError && error.errorCode === TASK_DELETE_DIRTY_WORKTREE_ERROR_CODE;
}

export function isTaskDeletePreviewOutdatedError(error: unknown): boolean {
  return (
    error instanceof ApiError &&
    error.status === 409 &&
    error.body !== null &&
    typeof error.body === "object" &&
    "error" in error.body &&
    // i18n-exempt: backend protocol error identifier
    error.body.error === "task deletion preview is no longer current"
  );
}
