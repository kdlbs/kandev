import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import { mergeTaskRepositoryFields } from "@/lib/ws/handlers/task-repositories";

type KanbanTask = KanbanState["tasks"][number];
type KanbanStep = KanbanState["steps"][number];

function queueFields(
  task: KanbanUpdateTask,
  existing: KanbanTask | undefined,
): Pick<KanbanTask, "wipAdmitted" | "queuedForStepId" | "queuedAt"> {
  return {
    wipAdmitted: task.wip_admitted ?? task.wipAdmitted ?? existing?.wipAdmitted,
    queuedForStepId: task.queued_for_step_id ?? task.queuedForStepId ?? existing?.queuedForStepId,
    queuedAt: task.queued_at ?? task.queuedAt ?? existing?.queuedAt,
  };
}

type KanbanUpdateTask = {
  id: string;
  workflowStepId: string;
  title: string;
  description?: string;
  position?: number;
  state?: KanbanTask["state"];
  repository_id?: string;
  repositories?: KanbanTask["repositories"];
  is_ephemeral?: boolean;
  wip_admitted?: boolean;
  queued_for_step_id?: string;
  queued_at?: string;
  wipAdmitted?: boolean;
  queuedForStepId?: string;
  queuedAt?: string;
};

// kanban.update doesn't carry primarySessionId / primarySessionState / live
// activity fields — those are set by task.updated WS events. Preserve
// whatever the existing row already had.
function buildKanbanTaskFromUpdate(
  task: KanbanUpdateTask,
  existing: KanbanTask | undefined,
): KanbanTask {
  const repoFields = mergeTaskRepositoryFields(existing, {
    repositoryId: task.repository_id,
    repositories: task.repositories,
  });
  return {
    id: task.id,
    workflowStepId: task.workflowStepId,
    title: task.title,
    description: task.description,
    position: task.position ?? 0,
    state: task.state,
    ...repoFields,
    primarySessionId: existing?.primarySessionId,
    primarySessionState: existing?.primarySessionState,
    primarySessionPendingAction: existing?.primarySessionPendingAction,
    taskPendingAction: existing?.taskPendingAction,
    interrupted: existing?.interrupted,
    foregroundActivity: existing?.foregroundActivity,
    parkedOnBackgroundWork: existing?.parkedOnBackgroundWork,
    parkedEpoch: existing?.parkedEpoch,
    parkedRevision: existing?.parkedRevision,
    ...queueFields(task, existing),
  };
}

// Fall back to the multi-snapshot's own value only when the main kanban
// lookup returned `undefined` (task absent from kanban.tasks). An explicit
// `null` means the primary was intentionally cleared and must NOT be
// replaced by a stale snapshot value.
function preserveIfUndefined<T>(value: T | undefined, fallback: T | undefined): T | undefined {
  return value === undefined ? fallback : value;
}

function mergeMultiSnapshotTask(t: KanbanTask, fallback: KanbanTask | undefined): KanbanTask {
  const repoFields = mergeTaskRepositoryFields(fallback, t);
  return {
    ...t,
    ...repoFields,
    primarySessionId: preserveIfUndefined(t.primarySessionId, fallback?.primarySessionId),
    primarySessionState: preserveIfUndefined(t.primarySessionState, fallback?.primarySessionState),
    primarySessionPendingAction: preserveIfUndefined(
      t.primarySessionPendingAction,
      fallback?.primarySessionPendingAction,
    ),
    taskPendingAction: preserveIfUndefined(t.taskPendingAction, fallback?.taskPendingAction),
    foregroundActivity: preserveIfUndefined(t.foregroundActivity, fallback?.foregroundActivity),
    interrupted: preserveIfUndefined(t.interrupted, fallback?.interrupted),
    // AC-58a: follow the same omission-preserves-existing pattern as
    // foregroundActivity above, not an unconditional overwrite — a task
    // absent from state.kanban.tasks (different active board) must not wipe
    // this multi-snapshot's own cached parked triple.
    parkedOnBackgroundWork: preserveIfUndefined(
      t.parkedOnBackgroundWork,
      fallback?.parkedOnBackgroundWork,
    ),
    parkedEpoch: preserveIfUndefined(t.parkedEpoch, fallback?.parkedEpoch),
    parkedRevision: preserveIfUndefined(t.parkedRevision, fallback?.parkedRevision),
  };
}

export function registerKanbanHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "kanban.update": (message) => {
      const workflowId = message.payload.workflowId;
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const steps: KanbanStep[] = message.payload.steps.map((step: any, index: number) => ({
        id: step.id,
        title: step.title,
        color: step.color ?? "bg-neutral-400",
        position: step.position ?? index,
        events: step.events,
        show_in_command_panel: step.show_in_command_panel,
        agent_profile_id: step.agent_profile_id,
        wip_limit: step.wip_limit,
        pull_from_step_id: step.pull_from_step_id ?? null,
      }));

      store.setState((state) => {
        // Build tasks inside setState so we can read existing values and
        // preserve them (see buildKanbanTaskFromUpdate).
        const existingById = new Map(state.kanban.tasks.map((t) => [t.id, t]));
        const tasks: KanbanTask[] = message.payload.tasks
          // Filter out ephemeral tasks (e.g., quick chat)
          .filter((task: KanbanUpdateTask) => !task.is_ephemeral)
          .map((task: KanbanUpdateTask) =>
            buildKanbanTaskFromUpdate(task, existingById.get(task.id)),
          );

        const next = {
          ...state,
          kanban: { workflowId, steps, tasks },
        };

        // Also update multi-workflow snapshots if this workflow is tracked
        const snapshot = state.kanbanMulti.snapshots[workflowId];
        if (snapshot) {
          const existingMultiById = new Map(snapshot.tasks.map((t) => [t.id, t]));
          const multiTasks = tasks.map((t) =>
            mergeMultiSnapshotTask(t, existingMultiById.get(t.id)),
          );
          return {
            ...next,
            kanbanMulti: {
              ...next.kanbanMulti,
              snapshots: {
                ...next.kanbanMulti.snapshots,
                [workflowId]: { ...snapshot, steps, tasks: multiTasks },
              },
            },
          };
        }

        return next;
      });
    },
  };
}
