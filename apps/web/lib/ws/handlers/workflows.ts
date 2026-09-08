import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { WorkflowPayload } from "@/lib/types/backend";
import {
  normalizeWorkflowProfileSessionStartPolicy,
  normalizeWorkflowProfileSessionEndPolicy,
} from "@/lib/types/http";

type KanbanStep = KanbanState["steps"][number];

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
    profile_session_start_policy: normalizeWorkflowProfileSessionStartPolicy(
      step.profile_session_start_policy,
    ),
    profile_session_end_policy: normalizeWorkflowProfileSessionEndPolicy(
      step.profile_session_end_policy,
    ),
    wip_limit: step.wip_limit,
    pull_from_step_id: step.pull_from_step_id ?? null,
    stage_type: step.stage_type,
  };
}

function applyWorkflowStepChange(
  state: AppState,
  workflowId: string,
  update: (steps: KanbanStep[]) => KanbanStep[],
): AppState {
  const nextKanbanSteps =
    state.kanban.workflowId === workflowId ? update(state.kanban.steps) : state.kanban.steps;
  const snapshot = state.kanbanMulti.snapshots[workflowId];
  const nextSnapshotSteps = snapshot ? update(snapshot.steps) : undefined;
  const kanbanChanged = nextKanbanSteps !== state.kanban.steps;
  const snapshotChanged = snapshot !== undefined && nextSnapshotSteps !== snapshot.steps;

  if (!kanbanChanged && !snapshotChanged) return state;

  return {
    ...state,
    kanban: kanbanChanged ? { ...state.kanban, steps: nextKanbanSteps } : state.kanban,
    kanbanMulti:
      snapshotChanged && snapshot && nextSnapshotSteps
        ? {
            ...state.kanbanMulti,
            snapshots: {
              ...state.kanbanMulti.snapshots,
              [workflowId]: { ...snapshot, steps: nextSnapshotSteps },
            },
          }
        : state.kanbanMulti,
  };
}

function applyWorkflowCreated(state: AppState, payload: WorkflowPayload): AppState {
  if (state.workspaces.activeId !== payload.workspace_id) return state;
  if (state.workflows.items.some((item) => item.id === payload.id)) return state;
  const isHidden = Boolean(payload.hidden);
  // Never use `??` here: null is a valid "All Workflows" selection, not a missing value.
  return {
    ...state,
    workflows: {
      items: [
        {
          id: payload.id,
          workspaceId: payload.workspace_id,
          name: payload.name,
          hidden: isHidden,
          style: payload.style,
        },
        ...state.workflows.items,
      ],
      activeId: state.workflows.activeId,
    },
  };
}

function applyWorkflowUpdated(state: AppState, payload: WorkflowPayload): AppState {
  const items = state.workflows.items.map((item) =>
    item.id === payload.id
      ? {
          ...item,
          name: payload.name,
          description: payload.description,
          prompt: payload.prompt,
          agent_profile_id: payload.agent_profile_id,
          hidden: payload.hidden !== undefined ? Boolean(payload.hidden) : item.hidden,
          style: payload.style ?? item.style,
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
      const mappedStep = stepFromPayload(step);
      store.setState((state) => {
        return applyWorkflowStepChange(state, step.workflow_id, (steps) => {
          if (steps.some((s) => s.id === step.id)) return steps;
          return [...steps, mappedStep].sort((a, b) => a.position - b.position);
        });
      });
    },
    "workflow.step.updated": (message) => {
      const step = message.payload.step;
      const mappedStep = stepFromPayload(step);
      store.setState((state) => {
        return applyWorkflowStepChange(state, step.workflow_id, (steps) => {
          const index = steps.findIndex((s) => s.id === step.id);
          if (index < 0) return steps;
          const nextSteps = [...steps];
          nextSteps[index] = mappedStep;
          return nextSteps.sort((a, b) => a.position - b.position);
        });
      });
    },
    "workflow.step.deleted": (message) => {
      const step = message.payload.step;
      store.setState((state) => {
        return applyWorkflowStepChange(state, step.workflow_id, (steps) => {
          if (!steps.some((s) => s.id === step.id)) return steps;
          return steps.filter((s) => s.id !== step.id);
        });
      });
    },
  };
}
