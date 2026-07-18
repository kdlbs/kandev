"use client";

import { IconBrandAzure, IconExternalLink } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { useAzureDevOpsTaskPullRequests } from "@/hooks/domains/azure-devops/use-azure-devops-task-pull-requests";
import { getAzureDevOpsPullRequestPresentation } from "./azure-devops-status";

const TONE_CLASS = {
  success: "border-green-500/40 text-green-700 dark:text-green-300",
  danger: "border-destructive/40 text-destructive",
  warning: "border-amber-500/40 text-amber-700 dark:text-amber-300",
  muted: "text-muted-foreground",
  info: "border-cyan-500/40 text-cyan-700 dark:text-cyan-300",
};

export function AzureDevOpsTaskPullRequestChip({ taskId }: { taskId: string | null }) {
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const pullRequests = useAzureDevOpsTaskPullRequests(workspaceId, taskId);
  const first = pullRequests[0];
  if (!first) return null;
  const presentation = getAzureDevOpsPullRequestPresentation(first);
  const suffix = pullRequests.length > 1 ? ` +${pullRequests.length - 1}` : "";
  const label = `Azure PR ${first.pullRequestId}: ${presentation.label}${suffix}`;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          asChild
          variant="outline"
          size="sm"
          className={`h-7 max-w-56 cursor-pointer gap-1.5 px-2 ${TONE_CLASS[presentation.tone]}`}
        >
          <a href={first.pullRequestUrl} target="_blank" rel="noreferrer" aria-label={label}>
            <IconBrandAzure className="h-3.5 w-3.5 shrink-0" />
            <span className="truncate">PR {first.pullRequestId}</span>
            <span className="hidden truncate sm:inline">{presentation.label}</span>
            {suffix && <span className="shrink-0">{suffix}</span>}
            <IconExternalLink className="h-3 w-3 shrink-0" />
          </a>
        </Button>
      </TooltipTrigger>
      <TooltipContent className="max-w-80">
        <p className="font-medium">{first.title}</p>
        <p>{presentation.label}</p>
      </TooltipContent>
    </Tooltip>
  );
}
