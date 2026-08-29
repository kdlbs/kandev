import type { TaskCIAutomationOptions, TaskPR } from "@/lib/types/github";
import { automationForPR } from "./pr-status-automation-badges";
import type { TaskPRInfo } from "@/components/task/task-pr-info";

export type { TaskPRInfo } from "@/components/task/task-pr-info";

export type TaskPRAutomationDetail = {
  number: number;
  repository?: string;
  autoFixEnabled: boolean;
  autoMergeEnabled: boolean;
};

export type TaskPRAutomationSummary = {
  autoFixEnabled: boolean;
  autoMergeEnabled: boolean;
  details: TaskPRAutomationDetail[];
};

export function getTaskPRAutomationSummary(
  prs: TaskPR[],
  prInfo?: TaskPRInfo,
  options?: TaskCIAutomationOptions | null,
): TaskPRAutomationSummary {
  const activePRs = prs.filter((pr) => pr.state.trim().toLowerCase() === "open");
  // The bounded task-row projection is the source of truth until the full PR
  // list is hydrated. Keep its flags visible even when a previously hydrated
  // options response is present but the scoped PR records are not.
  if (!options || prs.length === 0) {
    return {
      autoFixEnabled: prInfo?.autoFixEnabled === true,
      autoMergeEnabled: prInfo?.autoMergeEnabled === true,
      details: [],
    };
  }
  const details = activePRs.map((pr) => {
    const automation = automationForPR(options, pr);
    const repository = [pr.owner, pr.repo].filter(Boolean).join("/") || undefined;
    return {
      number: pr.pr_number,
      ...(repository ? { repository } : {}),
      autoFixEnabled: automation.autoFix,
      autoMergeEnabled: automation.autoMerge,
    };
  });
  return {
    autoFixEnabled: details.some((detail) => detail.autoFixEnabled),
    autoMergeEnabled: details.some((detail) => detail.autoMergeEnabled),
    details,
  };
}
