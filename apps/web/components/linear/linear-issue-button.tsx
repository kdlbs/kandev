"use client";

import { useState } from "react";
import { IconHexagon } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { LinearIssueDialog } from "./linear-issue-dialog";
import { extractLinearKey } from "./linear-issue-common";

export { extractLinearKey };

type LinearIssueButtonProps = {
  workspaceId: string | null | undefined;
  taskTitle: string | undefined | null;
};

// LinearIssueButton sits in the task top bar. It extracts a Linear identifier
// from the task title and opens a full issue dialog on click.
export function LinearIssueButton({ workspaceId, taskTitle }: LinearIssueButtonProps) {
  const identifier = extractLinearKey(taskTitle);
  const [open, setOpen] = useState(false);

  if (!workspaceId || !identifier) return null;

  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            size="sm"
            variant="outline"
            className="cursor-pointer px-2 gap-1"
            onClick={() => setOpen(true)}
          >
            <IconHexagon className="h-4 w-4" />
            <span className="text-xs font-medium">{identifier}</span>
          </Button>
        </TooltipTrigger>
        <TooltipContent>Open Linear issue {identifier}</TooltipContent>
      </Tooltip>
      <LinearIssueDialog
        open={open}
        onOpenChange={setOpen}
        workspaceId={workspaceId}
        identifier={identifier}
      />
    </>
  );
}
