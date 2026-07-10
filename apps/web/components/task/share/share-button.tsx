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

// Matches the ghost-style action buttons in the dockview header bar so the
// share entry point sits flush with the maximize/split/browser icons.
const TOP_BAR_BUTTON_CLASS =
  "h-6 w-6 p-0 cursor-pointer text-muted-foreground hover:bg-muted/70 hover:text-foreground focus-visible:ring-1 focus-visible:ring-ring";

/**
 * Renders the "Share" entry point. Callers are responsible for gating
 * visibility on `session.state` via {@link shareableSessionStateClient};
 * keeping that check at the call site lets us hide the button entirely
 * (rather than disabling it) for unshareable sessions.
 */
export function ShareButton({ taskId, sessionId, iconOnly = false }: Props) {
  const [open, setOpen] = useState(false);

  const button = iconOnly ? (
    <Button
      variant="ghost"
      size="sm"
      onClick={() => setOpen(true)}
      className={TOP_BAR_BUTTON_CLASS}
      aria-label="Share this task"
      data-testid="share-task-button"
    >
      <IconShare className="h-3.5 w-3.5" />
    </Button>
  ) : (
    <Button
      variant="outline"
      size="sm"
      onClick={() => setOpen(true)}
      className="cursor-pointer"
      aria-label="Share this task"
      data-testid="share-task-button"
    >
      <IconShare className="h-3.5 w-3.5" />
      <span className="ml-1">Share</span>
    </Button>
  );

  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>{button}</TooltipTrigger>
        <TooltipContent>Share this task conversation as a public link</TooltipContent>
      </Tooltip>
      <ShareDialog open={open} onOpenChange={setOpen} taskId={taskId} sessionId={sessionId} />
    </>
  );
}

// shareableSessionStateClient gates the Share entry point. Sessions that
// haven't produced any conversation yet (CREATED / STARTING) have nothing
// worth publishing; everything else — RUNNING, IDLE, WAITING_FOR_INPUT,
// COMPLETED, FAILED, CANCELLED — gets the button. Backend enforces the
// same rule (see internal/task/share/builder.go).
export function shareableSessionStateClient(state?: string | null): boolean {
  if (!state) return false;
  return state !== "CREATED" && state !== "STARTING";
}
