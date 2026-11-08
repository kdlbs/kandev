import type { NativeDashboard, NativeTask } from "@/lib/types/control-native";
import { isTerminalTask, taskWaitReason, type TaskWaitReason } from "./control-task-wait";

export { capacityGradeWaiting, capacityWaitingTotal } from "./control-capacity-waiting";
export { isTerminalTask, taskWaitReason, type TaskWaitReason };

/** Task states the board counts as work in progress. */
export const ACTIVE_TASK_STATES = ["IN_PROGRESS", "REVIEW", "SCHEDULING"] as const;
/** Task states that end a task; a failed session on one of these is history, not trouble. */
export const TERMINAL_TASK_STATES = new Set(["COMPLETED", "CANCELLED"]);

export type WorkspaceFigures = {
  inProgress: number;
  running: number;
  waitingInput: number;
  queued: number;
  completed24h: number;
  moves24h: number;
  failedSessions: number;
};

/** The comparison figures one recorded dashboard yields. */
export function workspaceFigures(data: NativeDashboard): WorkspaceFigures {
  return {
    inProgress: ACTIVE_TASK_STATES.reduce((sum, state) => sum + (data.tasks[state] ?? 0), 0),
    running: data.sessions.RUNNING ?? 0,
    waitingInput: data.waiting_input_tasks,
    queued: data.queued_messages,
    completed24h: data.completed_tasks_24h,
    moves24h: data.transitions_24h,
    failedSessions: data.sessions.FAILED ?? 0,
  };
}

export function sumFigures(rows: WorkspaceFigures[]): WorkspaceFigures {
  const total: WorkspaceFigures = {
    inProgress: 0,
    running: 0,
    waitingInput: 0,
    queued: 0,
    completed24h: 0,
    moves24h: 0,
    failedSessions: 0,
  };
  for (const row of rows) {
    for (const key of Object.keys(total) as (keyof WorkspaceFigures)[]) total[key] += row[key];
  }
  return total;
}

/**
 * The installation-wide shared-capacity figure. Every workspace response
 * carries the same snapshot, so this picks the first one and never sums.
 */
export function sharedCapacityWaiting(
  rows: ReadonlyArray<NativeDashboard | undefined>,
): NativeDashboard["shared_capacity_waiting"] | undefined {
  for (const row of rows) {
    if (row?.shared_capacity_waiting) return row.shared_capacity_waiting;
  }
  return undefined;
}

export type WorkspaceStatus = "stalled" | "capacity" | "active" | "idle" | "pending" | "unknown";

/**
 * Recorded WIP without a running session is not proof of a stall. Holds,
 * workflow steps without execution actions and paused queues can all wait
 * legitimately. Installation-wide capacity counts cannot attribute a wait
 * to this workspace either; an incomplete observation remains unknown.
 */
export function workspaceStatus(
  figures: WorkspaceFigures,
  waiting: number | null,
  recordedTasks?: NativeDashboard["tasks"],
): WorkspaceStatus {
  if (figures.running > 0) return "active";
  const pending = recordedTasks
    ? Object.entries(recordedTasks).some(
        ([state, count]) => count > 0 && !TERMINAL_TASK_STATES.has(state.toUpperCase()),
      )
    : figures.inProgress > 0 || figures.queued > 0 || figures.waitingInput > 0;
  if (!pending) return "idle";
  return waiting === null ? "unknown" : "pending";
}

export type TaskSignal =
  | "repeat"
  | "stalled"
  | "failed"
  | "running"
  | "waiting"
  | "held"
  | "paused"
  | "queued"
  | "noSession"
  | "other";

/** Signals that need a person, in the order the attention lists rank them. */
export const ATTENTION_SIGNALS: TaskSignal[] = ["repeat", "stalled", "failed"];
const SIGNAL_RANK: TaskSignal[] = [
  ...ATTENTION_SIGNALS,
  "running",
  "waiting",
  "held",
  "paused",
  "queued",
  "other",
  "noSession",
];

/** One task's most urgent signal; `repeating` holds tasks flagged by the move-rate rule. */
const WAIT_SIGNALS: Partial<Record<TaskWaitReason, TaskSignal>> = {
  hold: "held",
  approval: "held",
  blocked: "held",
  paused: "paused",
  queued: "queued",
  queueUnknown: "queued",
};

export function taskSignal(task: NativeTask, repeating: ReadonlySet<string>): TaskSignal {
  if (repeating.has(task.task_id) && !isTerminalTask(task)) return "repeat";
  if (task.running_sessions > 0 || task.session_state === "RUNNING") return "running";
  if (task.session_state === "FAILED" && !isTerminalTask(task)) return "failed";
  if (!isTerminalTask(task)) {
    const reason = taskWaitReason(task);
    const signal = reason && WAIT_SIGNALS[reason];
    if (signal) return signal;
  }
  if (task.session_state === "WAITING_FOR_INPUT") return "waiting";
  if (!task.session_id) return "noSession";
  return "other";
}

export function signalRank(signal: TaskSignal): number {
  return SIGNAL_RANK.indexOf(signal);
}

/** Attention first, then running and waiting work; most recent change first within a signal. */
export function sortBySignal<T extends { signal: TaskSignal; task: NativeTask }>(rows: T[]): T[] {
  return [...rows].sort(
    (a, b) =>
      signalRank(a.signal) - signalRank(b.signal) ||
      Date.parse(b.task.updated_at) - Date.parse(a.task.updated_at),
  );
}
