"use client";

import { useState } from "react";
import { IconShare } from "@tabler/icons-react";

import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";

import { ShareDialog } from "./share-dialog";

type Props = {
  taskId: string;
  sessionId: string;
  /** When true, render as an icon-only button suited for top bars. */
  iconOnly?: boolean;
};

/**
 * Renders the "Share" entry point. Callers are responsible for gating
 * visibility on `session.state === "COMPLETED"`. Keeping that check at the
 * call site lets us hide the button entirely (rather than disabling it)
 * for in-progress tasks, which avoids visual noise.
 */
export function ShareButton({ taskId, sessionId, iconOnly = false }: Props) {
  const [open, setOpen] = useState(false);

  const button = (
    <Button
      variant="outline"
      size="sm"
      onClick={() => setOpen(true)}
      className="cursor-pointer"
      aria-label="Share this task"
      data-testid="share-task-button"
    >
      <IconShare className="h-3.5 w-3.5" />
      {!iconOnly && <span className="ml-1">Share</span>}
    </Button>
  );

  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>{button}</TooltipTrigger>
        <TooltipContent>Share this completed task with a public link</TooltipContent>
      </Tooltip>
      <ShareDialog open={open} onOpenChange={setOpen} taskId={taskId} sessionId={sessionId} />
    </>
  );
}
