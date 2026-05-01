// GitHub integration types

export type GitHubAuthMethod = "gh_cli" | "pat" | "none";

export type AuthDiagnostics = {
  command: string;
  output: string;
  exit_code: number;
};

export type GitHubStatus = {
  authenticated: boolean;
  username: string;
  auth_method: GitHubAuthMethod;
  token_configured: boolean;
  token_secret_id?: string;
  required_scopes: string[];
  diagnostics?: AuthDiagnostics;
};

export type GitHubPR = {
  number: number;
  title: string;
  body?: string;
  url: string;
  html_url: string;
  state: "open" | "closed" | "merged";
  head_branch: string;
  base_branch: string;
  author_login: string;
  repo_owner: string;
  repo_name: string;
  draft: boolean;
  mergeable: boolean;
  additions: number;
  deletions: number;
  requested_reviewers: RequestedReviewer[];
  created_at: string;
  updated_at: string;
  merged_at: string | null;
  closed_at: string | null;
};

export type RequestedReviewer = {
  login: string;
  type: "user" | "team";
};

export type PRReview = {
  id: number;
  author: string;
  author_avatar: string;
  state: string;
  body: string;
  created_at: string;
};

export type PRComment = {
  id: number;
  author: string;
  author_avatar: string;
  author_is_bot: boolean;
  body: string;
  path: string;
  line: number;
  side: string;
  comment_type: "review" | "issue";
  created_at: string;
  updated_at: string;
  in_reply_to: number | null;
};

export type CheckRun = {
  name: string;
  source: "check_run" | "status_context";
  status: string;
  conclusion: string;
  html_url: string;
  output: string;
  started_at: string | null;
  completed_at: string | null;
};

export type PRFeedback = {
  pr: GitHubPR;
  reviews: PRReview[];
  comments: PRComment[];
  checks: CheckRun[];
  has_issues: boolean;
};

export type GitHubPRStatus = {
  pr: GitHubPR;
  review_state: "approved" | "changes_requested" | "pending" | "";
  checks_state: "success" | "failure" | "pending" | "";
  mergeable_state: MergeableState;
  review_count: number;
  pending_review_count: number;
  checks_total: number;
  checks_passing: number;
};

export type MergeableState =
  | "clean"
  | "blocked"
  | "behind"
  | "dirty"
  | "has_hooks"
  | "unstable"
  | "draft"
  | "unknown"
  | "";

export type TaskPR = {
  id: string;
  task_id: string;
  /** ID of the task repository this PR belongs to. Empty for legacy single-repo
   *  tasks persisted before multi-repo support. */
  repository_id?: string;
  owner: string;
  repo: string;
  pr_number: number;
  pr_url: string;
  pr_title: string;
  head_branch: string;
  base_branch: string;
  author_login: string;
  state: "open" | "closed" | "merged";
  review_state: "approved" | "changes_requested" | "pending" | "";
  checks_state: "success" | "failure" | "pending" | "";
  mergeable_state: MergeableState;
  review_count: number;
  pending_review_count: number;
  comment_count: number;
  additions: number;
  deletions: number;
  created_at: string;
  merged_at: string | null;
  closed_at: string | null;
  last_synced_at: string | null;
  updated_at: string;
};

export type PRWatch = {
  id: string;
  session_id: string;
  task_id: string;
  owner: string;
  repo: string;
  pr_number: number;
  branch: string;
  last_checked_at: string | null;
  last_comment_at: string | null;
  last_check_status: string;
  created_at: string;
  updated_at: string;
};

export type RepoFilter = {
  owner: string;
  name: string;
};

export type GitHubOrg = {
  login: string;
  avatar_url: string;
};

export type GitHubRepoInfo = {
  full_name: string;
  owner: string;
  name: string;
  private: boolean;
};

export type ReviewScope = "user" | "user_and_teams";

export type ReviewWatch = {
  id: string;
  workspace_id: string;
  workflow_id: string;
  workflow_step_id: string;
  repos: RepoFilter[];
  agent_profile_id: string;
  executor_profile_id: string;
  prompt: string;
  review_scope: ReviewScope;
  custom_query: string;
  enabled: boolean;
  poll_interval_seconds: number;
  last_polled_at: string | null;
  created_at: string;
  updated_at: string;
};

export type DailyCount = {
  date: string;
  count: number;
};

export type PRStats = {
  total_prs_created: number;
  total_prs_reviewed: number;
  total_comments: number;
  ci_pass_rate: number;
  approval_rate: number;
  avg_time_to_merge_hours: number;
  prs_by_day: DailyCount[];
};

// Response types
export type GitHubStatusResponse = GitHubStatus;

export type TaskPRsResponse = {
  /** Each task may have multiple PRs (one per repository for multi-repo tasks). */
  task_prs: Record<string, TaskPR[]>;
};

export type PRWatchesResponse = {
  watches: PRWatch[];
};

export type ReviewWatchesResponse = {
  watches: ReviewWatch[];
};

export type TriggerReviewResponse = {
  new_prs_found: number;
};

export type PRStatsResponse = PRStats;

// Request types
export type CreateReviewWatchRequest = {
  workspace_id: string;
  workflow_id: string;
  workflow_step_id: string;
  repos: RepoFilter[];
  agent_profile_id: string;
  executor_profile_id: string;
  prompt?: string;
  review_scope?: ReviewScope;
  custom_query?: string;
  enabled?: boolean;
  poll_interval_seconds?: number;
};

export type UpdateReviewWatchRequest = Partial<Omit<CreateReviewWatchRequest, "workspace_id">>;

// Issue watch types

export type IssueWatch = {
  id: string;
  workspace_id: string;
  workflow_id: string;
  workflow_step_id: string;
  repos: RepoFilter[];
  agent_profile_id: string;
  executor_profile_id: string;
  prompt: string;
  labels: string[];
  custom_query: string;
  enabled: boolean;
  poll_interval_seconds: number;
  last_polled_at: string | null;
  created_at: string;
  updated_at: string;
};

export type IssueWatchesResponse = {
  watches: IssueWatch[];
};

export type TriggerIssueResponse = {
  new_issues_found: number;
};

export type CreateIssueWatchRequest = {
  workspace_id: string;
  workflow_id: string;
  workflow_step_id: string;
  repos: RepoFilter[];
  agent_profile_id: string;
  executor_profile_id: string;
  prompt?: string;
  labels?: string[];
  custom_query?: string;
  poll_interval_seconds?: number;
};

export type UpdateIssueWatchRequest = Partial<Omit<CreateIssueWatchRequest, "workspace_id">> & {
  enabled?: boolean;
};

// PR diff file (from GitHub API)
export type PRDiffFile = {
  filename: string;
  status: string; // added, removed, modified, renamed, copied, changed, unchanged
  additions: number;
  deletions: number;
  patch: string;
  old_path?: string;
};

// PR commit info (from GitHub API)
export type PRCommitInfo = {
  sha: string;
  message: string;
  author_login: string;
  author_date: string;
  additions: number;
  deletions: number;
  files_changed: number;
};

// GitHub Issue (separate from Pull Request)
export type GitHubIssue = {
  number: number;
  title: string;
  body: string;
  url: string;
  html_url: string;
  state: "open" | "closed";
  author_login: string;
  repo_owner: string;
  repo_name: string;
  labels: string[];
  assignees: string[];
  created_at: string;
  updated_at: string;
  closed_at: string | null;
};

export type SearchPRsResponse = {
  prs: GitHubPR[];
  total_count: number;
  page: number;
  per_page: number;
};

export type SearchIssuesResponse = {
  issues: GitHubIssue[];
  total_count: number;
  page: number;
  per_page: number;
};

// Action presets — configurable quick-launch prompts on the /github page.
export type GitHubActionPresetKind = "pr" | "issue";

export type GitHubActionPresetIcon =
  | "eye"
  | "message"
  | "tool"
  | "code"
  | "search"
  | "bug"
  | "sparkle"
  | "check";

export type GitHubActionPreset = {
  id: string;
  label: string;
  hint: string;
  // `string & {}` preserves autocomplete for the known icon keys while still
  // accepting custom strings for forward compatibility.
  icon: GitHubActionPresetIcon | (string & {});
  prompt_template: string;
};

export type GitHubActionPresets = {
  workspace_id: string;
  pr: GitHubActionPreset[];
  issue: GitHubActionPreset[];
};

export type UpdateGitHubActionPresetsRequest = {
  workspace_id: string;
  pr?: GitHubActionPreset[];
  issue?: GitHubActionPreset[];
};
