import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";

/**
 * Registers WS handlers for office domain events.
 *
 * Strategy:
 * - Task status/move: direct update of the issue in store (fast, no network)
 * - Agent status: direct update of agent instance in store
 * - New tasks, comments, approvals, dashboard data: trigger refetch via
 *   the officeRefetchTrigger mechanism — pages that care watch this field
 *   and re-fetch their API data.
 */
export function registerOfficeHandlers(store: StoreApi<AppState>): WsHandlers {
  const triggerRefetch = (type: string) => {
    store.getState().setOfficeRefetchTrigger(type);
  };

  // Returns true if the event belongs to the active workspace (or has no
  // workspace_id, e.g. legacy events). Office WS broadcasts hit every
  // connected client; we filter here so cross-workspace events don't trigger
  // refetches for the wrong dashboard.
  const isCurrentWorkspace = (payload: Record<string, unknown>): boolean => {
    const wsId = payload.workspace_id as string | undefined;
    const activeId = store.getState().workspaces.activeId;
    return !wsId || wsId === activeId;
  };

  const updateTaskStatus = (taskId: string, fields: Record<string, unknown>) => {
    store.setState((state) => ({
      ...state,
      office: {
        ...state.office,
        tasks: {
          ...state.office.tasks,
          items: state.office.tasks.items.map((i) => (i.id === taskId ? { ...i, ...fields } : i)),
        },
      },
    }));
  };

  return {
    ...buildTaskHandlers(triggerRefetch, updateTaskStatus, isCurrentWorkspace),
    ...buildAgentHandlers(store, triggerRefetch, isCurrentWorkspace),
    ...buildMiscHandlers(triggerRefetch, isCurrentWorkspace),
    ...buildRoutingHandlers(store, isCurrentWorkspace),
  };
}

type WorkspaceCheck = (payload: Record<string, unknown>) => boolean;

function buildTaskHandlers(
  triggerRefetch: (type: string) => void,
  updateTaskStatus: (taskId: string, fields: Record<string, unknown>) => void,
  isCurrentWorkspace: WorkspaceCheck,
): WsHandlers {
  return {
    "office.task.updated": (message) => {
      const p = message.payload;
      if (!isCurrentWorkspace(p)) return;
      const taskId = (p.task_id ?? p.id) as string | undefined;
      if (!taskId) return;
      updateTaskStatus(taskId, normalizeIssueFields(p));
      // Per-task channel — the detail page subscribes to refresh the
      // server-authoritative task DTO after a property mutation.
      triggerRefetch(`task:${taskId}`);
      triggerRefetch("dashboard");
    },

    "office.task.created": (message) => {
      if (!isCurrentWorkspace(message.payload)) return;
      triggerRefetch("tasks");
      triggerRefetch("dashboard");
    },

    "office.task.moved": (message) => {
      const p = message.payload;
      if (!isCurrentWorkspace(p)) return;
      const taskId = (p.task_id ?? p.id) as string | undefined;
      const newStatus = p.new_status as string | undefined;
      if (!taskId || !newStatus) {
        triggerRefetch("tasks");
        triggerRefetch("dashboard");
        return;
      }
      updateTaskStatus(taskId, { status: newStatus as OfficeTaskStatus });
      triggerRefetch("dashboard");
      triggerRefetch("activity");
    },

    "office.task.status_changed": (message) => {
      const p = message.payload;
      if (!isCurrentWorkspace(p)) return;
      const taskId = (p.task_id ?? p.id) as string | undefined;
      const newStatus = p.new_status as string | undefined;
      if (!taskId || !newStatus) {
        triggerRefetch("tasks");
        return;
      }
      updateTaskStatus(taskId, { status: newStatus as OfficeTaskStatus });
      triggerRefetch("dashboard");
    },

    "office.comment.created": (message) => {
      if (!isCurrentWorkspace(message.payload)) return;
      const taskId = message.payload.task_id as string | undefined;
      triggerRefetch(taskId ? `comments:${taskId}` : "comments");
    },

    "office.task.decision_recorded": (message) => {
      const p = message.payload;
      if (!isCurrentWorkspace(p)) return;
      const taskId = p.task_id as string | undefined;
      if (taskId) triggerRefetch(`task:${taskId}`);
      // Inbox surfaces tasks awaiting review/approval; an incoming
      // decision can drop a task off the user's inbox just as easily as
      // adding one (e.g. when the final approver signs off).
      triggerRefetch("inbox");
    },

    "office.task.review_requested": (message) => {
      const p = message.payload;
      if (!isCurrentWorkspace(p)) return;
      triggerRefetch("inbox");
      const taskId = p.task_id as string | undefined;
      if (taskId) triggerRefetch(`task:${taskId}`);
    },
  };
}

function buildAgentHandlers(
  store: StoreApi<AppState>,
  triggerRefetch: (type: string) => void,
  isCurrentWorkspace: WorkspaceCheck,
): WsHandlers {
  return {
    "office.agent.completed": (message) => {
      if (!isCurrentWorkspace(message.payload)) return;
      const agentId = message.payload.agent_profile_id as string | undefined;
      if (agentId) store.getState().updateOfficeAgentProfile(agentId, { status: "idle" });
      triggerRefetch("dashboard");
      triggerRefetch("agents");
      triggerRefetch("activity");
    },

    "office.agent.failed": (message) => {
      if (!isCurrentWorkspace(message.payload)) return;
      const agentId = message.payload.agent_profile_id as string | undefined;
      if (agentId) store.getState().updateOfficeAgentProfile(agentId, { status: "idle" });
      triggerRefetch("dashboard");
      triggerRefetch("agents");
    },

    "office.agent.updated": (message) => {
      if (!isCurrentWorkspace(message.payload)) return;
      triggerRefetch("agents");
    },
  };
}

function buildMiscHandlers(
  triggerRefetch: (type: string) => void,
  isCurrentWorkspace: WorkspaceCheck,
): WsHandlers {
  return {
    "office.approval.created": (message) => {
      if (!isCurrentWorkspace(message.payload)) return;
      triggerRefetch("inbox");
      triggerRefetch("dashboard");
    },

    "office.approval.resolved": (message) => {
      if (!isCurrentWorkspace(message.payload)) return;
      triggerRefetch("inbox");
      triggerRefetch("approvals");
    },

    "office.cost.recorded": (message) => {
      if (!isCurrentWorkspace(message.payload)) return;
      triggerRefetch("costs");
      triggerRefetch("dashboard");
    },

    "office.run.queued": (message) => {
      if (!isCurrentWorkspace(message.payload)) return;
      triggerRefetch("runs");
      triggerRefetch("agents");
      // The user comment that produced this run now carries a runId /
      // runStatus = "queued" — the chat badge needs to flip from absent
      // to "Queued". Refetch the comments list for the affected task.
      const taskId = message.payload.task_id as string | undefined;
      if (taskId) triggerRefetch(`comments:${taskId}`);
    },

    "office.run.processed": (message) => {
      if (!isCurrentWorkspace(message.payload)) return;
      triggerRefetch("runs");
      triggerRefetch("agents");
      // The run lifecycle just advanced (claimed → finished/failed/
      // cancelled) — refresh the chat for the affected task so the
      // badge transitions to its new state (or hides on finished).
      const taskId = message.payload.task_id as string | undefined;
      if (taskId) triggerRefetch(`comments:${taskId}`);
    },

    "office.routine.triggered": (message) => {
      if (!isCurrentWorkspace(message.payload)) return;
      triggerRefetch("routines");
      triggerRefetch("activity");
    },
  };
}

function buildRoutingHandlers(
  store: StoreApi<AppState>,
  isCurrentWorkspace: WorkspaceCheck,
): WsHandlers {
  return {
    "office.provider.health_changed": (message) => {
      const p = message.payload;
      if (!isCurrentWorkspace(p)) return;
      const wsId = (p.workspace_id as string | undefined) ?? store.getState().workspaces.activeId;
      if (!wsId) return;
      const row = extractProviderHealth(p);
      if (!row) return;
      store.getState().upsertProviderHealth(wsId, row);
    },
    "office.route_attempt.appended": (message) => {
      const p = message.payload;
      if (!isCurrentWorkspace(p)) return;
      const runId = p.run_id as string | undefined;
      const attempt = p.attempt as Record<string, unknown> | undefined;
      if (!runId || !attempt) return;
      store.getState().appendRunAttempt(runId, attempt as RouteAttemptPayload);
    },
    "office.routing.settings_updated": (message) => {
      const p = message.payload;
      if (!isCurrentWorkspace(p)) return;
      const wsId = (p.workspace_id as string | undefined) ?? store.getState().workspaces.activeId;
      if (!wsId) return;
      store.getState().setWorkspaceRouting(wsId, undefined);
    },
  };
}

type ProviderHealthPayload = import("@/lib/state/slices/office/types").ProviderHealth;
type RouteAttemptPayload = import("@/lib/state/slices/office/types").RouteAttempt;

function extractProviderHealth(p: Record<string, unknown>): ProviderHealthPayload | null {
  if (typeof p.provider_id !== "string" || typeof p.scope !== "string") return null;
  return {
    workspace_id: typeof p.workspace_id === "string" ? p.workspace_id : undefined,
    provider_id: p.provider_id,
    scope: p.scope as ProviderHealthPayload["scope"],
    scope_value: typeof p.scope_value === "string" ? p.scope_value : "",
    state: (p.state as ProviderHealthPayload["state"]) ?? "healthy",
    error_code: p.error_code as ProviderHealthPayload["error_code"],
    retry_at: typeof p.retry_at === "string" ? p.retry_at : undefined,
    backoff_step: typeof p.backoff_step === "number" ? p.backoff_step : 0,
    last_failure: typeof p.last_failure === "string" ? p.last_failure : undefined,
    last_success: typeof p.last_success === "string" ? p.last_success : undefined,
    raw_excerpt: typeof p.raw_excerpt === "string" ? p.raw_excerpt : undefined,
  };
}

// Normalize snake_case event data fields to camelCase store fields
function normalizeIssueFields(p: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  if (p.title != null) out.title = p.title;
  if (p.description != null) out.description = p.description;
  if (p.status != null) out.status = p.status;
  if (p.new_status != null) out.status = p.new_status;
  if (p.priority != null) out.priority = p.priority;
  if (p.updated_at != null) out.updatedAt = p.updated_at;
  if (p.assignee_agent_profile_id != null) out.assigneeAgentProfileId = p.assignee_agent_profile_id;
  return out;
}

// Re-import the type for the status field cast
type OfficeTaskStatus = import("@/lib/state/slices/office/types").OfficeTaskStatus;
