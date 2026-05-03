import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { KanbanState } from "@/lib/state/slices/kanban/types";

type KanbanTask = KanbanState["tasks"][number];
type KanbanStep = KanbanState["steps"][number];

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
      }));

      store.setState((state) => {
        // kanban.update doesn't carry primarySessionId / primarySessionState —
        // those are set by task.updated WS events. Build tasks inside setState
        // so we can read existing values and preserve them.
        const existingById = new Map(state.kanban.tasks.map((t) => [t.id, t]));
        const tasks: KanbanTask[] = message.payload.tasks
          // Filter out ephemeral tasks (e.g., quick chat)
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          .filter((task: any) => !task.is_ephemeral)
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          .map((task: any) => {
            const existing = existingById.get(task.id);
            return {
              id: task.id,
              workflowStepId: task.workflowStepId,
              title: task.title,
              description: task.description,
              position: task.position ?? 0,
              state: task.state,
              primarySessionId: existing?.primarySessionId,
              primarySessionState: existing?.primarySessionState,
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
            // Fall back to the multi-snapshot's own value only when the main
            // kanban lookup returned `undefined` (task absent from kanban.tasks).
            // An explicit `null` means the primary was intentionally cleared
            // and must NOT be replaced by a stale snapshot value.
            return {
              ...t,
              primarySessionId:
                t.primarySessionId === undefined ? fallback?.primarySessionId : t.primarySessionId,
              primarySessionState:
                t.primarySessionState === undefined
                  ? fallback?.primarySessionState
                  : t.primarySessionState,
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
