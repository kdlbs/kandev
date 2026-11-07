import type { ComponentType } from "react";
import { IconClockHour4, IconGauge, IconHourglass, IconMessageQuestion } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import { formatTime } from "@/lib/i18n/formats";
import type { Task } from "@/lib/types/http";
import type {
  TaskStatusSummaryLaunchQueue,
  TaskStatusSummaryQuotaWait,
} from "@/lib/types/task-status-summary";

/**
 * Why a task is not doing work, when the reason is an admission, an answer or a
 * limit wait rather than a workflow state. The workflow state alone cannot
 * express these: a task waiting for a step slot, a task waiting for a session
 * slot, a task whose session is waiting for an answer and a task whose model is
 * rate-limited all land in the same REVIEW bucket, so they used to render the
 * one green "turn finished" icon.
 */
export type TaskWaitReason = "wip_queue" | "session_ceiling" | "awaiting_answer" | "quota";

/** The admission facts every task-shaped surface already carries. */
export type WaitReasonTaskLike = {
  primarySessionState?: string | null;
  wipAdmitted?: boolean | null;
  queuedForStepId?: string | null;
  launchQueue?: TaskStatusSummaryLaunchQueue | null;
  quotaWait?: TaskStatusSummaryQuotaWait | null;
};

/**
 * The one place that decides which wait a task is in. Precedence is fixed:
 * a provider limit outranks everything, because no answer and no slot frees it
 * and only the clock does; an admission wait outranks a WIP overflow (a session-capacity
 * deferral is the harder wait — it survives across turns, while a WIP slot
 * frees as soon as a neighbour finishes).
 *
 * Pending clarification and permission are deliberately NOT inputs: those own
 * their own icons upstream. WAITING_FOR_INPUT alone also describes an ordinary
 * finished turn, so it does not establish that a question needs an answer.
 */
export function resolveWaitReason(task: WaitReasonTaskLike): TaskWaitReason | null {
  if (task.quotaWait) return "quota";
  if (task.launchQueue?.reason === "session_capacity") return "session_ceiling";
  if (task.wipAdmitted !== true && task.queuedForStepId) return "wip_queue";
  return null;
}

/** Snake-case API task projection -> wait reason. */
export function resolveApiTaskWaitReason(
  task: Pick<
    Task,
    "primary_session_state" | "wip_admitted" | "queued_for_step_id" | "status_summary"
  >,
): TaskWaitReason | null {
  return resolveWaitReason({
    primarySessionState: task.primary_session_state,
    wipAdmitted: task.wip_admitted,
    queuedForStepId: task.queued_for_step_id,
    launchQueue: task.status_summary?.launch_queue,
    quotaWait: task.status_summary?.quota_wait,
  });
}

const WAIT_REASON_ICON: Record<TaskWaitReason, ComponentType<{ className?: string }>> = {
  wip_queue: IconHourglass,
  session_ceiling: IconClockHour4,
  awaiting_answer: IconMessageQuestion,
  quota: IconGauge,
};

// Colors come from the shared status tone vocabulary (the same tokens the
// overview legend and its status dots use), so a wait reads as the same kind of
// state wherever it is shown, and so a new tone never gets spelled as a raw
// palette color at a call site.
const WAIT_REASON_CLASS: Record<TaskWaitReason, string> = {
  wip_queue: "text-status-info focus-visible:ring-status-info",
  session_ceiling: "text-status-delayed focus-visible:ring-status-delayed",
  awaiting_answer: "text-status-stalled focus-visible:ring-status-stalled",
  quota: "text-status-hold focus-visible:ring-status-hold",
};

const WAIT_REASON_LABEL_KEY: Record<TaskWaitReason, string> = {
  wip_queue: "task:waitingReasonWipQueue",
  session_ceiling: "task:waitingReasonSessionCapacity",
  awaiting_answer: "task:waitingReasonAwaitingAnswer",
  quota: "task:waitingReasonQuota",
};

const WAIT_REASON_HELP_KEY: Record<TaskWaitReason, string> = {
  wip_queue: "task:waitingReasonWipQueueHelp",
  session_ceiling: "task:waitingReasonSessionCapacityHelp",
  awaiting_answer: "task:waitingReasonAwaitingAnswerHelp",
  quota: "task:waitingReasonQuotaHelp",
};

export function waitReasonLabelKey(reason: TaskWaitReason): string {
  return WAIT_REASON_LABEL_KEY[reason];
}

export function waitReasonGlyph(reason: TaskWaitReason): ComponentType<{ className?: string }> {
  return WAIT_REASON_ICON[reason];
}

export function waitReasonClass(reason: TaskWaitReason): string {
  return WAIT_REASON_CLASS[reason];
}

type WaitReasonTooltipCopy = { label: string; help: string };

/**
 * The one line a wait explains itself with. A capacity wait names the occupancy
 * it is queued behind and a limit wait names the instant the limit lifts,
 * because those are the two facts a reader cannot get from the icon alone. Both
 * come from measured values; nothing here formats an absent value into a guess.
 */
function useWaitReasonCopy(
  reason: TaskWaitReason,
  launchQueue: TaskStatusSummaryLaunchQueue | null | undefined,
  quotaWait: TaskStatusSummaryQuotaWait | null | undefined,
): WaitReasonTooltipCopy {
  const { t } = useTranslation();
  const label = t(WAIT_REASON_LABEL_KEY[reason]);
  const capacity = launchQueue?.capacity;
  if (reason === "session_ceiling" && capacity) {
    return {
      label,
      help: t("task:waitingReasonSessionCapacityOccupancy", {
        inUse: capacity.in_use,
        limit: capacity.limit,
      }),
    };
  }
  if (reason === "quota" && quotaWait) {
    const deadline = new Date(quotaWait.deadline);
    if (!Number.isNaN(deadline.getTime())) {
      return { label, help: t("task:waitingReasonQuotaClearsAt", { time: formatTime(deadline) }) };
    }
  }
  return { label, help: t(WAIT_REASON_HELP_KEY[reason]) };
}

/**
 * Shared affordance for a task that is waiting rather than working: a distinct
 * icon per wait reason, carrying the accessible label and the one-line reason,
 * so every surface that renders it presents the same thing (the
 * InterruptedTaskIcon / BackgroundWorkTaskIcon precedent).
 */
export function WaitingReasonTaskIcon({
  reason,
  launchQueue,
  quotaWait,
  className,
}: {
  reason: TaskWaitReason;
  launchQueue?: TaskStatusSummaryLaunchQueue | null;
  quotaWait?: TaskStatusSummaryQuotaWait | null;
  className?: string;
}) {
  const { label, help } = useWaitReasonCopy(reason, launchQueue, quotaWait);
  const Icon = WAIT_REASON_ICON[reason];
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          aria-label={label}
          tabIndex={0}
          className={cn(
            "mt-[1px] flex shrink-0 rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-1",
            WAIT_REASON_CLASS[reason],
            className,
          )}
        >
          <Icon
            aria-hidden="true"
            data-testid={`task-state-waiting-${reason}`}
            className="h-3.5 w-3.5 shrink-0"
          />
        </span>
      </TooltipTrigger>
      <TooltipContent side="right">
        <span className="flex flex-col">
          <span className="font-medium">{label}</span>
          <span className="text-muted-foreground">{help}</span>
        </span>
      </TooltipContent>
    </Tooltip>
  );
}
