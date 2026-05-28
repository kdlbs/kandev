/**
 * Office domain WS → TanStack Query bridge.
 *
 * Mirrors lib/ws/handlers/office.ts 1:1, replacing store.setState() with
 * queryClient.setQueryData() immutable updaters.
 *
 * Strategy (same as the original):
 * - Task status/move: patch the tasks cache directly (fast, no network).
 * - Agent status changes: patch agents cache directly.
 * - New tasks, comments, approvals, dashboard data: invalidate the relevant
 *   key so TQ re-fetches on next access.
 * - Provider health: direct upsert into the health cache.
 * - Route attempts: append to the per-run attempts cache.
 *
 * Sub-registrar breakdown:
 *   registerTaskHandlers       — 7 task events (~40 LOC)
 *   registerAgentHandlers      — 3 agent events (~30 LOC)
 *   registerMiscHandlers       — 5 misc events (~40 LOC)
 *   registerRoutingHandlers    — 3 routing events (~40 LOC)
 *   registerOfficeBridge       — top-level aggregator
 */
import type { QueryClient } from "@tanstack/react-query";
import type { WebSocketClient } from "@/lib/ws/client";
import { qk } from "@/lib/query/keys";
import type { ProviderHealth, RouteAttempt, OfficeTask } from "@/lib/state/slices/office/types";

// ---------------------------------------------------------------------------
// Workspace guard (same logic as original)
// ---------------------------------------------------------------------------

type WorkspaceResolver = () => string | undefined;

function makeWorkspaceGuard(getActiveWsId: WorkspaceResolver) {
  return (payload: Record<string, unknown>): boolean => {
    const wsId = payload.workspace_id as string | undefined;
    const activeId = getActiveWsId();
    return !wsId || wsId === activeId;
  };
}

// ---------------------------------------------------------------------------
// Provider health cache updater (upsert by composite key)
// ---------------------------------------------------------------------------

function upsertHealthRow(
  prev: ProviderHealth[] | undefined,
  row: ProviderHealth,
): ProviderHealth[] {
  const list = prev ?? [];
  const idx = list.findIndex(
    (r) =>
      r.provider_id === row.provider_id &&
      r.scope === row.scope &&
      r.scope_value === row.scope_value,
  );
  if (idx >= 0) {
    const next = [...list];
    next[idx] = row;
    return next;
  }
  return [...list, row];
}

// ---------------------------------------------------------------------------
// Run attempts cache updater (upsert by seq)
// ---------------------------------------------------------------------------

function upsertRunAttempt(prev: RouteAttempt[] | undefined, attempt: RouteAttempt): RouteAttempt[] {
  const list = prev ?? [];
  const idx = list.findIndex((a) => a.seq === attempt.seq);
  if (idx >= 0) {
    const next = [...list];
    next[idx] = attempt;
    return next;
  }
  return [...list, attempt];
}

// ---------------------------------------------------------------------------
// Task cache patch helper (patch by id without refetch)
// ---------------------------------------------------------------------------

function patchTask(
  prev: OfficeTask[] | undefined,
  taskId: string,
  fields: Partial<OfficeTask>,
): OfficeTask[] | undefined {
  if (!prev) return prev;
  const idx = prev.findIndex((t) => t.id === taskId);
  if (idx < 0) return prev;
  const next = [...prev];
  next[idx] = { ...next[idx], ...fields };
  return next;
}

// ---------------------------------------------------------------------------
// normalizeTaskFields (same mapping as original handler)
// ---------------------------------------------------------------------------

function normalizeTaskFields(p: Record<string, unknown>): Partial<OfficeTask> {
  const out: Partial<OfficeTask> = {};
  if (p.title != null) out.title = p.title as string;
  if (p.description != null) out.description = p.description as string;
  if (p.status != null) out.status = p.status as OfficeTask["status"];
  if (p.new_status != null) out.status = p.new_status as OfficeTask["status"];
  if (p.priority != null) out.priority = p.priority as OfficeTask["priority"];
  if (p.updated_at != null) out.updatedAt = p.updated_at as string;
  if (p.assignee_agent_profile_id != null) {
    out.assigneeAgentProfileId = p.assignee_agent_profile_id as string;
  }
  return out;
}

// ---------------------------------------------------------------------------
// extractProviderHealth (same as original)
// ---------------------------------------------------------------------------

function extractProviderHealth(p: Record<string, unknown>): ProviderHealth | null {
  if (typeof p.provider_id !== "string" || typeof p.scope !== "string") return null;
  return {
    workspace_id: typeof p.workspace_id === "string" ? p.workspace_id : undefined,
    provider_id: p.provider_id,
    scope: p.scope as ProviderHealth["scope"],
    scope_value: typeof p.scope_value === "string" ? p.scope_value : "",
    state: (p.state as ProviderHealth["state"]) ?? "healthy",
    error_code: p.error_code as ProviderHealth["error_code"],
    retry_at: typeof p.retry_at === "string" ? p.retry_at : undefined,
    backoff_step: typeof p.backoff_step === "number" ? p.backoff_step : 0,
    last_failure: typeof p.last_failure === "string" ? p.last_failure : undefined,
    last_success: typeof p.last_success === "string" ? p.last_success : undefined,
    raw_excerpt: typeof p.raw_excerpt === "string" ? p.raw_excerpt : undefined,
  };
}

// ---------------------------------------------------------------------------
// Task handlers sub-registrar
// ---------------------------------------------------------------------------

type TaskHandlerDeps = {
  ws: WebSocketClient;
  qc: QueryClient;
  isCurrentWorkspace: (p: Record<string, unknown>) => boolean;
  getWsId: WorkspaceResolver;
  invalidateTasks: (wsId: string) => void;
  invalidateDashboard: (wsId: string) => void;
  patchTasksKey: (wsId: string, taskId: string, fields: Partial<OfficeTask>) => void;
};

function registerTaskMutationHandlers(d: TaskHandlerDeps): Array<() => void> {
  const unsubUpdated = d.ws.on("office.task.updated", (message) => {
    const p = message.payload;
    if (!d.isCurrentWorkspace(p)) return;
    const wsId = (p.workspace_id as string | undefined) ?? d.getWsId();
    if (!wsId) return;
    const taskId = (p.task_id ?? p.id) as string | undefined;
    if (!taskId) return;
    d.patchTasksKey(wsId, taskId, normalizeTaskFields(p));
    void d.qc.invalidateQueries({ queryKey: ["office", "tasks", taskId] as const });
    d.invalidateDashboard(wsId);
  });

  const unsubCreated = d.ws.on("office.task.created", (message) => {
    const p = message.payload;
    if (!d.isCurrentWorkspace(p)) return;
    const wsId = (p.workspace_id as string | undefined) ?? d.getWsId();
    if (!wsId) return;
    d.invalidateTasks(wsId);
    d.invalidateDashboard(wsId);
  });

  const unsubMoved = d.ws.on("office.task.moved", (message) => {
    const p = message.payload;
    if (!d.isCurrentWorkspace(p)) return;
    const wsId = (p.workspace_id as string | undefined) ?? d.getWsId();
    if (!wsId) return;
    const taskId = (p.task_id ?? p.id) as string | undefined;
    const newStatus = p.new_status as string | undefined;
    if (!taskId || !newStatus) {
      d.invalidateTasks(wsId);
      d.invalidateDashboard(wsId);
      return;
    }
    d.patchTasksKey(wsId, taskId, { status: newStatus as OfficeTask["status"] });
    d.invalidateDashboard(wsId);
    void d.qc.invalidateQueries({ queryKey: qk.office.activity(wsId) });
  });

  const unsubStatusChanged = d.ws.on("office.task.status_changed", (message) => {
    const p = message.payload;
    if (!d.isCurrentWorkspace(p)) return;
    const wsId = (p.workspace_id as string | undefined) ?? d.getWsId();
    if (!wsId) return;
    const taskId = (p.task_id ?? p.id) as string | undefined;
    const newStatus = p.new_status as string | undefined;
    if (!taskId || !newStatus) {
      d.invalidateTasks(wsId);
      return;
    }
    d.patchTasksKey(wsId, taskId, { status: newStatus as OfficeTask["status"] });
    d.invalidateDashboard(wsId);
  });

  return [unsubUpdated, unsubCreated, unsubMoved, unsubStatusChanged];
}

function registerTaskNotificationHandlers(d: TaskHandlerDeps): Array<() => void> {
  const unsubDecision = d.ws.on("office.task.decision_recorded", (message) => {
    const p = message.payload;
    if (!d.isCurrentWorkspace(p)) return;
    const wsId = (p.workspace_id as string | undefined) ?? d.getWsId();
    if (!wsId) return;
    void d.qc.invalidateQueries({ queryKey: ["office", wsId, "inbox"] as const });
  });

  const unsubReviewRequested = d.ws.on("office.task.review_requested", (message) => {
    const p = message.payload;
    if (!d.isCurrentWorkspace(p)) return;
    const wsId = (p.workspace_id as string | undefined) ?? d.getWsId();
    if (!wsId) return;
    void d.qc.invalidateQueries({ queryKey: ["office", wsId, "inbox"] as const });
  });

  const unsubComment = d.ws.on("office.comment.created", (message) => {
    const p = message.payload;
    if (!d.isCurrentWorkspace(p)) return;
    const taskId = p.task_id as string | undefined;
    if (taskId) {
      void d.qc.invalidateQueries({ queryKey: ["office", "tasks", taskId] as const });
    }
  });

  return [unsubDecision, unsubReviewRequested, unsubComment];
}

function registerTaskHandlers(
  ws: WebSocketClient,
  qc: QueryClient,
  isCurrentWorkspace: (p: Record<string, unknown>) => boolean,
  getWsId: WorkspaceResolver,
): Array<() => void> {
  const invalidateTasks = (wsId: string) =>
    void qc.invalidateQueries({ queryKey: qk.office.tasks(wsId) });
  const invalidateDashboard = (wsId: string) =>
    void qc.invalidateQueries({ queryKey: qk.office.dashboard(wsId) });
  const patchTasksKey = (wsId: string, taskId: string, fields: Partial<OfficeTask>) =>
    qc.setQueriesData<OfficeTask[]>({ queryKey: qk.office.tasks(wsId) }, (prev) =>
      patchTask(prev, taskId, fields),
    );

  const deps: TaskHandlerDeps = {
    ws,
    qc,
    isCurrentWorkspace,
    getWsId,
    invalidateTasks,
    invalidateDashboard,
    patchTasksKey,
  };
  return [...registerTaskMutationHandlers(deps), ...registerTaskNotificationHandlers(deps)];
}

// ---------------------------------------------------------------------------
// Agent handlers sub-registrar
// ---------------------------------------------------------------------------

function registerAgentHandlers(
  ws: WebSocketClient,
  qc: QueryClient,
  isCurrentWorkspace: (p: Record<string, unknown>) => boolean,
  getWsId: WorkspaceResolver,
): Array<() => void> {
  const invalidateAgents = (wsId: string) =>
    void qc.invalidateQueries({ queryKey: qk.office.agents(wsId) });
  const invalidateDashboard = (wsId: string) =>
    void qc.invalidateQueries({ queryKey: qk.office.dashboard(wsId) });

  const unsubCompleted = ws.on("office.agent.completed", (message) => {
    const p = message.payload;
    if (!isCurrentWorkspace(p)) return;
    const wsId = (p.workspace_id as string | undefined) ?? getWsId();
    if (!wsId) return;
    invalidateDashboard(wsId);
    invalidateAgents(wsId);
    void qc.invalidateQueries({ queryKey: qk.office.activity(wsId) });
  });

  const unsubFailed = ws.on("office.agent.failed", (message) => {
    const p = message.payload;
    if (!isCurrentWorkspace(p)) return;
    const wsId = (p.workspace_id as string | undefined) ?? getWsId();
    if (!wsId) return;
    invalidateDashboard(wsId);
    invalidateAgents(wsId);
  });

  const unsubUpdated = ws.on("office.agent.updated", (message) => {
    const p = message.payload;
    if (!isCurrentWorkspace(p)) return;
    const wsId = (p.workspace_id as string | undefined) ?? getWsId();
    if (!wsId) return;
    invalidateAgents(wsId);
  });

  // `session.state_changed` doesn't carry workspace_id — it's fanned out
  // from the agent-session WS handler whenever a task session transitions
  // state (RUNNING / IDLE / FAILED). The office dashboard's agent_summaries
  // and per-agent live indicators depend on these transitions, so invalidate
  // dashboard + agents for the active workspace. `new_state` is undefined for
  // adjacent events (agentctl status, context window, model updates) that
  // reuse the same channel — skip those to avoid invalidation storms.
  const unsubSessionState = ws.on("session.state_changed", (message) => {
    if (message.payload?.new_state === undefined) return;
    const wsId = getWsId();
    if (!wsId) return;
    invalidateAgents(wsId);
    invalidateDashboard(wsId);
  });

  return [unsubCompleted, unsubFailed, unsubUpdated, unsubSessionState];
}

// ---------------------------------------------------------------------------
// Misc handlers sub-registrar (approvals, costs, runs, routines)
// ---------------------------------------------------------------------------

function registerMiscHandlers(
  ws: WebSocketClient,
  qc: QueryClient,
  isCurrentWorkspace: (p: Record<string, unknown>) => boolean,
  getWsId: WorkspaceResolver,
): Array<() => void> {
  const invalidateInbox = (wsId: string) =>
    void qc.invalidateQueries({ queryKey: ["office", wsId, "inbox"] as const });
  const invalidateRuns = (wsId: string) =>
    void qc.invalidateQueries({ queryKey: qk.office.runs(wsId) });
  const invalidateAgents = (wsId: string) =>
    void qc.invalidateQueries({ queryKey: qk.office.agents(wsId) });
  const invalidateDashboard = (wsId: string) =>
    void qc.invalidateQueries({ queryKey: qk.office.dashboard(wsId) });

  const unsubApprovalCreated = ws.on("office.approval.created", (message) => {
    const p = message.payload;
    if (!isCurrentWorkspace(p)) return;
    const wsId = (p.workspace_id as string | undefined) ?? getWsId();
    if (!wsId) return;
    invalidateInbox(wsId);
    invalidateDashboard(wsId);
  });

  const unsubApprovalResolved = ws.on("office.approval.resolved", (message) => {
    const p = message.payload;
    if (!isCurrentWorkspace(p)) return;
    const wsId = (p.workspace_id as string | undefined) ?? getWsId();
    if (!wsId) return;
    invalidateInbox(wsId);
    void qc.invalidateQueries({ queryKey: qk.office.approvals(wsId) });
  });

  const unsubCostRecorded = ws.on("office.cost.recorded", (message) => {
    const p = message.payload;
    if (!isCurrentWorkspace(p)) return;
    const wsId = (p.workspace_id as string | undefined) ?? getWsId();
    if (!wsId) return;
    void qc.invalidateQueries({ queryKey: ["office", wsId, "costs"] as const });
    invalidateDashboard(wsId);
  });

  const unsubRunQueued = ws.on("office.run.queued", (message) => {
    const p = message.payload;
    if (!isCurrentWorkspace(p)) return;
    const wsId = (p.workspace_id as string | undefined) ?? getWsId();
    if (!wsId) return;
    invalidateRuns(wsId);
    invalidateAgents(wsId);
  });

  const unsubRunProcessed = ws.on("office.run.processed", (message) => {
    const p = message.payload;
    if (!isCurrentWorkspace(p)) return;
    const wsId = (p.workspace_id as string | undefined) ?? getWsId();
    if (!wsId) return;
    invalidateRuns(wsId);
    invalidateAgents(wsId);
  });

  const unsubRoutineTriggered = ws.on("office.routine.triggered", (message) => {
    const p = message.payload;
    if (!isCurrentWorkspace(p)) return;
    const wsId = (p.workspace_id as string | undefined) ?? getWsId();
    if (!wsId) return;
    void qc.invalidateQueries({ queryKey: qk.office.routines(wsId) });
    void qc.invalidateQueries({ queryKey: qk.office.activity(wsId) });
  });

  return [
    unsubApprovalCreated,
    unsubApprovalResolved,
    unsubCostRecorded,
    unsubRunQueued,
    unsubRunProcessed,
    unsubRoutineTriggered,
  ];
}

// ---------------------------------------------------------------------------
// Routing handlers sub-registrar
// ---------------------------------------------------------------------------

function registerRoutingHandlers(
  ws: WebSocketClient,
  qc: QueryClient,
  isCurrentWorkspace: (p: Record<string, unknown>) => boolean,
  getWsId: WorkspaceResolver,
): Array<() => void> {
  const unsubHealthChanged = ws.on("office.provider.health_changed", (message) => {
    const p = message.payload;
    if (!isCurrentWorkspace(p)) return;
    const wsId = (p.workspace_id as string | undefined) ?? getWsId();
    if (!wsId) return;
    const row = extractProviderHealth(p);
    if (!row) return;
    qc.setQueryData<ProviderHealth[]>(qk.office.providerHealth(wsId), (prev) =>
      upsertHealthRow(prev, row),
    );
  });

  const unsubRouteAttempt = ws.on("office.route_attempt.appended", (message) => {
    const p = message.payload;
    if (!isCurrentWorkspace(p)) return;
    const runId = p.run_id as string | undefined;
    const attempt = p.attempt as RouteAttempt | undefined;
    if (!runId || !attempt) return;
    qc.setQueryData<RouteAttempt[]>(["office", "runs", runId, "attempts"] as const, (prev) =>
      upsertRunAttempt(prev, attempt),
    );
  });

  const unsubSettingsUpdated = ws.on("office.routing.settings_updated", (message) => {
    const p = message.payload;
    if (!isCurrentWorkspace(p)) return;
    const wsId = (p.workspace_id as string | undefined) ?? getWsId();
    if (!wsId) return;
    void qc.invalidateQueries({ queryKey: ["office", wsId, "routing"] as const });
    // Also invalidate the routing preview since it derives from routing config.
    void qc.invalidateQueries({ queryKey: ["office", wsId, "routingPreview"] as const });
  });

  return [unsubHealthChanged, unsubRouteAttempt, unsubSettingsUpdated];
}

// ---------------------------------------------------------------------------
// Top-level bridge registrar
// ---------------------------------------------------------------------------

/**
 * Registers WS handlers for office domain events.
 *
 * Requires a `getActiveWorkspaceId` resolver so the bridge can filter
 * cross-workspace events without depending on the Zustand store directly.
 * The QueryProvider passes `() => getBrowserQueryClient()` for this.
 *
 * To wire workspace filtering without Zustand: the caller provides a getter
 * that reads the active workspace from whatever source is available
 * (Zustand UI slice, URL params, etc.).
 */
export function registerOfficeBridge(
  ws: WebSocketClient,
  qc: QueryClient,
  getActiveWorkspaceId: WorkspaceResolver,
): () => void {
  const isCurrentWorkspace = makeWorkspaceGuard(getActiveWorkspaceId);

  const taskUnsubs = registerTaskHandlers(ws, qc, isCurrentWorkspace, getActiveWorkspaceId);
  const agentUnsubs = registerAgentHandlers(ws, qc, isCurrentWorkspace, getActiveWorkspaceId);
  const miscUnsubs = registerMiscHandlers(ws, qc, isCurrentWorkspace, getActiveWorkspaceId);
  const routingUnsubs = registerRoutingHandlers(ws, qc, isCurrentWorkspace, getActiveWorkspaceId);

  const all = [...taskUnsubs, ...agentUnsubs, ...miscUnsubs, ...routingUnsubs];
  return () => {
    for (const unsub of all) unsub();
  };
}
