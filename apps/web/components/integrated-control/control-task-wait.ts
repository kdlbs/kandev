import type { NativeTask } from "@/lib/types/control-native";

/** Task states that end a task; a failed session on one of these is history. */
const TERMINAL_TASK_STATES = new Set(["COMPLETED", "CANCELLED"]);

/**
 * A recorded reason stopped work is not executing. Values come from recorded
 * task/session/queue fields only, never from task or step names.
 */
export const TASK_WAIT_REASONS = [
  "hold",
  "approval",
  "blocked",
  "paused",
  "queued",
  "queueUnknown",
  "waiting",
  "failed",
  "unknown",
] as const;
export type TaskWaitReason = (typeof TASK_WAIT_REASONS)[number];

export function isTerminalTask(task: Pick<NativeTask, "state" | "archived">): boolean {
  return task.archived || TERMINAL_TASK_STATES.has(task.state.toUpperCase());
}

/**
 * One task's recorded reason for not executing, or null when it is finished or
 * running. A missing queue-policy read is unknown, never assumed auto-run.
 */
export function taskWaitReason(task: NativeTask): TaskWaitReason | null {
  if (isTerminalTask(task) || task.running_sessions > 0 || task.session_state === "RUNNING") {
    return null;
  }
  if (task.session_state === "FAILED") return "failed";
  if (task.hold_mode || task.stage_type === "hold") return "hold";
  if (task.stage_type === "approval") return "approval";
  if (task.state.toUpperCase() === "BLOCKED") return "blocked";
  if (task.queued_messages > 0) {
    if ((task.queued_paused_messages ?? 0) > 0) return "paused";
    return (task.queued_auto_run_messages ?? 0) > 0 ? "queued" : "queueUnknown";
  }
  if (task.session_state === "WAITING_FOR_INPUT") return "waiting";
  return "unknown";
}

/** The localized label key for one wait reason. */
export function taskWaitReasonLabelKey(reason: TaskWaitReason): string {
  return `sidebar:controlSummary.reason.${reason}`;
}
