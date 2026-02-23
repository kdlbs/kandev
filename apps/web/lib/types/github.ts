// GitHub integration types

export type GitHubAuthMethod = "gh_cli" | "pat" | "none";

export type GitHubStatus = {
  authenticated: boolean;
  username: string;
  auth_method: GitHubAuthMethod;
};

export type GitHubPR = {
  number: number;
  title: string;
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
  created_at: string;
  updated_at: string;
  merged_at: string | null;
  closed_at: string | null;
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
  body: string;
  path: string;
  line: number;
  side: string;
  created_at: string;
  updated_at: string;
  in_reply_to: number | null;
};

export type CheckRun = {
  name: string;
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

export type TaskPR = {
  id: string;
  task_id: string;
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
  task_prs: Record<string, TaskPR>;
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
