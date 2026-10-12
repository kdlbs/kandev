"use client";

import type { ReactNode } from "react";
import { ShareButton } from "@/components/task/share/share-button";
import { JumpToLatestButton } from "./jump-to-latest-button";
import { ScrollToLastPromptButton, ScrollToStartButton } from "./scroll-to-last-prompt-button";

export type TranscriptNavGroupProps = {
  canShare: boolean;
  taskId: string | null;
  sessionId: string | null;
  showJumpToLatest?: boolean;
  onJumpToLatest?: () => void;
  showScrollToLastPrompt?: boolean;
  onScrollToLastPrompt?: () => void;
  /** Where the last prompt sits relative to the viewport, i.e. which way
   * `onScrollToLastPrompt` will actually move the transcript. Defaults to
   * "up" for callers that don't track directionality. */
  lastPromptScrollDirection?: "up" | "down";
  showScrollToStart?: boolean;
  onScrollToStart?: () => void;
  usageControl?: ReactNode;
};

/** Transcript navigation, optional Usage entry point, and Share in status-row order. */
export function TranscriptNavGroup({
  canShare,
  taskId,
  sessionId,
  showJumpToLatest,
  onJumpToLatest,
  showScrollToLastPrompt,
  onScrollToLastPrompt,
  lastPromptScrollDirection,
  showScrollToStart,
  onScrollToStart,
  usageControl,
}: TranscriptNavGroupProps) {
  const hasNavigationControls = Boolean(
    (showJumpToLatest && onJumpToLatest) ||
    (showScrollToStart && onScrollToStart) ||
    (showScrollToLastPrompt && onScrollToLastPrompt),
  );

  return (
    <>
      {hasNavigationControls && (
        <div className="flex shrink-0 items-center gap-1.5">
          {onJumpToLatest && (
            <JumpToLatestButton isVisible={Boolean(showJumpToLatest)} onClick={onJumpToLatest} />
          )}
          {showScrollToStart && onScrollToStart && (
            <ScrollToStartButton onClick={onScrollToStart} />
          )}
          {showScrollToLastPrompt && onScrollToLastPrompt && (
            <ScrollToLastPromptButton
              onClick={onScrollToLastPrompt}
              direction={lastPromptScrollDirection}
            />
          )}
        </div>
      )}
      {usageControl}
      {canShare && taskId && sessionId && (
        <ShareButton taskId={taskId} sessionId={sessionId} iconOnly />
      )}
    </>
  );
}
