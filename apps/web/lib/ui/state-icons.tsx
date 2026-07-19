import type { ComponentType } from "react";
import {
  IconAlertCircle,
  IconAlertTriangle,
  IconCheck,
  IconCircleCheck,
  IconCircleFilled,
  IconLoader,
  IconLoader2,
  IconMessageQuestion,
  IconPlayerPause,
  IconShieldQuestion,
  IconX,
} from "@tabler/icons-react";
import type { ForegroundActivity, TaskSessionState, TaskState } from "@/lib/types/http";
import { cn } from "@/lib/utils";

type IconConfig = {
  Icon: ComponentType<{ className?: string }>;
  className: string;
};

const STYLE_MUTED = "text-muted-foreground";
const STYLE_LOADING = "text-blue-500 animate-spin";
const STYLE_WARNING = "text-yellow-500";
const STYLE_PERMISSION = "text-amber-500";
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
  // (a) generating: the foreground agent is actively producing output. This is
  // the established "session is running" indicator and is deliberately left
  // unchanged — the fine-grained busy signal only ADDS a distinct
  // background-work indicator (below); it does not restyle foreground running.
  RUNNING: { Icon: IconCircleFilled, className: "text-emerald-500" },
  // Office sessions: agent process torn down, conversation paused. Use the
  // pause icon — visually distinct from RUNNING and from terminal states.
  IDLE: { Icon: IconPlayerPause, className: STYLE_MUTED },
  WAITING_FOR_INPUT: { Icon: IconMessageQuestion, className: STYLE_WARNING },
  COMPLETED: { Icon: IconCircleCheck, className: "text-green-500" },
  FAILED: { Icon: IconAlertTriangle, className: STYLE_ERROR },
  CANCELLED: { Icon: IconPlayerPause, className: STYLE_MUTED },
};

// (b) background-running: the foreground turn has yielded to spawned background
// work (ADR-0046). A spinner — the operator can see the
// agent is not done — visually separate from the static "generating" dot (a) by
// its motion AND shape, and from the done checkmark (c) by its motion AND shape,
// so the three read apart even in a grayscale/desaturated scan (not hue alone,
// per §req:not-color-alone). The spinner (work in motion) reads as "something is
// still running in the background" while the foreground is idle; the solid dot
// stays reserved for the foreground actively generating.
//
// This is the single source for the background-running affordance: every
// session-level surface (session switcher, session-reopen menu, sidebar running
// indicator) renders it by calling getSessionStateIcon with the session's
// foreground_activity rather than re-deriving its own icon.
const SESSION_BACKGROUND_ICON: IconConfig = {
  Icon: IconLoader2,
  className: "text-emerald-500 animate-spin",
};

// The task-level generating affordance — the established running spinner
// (IconLoader2, smooth arc). Rendered when the task-level MOST-ACTIVE-WINS
// aggregate is "generating"; kept identical to the existing card spinner so the
// generating look is unchanged (§spec:task-level-indicator).
const TASK_GENERATING_ICON: IconConfig = {
  Icon: IconLoader2,
  className: STYLE_LOADING,
};

// The task-level background-running affordance (§spec:task-level-indicator):
// spawned background work is running while the foreground turns are idle. It is a
// violet segmented spinner (IconLoader) — distinct from the generating spinner
// (IconLoader2, a blue smooth arc) by BOTH shape AND hue, and from the done check
// (IconCheck, green) by shape, motion, AND hue. The compact scanning surfaces
// (board card, task-list row, graph/swimlane node) are dense, so the extra hue
// separation makes background read apart from generating at a glance, while the
// shape difference still carries the distinction in a grayscale/desaturated scan
// (§req:not-color-alone). Violet is otherwise unused by the task states (blue =
// generating/loading, green = done, yellow = waiting, red = error), so it reads as
// its own "still working in the background" state; the motion (a spinner) keeps it
// from ever being mistaken for the done check.
const TASK_BACKGROUND_ICON: IconConfig = {
  Icon: IconLoader,
  className: "text-violet-500 animate-spin",
};

const PENDING_PERMISSION_ICON: IconConfig = {
  Icon: IconShieldQuestion,
  className: STYLE_PERMISSION,
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
// this set (CREATED, WAITING_FOR_INPUT, IDLE, COMPLETED, FAILED, CANCELLED) is
// not-yet-started, paused, or terminal and must not drive the spinner on its
// own — even when the task is still in the IN_PROGRESS workflow column.
const ACTIVE_SESSION_STATES: ReadonlySet<string> = new Set<TaskSessionState>([
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
 * `CREATED` means the session row exists but the agent has not started. During
 * a genuine launch the task state is SCHEDULING/IN_PROGRESS, so we defer to the
 * task state; but an orphaned/resting CREATED session on an otherwise inactive
 * task (e.g. task CREATED, sitting in a Waiting column) must not spin.
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
  const sessionIsKnownAndNotCreated =
    primarySessionState != null && primarySessionState !== "CREATED";
  if (sessionIsKnownAndNotCreated) {
    return ACTIVE_SESSION_STATES.has(primarySessionState);
  }
  return taskState === "IN_PROGRESS" || taskState === "SCHEDULING";
}

export function shouldUsePermissionTaskIcon(hasPendingPermission = false): boolean {
  return hasPendingPermission;
}

export function isTaskInFlight(foregroundActivity?: ForegroundActivity | null): boolean {
  return foregroundActivity === "generating" || foregroundActivity === "background";
}

function getTaskStateIconConfig(
  state?: TaskState,
  hasPendingClarification = false,
  foregroundActivity?: ForegroundActivity | null,
  hasPendingPermission = false,
): IconConfig {
  // The task-level MOST-ACTIVE-WINS aggregate sits ABOVE the coarse task state
  // (§spec:task-level-indicator): a task whose foreground turns are idle while
  // spawned background work runs reads as background-running, never as done; and a
  // task with any generating session reads as generating even if its coarse state
  // (e.g. a finished primary session) would otherwise render done.
  if (foregroundActivity === "background") return TASK_BACKGROUND_ICON;
  if (foregroundActivity === "generating") return TASK_GENERATING_ICON;
  if (shouldUsePermissionTaskIcon(hasPendingPermission)) {
    return PENDING_PERMISSION_ICON;
  }
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
  foregroundActivity?: ForegroundActivity | null,
  hasPendingPermission = false,
) {
  const config = getTaskStateIconConfig(
    state,
    hasPendingClarification,
    foregroundActivity,
    hasPendingPermission,
  );
  return <config.Icon className={cn("h-4 w-4", config.className, className)} />;
}

function getSessionStateIconConfig(
  state?: TaskSessionState,
  foregroundActivity?: ForegroundActivity | null,
  hasPendingClarification = false,
  hasPendingPermission = false,
): IconConfig {
  // (b) background-running wins over the default RUNNING (generating) icon:
  // while the foreground turn waits on spawned background work the session must
  // read as "working in background", never as done (ADR-0046).
  if (state === "RUNNING" && foregroundActivity === "background") {
    return SESSION_BACKGROUND_ICON;
  }
  if (hasPendingPermission) return PENDING_PERMISSION_ICON;
  if (hasPendingClarification) return SESSION_STATE_ICONS.WAITING_FOR_INPUT;
  if (!state) return DEFAULT_SESSION_ICON;
  return SESSION_STATE_ICONS[state] ?? DEFAULT_SESSION_ICON;
}

export function getSessionStateIcon(
  state?: TaskSessionState,
  className?: string,
  foregroundActivity?: ForegroundActivity | null,
  hasPendingClarification = false,
  hasPendingPermission = false,
) {
  const config = getSessionStateIconConfig(
    state,
    foregroundActivity,
    hasPendingClarification,
    hasPendingPermission,
  );
  return <config.Icon className={cn("h-4 w-4", config.className, className)} />;
}
