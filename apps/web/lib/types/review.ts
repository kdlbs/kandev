import type { BackendMessage } from "@/lib/types/backend";

/** Lifecycle of one native code-review pass. */
export type ReviewRunStatus = "pending" | "running" | "completed" | "failed" | "cancelled";

/** What started a review pass. */
export type ReviewRunTrigger = "manual" | "workflow_step" | "agent";

/** How much a finding should worry the reader, most severe first. */
export type ReviewSeverity = "blocker" | "major" | "minor" | "nit";

/** The human's disposition of a finding. Findings are advisory. */
export type ReviewFindingStatus = "open" | "resolved" | "dismissed";

/** Machine-readable failure codes the Review surface branches on. */
export type ReviewErrorCode =
  | "review_agent_unavailable"
  | "review_workspace_unavailable"
  | "review_no_changes"
  | "review_unparseable_response"
  | "review_execution_failed"
  | "review_cancelled"
  | "review_interrupted";

export type TaskReviewRun = {
  id: string;
  task_id: string;
  session_id: string;
  trigger: ReviewRunTrigger;
  workflow_step_id: string;
  agent_id: string;
  model: string;
  status: ReviewRunStatus;
  error_code: string;
  error_message: string;
  summary: string;
  finding_count: number;
  file_count: number;
  repository_count: number;
  prompt_tokens: number;
  response_tokens: number;
  duration_ms: number;
  created_at: string;
  completed_at?: string | null;
};

export type TaskReviewFinding = {
  id: string;
  run_id: string;
  task_id: string;
  /** Empty for a single-repository task. */
  repository_id: string;
  /** Sanitized repo dir name; empty for a single-repository task. */
  repository_name: string;
  file_path: string;
  start_line: number;
  end_line: number;
  side: "additions" | "deletions";
  severity: ReviewSeverity;
  category: string;
  title: string;
  body: string;
  /** Display-only suggested replacement; never applied automatically. */
  suggestion: string;
  /** The anchored diff lines at publish time, used for best-effort relocation. */
  anchor_text: string;
  /**
   * djb2 hash of the file's normalized diff at publish time. Compared against a
   * freshly computed hash to decide staleness. Empty for an agent-published
   * finding, which is therefore never reported stale.
   */
  file_diff_hash: string;
  status: ReviewFindingStatus;
  resolved_at?: string | null;
  created_at: string;
  updated_at: string;
};

/** Response shape of the `task.review.get` action. */
export type TaskReviewSnapshot = {
  runs: TaskReviewRun[];
  findings: TaskReviewFinding[];
};

export type ReviewBackendMessageMap = {
  "task.review.run_updated": BackendMessage<
    "task.review.run_updated",
    { task_id: string; run: TaskReviewRun }
  >;
  "task.review.findings_published": BackendMessage<
    "task.review.findings_published",
    {
      task_id: string;
      run_id: string;
      findings: TaskReviewFinding[];
      /** Findings the backend replaced; clients must drop these ids. */
      superseded_ids?: string[];
    }
  >;
  "task.review.finding_updated": BackendMessage<
    "task.review.finding_updated",
    { task_id: string; finding: TaskReviewFinding }
  >;
  "task.review.cleared": BackendMessage<"task.review.cleared", { task_id: string }>;
};
