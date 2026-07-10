"use client";

import { memo, useCallback } from "react";
import {
  IconSettings,
  IconX,
  IconLayoutColumns,
  IconLayoutRows,
  IconTextWrap,
  IconRoute,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { Checkbox } from "@kandev/ui/checkbox";
import type { DiffComment } from "@/lib/diff/types";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { updateUserSettings } from "@/lib/api";
import { VcsSplitButton } from "@/components/vcs-split-button";
import { FixCommentsButton } from "./review-fix-comments-button";

type ReviewTopBarProps = {
  sessionId: string;
  reviewedCount: number;
  totalCount: number;
  commentCount: number;
  baseBranch?: string;
  splitView: boolean;
  onToggleSplitView: (split: boolean) => void;
  wordWrap: boolean;
  onToggleWordWrap: (wrap: boolean) => void;
  onSendComments: (comments: DiffComment[]) => void;
  onClose: () => void;
  onRequestWalkthrough?: () => void;
  requestWalkthroughDisabled?: boolean;
  getPendingComments: () => DiffComment[];
  markCommentsSent: (ids: string[]) => void;
};

function sendAutoMarkSetting(checked: boolean) {
  const payload = { review_auto_mark_on_scroll: checked };
  const client = getWebSocketClient();
  if (client) {
    client.request("user.settings.update", payload).catch(() => {
      updateUserSettings(payload, { cache: "no-store" }).catch(() => {});
    });
  } else {
    updateUserSettings(payload, { cache: "no-store" }).catch(() => {});
  }
}

type ReviewSettingsMenuProps = {
  reviewAutoMarkOnScroll: boolean;
  onToggleAutoMark: (checked: boolean) => void;
};

function ReviewSettingsMenu({ reviewAutoMarkOnScroll, onToggleAutoMark }: ReviewSettingsMenuProps) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button size="sm" variant="ghost" className="px-2 cursor-pointer">
          <IconSettings className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-64">
        <DropdownMenuItem
          className="cursor-pointer gap-2"
          onSelect={(e) => {
            e.preventDefault();
            onToggleAutoMark(!reviewAutoMarkOnScroll);
          }}
        >
          <Checkbox checked={reviewAutoMarkOnScroll} className="pointer-events-none" />
          <span className="text-sm flex-1">Auto-mark reviewed on scroll</span>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

type ReviewProgressProps = { reviewedCount: number; totalCount: number };

function ReviewProgress({ reviewedCount, totalCount }: ReviewProgressProps) {
  const progressPercent = totalCount > 0 ? (reviewedCount / totalCount) * 100 : 0;
  return (
    <div className="flex items-center gap-2 flex-1 min-w-0 overflow-hidden">
      <div className="flex-1 h-2 rounded-full bg-muted overflow-hidden max-w-[200px]">
        <div
          className="h-full bg-primary rounded-full transition-all duration-300"
          style={{ width: `${progressPercent}%` }}
        />
      </div>
      <span className="text-xs text-muted-foreground truncate">
        {reviewedCount} of {totalCount} files reviewed
      </span>
    </div>
  );
}

function ReviewWalkthroughButton({
  onRequestWalkthrough,
  disabled,
}: {
  onRequestWalkthrough: (() => void) | undefined;
  disabled?: boolean;
}) {
  if (!onRequestWalkthrough) return null;
  const tooltip = disabled ? "Loading changed files..." : "Walk me through these changes";
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className="inline-flex"
          tabIndex={disabled ? 0 : undefined}
          aria-label={disabled ? tooltip : undefined}
        >
          <Button
            size="sm"
            variant="ghost"
            className="px-2 cursor-pointer"
            aria-label="Walk me through these review changes"
            data-testid="review-request-walkthrough"
            disabled={disabled}
            onClick={onRequestWalkthrough}
          >
            <IconRoute className="h-4 w-4" />
          </Button>
        </span>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}

export const ReviewTopBar = memo(function ReviewTopBar({
  sessionId,
  reviewedCount,
  totalCount,
  commentCount,
  baseBranch,
  splitView,
  onToggleSplitView,
  wordWrap,
  onToggleWordWrap,
  onSendComments,
  onClose,
  onRequestWalkthrough,
  requestWalkthroughDisabled,
  getPendingComments,
  markCommentsSent,
}: ReviewTopBarProps) {
  const reviewAutoMarkOnScroll = useAppStore((state) => state.userSettings.reviewAutoMarkOnScroll);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const userSettings = useAppStore((state) => state.userSettings);

  const handleFixComments = useCallback(() => {
    const comments = getPendingComments();
    if (comments.length === 0) return;
    onSendComments(comments);
    markCommentsSent(comments.map((c) => c.id));
  }, [getPendingComments, onSendComments, markCommentsSent]);

  const handleToggleAutoMark = useCallback(
    (checked: boolean) => {
      setUserSettings({ ...userSettings, reviewAutoMarkOnScroll: checked });
      sendAutoMarkSetting(checked);
    },
    [userSettings, setUserSettings],
  );

  return (
    <div className="flex items-center gap-3 px-4 py-2 border-b border-border bg-card/50 min-h-[48px]">
      <ReviewSettingsMenu
        reviewAutoMarkOnScroll={reviewAutoMarkOnScroll}
        onToggleAutoMark={handleToggleAutoMark}
      />
      <ReviewProgress reviewedCount={reviewedCount} totalCount={totalCount} />
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            size="sm"
            variant="ghost"
            className={`px-2 cursor-pointer ${wordWrap ? "bg-muted" : ""}`}
            onClick={() => onToggleWordWrap(!wordWrap)}
          >
            <IconTextWrap className="h-4 w-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>Toggle word wrap</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            size="sm"
            variant="ghost"
            className="px-2 cursor-pointer"
            onClick={() => onToggleSplitView(!splitView)}
          >
            {splitView ? (
              <IconLayoutRows className="h-4 w-4" />
            ) : (
              <IconLayoutColumns className="h-4 w-4" />
            )}
          </Button>
        </TooltipTrigger>
        <TooltipContent>
          {splitView ? "Switch to unified view" : "Switch to split view"}
        </TooltipContent>
      </Tooltip>
      {commentCount > 0 && (
        <FixCommentsButton
          commentCount={commentCount}
          getPendingComments={getPendingComments}
          onFixComments={handleFixComments}
        />
      )}
      <ReviewWalkthroughButton
        onRequestWalkthrough={onRequestWalkthrough}
        disabled={requestWalkthroughDisabled}
      />
      <VcsSplitButton sessionId={sessionId} baseBranch={baseBranch} />
      <Button
        size="sm"
        variant="ghost"
        className="px-2 cursor-pointer"
        onClick={onClose}
        aria-label="Close review"
      >
        <IconX className="h-4 w-4" />
      </Button>
    </div>
  );
});
