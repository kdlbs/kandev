"use client";

import { Trans, useTranslation } from "react-i18next";

import { formatRelativeTime } from "@/lib/utils";
import { t as translateStatic } from "@/lib/i18n";
import type { TaskDecision, TimelineEvent } from "@/app/office/tasks/[id]/types";

// formatDecisionLine renders the human-readable summary for a single
// task decision entry in the timeline. Approve rows read like
// "CEO approved this task"; request-changes rows append the comment.
//
// Module-level `t` rather than a hook: this is a plain helper called from
// render, so the call happens after a locale is active. `deciderName` is user
// data and is interpolated, never translated.
export function formatDecisionLine(decision: TaskDecision): string {
  const who = decision.deciderName?.trim() || translateStatic("task:someone");
  if (decision.decision === "approved") {
    return translateStatic("task:approvedThisTask", { who });
  }
  return decision.comment
    ? translateStatic("task:requestedChangesWithComment", { who, comment: decision.comment })
    : translateStatic("task:requestedChangesPlain", { who });
}

export function DecisionTimelineEntry({ decision }: { decision: TaskDecision }) {
  const { t } = useTranslation();
  const isApproval = decision.decision === "approved";
  return (
    <div
      className="flex items-center gap-2 px-4 py-1.5 text-xs text-muted-foreground"
      data-testid="decision-timeline-entry"
    >
      <span className={isApproval ? "text-green-600" : "text-red-600"}>
        {isApproval ? t("task:approvedLowercase") : t("task:requestedChanges")}
      </span>
      <span className="truncate">{formatDecisionLine(decision)}</span>
      <span className="ml-auto shrink-0">{formatRelativeTime(decision.createdAt)}</span>
    </div>
  );
}

export function TimelineEntry({ event }: { event: TimelineEvent }) {
  return (
    <div className="flex items-center gap-2 px-4 py-1.5 text-xs text-muted-foreground">
      {event.type === "status_change" && event.from && event.to ? (
        <span>
          <Trans i18nKey="task:statusChangedFromTo" values={{ from: event.from, to: event.to }}>
            Status changed from <strong>{event.from}</strong> to <strong>{event.to}</strong>
          </Trans>
        </span>
      ) : (
        // `event.type` is a backend enum; the underscore-to-space form is the
        // existing fallback rendering, not translated copy.
        <span>{event.type.replaceAll("_", " ")}</span>
      )}
      <span className="ml-auto shrink-0">{formatRelativeTime(event.at)}</span>
    </div>
  );
}
