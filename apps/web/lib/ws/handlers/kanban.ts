import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import { mergeTaskRepositoryFields } from "@/lib/ws/handlers/task-repositories";

type KanbanTask = KanbanState["tasks"][number];
type KanbanStep = KanbanState["steps"][number];

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
};

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
        // kanban.update doesn't carry primarySessionId / primarySessionState —
        // those are set by task.updated WS events. Build tasks inside setState
        // so we can read existing values and preserve them.
        const existingById = new Map(state.kanban.tasks.map((t) => [t.id, t]));
        const tasks: KanbanTask[] = message.payload.tasks
          // Filter out ephemeral tasks (e.g., quick chat)
          .filter((task: KanbanUpdateTask) => !task.is_ephemeral)
          .map((task: KanbanUpdateTask) => {
            const existing = existingById.get(task.id);
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
              foregroundActivity: existing?.foregroundActivity,
            };
          });

        const next = {
          ...state,
          kanban: { workflowId, steps, tasks },
        };

        // Also update multi-workflow snapshots if this workflow is tracked
        const snapshot = state.kanbanMulti.snapshots[workflowId];
        if (snapshot) {
          const existingMultiById = new Map(snapshot.tasks.map((t) => [t.id, t]));
          const multiTasks = tasks.map((t) => {
            const fallback = existingMultiById.get(t.id);
            const repoFields = mergeTaskRepositoryFields(fallback, t);
            // Fall back to the multi-snapshot's own value only when the main
            // kanban lookup returned `undefined` (task absent from kanban.tasks).
            // An explicit `null` means the primary was intentionally cleared
            // and must NOT be replaced by a stale snapshot value.
            return {
              ...t,
              ...repoFields,
              primarySessionId:
                t.primarySessionId === undefined ? fallback?.primarySessionId : t.primarySessionId,
              primarySessionState:
                t.primarySessionState === undefined
                  ? fallback?.primarySessionState
                  : t.primarySessionState,
              primarySessionPendingAction:
                t.primarySessionPendingAction === undefined
                  ? fallback?.primarySessionPendingAction
                  : t.primarySessionPendingAction,
              taskPendingAction:
                t.taskPendingAction === undefined
                  ? fallback?.taskPendingAction
                  : t.taskPendingAction,
              foregroundActivity:
                t.foregroundActivity === undefined
                  ? fallback?.foregroundActivity
                  : t.foregroundActivity,
            };
          });
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
