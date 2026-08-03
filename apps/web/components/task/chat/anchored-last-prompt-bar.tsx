"use client";

import { useLayoutEffect, useRef, useState } from "react";
import { IconChevronDown, IconChevronUp } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { cn } from "@/lib/utils";
import { stripSystemTags } from "@/lib/utils/system-tags";
import { MemoizedMarkdown } from "@/components/shared/memoized-markdown";
import { ScrollToLastPromptButton } from "./scroll-to-last-prompt-button";

type AnchoredLastPromptBarProps = {
  /** Raw content of the user's last prompt. */
  promptText: string;
  /** Whether the last prompt sits fully outside the transcript viewport. */
  isVisible: boolean;
  /** Scrolls the transcript back to the top of the last prompt. */
  onScrollUp: () => void;
  /** Whether the scroll-to-last-prompt control is enabled. */
  showScrollToLastPrompt?: boolean;
  /** Reports the pinned content's current rendered height (px) whenever it
   * changes, and 0 on unmount. Measures the content row nested *inside*
   * the grid item that the open/closed transform collapses via
   * `grid-template-rows` — that grandchild is normal block content, not
   * itself a grid item, so its own box keeps its natural (open) height
   * even while an ancestor's `overflow-hidden` clips it down to nothing
   * on screen. This lets a caller reserve scroll room for the bar ahead
   * of it actually opening (see resolveLastPromptControls). */
  onHeightChange?: (height: number) => void;
};

/** Reports `contentRef`'s rendered height to `onHeightChange` on mount and
 * on every subsequent resize, and 0 once unmounted. */
function useReportContentHeight(
  contentRef: React.RefObject<HTMLDivElement | null>,
  onHeightChange: ((height: number) => void) | undefined,
) {
  useLayoutEffect(() => {
    const el = contentRef.current;
    if (!el || !onHeightChange) return;

    const report = () => onHeightChange(el.offsetHeight);
    report();

    let observer: ResizeObserver | undefined;
    if ("ResizeObserver" in window) {
      observer = new ResizeObserver(report);
      observer.observe(el);
    }
    return () => {
      observer?.disconnect();
      onHeightChange(0);
    };
  }, [contentRef, onHeightChange]);
}

/** Proportional cap for the expanded view: 40% of the transcript scroll
 * container's actual height, so it stays sensible on both a tall
 * full-screen panel and a short embedded/split view. `.chat-message-list`
 * is the transcript's established scroll-container selector (also used by
 * dockview-scroll-preserve.ts and use-resizable-input.ts). Falls back to
 * 40% of the viewport height only when no such ancestor is mounted (e.g.
 * an isolated render) — the real app always has one. */
function useExpandedMaxHeight(textRef: React.RefObject<HTMLDivElement | null>) {
  const [expandedMaxHeight, setExpandedMaxHeight] = useState("40vh");

  useLayoutEffect(() => {
    const transcriptContainer = textRef.current?.closest<HTMLElement>(".chat-message-list");
    if (!transcriptContainer) return;

    const updateMaxHeight = () => {
      setExpandedMaxHeight(`${Math.round(transcriptContainer.clientHeight * 0.4)}px`);
    };

    updateMaxHeight();
    if (!("ResizeObserver" in window)) return;

    const observer = new ResizeObserver(updateMaxHeight);
    observer.observe(transcriptContainer);
    return () => observer.disconnect();
  }, [textRef]);

  return expandedMaxHeight;
}

/** The collapsed prompt always shows two lines; actual rendered overflow, not
 * character count, determines whether its expand control is useful. */
function useCanExpand(
  textRef: React.RefObject<HTMLDivElement | null>,
  expanded: boolean,
  visible: string,
) {
  const [canExpand, setCanExpand] = useState(false);

  useLayoutEffect(() => {
    const textElement = textRef.current;
    if (!textElement || expanded) return;

    const updateOverflow = () => {
      setCanExpand(textElement.scrollHeight > textElement.clientHeight);
    };

    updateOverflow();
    if (!("ResizeObserver" in window)) return;

    const observer = new ResizeObserver(updateOverflow);
    observer.observe(textElement);
    return () => observer.disconnect();
  }, [textRef, expanded, visible]);

  return canExpand;
}

/**
 * Desktop-only, opt-in "anchored bar" affordance: while the user's last
 * prompt sits fully outside the transcript viewport, a shortened copy sticks
 * to the top of the transcript (directly under the view tab selector, full
 * width) with a fluid grid-based show/hide transform. Offers an expand toggle
 * into a scrollable, max-height-bounded view — exactly like the message
 * queue's per-row expand — plus the same scroll-up button as the always-on
 * scroll-to-last-prompt affordance.
 */
export function AnchoredLastPromptBar({
  promptText,
  isVisible,
  onScrollUp,
  showScrollToLastPrompt = true,
  onHeightChange,
}: AnchoredLastPromptBarProps) {
  const [expanded, setExpanded] = useState(false);
  const textRef = useRef<HTMLDivElement>(null);
  const contentRef = useRef<HTMLDivElement>(null);
  const visible = stripSystemTags(promptText);
  const expandedMaxHeight = useExpandedMaxHeight(textRef);
  const canExpand = useCanExpand(textRef, expanded, visible);
  useReportContentHeight(contentRef, onHeightChange);

  useLayoutEffect(() => {
    setExpanded(false);
  }, [visible]);

  return (
    <div
      data-testid="anchored-last-prompt-bar"
      data-state={isVisible ? "open" : "closed"}
      aria-hidden={!isVisible}
      inert={!isVisible ? true : undefined}
      // The sticky zero-height anchor preserves the transcript's scroll
      // geometry. Its absolute child overlays the rows, so jumping to the
      // prompt can reveal it and collapse the bar without a layout feedback
      // loop that immediately hides the prompt again.
      // It must also sit above code-block copy controls, which use z-10.
      className="sticky top-0 z-20 h-0 w-full"
    >
      <div
        className={cn(
          "absolute inset-x-0 top-0 grid w-full border-b bg-card",
          "transition-[grid-template-rows] duration-300 ease-out motion-reduce:transition-none",
          isVisible ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
        )}
      >
        <div className="min-h-0 overflow-hidden">
          <div
            ref={contentRef}
            data-testid="anchored-last-prompt-content"
            className="flex items-start gap-2 px-4 py-4"
          >
            {showScrollToLastPrompt && (
              <ScrollToLastPromptButton
                onClick={onScrollUp}
                className="mt-0.5 h-6 w-6 shrink-0 cursor-pointer text-muted-foreground hover:text-foreground"
              />
            )}
            {/* Mirrors the real user-message bubble (rounded-2xl bg-primary/30)
              so the pinned copy reads as "the same prompt", just docked. */}
            <div
              ref={textRef}
              data-testid="anchored-last-prompt-text"
              data-expanded={expanded ? "true" : "false"}
              style={expanded ? { maxHeight: expandedMaxHeight } : undefined}
              className={cn(
                "min-w-0 flex-1 break-words rounded-2xl bg-primary/30 px-4 py-2.5 text-sm text-foreground/80",
                expanded ? "overflow-y-auto" : "max-h-[3.75rem] overflow-hidden leading-5",
              )}
            >
              <div className="markdown-body markdown-body-user max-w-none">
                <MemoizedMarkdown content={visible} />
              </div>
            </div>
            {canExpand && (
              <Button
                type="button"
                variant="ghost"
                size="icon"
                onClick={() => setExpanded((v) => !v)}
                aria-label={expanded ? "Collapse last prompt" : "Expand last prompt"}
                aria-expanded={expanded}
                data-testid="anchored-last-prompt-expand"
                className="h-6 w-6 shrink-0 cursor-pointer text-muted-foreground hover:text-foreground"
              >
                {expanded ? (
                  <IconChevronUp className="h-3.5 w-3.5" />
                ) : (
                  <IconChevronDown className="h-3.5 w-3.5" />
                )}
              </Button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
