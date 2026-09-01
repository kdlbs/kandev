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
import {
  InterruptedTaskIcon,
  isTerminalInterruptedState,
  shouldUsePermissionTaskIcon,
  shouldUseQuestionTaskIcon,
} from "@/lib/ui/state-icons";
import { classifyTask } from "./task-classify";

export type TaskStateIconProps = {
  sessionState?: TaskSessionState;
  state?: TaskState;
  foregroundActivity?: ForegroundActivity | null;
  hasPendingClarification?: boolean;
  hasPendingPermission?: boolean;
  isOnLastWorkflowStep?: boolean;
  interrupted?: boolean;
  accessibleLabel?: string;
  showBackgroundTooltip?: boolean;
};

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

function BackgroundWorkTaskIcon({ showTooltip }: { showTooltip: boolean }) {
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
          aria-label={t("task:backgroundWorkIsRunning")}
          tabIndex={0}
          className="mt-[1px] flex shrink-0 rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:ring-offset-1"
        >
          {spinner}
        </span>
      </TooltipTrigger>
      <TooltipContent side="right">{t("task:backgroundWorkIsRunning")}</TooltipContent>
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

export function TaskStateIcon({
  sessionState,
  state,
  foregroundActivity,
  hasPendingClarification,
  hasPendingPermission,
  isOnLastWorkflowStep,
  interrupted,
  accessibleLabel,
  showBackgroundTooltip = false,
}: TaskStateIconProps) {
  if (shouldUsePermissionTaskIcon(hasPendingPermission)) {
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
  return "common:taskStateReview";
}

export function getTaskStateIconLabelKey({
  sessionState,
  state,
  foregroundActivity,
  hasPendingClarification,
  hasPendingPermission,
  isOnLastWorkflowStep,
  interrupted,
}: TaskStateIconProps) {
  if (shouldUsePermissionTaskIcon(hasPendingPermission) || hasPendingClarification) {
    return "common:taskStateWaitingForInput";
  }
  if (foregroundActivity === "generating") return "common:taskStateInProgress";
  if (foregroundActivity === "background") return "task:backgroundWorkIsRunning";
  if (shouldUseQuestionTaskIcon(state)) return "common:taskStateWaitingForInput";
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
