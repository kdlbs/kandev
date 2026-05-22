/**
 * Connection status for the GitLab integration. Returned by
 * `GET /api/v1/gitlab/status` and shaped by `internal/gitlab.Status`.
 */
export type GitLabStatus = {
  authenticated: boolean;
  username: string;
  auth_method: "glab_cli" | "pat" | "none" | "mock";
  host: string;
  token_configured: boolean;
  token_secret_id?: string;
  glab_version?: string;
  glab_outdated?: boolean;
  required_scopes: string[];
  diagnostics?: GitLabAuthDiagnostics;
  /**
   * Transport-layer error from the most recent `IsAuthenticated` probe
   * (network down, 5xx, parse failure). Distinct from `authenticated:
   * false` with no `connection_error`, which means a 401/403 / no token
   * configured. Present so the UI can render "GitLab unreachable" instead
   * of "not connected" during an outage and stop users from rotating an
   * actually-valid token.
   */
  connection_error?: string;
};

export type GitLabAuthDiagnostics = {
  command: string;
  output: string;
  exit_code: number;
};

/**
 * Task ↔ MR association — parallel to github's TaskPR. Surfaces what the MR
 * topbar button needs to render (state + counts + a click target). Backend
 * row in `gitlab_task_mrs`.
 */
export type TaskMR = {
  id: string;
  task_id: string;
  repository_id?: string;
  host: string;
  project_path: string;
  mr_iid: number;
  mr_url: string;
  mr_title: string;
  head_branch: string;
  base_branch: string;
  author_username: string;
  state: "open" | "closed" | "merged" | "locked" | string;
  approval_state: "" | "approved" | "pending" | string;
  pipeline_state: "" | "success" | "failure" | "pending" | string;
  merge_status: string;
  draft: boolean;
  approval_count: number;
  required_approvals: number;
  pipeline_jobs_total: number;
  pipeline_jobs_pass: number;
  created_at: string;
  merged_at?: string;
  closed_at?: string;
  last_synced_at?: string;
  updated_at: string;
};

/** Response shape for `GET /api/v1/gitlab/workspaces/:id/task-mrs`. */
export type TaskMRsResponse = {
  task_mrs: Record<string, TaskMR[]>;
};

/** Merge request returned by /api/v1/gitlab/user/mrs (matches backend MR). */
export type MR = {
  id: number;
  iid: number;
  project_id: number;
  title: string;
  url: string;
  web_url: string;
  state: "open" | "closed" | "merged" | "locked" | "opened" | string;
  head_branch: string;
  head_sha: string;
  base_branch: string;
  author_username: string;
  project_namespace: string;
  project_path: string;
  body: string;
  draft: boolean;
  merge_status: string;
  has_conflicts: boolean;
  additions: number;
  deletions: number;
  reviewers: { username: string; name: string; type: string }[];
  assignees: { username: string; name: string; type: string }[];
  created_at: string;
  updated_at: string;
  merged_at?: string;
  closed_at?: string;
};

/** Issue returned by /api/v1/gitlab/user/issues. */
export type Issue = {
  id: number;
  iid: number;
  project_id: number;
  title: string;
  body: string;
  url: string;
  web_url: string;
  state: "opened" | "closed" | string;
  author_username: string;
  project_namespace: string;
  project_path: string;
  labels: string[];
  assignees: string[];
  created_at: string;
  updated_at: string;
  closed_at?: string;
};

export type MRSearchPage = {
  mrs: MR[];
  total_count: number;
  page: number;
  per_page: number;
};

export type IssueSearchPage = {
  issues: Issue[];
  total_count: number;
  page: number;
  per_page: number;
};

export type GitLabConfigureTokenResponse = { configured: boolean };
export type GitLabClearTokenResponse = { cleared: boolean };
export type GitLabConfigureHostResponse = { configured: boolean; host: string };

export type GitLabMRNote = {
  id: number;
  author: string;
  author_avatar?: string;
  author_is_bot?: boolean;
  body: string;
  type?: string;
  system?: boolean;
  created_at: string;
  updated_at: string;
};

export type GitLabMRDiscussion = {
  id: string;
  resolvable: boolean;
  resolved: boolean;
  notes: GitLabMRNote[];
  path?: string;
  line?: number;
  old_line?: number;
  created_at: string;
  updated_at: string;
};
