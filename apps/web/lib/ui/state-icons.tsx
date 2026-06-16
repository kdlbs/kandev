import type { ComponentType } from "react";
import {
  IconAlertCircle,
  IconAlertTriangle,
  IconCheck,
  IconCircleFilled,
  IconCircleCheck,
  IconClock,
  IconLoader2,
  IconMessageQuestion,
  IconPlayerPause,
  IconX,
} from "@tabler/icons-react";
import type { TaskSessionState, TaskState } from "@/lib/types/http";
import { cn } from "@/lib/utils";

type IconConfig = {
  Icon: ComponentType<{ className?: string }>;
  className: string;
};

const STYLE_MUTED = "text-muted-foreground";
const STYLE_LOADING = "text-blue-500 animate-spin";
const STYLE_WARNING = "text-yellow-500";
const STYLE_ERROR = "text-red-500";
const WAITING_FOR_INPUT = "WAITING_FOR_INPUT";

const TASK_STATE_ICONS: Record<TaskState, IconConfig> = {
  CREATED: { Icon: IconAlertCircle, className: STYLE_MUTED },
  SCHEDULING: { Icon: IconLoader2, className: STYLE_LOADING },
  IN_PROGRESS: { Icon: IconLoader2, className: STYLE_LOADING },
  REVIEW: { Icon: IconCheck, className: STYLE_WARNING },
  BLOCKED: { Icon: IconAlertCircle, className: STYLE_WARNING },
  WAITING_FOR_INPUT: { Icon: IconMessageQuestion, className: STYLE_WARNING },
  COMPLETED: { Icon: IconCheck, className: "text-green-500" },
  FAILED: { Icon: IconX, className: STYLE_ERROR },
  CANCELLED: { Icon: IconX, className: STYLE_ERROR },
  TODO: { Icon: IconAlertCircle, className: STYLE_MUTED },
};

const SESSION_STATE_ICONS: Record<TaskSessionState, IconConfig> = {
  CREATED: { Icon: IconAlertCircle, className: STYLE_MUTED },
  STARTING: { Icon: IconLoader2, className: STYLE_LOADING },
  RUNNING: { Icon: IconCircleFilled, className: "text-emerald-500" },
  // Office sessions: agent process torn down, conversation paused. Use the
  // pause icon — visually distinct from RUNNING and from terminal states.
  IDLE: { Icon: IconPlayerPause, className: STYLE_MUTED },
  WAITING_FOR_INPUT: { Icon: IconClock, className: STYLE_MUTED },
  COMPLETED: { Icon: IconCircleCheck, className: "text-green-500" },
  FAILED: { Icon: IconAlertTriangle, className: STYLE_ERROR },
  CANCELLED: { Icon: IconPlayerPause, className: STYLE_MUTED },
};

const DEFAULT_TASK_ICON: IconConfig = {
  Icon: IconAlertCircle,
  className: STYLE_MUTED,
};

const DEFAULT_SESSION_ICON: IconConfig = {
  Icon: IconAlertCircle,
  className: STYLE_MUTED,
};

export function isWaitingForInputState(state?: TaskState): boolean {
  return state === WAITING_FOR_INPUT;
}

export function shouldUseQuestionTaskIcon(
  state?: TaskState,
  hasPendingClarification = false,
): boolean {
  return isWaitingForInputState(state) || hasPendingClarification;
}

// Session states where the agent is actively running work. Anything outside
// this set (WAITING_FOR_INPUT, IDLE, COMPLETED, FAILED, CANCELLED) is paused
// or terminal and must not drive the spinner — even when the task is still
// in the IN_PROGRESS workflow column.
const ACTIVE_SESSION_STATES: ReadonlySet<string> = new Set<TaskSessionState>([
  "CREATED",
  "STARTING",
  "RUNNING",
]);

/**
 * Returns true when the kanban card should show the spinning loader. The task
 * workflow state and the primary session's runtime state are decoupled — the
 * workflow can keep a task in `IN_PROGRESS` after the agent has finished, or
 * move it to `REVIEW` while the current primary session is still running — so
 * an explicit primary session state takes precedence.
 *
 * When no primary session is attached yet (task just created / scheduling),
 * we still show the spinner so users see the imminent work; otherwise we
 * require an active session state.
 *
 * Exception: `TODO` is the queued/not-started column. Any active session
 * state reported there is stale (task moved back from IN_PROGRESS, session
 * still alive) or transient, and the spinner would mislead — suppress it.
 */
export function shouldShowTaskRunningSpinner(
  taskState?: TaskState,
  primarySessionState?: string | null,
): boolean {
  if (taskState === "TODO") return false;
  if (primarySessionState) return ACTIVE_SESSION_STATES.has(primarySessionState);
  return taskState === "IN_PROGRESS" || taskState === "SCHEDULING";
}

export function shouldUsePermissionTaskIcon(hasPendingPermission = false): boolean {
  return hasPendingPermission;
}

function getTaskStateIconConfig(state?: TaskState, hasPendingClarification = false): IconConfig {
  if (shouldUseQuestionTaskIcon(state, hasPendingClarification)) {
    return TASK_STATE_ICONS.WAITING_FOR_INPUT;
  }
  if (!state) return DEFAULT_TASK_ICON;
  return TASK_STATE_ICONS[state] ?? DEFAULT_TASK_ICON;
}

export function getTaskStateIcon(
  state?: TaskState,
  className?: string,
  hasPendingClarification = false,
) {
  const config = getTaskStateIconConfig(state, hasPendingClarification);
  return <config.Icon className={cn("h-4 w-4", config.className, className)} />;
}

export function getSessionStateIcon(state?: TaskSessionState, className?: string) {
  const config = state
    ? (SESSION_STATE_ICONS[state] ?? DEFAULT_SESSION_ICON)
    : DEFAULT_SESSION_ICON;
  return <config.Icon className={cn("h-4 w-4", config.className, className)} />;
}
