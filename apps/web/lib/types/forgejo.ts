export type ForgejoConfig = {
  workspace_id: string;
  origin: string;
  username: string;
  has_secret: boolean;
  last_ok: boolean;
  last_error?: string;
  last_checked_at?: string;
  created_at: string;
  updated_at: string;
};

export type SetForgejoConfigRequest = { origin: string; token?: string };
export type TestForgejoConnectionResult = { ok: boolean; username?: string; error?: string };

export type ForgejoRepository = { owner: string; name: string; full_name: string; default_branch: string; html_url: string };
export type ForgejoIssue = { number: number; title: string; state: string; html_url: string; body: string };
export type ForgejoPullRequest = { number: number; title: string; state: string; html_url: string; head: string; base: string; draft: boolean };
export type ForgejoTaskIssue = ForgejoIssue & { id: string; task_id: string; repository_id?: string; origin: string; issue_number: number; issue_url: string; last_synced_at?: string };
export type ForgejoTaskPR = { id: string; task_id: string; repository_id?: string; origin: string; owner: string; repo: string; pr_number: number; pr_url: string; pr_title: string; head_branch: string; base_branch: string; state: string; draft: boolean; mergeable: boolean; ci_state: string; last_synced_at?: string };
export type ForgejoIssueWatch = { id: string; workspace_id: string; owner: string; repo: string; labels: string; enabled: boolean; poll_interval_seconds: number; last_polled_at?: string; last_error?: string };
