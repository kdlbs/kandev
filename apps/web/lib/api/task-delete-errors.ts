import { ApiError } from "./client";

export const TASK_DELETE_DIRTY_WORKTREE_ERROR_CODE = "task_delete_dirty_worktree";

export function isTaskDeleteDirtyWorktreeError(error: unknown): error is ApiError {
  return error instanceof ApiError && error.errorCode === TASK_DELETE_DIRTY_WORKTREE_ERROR_CODE;
}
