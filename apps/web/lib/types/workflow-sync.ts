/** The repository host workflow sync fetches definitions from. */
export type WorkflowSyncProvider = "github" | "gitlab";

/**
 * Per-workspace workflow sync configuration: a repo + branch + directory
 * containing workflow export files (`.yml`/`.yaml`/`.json` in the
 * `kandev_workflow` format). The backend polls the directory on
 * `interval_seconds` cadence and syncs workflows; `last_*` fields report the
 * outcome of the most recent sync attempt (poller or forced).
 *
 * `provider` selects the source: `"github"` uses `repo_owner` + `repo_name`;
 * `"gitlab"` uses `project_path` (a namespace path, e.g. "group/project" or
 * "group/subgroup/project") and leaves `repo_owner`/`repo_name` empty. GitLab
 * sync always uses the workspace's own configured GitLab connection — there
 * is no host field here.
 */
export interface WorkflowSyncConfig {
  workspace_id: string;
  provider: WorkflowSyncProvider;
  repo_owner: string;
  repo_name: string;
  project_path: string;
  branch: string;
  path: string;
  interval_seconds: number;
  /** When false, the workspace only syncs via "Sync now". */
  poll_enabled: boolean;
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
 * (`main`, `.kandev/workflows`, 300s / min 60s) when omitted. `provider`
 * omitted means `"github"`, matching pre-GitLab clients. GitHub requires
 * `repo_owner`/`repo_name`; GitLab requires `project_path` instead.
 */
export interface WorkflowSyncSetConfigRequest {
  provider?: WorkflowSyncProvider;
  repo_owner?: string;
  repo_name?: string;
  project_path?: string;
  branch?: string;
  path?: string;
  interval_seconds?: number;
  /** Defaults to true server-side when omitted. */
  poll_enabled?: boolean;
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
