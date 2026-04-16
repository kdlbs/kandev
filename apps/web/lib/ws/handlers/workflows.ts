import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function stepFromPayload(step: any) {
  return {
    id: step.id as string,
    title: (step.name ?? step.title) as string,
    color: (step.color ?? "bg-neutral-400") as string,
    position: (step.position ?? 0) as number,
    events: step.events,
    show_in_command_panel: step.show_in_command_panel,
    allow_manual_move: step.allow_manual_move,
    prompt: step.prompt,
    is_start_step: step.is_start_step,
    agent_profile_id: step.agent_profile_id,
  };
}

export function registerWorkflowsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "workflow.created": (message) => {
      store.setState((state) => {
        if (state.workspaces.activeId !== message.payload.workspace_id) {
          return state;
        }
        const exists = state.workflows.items.some((item) => item.id === message.payload.id);
        if (exists) {
          return state;
        }
        return {
          ...state,
          workflows: {
            items: [
              {
                id: message.payload.id,
                workspaceId: message.payload.workspace_id,
                name: message.payload.name,
              },
              ...state.workflows.items,
            ],
            activeId: state.workflows.activeId ?? message.payload.id,
          },
        };
      });
    },
    "workflow.updated": (message) => {
      store.setState((state) => ({
        ...state,
        workflows: {
          ...state.workflows,
          items: state.workflows.items.map((item) =>
            item.id === message.payload.id
              ? {
                  ...item,
                  name: message.payload.name,
                  agent_profile_id: message.payload.agent_profile_id,
                }
              : item,
          ),
        },
      }));
    },
    "workflow.deleted": (message) => {
      store.setState((state) => {
        const items = state.workflows.items.filter((item) => item.id !== message.payload.id);
        const nextActiveId =
          state.workflows.activeId === message.payload.id
            ? (items[0]?.id ?? null)
            : state.workflows.activeId;
        return {
          ...state,
          workflows: {
            items,
            activeId: nextActiveId,
          },
          kanban:
            state.kanban.workflowId === message.payload.id
              ? { workflowId: nextActiveId, steps: [], tasks: [] }
              : state.kanban,
        };
      });
    },
    "workflow.step.created": (message) => {
      const step = message.payload.step;
      store.setState((state) => {
        if (state.kanban.workflowId !== step.workflow_id) return state;
        if (state.kanban.steps.some((s) => s.id === step.id)) return state;
        const steps = [...state.kanban.steps, stepFromPayload(step)].sort(
          (a, b) => a.position - b.position,
        );
        return { ...state, kanban: { ...state.kanban, steps } };
      });
    },
    "workflow.step.updated": (message) => {
      const step = message.payload.step;
      store.setState((state) => {
        if (state.kanban.workflowId !== step.workflow_id) return state;
        const steps = state.kanban.steps
          .map((s) => (s.id === step.id ? stepFromPayload(step) : s))
          .sort((a, b) => a.position - b.position);
        return { ...state, kanban: { ...state.kanban, steps } };
      });
    },
    "workflow.step.deleted": (message) => {
      const step = message.payload.step;
      store.setState((state) => {
        if (state.kanban.workflowId !== step.workflow_id) return state;
        const steps = state.kanban.steps.filter((s) => s.id !== step.id);
        return { ...state, kanban: { ...state.kanban, steps } };
      });
    },
  };
}
