import type { ReactNode } from "react";
import {
  IconCircleCheck,
  IconCircleDashed,
  IconMessageQuestion,
  IconProgressCheck,
  IconShieldQuestion,
} from "@tabler/icons-react";
import { CompositorSpin } from "@kandev/ui/compositor-spin";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";
import type { ForegroundActivity, TaskSessionState, TaskState } from "@/lib/types/http";
import type {
  TaskStatusSummaryLaunchQueue,
  TaskStatusSummaryQuotaWait,
} from "@/lib/types/task-status-summary";
import {
  InterruptedTaskIcon,
  isTerminalInterruptedState,
  shouldUseQuestionTaskIcon,
} from "@/lib/ui/state-icons";
import {
  WaitingReasonTaskIcon,
  waitReasonLabelKey,
  type TaskWaitReason,
} from "@/lib/ui/waiting-reason";
import { classifyTask } from "./task-classify";

export type TaskStateIconProps = {
  sessionState?: TaskSessionState;
  state?: TaskState;
  foregroundActivity?: ForegroundActivity | null;
  hasPendingClarification?: boolean;
  hasPendingPermission?: boolean;
  isOnLastWorkflowStep?: boolean;
  interrupted?: boolean;
  /**
   * True when the task is waiting on the operator to notice, not on the
   * operator to act — a settled session with a positively-sampled background
   * process still live (spec: docs/specs/disambiguate-waiting/spec.md).
   * Outranked by pending-input (permission/clarification) and by an active
   * foregroundActivity (AC-34).
   */
  parkedOnBackgroundWork?: boolean;
  /** Why the task is waiting rather than working (lib/ui/waiting-reason). */
  waitReason?: TaskWaitReason | null;
  /** The launch-queue projection behind a `session_ceiling` wait, for its tooltip. */
  launchQueue?: TaskStatusSummaryLaunchQueue | null;
  /** The limit-wait projection behind a `quota` wait, for its tooltip. */
  quotaWait?: TaskStatusSummaryQuotaWait | null;
  /** True while an accepted archive or delete request is still in flight. */
  isPendingRemoval?: boolean;
  accessibleLabel?: string;
  showBackgroundTooltip?: boolean;
};

const BACKGROUND_WORK_RUNNING_LABEL_KEY = "task:backgroundWorkIsRunning";

function computeIsInProgress(state?: TaskState, sessionState?: TaskSessionState): boolean {
  return classifyTask(sessionState, state) === "in_progress";
}

function computeIsPreparing(state?: TaskState, sessionState?: TaskSessionState): boolean {
  if (state === "SCHEDULING") return true;
  return sessionState === "STARTING" && classifyTask(sessionState, state) !== "review";
}

function withAccessibleLabel(node: ReactNode, label?: string) {
  if (!label) return node;
  return (
    <span role="img" aria-label={label} className="inline-flex shrink-0">
      {node}
    </span>
  );
}

function BackgroundWorkTaskIcon({ showTooltip = true }: { showTooltip?: boolean }) {
  const { t } = useTranslation();
  const spinner = (
    <CompositorSpin
      aria-hidden="true"
      data-testid="task-state-background-running"
      className={
        showTooltip
          ? "h-3.5 w-3.5 shrink-0 text-violet-500"
          : "mt-[1px] h-3.5 w-3.5 shrink-0 text-violet-500"
      }
    >
      <IconCircleDashed className="size-full" />
    </CompositorSpin>
  );
  if (!showTooltip) return spinner;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          aria-label={t(BACKGROUND_WORK_RUNNING_LABEL_KEY)}
          tabIndex={0}
          className="mt-[1px] flex shrink-0 rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:ring-offset-1"
        >
          {spinner}
        </span>
      </TooltipTrigger>
      <TooltipContent side="right">{t(BACKGROUND_WORK_RUNNING_LABEL_KEY)}</TooltipContent>
    </Tooltip>
  );
}

function TaskRunningIcon({
  phase,
  className,
}: {
  phase: "running" | "preparing";
  className: string;
}) {
  return (
    <CompositorSpin
      aria-hidden="true"
      data-testid="task-state-running"
      data-loading-phase={phase}
      className={className}
    >
      <IconCircleDashed className="size-full" />
    </CompositorSpin>
  );
}

function TaskReviewIcon({
  isOnLastWorkflowStep,
  accessibleLabel,
}: Pick<TaskStateIconProps, "isOnLastWorkflowStep" | "accessibleLabel">) {
  if (isOnLastWorkflowStep) {
    return withAccessibleLabel(
      <IconCircleCheck
        aria-hidden="true"
        data-testid="task-state-workflow-complete"
        className="mt-[1px] h-3.5 w-3.5 shrink-0 text-green-500"
      />,
      accessibleLabel,
    );
  }
  return withAccessibleLabel(
    <IconProgressCheck
      aria-hidden="true"
      data-testid="task-state-turn-finished"
      className="mt-[1px] h-3.5 w-3.5 shrink-0 text-green-500"
    />,
    accessibleLabel,
  );
}

function TaskReadyIcon({ accessibleLabel }: Pick<TaskStateIconProps, "accessibleLabel">) {
  return withAccessibleLabel(
    <IconCircleCheck
      aria-hidden="true"
      data-testid="task-state-ready"
      className="mt-[1px] h-3.5 w-3.5 shrink-0 text-green-500"
    />,
    accessibleLabel,
  );
}

function PendingRemovalTaskIcon() {
  return (
    <CompositorSpin
      aria-hidden="true"
      data-testid="task-state-removal-pending"
      data-legacy-testid="task-state-archive-pending"
      className="mt-[1px] h-3.5 w-3.5 shrink-0 text-muted-foreground/60"
    >
      <IconCircleDashed className="size-full" />
    </CompositorSpin>
  );
}

export function TaskStateIcon(props: TaskStateIconProps) {
  if (props.isPendingRemoval) return <PendingRemovalTaskIcon />;
  return <TaskStateIconContent {...props} />;
}

/**
 * The wait reason a task surface shows. An interrupted task, or one whose
 * interruption is terminal, keeps its interruption affordance instead.
 */
function visibleTaskWaitReason(
  waitReason: TaskStateIconProps["waitReason"],
  state: TaskStateIconProps["state"],
  interrupted: TaskStateIconProps["interrupted"],
) {
  return waitReason && !interrupted && !isTerminalInterruptedState(state) ? waitReason : null;
}

function resolveActiveTaskIcon({
  foregroundActivity,
  hasPendingClarification,
  hasPendingPermission,
  parkedOnBackgroundWork,
  state,
  interrupted,
  waitReason,
  launchQueue,
  quotaWait,
  accessibleLabel,
  showBackgroundTooltip = false,
}: TaskStateIconProps) {
  if (hasPendingPermission ?? false) {
    return withAccessibleLabel(
      <IconShieldQuestion
        aria-hidden="true"
        data-testid="task-state-pending-permission"
        className="mt-[1px] h-3.5 w-3.5 shrink-0 text-amber-500"
      />,
      accessibleLabel,
    );
  }
  if (hasPendingClarification) {
    return withAccessibleLabel(
      <IconMessageQuestion
        aria-hidden="true"
        data-testid="task-state-waiting-for-input"
        className="mt-[1px] h-3.5 w-3.5 shrink-0 text-yellow-500"
      />,
      accessibleLabel,
    );
  }
  if (foregroundActivity === "generating") {
    return withAccessibleLabel(
      <TaskRunningIcon phase="running" className="mt-[1px] h-3.5 w-3.5 shrink-0 text-yellow-500" />,
      accessibleLabel,
    );
  }
  if (foregroundActivity === "background") {
    return withAccessibleLabel(
      <BackgroundWorkTaskIcon showTooltip={showBackgroundTooltip} />,
      showBackgroundTooltip ? undefined : accessibleLabel,
    );
  }
  // Parked-on-background-work (AC-23): a settled session whose only
  // remaining life is a positively-sampled background process.
  if (parkedOnBackgroundWork) {
    return withAccessibleLabel(
      <BackgroundWorkTaskIcon showTooltip={showBackgroundTooltip} />,
      showBackgroundTooltip ? undefined : accessibleLabel,
    );
  }
  // A recorded wait outranks the coarse WAITING_FOR_INPUT state, which alone
  // does not establish that a question needs an answer.
  const visibleWait = visibleTaskWaitReason(waitReason, state, interrupted);
  if (visibleWait) {
    return (
      <WaitingReasonTaskIcon reason={visibleWait} launchQueue={launchQueue} quotaWait={quotaWait} />
    );
  }
  if (shouldUseQuestionTaskIcon(state)) {
    return withAccessibleLabel(
      <IconMessageQuestion
        aria-hidden="true"
        data-testid="task-state-waiting-for-input"
        className="mt-[1px] h-3.5 w-3.5 shrink-0 text-yellow-500"
      />,
      accessibleLabel,
    );
  }
  return null;
}

function TaskStateIconContent({
  sessionState,
  state,
  foregroundActivity,
  hasPendingClarification,
  hasPendingPermission,
  isOnLastWorkflowStep,
  interrupted,
  parkedOnBackgroundWork,
  waitReason,
  launchQueue,
  quotaWait,
  accessibleLabel,
  showBackgroundTooltip = false,
}: TaskStateIconProps) {
  const activeIcon = resolveActiveTaskIcon({
    foregroundActivity,
    hasPendingClarification,
    hasPendingPermission,
    parkedOnBackgroundWork,
    state,
    // A visible wait reason outranks preparing/in-progress and the
    // finished-turn icon: REVIEW plus a queued admission is not a finished
    // turn, and IN_PROGRESS plus a WIP overflow is not progress.
    interrupted,
    waitReason,
    launchQueue,
    quotaWait,
    accessibleLabel,
    showBackgroundTooltip,
  });
  if (activeIcon) return activeIcon;
  return (
    <TaskSessionStateIcon
      sessionState={sessionState}
      state={state}
      interrupted={interrupted}
      isOnLastWorkflowStep={isOnLastWorkflowStep}
      accessibleLabel={accessibleLabel}
    />
  );
}

function TaskSessionStateIcon({
  sessionState,
  state,
  interrupted,
  isOnLastWorkflowStep,
  accessibleLabel,
}: Pick<
  TaskStateIconProps,
  "sessionState" | "state" | "interrupted" | "isOnLastWorkflowStep" | "accessibleLabel"
>) {
  if (sessionState === "IDLE" && state !== "SCHEDULING") {
    return <TaskReadyIcon accessibleLabel={accessibleLabel} />;
  }
  if (computeIsPreparing(state, sessionState)) {
    return withAccessibleLabel(
      <TaskRunningIcon
        phase="preparing"
        className="mt-[1px] h-3.5 w-3.5 shrink-0 text-muted-foreground/40 [animation-duration:2s]"
      />,
      accessibleLabel,
    );
  }
  if (computeIsInProgress(state, sessionState)) {
    return withAccessibleLabel(
      <TaskRunningIcon phase="running" className="mt-[1px] h-3.5 w-3.5 shrink-0 text-yellow-500" />,
      accessibleLabel,
    );
  }
  if (interrupted && !isTerminalInterruptedState(state, sessionState)) {
    return withAccessibleLabel(
      <InterruptedTaskIcon className="mt-[1px] h-3.5 w-3.5 shrink-0" />,
      accessibleLabel,
    );
  }
  if (classifyTask(sessionState, state) === "review") {
    return (
      <TaskReviewIcon
        isOnLastWorkflowStep={isOnLastWorkflowStep}
        accessibleLabel={accessibleLabel}
      />
    );
  }
  return withAccessibleLabel(
    <IconCircleDashed
      aria-hidden="true"
      data-testid="task-state-backlog"
      className="mt-[1px] h-3.5 w-3.5 shrink-0 text-muted-foreground/40"
    />,
    accessibleLabel,
  );
}

function getReviewLabelKey(state?: TaskState, isOnLastWorkflowStep?: boolean) {
  if (isOnLastWorkflowStep) return "common:taskStateCompleted";
  if (state === "FAILED") return "common:taskStateFailed";
  if (state === "CANCELLED") return "common:taskStateCancelled";
  // A settled turn on a task that is still in flight: the workflow column says
  // REVIEW, but the next instruction has not arrived. Distinct from
  // `common:taskStateReview`, which stays the task-state label shared with the
  // state chips and filters.
  return "task:turnFinishedAwaitingInstruction";
}

function activeTaskStateIconLabel({
  state,
  foregroundActivity,
  hasPendingClarification,
  hasPendingPermission,
  parkedOnBackgroundWork,
  interrupted,
  waitReason,
}: TaskStateIconProps) {
  if ((hasPendingPermission ?? false) || hasPendingClarification) {
    return "common:taskStateWaitingForInput";
  }
  if (foregroundActivity === "generating") return "common:taskStateInProgress";
  if (foregroundActivity === "background") return BACKGROUND_WORK_RUNNING_LABEL_KEY;
  if (parkedOnBackgroundWork) return BACKGROUND_WORK_RUNNING_LABEL_KEY;
  const visibleWait = visibleTaskWaitReason(waitReason, state, interrupted);
  if (visibleWait) return waitReasonLabelKey(visibleWait);
  if (shouldUseQuestionTaskIcon(state)) return "common:taskStateWaitingForInput";
  return undefined;
}

export function getTaskStateIconLabelKey({
  sessionState,
  state,
  foregroundActivity,
  hasPendingClarification,
  hasPendingPermission,
  isOnLastWorkflowStep,
  interrupted,
  parkedOnBackgroundWork,
  waitReason,
}: TaskStateIconProps) {
  const active = activeTaskStateIconLabel({
    state,
    foregroundActivity,
    hasPendingClarification,
    hasPendingPermission,
    parkedOnBackgroundWork,
    interrupted,
    waitReason,
  });
  if (active) return active;
  if (computeIsPreparing(state, sessionState)) return "common:taskStateScheduling";
  if (computeIsInProgress(state, sessionState)) {
    return "common:taskStateInProgress";
  }
  if (interrupted && !isTerminalInterruptedState(state, sessionState)) {
    return "common:interruptedByRestart";
  }
  if (classifyTask(sessionState, state) === "review") {
    return getReviewLabelKey(state, isOnLastWorkflowStep);
  }
  const stateLabelKeys: Partial<Record<TaskState, string>> = {
    CREATED: "common:taskStateCreated",
    TODO: "common:taskStateTodo",
    BLOCKED: "common:taskStateBlocked",
    FAILED: "common:taskStateFailed",
    CANCELLED: "common:taskStateCancelled",
    COMPLETED: "common:taskStateCompleted",
  };
  return state
    ? (stateLabelKeys[state] ?? "common:taskStateNotStarted")
    : "common:taskStateNotStarted";
}
