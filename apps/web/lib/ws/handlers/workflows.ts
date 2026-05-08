import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { WorkflowPayload } from "@/lib/types/backend";

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

function applyWorkflowCreated(state: AppState, payload: WorkflowPayload): AppState {
  if (state.workspaces.activeId !== payload.workspace_id) return state;
  if (state.workflows.items.some((item) => item.id === payload.id)) return state;
  const isHidden = Boolean(payload.hidden);
  return {
    ...state,
    workflows: {
      items: [
        {
          id: payload.id,
          workspaceId: payload.workspace_id,
          name: payload.name,
          hidden: isHidden,
        },
        ...state.workflows.items,
      ],
      // Hidden workflows must never be promoted to the active selection;
      // they are system-only and would surface in the workflow picker.
      activeId: state.workflows.activeId ?? (isHidden ? null : payload.id),
    },
  };
}

function applyWorkflowUpdated(state: AppState, payload: WorkflowPayload): AppState {
  const items = state.workflows.items.map((item) =>
    item.id === payload.id
      ? {
          ...item,
          name: payload.name,
          agent_profile_id: payload.agent_profile_id,
          hidden: payload.hidden !== undefined ? Boolean(payload.hidden) : item.hidden,
        }
      : item,
  );
  // If the active workflow just became hidden, fall back to the next visible
  // entry so the kanban / picker isn't left bound to a workflow the user can
  // no longer reach (the backend fires `workflow.updated`, not `workflow.deleted`,
  // when `SetWorkflowHidden` flips the flag).
  const activeBecameHidden = state.workflows.activeId === payload.id && payload.hidden === true;
  const nextActiveId = activeBecameHidden
    ? (items.find((item) => !item.hidden)?.id ?? null)
    : state.workflows.activeId;
  return {
    ...state,
    workflows: {
      ...state.workflows,
      activeId: nextActiveId,
      items,
    },
  };
}

export function registerWorkflowsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "workflow.created": (message) => {
      store.setState((state) => applyWorkflowCreated(state, message.payload));
    },
    "workflow.updated": (message) => {
      store.setState((state) => applyWorkflowUpdated(state, message.payload));
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
