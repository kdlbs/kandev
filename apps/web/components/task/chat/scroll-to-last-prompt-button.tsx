"use client";

import { IconArrowBarToUp, IconArrowDown, IconArrowUp, type Icon } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";

// Matches the ghost-style action buttons in the chat status bar (same row as
// Share) so both transcript-navigation entry points sit flush together.
const DEFAULT_CLASS =
  "h-6 w-6 p-0 cursor-pointer text-muted-foreground hover:bg-muted/70 hover:text-foreground focus-visible:ring-1 focus-visible:ring-ring";

type TranscriptScrollButtonProps = {
  onClick: () => void;
  label: string;
  testId: string;
  Icon: Icon;
  className?: string;
};

/** Shared transcript-jump button. Each destination owns its distinct icon. */
function TranscriptScrollButton({
  onClick,
  label,
  testId,
  Icon,
  className,
}: TranscriptScrollButtonProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          onClick={onClick}
          aria-label={label}
          data-testid={testId}
          className={cn(DEFAULT_CLASS, className)}
        >
          <Icon className="h-3.5 w-3.5" />
        </Button>
      </TooltipTrigger>
      <TooltipContent side="top">{label}</TooltipContent>
    </Tooltip>
  );
}

type ScrollButtonProps = {
  onClick: () => void;
  className?: string;
};

/**
 * Always-available affordance (desktop and mobile, regardless of the
 * anchored-bar setting): scrolls the transcript to the user's most recent
 * prompt. `direction` reflects where that prompt actually sits relative to
 * the current scroll position — `"up"` (default) when it's above the
 * viewport (scrolled past going down), `"down"` when it's below (not yet
 * reached, e.g. while browsing earlier history) — so the icon always points
 * the way the transcript will actually move.
 */
export function ScrollToLastPromptButton({
  onClick,
  className,
  direction = "up",
}: ScrollButtonProps & { direction?: "up" | "down" }) {
  return (
    <TranscriptScrollButton
      onClick={onClick}
      label="Scroll to last prompt"
      testId="scroll-to-last-prompt-button"
      className={className}
      Icon={direction === "down" ? IconArrowDown : IconArrowUp}
    />
  );
}

/**
 * Always-available affordance, shown next to {@link ScrollToLastPromptButton}
 * whenever the first message has scrolled out of view: jumps all the way
 * back to the start of the transcript.
 */
export function ScrollToStartButton({ onClick, className }: ScrollButtonProps) {
  return (
    <TranscriptScrollButton
      onClick={onClick}
      label="Scroll to start of transcript"
      testId="scroll-to-start-button"
      className={className}
      Icon={IconArrowBarToUp}
    />
  );
}
