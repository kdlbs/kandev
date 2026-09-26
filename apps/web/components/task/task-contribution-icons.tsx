"use client";

import { useTranslation } from "react-i18next";
import {
  getCompactPRStatusAccessibleLabels,
  getPRAggregateStatusColor,
  getTaskPRAutomationSummary,
  PRTaskIcon,
  type TaskPRInfo,
} from "@/components/github/pr-task-icon";
import { PRStatusGlyph } from "@/components/github/pr-status-glyph";
import { MRTaskIcon } from "@/components/gitlab/mr-task-icon";
import { cn } from "@/lib/utils";

/** Shows PR icon from store (real data) or from prInfo prop (prototype/mock). */
function TaskPRIcon({ taskId, prInfo }: { taskId?: string; prInfo?: TaskPRInfo }) {
  const { t } = useTranslation();
  if (taskId) return <PRTaskIcon taskId={taskId} prInfo={prInfo} />;
  if (!prInfo) return null;
  const color = getPRAggregateStatusColor(prInfo.aggregateState ?? prInfo.state);
  const automation = getTaskPRAutomationSummary([], prInfo);
  const ariaLabel = [
    t("github:pullRequestStatus", { number: prInfo.number }),
    ...getCompactPRStatusAccessibleLabels(prInfo, t),
    prInfo.hasMergeConflicts ? t("github:conflicts") : null,
    automation.autoFixEnabled ? t("github:autoFixEnabledAria") : null,
    automation.autoMergeEnabled ? t("github:autoMergeEnabledAria") : null,
  ]
    .filter((label): label is string => label !== null)
    .join(", ");
  return (
    <span
      data-testid={taskId ? `pr-task-icon-${taskId}` : "pr-task-icon"}
      data-pr-state={prInfo.state}
      role="img"
      aria-label={ariaLabel}
      className={cn("inline-flex items-center shrink-0", color)}
    >
      <PRStatusGlyph
        hasMergeConflicts={prInfo.hasMergeConflicts}
        autoFixEnabled={automation.autoFixEnabled}
        autoMergeEnabled={automation.autoMergeEnabled}
      />
    </span>
  );
}

/**
 * PR badge (store or prInfo fallback) and MR badge as siblings, PR first. The
 * MR branch needs its own taskId guard because MRTaskIcon reads task-scoped
 * store data and cannot render without a task identity.
 */
export function TaskContributionIcons({
  taskId,
  prInfo,
}: {
  taskId?: string;
  prInfo?: TaskPRInfo;
}) {
  return (
    <>
      <TaskPRIcon taskId={taskId} prInfo={prInfo} />
      {taskId ? <MRTaskIcon taskId={taskId} /> : null}
    </>
  );
}
