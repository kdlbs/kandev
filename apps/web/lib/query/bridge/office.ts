import type { QueryClient } from "@tanstack/react-query";
import type { BackendMessageMap } from "@/lib/types/backend";
import type { OfficeEventPayload } from "@/lib/types/office-events";
import type { WebSocketClient } from "@/lib/ws/client";
import { qk } from "../keys";
import { registerBridgeHandlers, type QueryBridgeRegistration } from "./registrar";

type SessionStateChangedMessage = BackendMessageMap["session.state_changed"];

export function registerOfficeBridge(
  ws: WebSocketClient,
  queryClient: QueryClient,
): QueryBridgeRegistration {
  return registerBridgeHandlers(ws, queryClient, {
    "office.task.updated": (message) => {
      patchOfficeTask(queryClient, message.payload);
      invalidateTaskSurfaces(queryClient, message.payload);
      invalidateDashboard(queryClient, message.payload.workspace_id);
    },
    "office.task.created": (message) => {
      invalidateTaskSurfaces(queryClient, message.payload);
      invalidateProjects(queryClient, message.payload.workspace_id);
      invalidateDashboard(queryClient, message.payload.workspace_id);
    },
    "office.task.moved": (message) => {
      patchOfficeTask(queryClient, message.payload);
      invalidateTaskSurfaces(queryClient, message.payload);
      invalidateActivity(queryClient, message.payload.workspace_id);
      invalidateDashboard(queryClient, message.payload.workspace_id);
    },
    "office.task.status_changed": (message) => {
      patchOfficeTask(queryClient, message.payload);
      invalidateTaskSurfaces(queryClient, message.payload);
      invalidateDashboard(queryClient, message.payload.workspace_id);
    },
    "office.comment.created": (message) => {
      invalidateTaskDetail(queryClient, message.payload);
      invalidateTaskComments(queryClient, message.payload);
      invalidateActivity(queryClient, message.payload.workspace_id);
    },
    "office.task.decision_recorded": (message) => {
      invalidateTaskDetail(queryClient, message.payload);
      invalidateInbox(queryClient, message.payload.workspace_id);
    },
    "office.task.review_requested": (message) => {
      invalidateTaskDetail(queryClient, message.payload);
      invalidateInbox(queryClient, message.payload.workspace_id);
    },
    "office.agent.completed": (message) => {
      patchAgentStatus(queryClient, message.payload, "idle");
      invalidateAgentsDashboardRuns(queryClient, message.payload.workspace_id);
      invalidateActivity(queryClient, message.payload.workspace_id);
    },
    "office.agent.failed": (message) => {
      patchAgentStatus(queryClient, message.payload, "idle");
      invalidateAgentsDashboardRuns(queryClient, message.payload.workspace_id);
      invalidateInbox(queryClient, message.payload.workspace_id);
    },
    "office.agent.updated": (message) => {
      invalidateAgents(queryClient, message.payload.workspace_id);
      invalidateDashboard(queryClient, message.payload.workspace_id);
    },
    "office.approval.created": (message) => {
      invalidateInbox(queryClient, message.payload.workspace_id);
      invalidateDashboard(queryClient, message.payload.workspace_id);
    },
    "office.approval.resolved": (message) => {
      invalidateInbox(queryClient, message.payload.workspace_id);
      invalidateDashboard(queryClient, message.payload.workspace_id);
    },
    "office.cost.recorded": (message) => {
      invalidateCosts(queryClient, message.payload.workspace_id);
      invalidateDashboard(queryClient, message.payload.workspace_id);
    },
    "office.run.queued": (message) => {
      invalidateRunsAndTask(queryClient, message.payload);
    },
    "office.run.processed": (message) => {
      invalidateRunsAndTask(queryClient, message.payload);
    },
    "office.routine.triggered": (message) => {
      invalidateRoutines(queryClient, message.payload.workspace_id);
      invalidateActivity(queryClient, message.payload.workspace_id);
      invalidateDashboard(queryClient, message.payload.workspace_id);
    },
    "office.provider.health_changed": (message) => {
      patchProviderHealth(queryClient, message.payload);
      invalidateRoutingSurfaces(queryClient, message.payload.workspace_id);
    },
    "office.route_attempt.appended": (message) => {
      patchRouteAttempt(queryClient, message.payload);
    },
    "office.routing.settings_updated": (message) => {
      invalidateRoutingConfig(queryClient, message.payload.workspace_id);
    },
    "session.state_changed": (message) => {
      invalidateSessionDrivenOfficeSurfaces(queryClient, message);
    },
  });
}

function patchOfficeTask(queryClient: QueryClient, payload: OfficeEventPayload): void {
  const taskId = readId(payload.task_id ?? payload.id);
  if (!taskId) return;
  const patch = normalizeTaskPatch(payload);

  if (payload.workspace_id) {
    queryClient.setQueryData(qk.office.task(payload.workspace_id, taskId), (current: unknown) =>
      isRecord(current) ? { ...current, task: patchNestedTask(current.task, patch) } : current,
    );
    queryClient.setQueriesData(
      { queryKey: ["office", "workspaces", payload.workspace_id, "tasks"] },
      (current: unknown) => patchTaskPages(current, taskId, patch),
    );
  }
}

function patchAgentStatus(
  queryClient: QueryClient,
  payload: OfficeEventPayload,
  status: string,
): void {
  const agentId = readId(payload.agent_profile_id ?? payload.agent_id);
  if (!agentId || !payload.workspace_id) return;
  queryClient.setQueryData(qk.office.agents(payload.workspace_id), (current: unknown) => {
    if (!isRecord(current) || !Array.isArray(current.agents)) return current;
    return {
      ...current,
      agents: current.agents.map((agent) =>
        isRecord(agent) && agent.id === agentId ? { ...agent, status } : agent,
      ),
    };
  });
}

function patchProviderHealth(queryClient: QueryClient, payload: OfficeEventPayload): void {
  if (!payload.workspace_id || typeof payload.provider_id !== "string") return;
  const row = {
    provider_id: payload.provider_id,
    scope: typeof payload.scope === "string" ? payload.scope : "provider",
    scope_value: typeof payload.scope_value === "string" ? payload.scope_value : "",
    state: payload.state ?? "healthy",
    error_code: payload.error_code,
    retry_at: payload.retry_at,
    backoff_step: typeof payload.backoff_step === "number" ? payload.backoff_step : 0,
    last_failure: payload.last_failure,
    last_success: payload.last_success,
    raw_excerpt: payload.raw_excerpt,
    workspace_id: payload.workspace_id,
  };
  queryClient.setQueryData(qk.office.providerHealth(payload.workspace_id), (current: unknown) => {
    const health = isRecord(current) && Array.isArray(current.health) ? current.health : [];
    const next = [...health];
    const index = next.findIndex(
      (item) =>
        isRecord(item) &&
        item.provider_id === row.provider_id &&
        item.scope === row.scope &&
        item.scope_value === row.scope_value,
    );
    if (index >= 0) next[index] = row;
    else next.push(row);
    return { health: next };
  });
}

function patchRouteAttempt(queryClient: QueryClient, payload: OfficeEventPayload): void {
  const runId = readId(payload.run_id);
  const attempt = isRecord(payload.attempt) ? payload.attempt : null;
  if (!runId || !attempt) return;
  queryClient.setQueryData(qk.office.runAttempts(runId), (current: unknown) => {
    const attempts = isRecord(current) && Array.isArray(current.attempts) ? current.attempts : [];
    const next = [...attempts];
    const seq = attempt.seq;
    const index = next.findIndex((item) => isRecord(item) && item.seq === seq);
    if (index >= 0) next[index] = attempt;
    else next.push(attempt);
    return { attempts: next };
  });
}

function invalidateTaskSurfaces(queryClient: QueryClient, payload: OfficeEventPayload): void {
  invalidateWorkspaceFamily(queryClient, payload.workspace_id, "tasks");
  invalidateTaskDetail(queryClient, payload);
}

function invalidateTaskDetail(queryClient: QueryClient, payload: OfficeEventPayload): void {
  const taskId = readId(payload.task_id ?? payload.id);
  if (!taskId) return;
  if (payload.workspace_id) {
    queryClient.invalidateQueries({
      exact: true,
      queryKey: qk.office.task(payload.workspace_id, taskId),
    });
  } else {
    queryClient.invalidateQueries({ queryKey: ["office", "workspaces"] });
  }
}

function invalidateTaskComments(queryClient: QueryClient, payload: OfficeEventPayload): void {
  const taskId = readId(payload.task_id ?? payload.id);
  if (!taskId) return;
  queryClient.invalidateQueries({ exact: true, queryKey: qk.office.taskComments(taskId) });
}

function invalidateAgentsDashboardRuns(queryClient: QueryClient, workspaceId?: string): void {
  invalidateAgents(queryClient, workspaceId);
  invalidateDashboard(queryClient, workspaceId);
  invalidateWorkspaceFamily(queryClient, workspaceId, "runs");
}

function invalidateRunsAndTask(queryClient: QueryClient, payload: OfficeEventPayload): void {
  invalidateWorkspaceFamily(queryClient, payload.workspace_id, "runs");
  invalidateAgents(queryClient, payload.workspace_id);
  invalidateTaskDetail(queryClient, payload);
  invalidateTaskComments(queryClient, payload);
}

function invalidateSessionDrivenOfficeSurfaces(
  queryClient: QueryClient,
  message: SessionStateChangedMessage,
): void {
  const payload = message.payload as Record<string, unknown>;
  if (payload.new_state === payload.old_state) return;
  queryClient.invalidateQueries({
    queryKey: ["office", "workspaces"],
    predicate: isDashboardOrAgentsQuery,
  });
}

function invalidateDashboard(queryClient: QueryClient, workspaceId?: string): void {
  invalidateWorkspaceFamily(queryClient, workspaceId, "dashboard");
}

function invalidateAgents(queryClient: QueryClient, workspaceId?: string): void {
  invalidateWorkspaceFamily(queryClient, workspaceId, "agents");
}

function invalidateProjects(queryClient: QueryClient, workspaceId?: string): void {
  invalidateWorkspaceFamily(queryClient, workspaceId, "projects");
}

function invalidateInbox(queryClient: QueryClient, workspaceId?: string): void {
  invalidateWorkspaceFamily(queryClient, workspaceId, "inbox");
}

function invalidateActivity(queryClient: QueryClient, workspaceId?: string): void {
  invalidateWorkspaceFamily(queryClient, workspaceId, "activity");
}

function invalidateCosts(queryClient: QueryClient, workspaceId?: string): void {
  invalidateWorkspaceFamily(queryClient, workspaceId, "costs");
  invalidateWorkspaceFamily(queryClient, workspaceId, "costBreakdown");
  invalidateWorkspaceFamily(queryClient, workspaceId, "budgets");
}

function invalidateRoutines(queryClient: QueryClient, workspaceId?: string): void {
  invalidateWorkspaceFamily(queryClient, workspaceId, "routines");
  invalidateWorkspaceFamily(queryClient, workspaceId, "routineRuns");
}

function invalidateRoutingSurfaces(queryClient: QueryClient, workspaceId?: string): void {
  invalidateWorkspaceFamily(queryClient, workspaceId, "providerHealth");
  invalidateWorkspaceFamily(queryClient, workspaceId, "routingPreview");
  invalidateDashboard(queryClient, workspaceId);
}

function invalidateRoutingConfig(queryClient: QueryClient, workspaceId?: string): void {
  if (workspaceId) {
    queryClient.removeQueries({ exact: true, queryKey: qk.office.routing(workspaceId) });
    queryClient.invalidateQueries({ exact: true, queryKey: qk.office.routingPreview(workspaceId) });
  } else {
    queryClient.invalidateQueries({ queryKey: ["office", "workspaces"] });
  }
}

function invalidateWorkspaceFamily(
  queryClient: QueryClient,
  workspaceId: string | undefined,
  family: string,
): void {
  if (workspaceId) {
    queryClient.invalidateQueries({ queryKey: ["office", "workspaces", workspaceId, family] });
    return;
  }
  queryClient.invalidateQueries({
    queryKey: ["office", "workspaces"],
    predicate: (query) => query.queryKey[3] === family,
  });
}

function isDashboardOrAgentsQuery(query: { queryKey: readonly unknown[] }): boolean {
  return (
    query.queryKey[0] === "office" &&
    (query.queryKey[3] === "dashboard" || query.queryKey[3] === "agents")
  );
}

function patchNestedTask(currentTask: unknown, patch: Record<string, unknown>) {
  return isRecord(currentTask) ? { ...currentTask, ...patch } : currentTask;
}

function patchTaskPages(current: unknown, taskId: string, patch: Record<string, unknown>) {
  if (!isRecord(current) || !Array.isArray(current.pages)) return current;
  return {
    ...current,
    pages: current.pages.map((page) =>
      isRecord(page) && Array.isArray(page.tasks)
        ? {
            ...page,
            tasks: page.tasks.map((task) =>
              isRecord(task) && task.id === taskId ? { ...task, ...patch } : task,
            ),
          }
        : page,
    ),
  };
}

function normalizeTaskPatch(payload: OfficeEventPayload): Record<string, unknown> {
  const patch: Record<string, unknown> = {};
  copyField(payload, patch, "title", "title");
  copyField(payload, patch, "description", "description");
  copyField(payload, patch, "status", "status");
  copyField(payload, patch, "new_status", "status");
  copyField(payload, patch, "priority", "priority");
  copyField(payload, patch, "updated_at", "updatedAt");
  copyField(payload, patch, "assignee_agent_profile_id", "assigneeAgentProfileId");
  return patch;
}

function copyField(
  source: Record<string, unknown>,
  target: Record<string, unknown>,
  sourceKey: string,
  targetKey: string,
): void {
  if (source[sourceKey] !== undefined) target[targetKey] = source[sourceKey];
}

function readId(value: unknown): string | null {
  return typeof value === "string" && value.length > 0 ? value : null;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
