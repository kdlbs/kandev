import type { Task } from "@/components/kanban-card";
import type { MoveTaskError } from "@/hooks/use-drag-and-drop";
import { reorderAdmittedInStep } from "@/lib/kanban/reorder-admitted";
import { isQueuedOverflowKanbanTask } from "@/lib/kanban/task-order";
import type { KanbanState, WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";

type MoveTaskById = (
  taskId: string,
  payload: { workflow_id: string; workflow_step_id: string; position: number },
) => Promise<unknown>;

type StoreLike = {
  getState: () => {
    kanbanMulti: { snapshots: Record<string, WorkflowSnapshotData | undefined> };
    setWorkflowSnapshot: (workflowId: string, snapshot: WorkflowSnapshotData) => void;
  };
};

type PersistDeps = {
  store: StoreLike;
  workflowId: string;
  moveTaskById: MoveTaskById;
  onMoveError?: (error: MoveTaskError) => void;
};

function rollbackSnapshot(
  store: StoreLike,
  workflowId: string,
  originalTasks: WorkflowSnapshotData["tasks"],
) {
  const currentSnapshot = store.getState().kanbanMulti.snapshots[workflowId];
  if (!currentSnapshot) return;
  store.getState().setWorkflowSnapshot(workflowId, {
    ...currentSnapshot,
    tasks: originalTasks,
  });
}

export async function persistCrossStepMove(
  deps: PersistDeps,
  task: Task,
  targetStepId: string,
): Promise<void> {
  const { store, workflowId, moveTaskById, onMoveError } = deps;
  const state = store.getState();
  const snapshot = state.kanbanMulti.snapshots[workflowId];
  if (!snapshot) return;

  const targetTasks = snapshot.tasks.filter(
    (t: KanbanState["tasks"][number]) => t.workflowStepId === targetStepId && t.id !== task.id,
  );
  const nextPosition = targetTasks.length;
  const originalTasks = snapshot.tasks;

  state.setWorkflowSnapshot(workflowId, {
    ...snapshot,
    tasks: snapshot.tasks.map((t: KanbanState["tasks"][number]) =>
      t.id === task.id ? { ...t, workflowStepId: targetStepId, position: nextPosition } : t,
    ),
  });

  try {
    await moveTaskById(task.id, {
      workflow_id: workflowId,
      workflow_step_id: targetStepId,
      position: nextPosition,
    });
  } catch (error) {
    rollbackSnapshot(store, workflowId, originalTasks);
    const message = error instanceof Error ? error.message : "Failed to move task";
    onMoveError?.({ message, taskId: task.id, sessionId: task.primarySessionId ?? null });
  }
}

export async function persistSameStepReorder(
  deps: PersistDeps,
  task: Task,
  overId: string,
  targetStepId: string,
): Promise<void> {
  if (isQueuedOverflowKanbanTask(task)) return;

  const { store, workflowId, moveTaskById, onMoveError } = deps;
  const state = store.getState();
  const snapshot = state.kanbanMulti.snapshots[workflowId];
  if (!snapshot) return;

  const result = reorderAdmittedInStep(snapshot.tasks, targetStepId, task.id, overId);
  if (!result) return;

  const originalTasks = snapshot.tasks;
  state.setWorkflowSnapshot(workflowId, { ...snapshot, tasks: result.tasks });

  try {
    // Sequential on purpose: a mid-batch failure rolls the UI back. Server may
    // still hold earlier patches (no bulk reorder API); reload reconciles.
    for (const patch of result.patches) {
      await moveTaskById(patch.taskId, {
        workflow_id: workflowId,
        workflow_step_id: patch.workflowStepId,
        position: patch.position,
      });
    }
  } catch (error) {
    rollbackSnapshot(store, workflowId, originalTasks);
    const message = error instanceof Error ? error.message : "Failed to reorder task";
    onMoveError?.({ message, taskId: task.id, sessionId: task.primarySessionId ?? null });
  }
}
