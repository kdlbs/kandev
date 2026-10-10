export type CheckRun = {
  id?: number;
  app_id?: number;
  app_slug?: string;
  check_suite_id?: number;
  workflow_id?: number;
  workflow_name?: string;
  workflow_run_id?: number;
  workflow_event?: string;
  head_repo_id?: number;
  head_repo_owner?: string;
  head_repo_name?: string;
  head_branch?: string;
  name: string;
  source: "check_run" | "status_context";
  status: string;
  conclusion: string;
  html_url: string;
  output: string;
  started_at: string | null;
  completed_at: string | null;
};

export type WorkflowAttentionState = "unknown" | "none" | "approval_required" | "action_required";

export type WorkflowAttentionRun = {
  run_id: number;
  run_attempt: number;
  workflow_id: number;
  name: string;
  url: string;
  reason: string;
};

export type WorkflowAttention = {
  state: WorkflowAttentionState;
  head_sha: string;
  observed_at: string;
  stale: boolean;
  runs: WorkflowAttentionRun[];
};
