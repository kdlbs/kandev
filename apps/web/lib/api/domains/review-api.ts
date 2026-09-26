import { getWebSocketClient } from "@/lib/ws/connection";
import type {
  ReviewFindingStatus,
  TaskReviewFinding,
  TaskReviewRun,
  TaskReviewSnapshot,
} from "@/lib/types/review";

// i18n-exempt: precondition diagnostic for a programmer error; callers branch
// on the error type, never render this message.
const WS_CLIENT_UNAVAILABLE = "WebSocket client not available";

/**
 * Error thrown by a rejected review action, carrying the backend's
 * machine-readable code so the Review surface can show an actionable message
 * (for example "configure a utility agent") instead of a generic failure.
 */
export class ReviewRequestError extends Error {
  readonly code: string;

  constructor(message: string, code: string) {
    super(message);
    this.name = "ReviewRequestError";
    this.code = code;
  }
}

function requireClient() {
  const client = getWebSocketClient();
  if (!client) throw new Error(WS_CLIENT_UNAVAILABLE);
  return client;
}

/**
 * Extracts the `code` the backend attaches to a rejected review request. WS
 * errors surface as plain Errors, so the code is read from the structured
 * details when present and falls back to the message text.
 */
function toReviewError(error: unknown): ReviewRequestError {
  if (error instanceof ReviewRequestError) return error;
  const message = error instanceof Error ? error.message : String(error);
  const details = (error as { details?: Record<string, unknown> } | null)?.details;
  const code = typeof details?.code === "string" ? details.code : extractCodeFromMessage(message);
  return new ReviewRequestError(message, code);
}

const KNOWN_CODES = [
  "review_agent_unavailable",
  "review_workspace_unavailable",
  "review_no_changes",
  "review_unparseable_response",
  "review_execution_failed",
];

function extractCodeFromMessage(message: string): string {
  return KNOWN_CODES.find((code) => message.includes(code)) ?? "";
}

/** Starts a review pass. Resolves with the pending run; inference continues server-side. */
export async function runTaskReview(params: {
  taskId: string;
  sessionId?: string;
  repositoryId?: string;
  agentProfileId?: string;
}): Promise<TaskReviewRun> {
  try {
    const response = await requireClient().request<{ run: TaskReviewRun }>(
      "task.review.run",
      {
        task_id: params.taskId,
        session_id: params.sessionId ?? "",
        repository_id: params.repositoryId ?? "",
        agent_profile_id: params.agentProfileId ?? "",
      },
      20000,
    );
    return response.run;
  } catch (error) {
    throw toReviewError(error);
  }
}

/** Cancels an in-flight run. Idempotent for an already-finished run. */
export async function cancelTaskReview(runId: string): Promise<TaskReviewRun> {
  const response = await requireClient().request<{ run: TaskReviewRun }>("task.review.cancel", {
    run_id: runId,
  });
  return response.run;
}

/** Loads a task's run history and findings, used to backfill the store on mount. */
export async function getTaskReview(taskId: string): Promise<TaskReviewSnapshot> {
  const response = await requireClient().request<TaskReviewSnapshot>("task.review.get", {
    task_id: taskId,
  });
  return { runs: response?.runs ?? [], findings: response?.findings ?? [] };
}

/** Records the human's disposition of a finding. */
export async function updateReviewFindingStatus(
  findingId: string,
  status: ReviewFindingStatus,
): Promise<TaskReviewFinding> {
  const response = await requireClient().request<{ finding: TaskReviewFinding }>(
    "task.review.finding.update",
    { finding_id: findingId, status },
  );
  return response.finding;
}

/** Removes a task's runs and findings. */
export async function clearTaskReview(taskId: string): Promise<void> {
  await requireClient().request("task.review.clear", { task_id: taskId });
}
