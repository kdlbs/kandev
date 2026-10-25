"use client";

import { IconClockHour4 } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import type { ChangeRequestTaskSummaryRowDetail } from "@/components/integrations/change-request-task-status-summary";
import type { TaskPR } from "@/lib/types/github";

export type MergeQueueSummaryStatus =
  | "queue_queued"
  | "queue_awaiting_checks"
  | "queue_mergeable"
  | "queue_unmergeable"
  | "queue_locked";

export type MergeQueueEstimateDescription =
  | { kind: "none" }
  | { kind: "sub_minute" }
  | { kind: "minutes"; minutes: number };

const KNOWN_QUEUE_STATES: Record<string, MergeQueueSummaryStatus> = {
  queued: "queue_queued",
  awaiting_checks: "queue_awaiting_checks",
  mergeable: "queue_mergeable",
  unmergeable: "queue_unmergeable",
  locked: "queue_locked",
};

const QUEUE_STATE_LABEL_KEYS: Record<MergeQueueSummaryStatus, string> = {
  queue_queued: "github:mergeQueueStateQueued",
  queue_awaiting_checks: "github:mergeQueueStateAwaitingChecks",
  queue_mergeable: "github:mergeQueueStateMergeable",
  queue_unmergeable: "github:mergeQueueStateUnmergeable",
  queue_locked: "github:mergeQueueStateLocked",
};

export function normalizeMergeQueueState(state: string | null | undefined): string {
  return state?.trim().toLowerCase() ?? "";
}

export function getMergeQueueSummaryStatus(
  state: string | null | undefined,
): MergeQueueSummaryStatus {
  return KNOWN_QUEUE_STATES[normalizeMergeQueueState(state)] ?? "queue_queued";
}

export function describeMergeQueueEstimate(
  seconds: number | null | undefined,
): MergeQueueEstimateDescription {
  if (seconds == null || !Number.isFinite(seconds) || seconds < 0) {
    return { kind: "none" };
  }
  if (seconds < 60) return { kind: "sub_minute" };
  return { kind: "minutes", minutes: Math.ceil(seconds / 60) };
}

function validQueuePosition(position: number | null | undefined): number | null {
  if (position == null || !Number.isInteger(position) || position < 1) return null;
  return position;
}

export function getMergeQueueSummaryDetail(
  position: number | null | undefined,
  estimateSeconds: number | null | undefined,
): ChangeRequestTaskSummaryRowDetail | undefined {
  const queuePosition = validQueuePosition(position);
  const estimate = describeMergeQueueEstimate(estimateSeconds);
  if (queuePosition != null) {
    if (estimate.kind === "sub_minute") {
      return {
        key: "github:mergeQueuePositionAndSubMinuteEstimate",
        values: { position: queuePosition },
      };
    }
    if (estimate.kind === "minutes") {
      return {
        key: "github:mergeQueuePositionAndEstimate",
        values: { position: queuePosition, count: estimate.minutes },
      };
    }
    return { key: "github:mergeQueuePosition", values: { position: queuePosition } };
  }
  if (estimate.kind === "sub_minute") {
    return { key: "github:mergeQueueSubMinuteEstimate" };
  }
  if (estimate.kind === "minutes") {
    return { key: "github:mergeQueueEstimate", values: { count: estimate.minutes } };
  }
  return undefined;
}

export function hasActiveMergeQueueEntry(pr: Pick<TaskPR, "state" | "merge_queue_state">): boolean {
  return pr.state === "open" && normalizeMergeQueueState(pr.merge_queue_state) !== "";
}

export function PRMergeQueueStatus({ pr }: { pr: TaskPR }) {
  const { t } = useTranslation();
  if (!hasActiveMergeQueueEntry(pr)) return null;

  const status = getMergeQueueSummaryStatus(pr.merge_queue_state);
  const stateLabel = t(QUEUE_STATE_LABEL_KEYS[status]);
  const detail = getMergeQueueSummaryDetail(
    pr.merge_queue_position,
    pr.merge_queue_estimated_time_to_merge_seconds,
  );

  return (
    <div
      data-testid="pr-merge-queue-status"
      className="flex items-start gap-1.5 px-1 py-1 text-xs text-[#966600]"
    >
      <IconClockHour4 className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" />
      <div className="min-w-0">
        <div className="font-medium">{t("github:mergeQueueStatus", { state: stateLabel })}</div>
        {detail && (
          <p className="text-[11px] leading-snug text-muted-foreground">
            {t(detail.key, detail.values)}
          </p>
        )}
      </div>
    </div>
  );
}
