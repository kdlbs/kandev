import { fetchJson, type ApiRequestOptions } from "@/lib/api/client";
import { coordinatorPath } from "@/lib/api/domains/coordinator-api";

// Mirrors internal/coordinator/autonomy_read.go (wake-screens.md#autonomy-read).
export type TurnRead = {
  id: string;
  coordinator_id: string;
  conversation_task_id: string;
  session_id: string;
  started_at: string;
  finished_at: string | null;
  outcome: string | null;
  wake_count: number;
  denied_permissions: number;
  cost_subcents: number | null;
  stop_requested_at: string | null;
  stop_state: "stop_failing" | null;
};

export type RunWake = {
  id: string;
  kind: string;
  task_id: string;
  task_identifier: string | null;
  task_title: string | null;
};

export type RunRead = TurnRead & { wakes: RunWake[] };

export type AutonomyAdmission = {
  ok: boolean;
  reason?: string;
  detail: string;
  until?: string;
};

export type AutonomyCondition = { name: string; met: boolean; detail: string };

export type AutonomySpend = {
  measurable: boolean;
  degraded: boolean;
  window_subcents: number | null;
  mean_daily_subcents_7d: number | null;
  mean_known: boolean;
  ceiling_subcents: number | null;
};

// The paused state the autonomy read and the coordinator read carry, whatever
// the phase 3.1 flag. paused_by is null when the manager is not readable.
export type PauseState = {
  paused?: boolean;
  paused_at?: string | null;
  paused_by?: { id: string; name: string } | null;
};

export type AutonomyRead = PauseState & {
  server_time: string;
  autonomy_enabled: boolean;
  admission?: AutonomyAdmission;
  pending_wakes: number;
  oldest_pending_at: string | null;
  last_woke_at: string | null;
  last_turn: TurnRead | null;
  containment: { conditions: AutonomyCondition[] };
  spend: AutonomySpend;
};

export function putCoordinatorPause(
  workspaceId: string,
  coordinatorId: string,
  paused: boolean,
  options?: ApiRequestOptions,
): Promise<PauseState> {
  return fetchJson<PauseState>(coordinatorPath(workspaceId, coordinatorId, "/pause"), {
    ...options,
    init: {
      ...options?.init,
      method: "PUT",
      headers: { "Content-Type": "application/json", ...options?.init?.headers },
      body: JSON.stringify({ paused }),
    },
  });
}

export function getAutonomy(
  workspaceId: string,
  coordinatorId: string,
  options?: ApiRequestOptions,
): Promise<AutonomyRead> {
  return fetchJson<AutonomyRead>(coordinatorPath(workspaceId, coordinatorId, "/autonomy"), options);
}

export function getRun(
  workspaceId: string,
  coordinatorId: string,
  runId: string,
  options?: ApiRequestOptions,
): Promise<RunRead> {
  return fetchJson<RunRead>(
    coordinatorPath(workspaceId, coordinatorId, `/runs/${encodeURIComponent(runId)}`),
    options,
  );
}
