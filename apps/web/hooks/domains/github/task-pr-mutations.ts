import type { StoreApi } from "zustand";
import { deleteTaskPR } from "@/lib/api/domains/github-api";
import type { AppState } from "@/lib/state/store";
import { beginTaskPRUnlink, endTaskPRUnlink } from "./task-pr-unlink-registry";

export const TASK_PR_UNLINK_NO_ACTIVE_WORKSPACE = Symbol();

export function taskPRUnlinkErrorDescription(
  error: unknown,
  noWorkspaceMessage: string,
  fallbackMessage: string,
): string {
  if (error === TASK_PR_UNLINK_NO_ACTIVE_WORKSPACE) return noWorkspaceMessage;
  if (error instanceof Error) return error.message;
  return fallbackMessage;
}

type UnlinkTaskPRAssociationOptions = {
  associationId: string;
  store: StoreApi<AppState>;
  taskId: string | null;
  workspaceId: string | null;
  workspaceContextGeneration: number;
  isWorkspaceContextCurrent: () => boolean;
  removeTaskPR: AppState["removeTaskPR"];
  invalidateSync: () => void;
};

export async function unlinkTaskPRAssociation({
  associationId,
  store,
  taskId,
  workspaceId,
  workspaceContextGeneration,
  isWorkspaceContextCurrent,
  removeTaskPR,
  invalidateSync,
}: UnlinkTaskPRAssociationOptions): Promise<boolean> {
  if (!taskId || !workspaceId) throw TASK_PR_UNLINK_NO_ACTIVE_WORKSPACE;

  const state = store.getState();
  if (!isWorkspaceContextCurrent()) return false;
  const taskPRs =
    state.taskPRs.workspaceId === workspaceId &&
    state.taskPRs.workspaceContextGeneration === workspaceContextGeneration &&
    Array.isArray(state.taskPRs.byTaskId[taskId])
      ? state.taskPRs.byTaskId[taskId]
      : [];
  if (!taskPRs.some((pr) => pr.id === associationId)) return false;

  const key = { workspaceId, workspaceContextGeneration, taskId, associationId };
  if (!beginTaskPRUnlink(store, key)) return false;
  try {
    await deleteTaskPR(associationId, workspaceId);
    if (isWorkspaceContextCurrent()) {
      removeTaskPR(taskId, associationId, { workspaceId, workspaceContextGeneration });
      invalidateSync();
    }
    return true;
  } finally {
    endTaskPRUnlink(store, key);
  }
}
