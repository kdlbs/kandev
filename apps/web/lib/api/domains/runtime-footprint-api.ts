import { fetchJson, type ApiRequestOptions } from "../client";

/**
 * One live agent runtime's footprint as the backend measured it.
 *
 * `unreadableProcesses` is the lower-bound marker: when it is greater than zero
 * the byte totals cover only the processes the backend could actually measure,
 * so they must be presented as a lower bound rather than as a total.
 */
export type RuntimeFootprintRow = {
  sessionId: string;
  taskId: string;
  status?: string;
  processes: number;
  committedBytes: number;
  residentBytes: number;
  unreadableProcesses: number;
  lastActivityAt: string;
};

/** One workspace's view of the installation's live agent runtimes. */
export type WorkspaceRuntimeFootprint = {
  liveRuntimes: number;
  workspaceRuntimes: number;
  processes: number;
  committedBytes: number;
  residentBytes: number;
  unreadableRuntimes: number;
  complete: boolean;
  observedAt: string;
  runtimes: RuntimeFootprintRow[];
};

type WorkspaceRuntimeFootprintDTO = {
  live_runtimes: number;
  workspace_runtimes: number;
  processes: number;
  committed_bytes: number;
  resident_bytes: number;
  unreadable_runtimes: number;
  complete: boolean;
  observed_at: string;
  runtimes: {
    session_id: string;
    task_id: string;
    status?: string;
    processes: number;
    committed_bytes: number;
    resident_bytes: number;
    unreadable_processes: number;
    last_activity_at: string;
  }[];
};

function normalizeRow(row: WorkspaceRuntimeFootprintDTO["runtimes"][number]): RuntimeFootprintRow {
  return {
    sessionId: row.session_id,
    taskId: row.task_id,
    status: row.status,
    processes: row.processes,
    committedBytes: row.committed_bytes,
    residentBytes: row.resident_bytes,
    unreadableProcesses: row.unreadable_processes,
    lastActivityAt: row.last_activity_at,
  };
}

/**
 * Fetches the live agent-runtime footprint for one workspace.
 *
 * The request performs one bounded observation. An observation that could not
 * be taken arrives with `complete: false` and no rows, which the caller must
 * present as "not measured" rather than as "no memory in use".
 */
export async function fetchWorkspaceRuntimeFootprint(
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<WorkspaceRuntimeFootprint> {
  const dto = await fetchJson<WorkspaceRuntimeFootprintDTO>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/runtime-footprint`,
    options,
  );
  return {
    liveRuntimes: dto.live_runtimes,
    workspaceRuntimes: dto.workspace_runtimes,
    processes: dto.processes,
    committedBytes: dto.committed_bytes,
    residentBytes: dto.resident_bytes,
    unreadableRuntimes: dto.unreadable_runtimes,
    complete: dto.complete,
    observedAt: dto.observed_at,
    runtimes: (dto.runtimes ?? []).map(normalizeRow),
  };
}

/**
 * Reports whether the byte totals can be read as a complete measurement.
 *
 * A partial measurement and a platform that cannot report commit charge both
 * mean the same thing to a reader: the number shown is a floor, not a total.
 * Committed bytes are zero on Linux and other unix platforms because the OS
 * does not expose commit charge per process, and estimating it from resident
 * bytes would invent a number the backend never measured.
 */
export function isFootprintPartial(footprint: WorkspaceRuntimeFootprint): boolean {
  return footprint.unreadableRuntimes > 0;
}

/** Reports whether this platform reported commit charge at all. */
export function reportsCommittedBytes(footprint: WorkspaceRuntimeFootprint): boolean {
  return footprint.committedBytes > 0;
}
