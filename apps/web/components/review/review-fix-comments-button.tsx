"use client";

import { useCallback } from "react";
import { IconMessageForward } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverAnchor, PopoverContent } from "@kandev/ui/popover";
import type { DiffComment } from "@/lib/diff/types";
import { useHoverPopover } from "@/hooks/domains/github/use-hover-popover";
import { ReviewCommentsOverview } from "./review-comments-overview";

const COMMENTS_HOVER_OPEN_DELAY_MS = 150;
const COMMENTS_HOVER_CLOSE_DELAY_MS = 150;

function shouldOpenOverviewOnClick(): boolean {
  return (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia("(hover: none), (pointer: coarse)").matches
  );
}

type FixCommentsButtonProps = {
  commentCount: number;
  getPendingComments: () => DiffComment[];
  onFixComments: () => void;
};

export function FixCommentsButton({
  commentCount,
  getPendingComments,
  onFixComments,
}: FixCommentsButtonProps) {
  const { open, onOpenChange, onTriggerEnter, onTriggerLeave, onContentEnter, onContentLeave } =
    useHoverPopover({
      openDelayMs: COMMENTS_HOVER_OPEN_DELAY_MS,
      closeDelayMs: COMMENTS_HOVER_CLOSE_DELAY_MS,
    });
  const handleClick = useCallback(() => {
    if (!open && shouldOpenOverviewOnClick()) {
      onOpenChange(true);
      return;
    }
    onFixComments();
  }, [open, onOpenChange, onFixComments]);

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverAnchor asChild>
        <span
          className="inline-flex"
          onMouseOver={onTriggerEnter}
          onMouseEnter={onTriggerEnter}
          onMouseMove={onTriggerEnter}
          onPointerOver={onTriggerEnter}
          onPointerEnter={onTriggerEnter}
          onPointerMove={onTriggerEnter}
          onMouseLeave={onTriggerLeave}
          onPointerLeave={onTriggerLeave}
          onFocus={onTriggerEnter}
          onBlur={onTriggerLeave}
        >
          <Button
            size="sm"
            variant="outline"
            className="cursor-pointer"
            onClick={handleClick}
            data-testid="review-fix-comments-button"
          >
            <IconMessageForward className="h-4 w-4" />
            Fix Comments
            <span className="ml-1 rounded-full bg-blue-500/10 px-1.5 py-0.5 text-xs font-medium text-blue-600 dark:text-blue-400">
              {commentCount}
            </span>
          </Button>
        </span>
      </PopoverAnchor>
      <PopoverContent
        side="bottom"
        align="end"
        sideOffset={8}
        className="w-80 p-0"
        onMouseEnter={onContentEnter}
        onMouseMove={onContentEnter}
        onPointerEnter={onContentEnter}
        onPointerMove={onContentEnter}
        onMouseLeave={onContentLeave}
        onPointerLeave={onContentLeave}
        onOpenAutoFocus={(e) => e.preventDefault()}
      >
        <ReviewCommentsOverview comments={getPendingComments()} />
      </PopoverContent>
    </Popover>
  );
}
