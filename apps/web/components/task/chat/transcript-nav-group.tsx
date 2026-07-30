"use client";

import { ShareButton } from "@/components/task/share/share-button";
import { ScrollToLastPromptButton, ScrollToStartButton } from "./scroll-to-last-prompt-button";

export type TranscriptNavGroupProps = {
  canShare: boolean;
  taskId: string | null;
  sessionId: string | null;
  showScrollToLastPrompt?: boolean;
  onScrollToLastPrompt?: () => void;
  /** Where the last prompt sits relative to the viewport, i.e. which way
   * `onScrollToLastPrompt` will actually move the transcript. Defaults to
   * "up" for callers that don't track directionality. */
  lastPromptScrollDirection?: "up" | "down";
  showScrollToStart?: boolean;
  onScrollToStart?: () => void;
};

/** The "jump" buttons (scroll-to-start, scroll-to-last-prompt) plus Share,
 * grouped together and right-aligned in the chat status bar. */
export function TranscriptNavGroup({
  canShare,
  taskId,
  sessionId,
  showScrollToLastPrompt,
  onScrollToLastPrompt,
  lastPromptScrollDirection,
  showScrollToStart,
  onScrollToStart,
}: TranscriptNavGroupProps) {
  return (
    <div className="ml-auto flex shrink-0 items-center gap-1.5">
      {showScrollToStart && onScrollToStart && <ScrollToStartButton onClick={onScrollToStart} />}
      {showScrollToLastPrompt && onScrollToLastPrompt && (
        <ScrollToLastPromptButton
          onClick={onScrollToLastPrompt}
          direction={lastPromptScrollDirection}
        />
      )}
      {canShare && taskId && sessionId && (
        <ShareButton taskId={taskId} sessionId={sessionId} iconOnly />
      )}
    </div>
  );
}
