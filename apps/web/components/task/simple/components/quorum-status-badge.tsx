"use client";

import { Badge } from "@kandev/ui/badge";
import { useTranslation } from "react-i18next";
import { useTaskQuorum } from "@/hooks/domains/office/use-task-quorum";
import type { QuorumGuardEntry } from "@/lib/state/slices/office/quorum-types";
import type { Task } from "@/app/office/tasks/[id]/types";

export type QuorumAggregate =
  | { state: "clear" }
  | { state: "awaiting"; entry: QuorumGuardEntry }
  | { state: "stuck"; entry: QuorumGuardEntry | undefined };

// aggregateQuorum implements AC-61: a task's AC-24b guard entries collapse to
// exactly one card-level state. `stuck` outranks `awaiting` because a guard
// reporting a real diagnosis (e.g. a mistyped threshold) must not be masked
// by a sibling that is merely waiting on a decision — rendering it as
// "awaiting decisions" forever is the defect AC-53 exists to close. The
// selected entry is the first match in the entries' configured order
// (already AC-17-ordered by the AC-57d snapshot); `reevaluation_blocked`
// alone can trigger `stuck` with no qualifying entry (AC-62), hence the
// `undefined` case.
export function aggregateQuorum(
  entries: QuorumGuardEntry[],
  reevaluationBlocked: boolean,
): QuorumAggregate {
  const stuckEntry = entries.find((e) => !e.satisfied && e.reason !== "threshold_not_met");
  if (reevaluationBlocked || stuckEntry) {
    return { state: "stuck", entry: stuckEntry };
  }
  const awaitingEntry = entries.find((e) => !e.satisfied);
  if (awaitingEntry) {
    return { state: "awaiting", entry: awaitingEntry };
  }
  return { state: "clear" };
}

const ROLE_LABEL_KEYS: Record<string, string> = {
  approver: "task:roleApprover",
  reviewer: "task:roleReviewer",
};

type QuorumStatusBadgeProps = {
  task: Task;
};

export function QuorumStatusBadge({ task }: QuorumStatusBadgeProps) {
  const { t } = useTranslation();
  const { quorum } = useTaskQuorum(task.id, task.workspaceId);

  if (!quorum) return null;

  const aggregate = aggregateQuorum(quorum.guards, quorum.reevaluation_blocked);
  if (aggregate.state === "clear") return null;

  if (aggregate.state === "stuck") {
    const reasonLabel = aggregate.entry
      ? t(`task:quorumReason.${aggregate.entry.reason}`, { defaultValue: aggregate.entry.reason })
      : t("task:quorumStuckPendingReevaluation");
    return (
      <Badge variant="destructive" className="text-xs" data-testid="quorum-status-badge">
        <span>{t("task:quorumStuck")}</span>
        <span data-testid="quorum-status-reason">{reasonLabel}</span>
      </Badge>
    );
  }

  const { entry } = aggregate;
  const roleLabel = t(ROLE_LABEL_KEYS[entry.role] ?? entry.role, { defaultValue: entry.role });

  return (
    <Badge variant="outline" className="text-xs" data-testid="quorum-status-badge">
      {t("task:quorumAwaiting", {
        received: entry.received_count,
        required: entry.required_count,
        role: roleLabel,
      })}
    </Badge>
  );
}
