"use client";

import { useCallback } from "react";
import { toast } from "sonner";
import { detachTask, updateTask } from "@/lib/api";
import { useAppStoreApi } from "@/components/state-provider";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";

type StoreApi = ReturnType<typeof useAppStoreApi>;
type SnapshotTask = WorkflowSnapshotData["tasks"][number];

// One re-parent operation's context, threaded to the optimistic helpers.
type NestOp = {
  store: StoreApi;
  workflowId: string;
  taskId: string;
  snapshot: WorkflowSnapshotData | undefined;
  original: SnapshotTask | undefined;
  nextParent: string | undefined;
};

/** Shallow-clone a snapshot with `taskId`'s parent set to `parent`. */
function snapshotWithParent(
  snapshot: WorkflowSnapshotData,
  taskId: string,
  parent: string | undefined,
): WorkflowSnapshotData {
  return {
    ...snapshot,
    tasks: snapshot.tasks.map((t) => (t.id === taskId ? { ...t, parentTaskId: parent } : t)),
  };
}

/**
 * Apply the optimistic parent change to the workflow snapshot. No-op when the
 * multi snapshot / task row isn't available (initial-load fallback state) —
 * the request is still sent and the WS event reconciles state.
 */
function applyOptimistic(op: NestOp): void {
  if (!op.snapshot || !op.original) return;
  op.store
    .getState()
    .setWorkflowSnapshot(op.workflowId, snapshotWithParent(op.snapshot, op.taskId, op.nextParent));
}

/**
 * Roll the optimistic parent change back to its original value, but only when
 * we actually applied one and the task still holds the optimistic value (a
 * concurrent WS update may have already reconciled it).
 */
function rollbackParent(op: NestOp): void {
  if (!op.snapshot || !op.original) return;
  const cur = op.store.getState().kanbanMulti?.snapshots?.[op.workflowId];
  const curTask = cur?.tasks.find((t) => t.id === op.taskId);
  if (cur && (curTask?.parentTaskId ?? undefined) === op.nextParent) {
    const restored = snapshotWithParent(cur, op.taskId, op.original.parentTaskId ?? undefined);
    op.store.getState().setWorkflowSnapshot(op.workflowId, restored);
  }
}

/**
 * useNestTask returns a function that nests a task under a parent (or un-nests
 * it when `parentId` is null). The multi-workflow snapshot may not be
 * populated yet (e.g. initial /t/:id load renders the sidebar from the active
 * kanban), so the optimistic snapshot patch is best-effort: the request is
 * always sent and the `task.updated` WS event reconciles state either way.
 */
export function useNestTask() {
  const store = useAppStoreApi();

  return useCallback(
    async (taskId: string, workflowId: string, parentId: string | null) => {
      const nextParent = parentId ?? undefined;
      const snapshot = store.getState().kanbanMulti?.snapshots?.[workflowId];
      const original = snapshot?.tasks.find((t) => t.id === taskId);
      const op: NestOp = { store, workflowId, taskId, snapshot, original, nextParent };

      // No-op only when we can see the current parent and it already matches.
      if (original && (original.parentTaskId ?? undefined) === nextParent) return;

      applyOptimistic(op);

      try {
        if (parentId === null) {
          // Un-nesting goes through the dedicated detach endpoint: unlike a
          // plain parent_id clear, it also normalizes an inherit_parent
          // subtask's workspace mode back to shared_group and emits the Office
          // task-update event, so the promoted root isn't left marked as
          // parent-inherited.
          await detachTask(taskId);
        } else {
          await updateTask(taskId, { parent_id: parentId });
        }
      } catch (error) {
        rollbackParent(op);
        toast.error(error instanceof Error ? error.message : "Failed to nest task");
      }
    },
    [store],
  );
}
