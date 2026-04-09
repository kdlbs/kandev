"use client";

import { useCallback } from "react";
import type { StoreApi } from "zustand";
import type { KanbanState } from "@/lib/state/slices";
import type { AppState } from "@/lib/state/store";
import type { useTaskActions } from "@/hooks/use-task-actions";

type TaskActions = ReturnType<typeof useTaskActions>;

// Max concurrent API requests per bulk operation. Avoids thundering-herd
// against the backend when a lane has many tasks.
const BULK_ACTION_BATCH_SIZE = 10;

/**
 * Runs `worker` over `items` in serial batches of BULK_ACTION_BATCH_SIZE,
 * collecting settled results in the same order as the input array.
 * Index passed to `worker` is the global index across all batches so callers
 * can use it for stable position assignment.
 */
async function allSettledInBatches<T>(
  items: T[],
  worker: (item: T, index: number) => Promise<unknown>,
): Promise<PromiseSettledResult<unknown>[]> {
  const results: PromiseSettledResult<unknown>[] = [];
  for (let i = 0; i < items.length; i += BULK_ACTION_BATCH_SIZE) {
    const batch = items.slice(i, i + BULK_ACTION_BATCH_SIZE);
    results.push(...(await Promise.allSettled(batch.map((item, offset) => worker(item, i + offset)))));
  }
  return results;
}

/**
 * Runs `action` on each task in batches, then calls `applySuccess` with the
 * set of IDs whose requests fulfilled. Partial failures are silently ignored —
 * only successful task IDs are passed to the store update.
 */
async function performBulkAction(
  tasks: KanbanState["tasks"][number][],
  action: (id: string) => Promise<unknown>,
  applySuccess: (succeededIds: Set<string>) => void,
): Promise<void> {
  const results = await allSettledInBatches(tasks, (task) => action(task.id));
  const succeededIds = new Set(
    tasks.filter((_, i) => results[i].status === "fulfilled").map((t) => t.id),
  );
  applySuccess(succeededIds);
}

export function useLaneTaskActions({
  workflowId,
  store,
  deleteTaskById,
  archiveTaskById,
  moveTaskById,
}: {
  workflowId: string | null | undefined;
  store: StoreApi<AppState>;
  deleteTaskById: TaskActions["deleteTaskById"];
  archiveTaskById: TaskActions["archiveTaskById"];
  moveTaskById: TaskActions["moveTaskById"];
}) {
  const handleClearLane = useCallback(
    async (tasks: KanbanState["tasks"][number][]) => {
      if (!workflowId || tasks.length === 0) return;
      await performBulkAction(tasks, deleteTaskById, (deletedIds) => {
        store.getState().hydrate({
          kanban: {
            ...store.getState().kanban,
            tasks: store
              .getState()
              .kanban.tasks.filter(
                (item: KanbanState["tasks"][number]) => !deletedIds.has(item.id),
              ),
          },
        });
      });
    },
    [deleteTaskById, workflowId, store],
  );

  const handleMoveLane = useCallback(
    async (tasks: KanbanState["tasks"][number][], targetStepId: string) => {
      if (!workflowId || tasks.length === 0) return;

      // Skip tasks already in the target step — moving them would be a no-op
      // and would wastefully re-sequence their positions.
      const actionableTasks = tasks.filter((t) => t.workflowStepId !== targetStepId);
      if (actionableTasks.length === 0) return;

      // Snapshot current tasks before any mutations so position math is stable.
      const currentTasks = store.getState().kanban.tasks;
      const movedIds = new Set(actionableTasks.map((t) => t.id));
      const maxTargetPos = currentTasks
        .filter(
          (t: KanbanState["tasks"][number]) =>
            t.workflowStepId === targetStepId && !movedIds.has(t.id),
        )
        .reduce(
          (max: number, t: KanbanState["tasks"][number]) => Math.max(max, t.position ?? 0),
          -1,
        );

      const results = await allSettledInBatches(actionableTasks, (task, i) =>
        moveTaskById(task.id, {
          workflow_id: workflowId!,
          workflow_step_id: targetStepId,
          position: maxTargetPos + 1 + i,
        }),
      );

      const succeededMoves = new Map(
        actionableTasks
          .map((task, i) =>
            results[i].status === "fulfilled" ? ([task.id, maxTargetPos + 1 + i] as const) : null,
          )
          .filter((entry): entry is [string, number] => entry !== null),
      );
      store.getState().hydrate({
        kanban: {
          ...store.getState().kanban,
          tasks: store.getState().kanban.tasks.map((t: KanbanState["tasks"][number]) => {
            const newPos = succeededMoves.get(t.id);
            if (newPos === undefined) return t;
            return { ...t, workflowStepId: targetStepId, position: newPos };
          }),
        },
      });
    },
    [moveTaskById, workflowId, store],
  );

  const handleArchiveLane = useCallback(
    async (tasks: KanbanState["tasks"][number][]) => {
      if (!workflowId || tasks.length === 0) return;
      await performBulkAction(tasks, archiveTaskById, (archivedIds) => {
        store.getState().hydrate({
          kanban: {
            ...store.getState().kanban,
            tasks: store
              .getState()
              .kanban.tasks.filter(
                (item: KanbanState["tasks"][number]) => !archivedIds.has(item.id),
              ),
          },
        });
      });
    },
    [archiveTaskById, workflowId, store],
  );

  return { handleClearLane, handleMoveLane, handleArchiveLane };
}
