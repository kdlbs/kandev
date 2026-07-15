/**
 * Per-workspace workflow sync configuration: a GitHub repo + branch +
 * directory containing workflow export files (`.yml`/`.yaml`/`.json` in the
 * `kandev_workflow` format). The backend polls the directory on
 * `interval_seconds` cadence and syncs workflows; `last_*` fields report the
 * outcome of the most recent sync attempt (poller or forced).
 */
export interface WorkflowSyncConfig {
  workspace_id: string;
  repo_owner: string;
  repo_name: string;
  branch: string;
  path: string;
  interval_seconds: number;
  /** RFC3339 timestamp; absent until the first sync attempt. */
  last_synced_at?: string;
  last_ok: boolean;
  last_error?: string;
  last_warnings?: string[];
  created_at: string;
  updated_at: string;
}

/**
 * Payload for creating or updating a workspace's workflow sync config.
 * `branch`, `path`, and `interval_seconds` fall back to server-side defaults
 * (`main`, `.kandev/workflows`, 300s / min 60s) when omitted.
 */
export interface WorkflowSyncSetConfigRequest {
  repo_owner: string;
  repo_name: string;
  branch?: string;
  path?: string;
  interval_seconds?: number;
}

/** Outcome of a single sync run (poller or forced). */
export interface WorkflowSyncResult {
  created: string[];
  updated: string[];
  deleted: string[];
  warnings: string[];
  unchanged: boolean;
}

/**
 * Response from a forced sync. A failed sync still responds 200 with `error`
 * set and `config.last_ok === false` — the request itself only rejects (404)
 * when no config exists for the workspace.
 */
export interface WorkflowSyncForceSyncResponse {
  config: WorkflowSyncConfig;
  result?: WorkflowSyncResult;
  error?: string;
}
