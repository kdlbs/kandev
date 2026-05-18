import { fetchJson, type ApiRequestOptions } from "../client";
import type { RouteAttempt, Run } from "@/lib/state/slices/office/types";

const BASE = "/api/v1/office";

export function listRuns(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ runs: Run[] }>(`${BASE}/workspaces/${workspaceId}/runs`, options);
}

// --- Per-agent runs (paginated list + per-run detail) ---

export type AgentRunSummary = {
  id: string;
  id_short: string;
  reason: string;
  status: "queued" | "claimed" | "finished" | "failed" | "cancelled";
  cancel_reason?: string;
  error_message?: string;
  task_id?: string;
  comment_id?: string;
  routine_id?: string;
  requested_at: string;
  claimed_at?: string;
  finished_at?: string;
  duration_ms?: number;
};

export type AgentRunsListPage = {
  runs: AgentRunSummary[];
  next_cursor: string;
  next_id?: string;
};

export type RunCostSummary = {
  input_tokens: number;
  output_tokens: number;
  cached_tokens: number;
  cost_subcents: number;
};

export type RunInvocationDetail = {
  adapter?: string;
  model?: string;
  working_dir?: string;
  command?: string;
  env?: Record<string, string>;
};

export type RunSessionDetail = {
  session_id?: string;
  session_id_before?: string;
  session_id_after?: string;
};

export type RunRuntimeDetail = {
  capabilities: Record<string, unknown>;
  input_snapshot: Record<string, unknown>;
  session_id?: string;
  skills: Array<{
    skill_id: string;
    version: string;
    content_hash: string;
    materialized_path: string;
  }>;
};

export type RunEvent = {
  seq: number;
  event_type: string;
  level: string;
  payload: string;
  created_at: string;
};

// RunRouting carries the per-run routing snapshot + attempt history.
// Present only when the run went through the provider-routing path;
// legacy concrete-profile runs omit this block.
export type RunRouting = {
  logical_provider_order: string[];
  requested_tier?: string;
  resolved_provider_id?: string;
  resolved_model?: string;
  blocked_status?: string;
  earliest_retry_at?: string;
  attempts: RouteAttempt[];
};

export type RunDetail = {
  id: string;
  id_short: string;
  agent_id: string;
  reason: string;
  status: "queued" | "claimed" | "finished" | "failed" | "cancelled";
  cancel_reason?: string;
  error_message?: string;
  task_id?: string;
  requested_at: string;
  claimed_at?: string;
  finished_at?: string;
  duration_ms?: number;
  costs: RunCostSummary;
  session: RunSessionDetail;
  invocation: RunInvocationDetail;
  runtime: RunRuntimeDetail;
  tasks_touched: string[];
  events: RunEvent[];
  // Heartbeat-rework PR 1 inspection fields. assembled_prompt + summary_injected
  // are populated by the dispatcher; result_json / context_snapshot /
  // output_summary mirror the persisted run row columns.
  assembled_prompt?: string;
  summary_injected?: string;
  result_json?: string;
  context_snapshot?: string;
  output_summary?: string;
  // Routing snapshot — omitted for legacy concrete-profile runs.
  routing?: RunRouting;
};

export function listAgentRuns(
  agentId: string,
  params?: { cursor?: string; cursorId?: string; limit?: number },
  options?: ApiRequestOptions,
) {
  const qs = new URLSearchParams();
  if (params?.cursor) qs.set("cursor", params.cursor);
  if (params?.cursorId) qs.set("cursor_id", params.cursorId);
  if (params?.limit !== undefined) qs.set("limit", String(params.limit));
  const suffix = qs.toString() ? `?${qs.toString()}` : "";
  return fetchJson<AgentRunsListPage>(`${BASE}/agents/${agentId}/runs${suffix}`, options);
}

export function getRunDetail(agentId: string, runId: string, options?: ApiRequestOptions) {
  return fetchJson<RunDetail>(`${BASE}/agents/${agentId}/runs/${runId}`, options);
}

export type RouteAttemptsResponse = { attempts: RouteAttempt[] };

export function getRunAttempts(runId: string, options?: ApiRequestOptions) {
  return fetchJson<RouteAttemptsResponse>(`${BASE}/runs/${runId}/attempts`, options);
}

// --- Agent dashboard summary ---

export type AgentLatestRun = {
  run_id: string;
  run_id_short: string;
  status: string;
  reason: string;
  task_id?: string;
  summary?: string;
  requested_at: string;
  finished_at?: string;
};

export type AgentRunActivityDay = {
  date: string;
  succeeded: number;
  failed: number;
  other: number;
  total: number;
};

export type AgentTaskPriorityDay = {
  date: string;
  critical: number;
  high: number;
  medium: number;
  low: number;
};

export type AgentTaskStatusDay = {
  date: string;
  todo: number;
  in_progress: number;
  in_review: number;
  done: number;
  blocked: number;
  cancelled: number;
  backlog: number;
};

export type AgentSuccessRateDay = {
  date: string;
  succeeded: number;
  total: number;
};

export type AgentRecentTask = {
  task_id: string;
  identifier: string;
  title: string;
  status: string;
  last_active_at: string;
};

export type AgentCostAggregate = {
  input_tokens: number;
  output_tokens: number;
  cached_tokens: number;
  total_cost_subcents: number;
};

export type AgentRunCost = {
  run_id: string;
  run_id_short: string;
  date: string;
  input_tokens: number;
  output_tokens: number;
  cost_subcents: number;
};

export type AgentSummaryResponse = {
  agent_id: string;
  days: number;
  latest_run: AgentLatestRun | null;
  run_activity: AgentRunActivityDay[];
  tasks_by_priority: AgentTaskPriorityDay[];
  tasks_by_status: AgentTaskStatusDay[];
  success_rate: AgentSuccessRateDay[];
  recent_tasks: AgentRecentTask[];
  cost_aggregate: AgentCostAggregate;
  recent_run_costs: AgentRunCost[];
};

/**
 * Fetch the agent dashboard summary for the given agent. `days`
 * defaults to 14 server-side; the backend clamps to [1, 90].
 */
export function getAgentSummary(agentId: string, days?: number, options?: ApiRequestOptions) {
  const qs = days !== undefined ? `?days=${days}` : "";
  return fetchJson<AgentSummaryResponse>(`${BASE}/agents/${agentId}/summary${qs}`, options);
}
