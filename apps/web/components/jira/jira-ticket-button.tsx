"use client";

import { useState } from "react";
import { IconTicket } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { JiraTicketDialog } from "./jira-ticket-dialog";
import { extractJiraKey } from "./jira-ticket-common";

export { extractJiraKey };

type JiraTicketButtonProps = {
  workspaceId: string | null | undefined;
  taskTitle: string | undefined | null;
};

// JiraTicketButton sits in the task top bar. It extracts a Jira key from the
// task title and opens a full ticket dialog on click.
export function JiraTicketButton({ workspaceId, taskTitle }: JiraTicketButtonProps) {
  const ticketKey = extractJiraKey(taskTitle);
  const [open, setOpen] = useState(false);

  if (!workspaceId || !ticketKey) return null;

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
            <IconTicket className="h-4 w-4" />
            <span className="text-xs font-medium">{ticketKey}</span>
          </Button>
        </TooltipTrigger>
        <TooltipContent>Open Jira ticket {ticketKey}</TooltipContent>
      </Tooltip>
      <JiraTicketDialog
        open={open}
        onOpenChange={setOpen}
        workspaceId={workspaceId}
        ticketKey={ticketKey}
      />
    </>
  );
}
