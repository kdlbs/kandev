"use client";

import { IconCircleDot } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";

type IssueInfo = { url: string; number: number };

export function IssueTaskIcon({ issueInfo }: { issueInfo: IssueInfo }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <a
          href={issueInfo.url}
          target="_blank"
          rel="noopener noreferrer"
          onClick={(e) => e.stopPropagation()}
          className="inline-flex items-center shrink-0 text-green-600 hover:text-green-500 cursor-pointer"
          data-testid="issue-task-icon"
          aria-label={`Issue #${issueInfo.number}`}
        >
          <IconCircleDot className="h-3.5 w-3.5" />
        </a>
      </TooltipTrigger>
      <TooltipContent>Issue #{issueInfo.number}</TooltipContent>
    </Tooltip>
  );
}
