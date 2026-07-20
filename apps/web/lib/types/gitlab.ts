/**
 * Connection status for the GitLab integration. Returned by
 * `GET /api/v1/gitlab/status` and shaped by `internal/gitlab.Status`.
 */
export type GitLabStatus = {
  authenticated: boolean;
  username: string;
  auth_method: "glab_cli" | "pat" | "environment" | "none" | "mock";
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

export type GitLabConfig = {
  workspace_id: string;
  host: string;
  auth_method: "glab_cli" | "pat" | "environment";
  username: string;
  has_secret: boolean;
  last_ok: boolean;
  last_error?: string;
  last_checked_at?: string;
  created_at: string;
  updated_at: string;
};

export type SetGitLabConfigRequest = {
  host: string;
  auth_method: GitLabConfig["auth_method"];
  token?: string;
};

export type TestGitLabConnectionResult = {
  ok: boolean;
  username?: string;
  error?: string;
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
  reviewers: GitLabMRUser[];
  assignees: GitLabMRUser[];
  labels?: string[];
  created_at: string;
  updated_at: string;
  merged_at?: string;
  closed_at?: string;
};

export type GitLabMRUser = {
  id: number;
  username: string;
  name: string;
  type: string;
  avatar_url?: string;
};

export type GitLabProjectMember = {
  id: number;
  username: string;
  name: string;
  avatar_url: string;
};

export type GitLabSubscriptionState = { subscribed: boolean };

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

/** Project filter for review/issue watch scoping. */
export type ProjectFilter = { path: string };

/** Review watch — saved search for MRs awaiting review. */
export type ReviewWatch = {
  id: string;
  workspace_id: string;
  workflow_id: string;
  workflow_step_id: string;
  projects: ProjectFilter[];
  agent_profile_id: string;
  executor_profile_id: string;
  prompt: string;
  repository_id?: string;
  base_branch?: string;
  review_scope: "user" | "user_and_teams" | string;
  custom_query: string;
  enabled: boolean;
  poll_interval_seconds: number;
  cleanup_policy: "auto" | "always" | "never" | string;
  max_inflight_tasks?: number;
  last_error?: string;
  last_error_at?: string;
  last_polled_at?: string;
  created_at: string;
  updated_at: string;
};

/** Issue watch — saved search for issues. */
export type IssueWatch = {
  id: string;
  workspace_id: string;
  workflow_id: string;
  workflow_step_id: string;
  projects: ProjectFilter[];
  agent_profile_id: string;
  executor_profile_id: string;
  prompt: string;
  repository_id?: string;
  base_branch?: string;
  labels: string[];
  custom_query: string;
  enabled: boolean;
  poll_interval_seconds: number;
  cleanup_policy: "auto" | "always" | "never" | string;
  max_inflight_tasks?: number;
  last_error?: string;
  last_error_at?: string;
  last_polled_at?: string;
  created_at: string;
  updated_at: string;
};

/** Branch-bound MR watch — links a session's source branch to a discovered MR. */
export type MRWatch = {
  id: string;
  session_id: string;
  task_id: string;
  repository_id?: string;
  project_path: string;
  mr_iid: number;
  branch: string;
  last_checked_at?: string;
  last_note_at?: string;
  last_pipeline_state: string;
  last_approval_state: string;
  created_at: string;
  updated_at: string;
};

/** Aggregate stats for the /gitlab page. */
export type GitLabStats = {
  open_mrs: number;
  mrs_awaiting_my_review: number;
  open_issues_assigned_me: number;
};

/** Action preset (quick-launch task template). */
export type GitLabActionPreset = {
  id: string;
  label: string;
  hint: string;
  icon: string;
  prompt_template: string;
};

/** Workspace's MR + issue presets. */
export type GitLabActionPresets = {
  workspace_id: string;
  mr: GitLabActionPreset[];
  issue: GitLabActionPreset[];
};

/** Project (lightweight, for autocomplete). */
export type GitLabProject = {
  id: number;
  path_with_namespace: string;
  namespace: string;
  path: string;
  name: string;
  visibility: string;
  web_url?: string;
  default_branch?: string;
};

/** Project allowed merge methods. */
export type ProjectMergeMethods = {
  merge: boolean;
  rebase_merge: boolean;
  fast_forward: boolean;
  allow_squash: boolean;
};

/** Single approval entry on an MR. */
export type GitLabMRApproval = {
  username: string;
  avatar?: string;
  created_at: string;
};

/** Pipeline (CI) for an MR. */
export type GitLabPipeline = {
  id: number;
  iid: number;
  status: string;
  source: string;
  ref: string;
  sha: string;
  web_url: string;
  jobs_total: number;
  jobs_passing: number;
  started_at?: string;
  finished_at?: string;
};

/** Aggregate feedback for an MR (used by the detail panel). */
export type GitLabMRFeedback = {
  mr: MR;
  approvals: GitLabMRApproval[];
  discussions: GitLabMRDiscussion[];
  pipelines: GitLabPipeline[];
  has_issues: boolean;
};

/** File changed in an MR. */
export type GitLabMRFile = {
  filename: string;
  status: string;
  additions: number;
  deletions: number;
  patch?: string;
  old_path?: string;
};

/** Commit in an MR. */
export type GitLabMRCommit = {
  sha: string;
  message: string;
  author_name: string;
  author_date: string;
};

/** Project branch entry. */
export type GitLabRepoBranch = { name: string };
