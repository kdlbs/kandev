"use client";

import { IconArrowBarToDown, IconBan } from "@tabler/icons-react";

import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import { useAppStore } from "@/components/state-provider";
import { useTranscriptAutoScrollEnabled } from "./use-transcript-auto-scroll-enabled";

// Matches the icon-only top-bar buttons (e.g. Share) so this control sits
// flush with them.
const TOP_BAR_BUTTON_CLASS =
  "h-6 w-6 p-0 cursor-pointer text-muted-foreground hover:bg-muted/70 hover:text-foreground focus-visible:ring-1 focus-visible:ring-ring";

type Props = {
  sessionId: string;
};

/**
 * Toggles whether the transcript auto-scrolls to the bottom as new messages
 * arrive. Enabled by default. Disabling freezes the current scroll position
 * (preserved across navigating away and back); re-enabling resumes
 * auto-scroll and catches the view up if the transcript progressed while
 * disabled. See message-list-native.tsx / message-list-virtuoso.tsx for the
 * renderer-side behavior driven by this preference.
 *
 * The icon renders pale green while enabled; disabling strips the color and
 * overlays a "forbidden" (ban) icon so the off state reads unambiguously
 * without relying on color alone.
 */
export function AutoScrollToggleButton({ sessionId }: Props) {
  const enabled = useTranscriptAutoScrollEnabled(sessionId);
  const setEnabled = useAppStore((state) => state.setTranscriptAutoScrollEnabled);

  const label = enabled ? "Turn off transcript auto-scroll" : "Turn on transcript auto-scroll";

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => setEnabled(sessionId, !enabled)}
          className={TOP_BAR_BUTTON_CLASS}
          aria-label={label}
          aria-pressed={enabled}
          data-testid="auto-scroll-toggle-button"
        >
          <span className="relative inline-flex h-3.5 w-3.5 items-center justify-center">
            <IconArrowBarToDown
              className={cn("h-3.5 w-3.5", enabled && "text-green-400")}
              data-testid="auto-scroll-toggle-icon"
            />
            {!enabled && (
              <IconBan
                className="absolute inset-0 h-3.5 w-3.5"
                aria-hidden="true"
                data-testid="auto-scroll-toggle-forbidden-icon"
              />
            )}
          </span>
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
