// GitHub integration types

import type { GitHubAppRegistration } from "./github-app";

export * from "./github-app";

export type GitHubAuthMethod =
  | "gh_cli"
  | "pat"
  | "github_app_installation"
  | "legacy_shared"
  | "none";

export type GitHubConnectionSource = Exclude<GitHubAuthMethod, "none">;
export type GitHubConnectionState = "active" | "invalid" | "suspended" | "revoked";

export type GitHubAuthPrincipal = {
  kind: "human" | "app";
  source: GitHubConnectionSource | "github_app_user";
  login?: string;
  installation_id?: number;
  app_registration_id?: string;
  app_credential_generation?: number;
  workspace_id: string;
  user_id?: string;
};

export type GitHubAutomationConnection = {
  workspace_id: string;
  source: GitHubConnectionSource;
  github_host: string;
  login?: string;
  installation_id?: number;
  installation_account_login?: string;
  installation_account_type?: string;
  app_registration_id?: string;
  status: GitHubConnectionState;
  actor?: GitHubAuthPrincipal;
  capabilities?: Record<string, boolean>;
  missing_capabilities?: string[];
  missing_permissions?: string[];
  legacy_migration?: boolean;
  credential_generation: number;
  last_error?: string;
};

export type GitHubPersonalConnection = {
  workspace_id: string;
  user_id: string;
  app_registration_id: string;
  github_user_id: number;
  login: string;
  status: GitHubConnectionState;
  access_expires_at: string;
  refresh_expires_at?: string;
  credential_generation: number;
  last_error?: string;
};

export type GitHubCLIAccount = {
  host: string;
  login: string;
  active: boolean;
  state: string;
  selected?: boolean;
};

export type AuthDiagnostics = {
  command: string;
  output: string;
  exit_code: number;
};

export type GitHubStatus = {
  workspace_id?: string;
  automation?: GitHubAutomationConnection | null;
  personal?: GitHubPersonalConnection | null;
  app_registration?: GitHubAppRegistration | null;
  app_available?: boolean;
  github_app_available?: boolean;
  effective_personal_actor?: GitHubAuthPrincipal | null;
  effective_manual_mutation_actor?: GitHubAuthPrincipal | null;
  /** Compatibility fields for existing PR/issue surfaces. */
  authenticated: boolean;
  username: string;
  auth_method: GitHubAuthMethod;
  token_configured: boolean;
  token_secret_id?: string;
  required_scopes: string[];
  diagnostics?: AuthDiagnostics;
  rate_limit?: GitHubRateLimitInfo;
};

export type GitHubRateLimitResource = "core" | "graphql" | "search";

export type GitHubRateLimitSnapshot = {
  resource: GitHubRateLimitResource;
  remaining: number;
  limit: number;
  reset_at: string;
  updated_at: string;
};

export type GitHubRateLimitInfo = {
  core?: GitHubRateLimitSnapshot;
  graphql?: GitHubRateLimitSnapshot;
  search?: GitHubRateLimitSnapshot;
};

export type GitHubRateLimitUpdate = {
  snapshots: GitHubRateLimitSnapshot[];
  trigger: GitHubRateLimitResource;
  exhaustion_transition?: "exhausted" | "recovered";
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
  /** Rich merge state (clean | blocked | behind | dirty | ...). Optional because
   *  legacy payloads predate it; falls back to TaskPR.mergeable_state. */
  mergeable_state?: MergeableState;
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

export type MergeMethod = "merge" | "squash" | "rebase";

export type RepoMergeMethods = {
  merge: boolean;
  squash: boolean;
  rebase: boolean;
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

/** Normalized GitHub merge-queue entry states. Future provider values are
 * retained as strings so the UI can use a generic queued presentation. */
export type MergeQueueState =
  | "queued"
  | "awaiting_checks"
  | "mergeable"
  | "unmergeable"
  | "locked"
  | (string & {});

export type TaskPR = {
  id: string;
  workspace_id: string;
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
  /** Number of approving reviews required by the base branch protection rule.
   *  Null when no protection rule exists or the token lacks scope to read it. */
  required_reviews?: number | null;
  comment_count: number;
  /** Count of unresolved review threads. Surfaced in the CI hover popover. */
  unresolved_review_threads: number;
  /** Aggregate check counts. Used by the CI hover popover to render the
   *  Passed/Failed/In-Progress count rows before the lazy PRFeedback loads. */
  checks_total: number;
  checks_passing: number;
  additions: number;
  deletions: number;
  created_at: string;
  merged_at: string | null;
  closed_at: string | null;
  last_synced_at: string | null;
  updated_at: string;
  /** Current pull-request head used to explain safe queue recovery. */
  head_sha?: string;
  /** Empty when GitHub did not return an active merge-queue entry. */
  merge_queue_state?: MergeQueueState;
  merge_queue_entry_id?: string;
  merge_queue_entry_head_sha?: string;
  /** GitHub's one-based queue position, when available. */
  merge_queue_position?: number | null;
  /** GitHub's estimated time to merge in seconds, when available. */
  merge_queue_estimated_time_to_merge_seconds?: number | null;
  merge_queue_last_removal_id?: string;
  merge_queue_last_removed_at?: string | null;
  merge_queue_last_removal_reason?: string;
  merge_queue_last_removal_before_sha?: string;
  // The five PR-outcome-attribution fields below are always present on a
  // real API/WS payload (the backend sends every key, never omits one) — the
  // `?:` here follows this file's existing convention for nullable fields
  // added after the type's original shape (e.g. required_reviews?) so
  // hand-written test fixtures aren't forced to enumerate all five. Treat
  // `undefined` the same as `null` ("nobody looked" / "never observed");
  // never treat `null` as "unknown" or vice versa.
  /** Never observed by a populating sync when null. */
  is_draft?: boolean | null;
  /** Never observed when null, distinct from 0 (a real "no files changed" observation). */
  changed_files?: number | null;
  /** Not merged, or merged but never observed by a populating sync, when null. */
  merged_by_login?: string | null;
  /** Not closed, or closure never observed by the GraphQL path specifically
   *  (closed_by is absent from the REST pulls endpoint and the gh CLI's PR
   *  field set), when null. A PR closed only through those paths keeps this
   *  null permanently. */
  closed_by_login?: string | null;
  /** A latched observation, never a merge cause: GitHub clears auto_merge
   *  once it fires, so this can only mean "armed at some instant while
   *  Kandev was watching." */
  auto_merge_observed_at?: string | null;
};

/** Workspace-scoped websocket payload emitted when a task PR association is detached. */
export type TaskPRDeletedEvent = {
  workspace_id: string;
  task_id: string;
  association_id: string;
};

export type CIAutomationQueueRemovalCause =
  | "checks_failed"
  | "checks_timed_out"
  | "conflict"
  | "manual"
  | "branch_protection"
  | "unknown";

export type TaskCIPRAutomationState = {
  task_id: string;
  repository_id: string;
  pr_number: number;
  last_fix_signature: string;
  last_fix_checkpoint_json: string;
  last_fix_enqueued_at: string | null;
  last_fix_session_id: string | null;
  auto_fix_round_count: number;
  auto_fix_exhausted_at: string | null;
  last_merge_signature: string;
  last_merge_attempt_at: string | null;
  last_queue_attempt_head_sha?: string;
  last_queue_fix_event_id?: string;
  last_queue_removal_cause?: CIAutomationQueueRemovalCause | string;
  review_request_initialized?: boolean;
  last_review_requested?: boolean;
  last_observed_pr_state?: string;
  last_lifecycle_event?: string;
  last_lifecycle_prompt_at?: string | null;
  last_lifecycle_session_id?: string | null;
  last_error: string | null;
  created_at: string;
  updated_at: string;
};

// TaskPRAutomationOptions holds the five automation switches for one linked
// PR. This is the per-PR source of truth; the aggregated booleans on
// TaskCIAutomationOptions only report "every linked PR has this on".
export type TaskPRAutomationOptions = {
  task_id: string;
  repository_id: string;
  pr_number: number;
  auto_fix_enabled: boolean;
  auto_merge_enabled: boolean;
  prompt_on_review_requested: boolean;
  prompt_on_merged: boolean;
  prompt_on_closed: boolean;
  created_at: string;
  updated_at: string;
};

export type TaskCIAutomationOptions = {
  task_id: string;
  workspace_id?: string;
  auto_fix_enabled: boolean;
  auto_merge_enabled: boolean;
  auto_fix_prompt_override: string | null;
  auto_fix_max_rounds?: number;
  effective_auto_fix_prompt: string;
  using_default_prompt: boolean;
  prompt_on_review_requested?: boolean;
  prompt_on_merged?: boolean;
  prompt_on_closed?: boolean;
  review_reviewer_login?: string;
  updated_at: string;
  pr_states: TaskCIPRAutomationState[];
  pr_options: TaskPRAutomationOptions[];
};

export type TaskCIAutomationPatch = {
  // Target one linked PR's automation switches; omit both to apply the
  // switches to every PR currently linked to the task.
  repository_id?: string;
  pr_number?: number;
  auto_fix_enabled?: boolean;
  auto_merge_enabled?: boolean;
  auto_fix_prompt_override?: string | null;
  prompt_on_review_requested?: boolean;
  prompt_on_merged?: boolean;
  prompt_on_closed?: boolean;
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

export type GitHubRepoScopeMode = "all" | "orgs" | "repos";
export type TaskGitCredentialsMode = "managed" | "executor";

export type GitHubWorkspaceSettings = {
  workspace_id: string;
  task_git_credentials_mode?: TaskGitCredentialsMode;
  repo_scope_mode: GitHubRepoScopeMode;
  repo_scope_orgs: string[];
  repo_scope_repos: RepoFilter[];
  saved_presets?: unknown;
  default_query_presets?: unknown | null;
  created_at: string;
  updated_at: string;
};

export type UpdateGitHubWorkspaceSettingsRequest = {
  workspace_id: string;
  task_git_credentials_mode?: TaskGitCredentialsMode;
  repo_scope_mode?: GitHubRepoScopeMode;
  repo_scope_orgs?: string[];
  repo_scope_repos?: RepoFilter[];
  saved_presets?: unknown;
  default_query_presets?: unknown | null;
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

/**
 * CleanupPolicy controls how a review or issue watch handles its
 * auto-created tasks once the underlying PR / issue is merged or closed.
 *
 * - "auto":   delete only when the user hasn't authored any messages on the
 *             task (the agent's auto-start prompt does not count).
 * - "always": delete on terminal state regardless of user interaction.
 * - "never":  never auto-delete; rely on the manual cleanup button.
 */
export type CleanupPolicy = "auto" | "always" | "never";

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
  cleanup_policy: CleanupPolicy;
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
  cleanup_policy?: CleanupPolicy;
};

export type UpdateReviewWatchRequest = Partial<Omit<CreateReviewWatchRequest, "workspace_id">>;

export type CleanupTasksResponse = {
  deleted: number;
};

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
  cleanup_policy: CleanupPolicy;
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
  cleanup_policy?: CleanupPolicy;
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
  /** True when the PR file belongs to an initialized Git submodule scope. */
  is_submodule?: boolean;
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
  /** False when the PR commit-list endpoint did not include exact stats. */
  stats_available?: boolean;
};

/** Exact metadata and files returned by the individual GitHub commit endpoint. */
export type PRCommitDetail = {
  sha: string;
  message: string;
  author_login: string;
  author_name: string;
  author_date: string;
  additions: number;
  deletions: number;
  files_changed: number;
  files: PRDiffFile[];
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

export type TaskIssueLink = {
  task_id: string;
  task_title: string;
  owner: string;
  repo: string;
  issue_number: number;
  issue_url: string;
  issue_title: string;
};

export type TaskIssueLinksResponse = {
  task_issues: Record<string, TaskIssueLink>;
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
