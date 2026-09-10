/** The repository host Office config sync fetches definitions from. */
export type OfficeConfigSyncProvider = "github" | "gitlab";

/**
 * Per-workspace Office config sync configuration: a repo + branch + directory
 * containing agent/skill/project/routine definition files. The backend polls
 * the directory on `interval_seconds` cadence and reconciles Office's tables
 * with it; `last_*` fields report the outcome of the most recent sync attempt
 * (poller or forced).
 *
 * `provider` selects the source: `"github"` uses `repo_owner` + `repo_name`;
 * `"gitlab"` uses `project_path` (a namespace path, e.g. "group/project") and
 * leaves `repo_owner`/`repo_name` empty. `path` is the repository-root when
 * empty — this is a real value here, not merely "unset".
 */
export interface OfficeConfigSyncConfig {
  workspace_id: string;
  provider: OfficeConfigSyncProvider;
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
 * Payload for creating or updating a workspace's config sync configuration.
 * `branch` and `interval_seconds` fall back to server-side defaults (`main`,
 * 300s / min 60s) when omitted; `provider` omitted means `"github"`. GitHub
 * requires `repo_owner`/`repo_name`; GitLab requires `project_path` instead.
 *
 * `path` is a *tri-state* field, unlike the rest: omitting it (`undefined`)
 * means "use the default", which for Office config sync IS the repository
 * root, while sending `""` means the repository root *explicitly* — the
 * backend keeps these apart (`SetConfigRequest.Path *string`) so that
 * re-saving a root-addressed config round-trips instead of colliding with a
 * different default. A form that always has a concrete directory value in
 * mind should send `path: ""` for root, not omit the field.
 */
export interface OfficeConfigSyncSetConfigRequest {
  provider?: OfficeConfigSyncProvider;
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
export interface OfficeConfigSyncResult {
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
export interface OfficeConfigSyncForceSyncResponse {
  config: OfficeConfigSyncConfig;
  result?: OfficeConfigSyncResult;
  error?: string;
}
