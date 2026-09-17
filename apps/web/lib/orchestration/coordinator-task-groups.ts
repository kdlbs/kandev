import type { Task } from "@/lib/types/http";
import { statusSummaryActiveErrorPreview } from "@/lib/task-status-summary";

export const COORDINATOR_GROUPS = [
  "input",
  "problems",
  "running",
  "review",
  "done",
  "queued",
  "other",
] as const;
export type CoordinatorTaskGroup = (typeof COORDINATOR_GROUPS)[number];

function needsInput(task: Task) {
  return task.status_summary
    ? task.status_summary.pending_action
    : (task.task_pending_action ?? task.primary_session_pending_action);
}
function hasProblem(
  task: Task,
  acknowledged?: Record<string, string>,
  dismissed?: Record<string, string>,
) {
  return (
    statusSummaryActiveErrorPreview(task.status_summary, acknowledged, dismissed) ||
    task.auto_start_failed ||
    task.interrupted ||
    ["FAILED", "BLOCKED"].includes(task.state)
  );
}
function isRunning(task: Task) {
  const summary = task.status_summary;
  const activity = summary ? summary.foreground_activity : task.foreground_activity;
  const session = summary ? summary.primary_session?.state : task.primary_session_state;
  return (
    activity === "generating" ||
    activity === "background" ||
    session === "RUNNING" ||
    session === "STARTING"
  );
}
function needsReview(task: Task) {
  return (
    task.state === "REVIEW" ||
    task.review_status === "pending" ||
    task.status_summary?.pull_request?.attention
  );
}
function isQueued(task: Task) {
  return (
    ["CREATED", "TODO", "SCHEDULING"].includes(task.state) ||
    task.queued_for_step_id ||
    (task.status_summary?.queued_prompt_count ?? 0) > 0
  );
}
export function coordinatorTaskGroup(
  task: Task,
  acknowledged?: Record<string, string>,
  dismissed?: Record<string, string>,
): CoordinatorTaskGroup {
  if (needsInput(task)) return "input";
  if (hasProblem(task, acknowledged, dismissed)) return "problems";
  if (isRunning(task)) return "running";
  if (needsReview(task)) return "review";
  if (task.state === "COMPLETED") return "done";
  if (isQueued(task)) return "queued";
  return "other";
}
