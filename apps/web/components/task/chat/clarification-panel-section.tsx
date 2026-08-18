"use client";

import { useEffect, useId, useRef, useState, type RefObject } from "react";
import { IconChevronDown, IconChevronUp, IconMessageQuestion } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { ClarificationInputOverlay } from "./clarification-input-overlay";
import { ResizeHandle } from "./resize-handle";
import { useResizableClarificationOverlay } from "@/hooks/use-resizable-clarification-overlay";
import type { ClarificationRequestMetadata, Message } from "@/lib/types/http";

type ClarificationPanelSectionProps = {
  pending: boolean;
  messages: readonly Message[] | null | undefined;
  onResolved: () => void;
  shortcutScopeRef: RefObject<HTMLElement | null>;
  /**
   * Caps the expanded overlay's height as a percentage of the viewport.
   * Drives both the rendered CSS max-height and the resize hook's drag
   * clamp — the two MUST agree, or the drag handle silently stops
   * responding before its own visible ceiling.
   */
  maxHeightVh: number;
};

function pendingIdFromMessages(messages: readonly Message[] | null | undefined): string | null {
  const first = messages?.[0];
  if (!first) return null;
  const metadata = first.metadata as ClarificationRequestMetadata | undefined;
  return metadata?.pending_id ?? null;
}

// Tracks collapse state per bundle: a bundle the user dismissed stays
// collapsed while it's still pending, but a *new* bundle (different
// pending_id) always opens expanded, even if the previous one never resolved.
function useCollapsedForBundle(pendingId: string | null) {
  const [collapsed, setCollapsed] = useState(false);
  const lastPendingId = useRef(pendingId);
  useEffect(() => {
    if (pendingId !== lastPendingId.current) {
      lastPendingId.current = pendingId;
      setCollapsed(false);
    }
  }, [pendingId]);
  return [collapsed, setCollapsed] as const;
}

/**
 * Collapsible clarification section shared by the task chat panel and Quick
 * Chat. Escape (or the labelled toggle) collapses this to a persistent header
 * bar instead of answering or rejecting the bundle, so a blocked agent stays
 * visibly blocked and the question stays reachable until answered or skipped.
 */
export function ClarificationPanelSection({
  pending,
  messages,
  onResolved,
  shortcutScopeRef,
  maxHeightVh,
}: ClarificationPanelSectionProps) {
  const { t } = useTranslation();
  const pendingId = pendingIdFromMessages(messages);
  const [collapsed, setCollapsed] = useCollapsedForBundle(pendingId);
  const contentId = useId();
  const { height, containerRef, resetHeight, resizeHandleProps } = useResizableClarificationOverlay(
    maxHeightVh / 100,
  );

  // A newly opened clarification starts expanded and auto-sized. Collapsing
  // an active one leaves its form state and user-selected height intact.
  useEffect(() => {
    if (!pending) resetHeight();
  }, [pending, resetHeight]);

  // A new bundle (different pending_id) replacing a still-pending one must
  // not inherit the previous bundle's dragged height.
  useEffect(() => {
    resetHeight();
  }, [pendingId, resetHeight]);

  if (!pending) return null;

  const questionCount = messages?.length ?? 0;
  const actionLabel = collapsed ? t("chat:expandClarification") : t("chat:collapseClarification");
  const compact = collapsed || questionCount === 0;

  return (
    <div className="relative flex-shrink-0 border-t border-sky-400/30 bg-card">
      {!compact && <ResizeHandle {...resizeHandleProps} />}
      <div
        ref={containerRef}
        data-testid="clarification-overlay-container"
        className={
          compact ? "h-11" : "flex min-h-[7.5rem] flex-col overflow-hidden overscroll-contain"
        }
        style={
          compact
            ? undefined
            : { maxHeight: `${maxHeightVh}vh`, ...(height !== null ? { height } : {}) }
        }
      >
        {compact && (
          <div className="flex h-11 flex-shrink-0 items-center justify-between gap-2 pl-4">
            <div className="flex min-w-0 items-center gap-2 text-sm font-medium">
              <IconMessageQuestion className="h-4 w-4 flex-shrink-0 text-blue-500" />
              <span className="truncate">{t("chat:clarificationNeeded")}</span>
              {questionCount > 0 && (
                <span
                  data-testid="clarification-question-count"
                  className="truncate text-xs font-normal text-muted-foreground"
                >
                  {t("task:questionCount", { count: questionCount })}
                </span>
              )}
            </div>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-11 w-11 flex-shrink-0 cursor-pointer rounded-none"
              aria-label={actionLabel}
              aria-expanded={!collapsed}
              aria-controls={contentId}
              title={actionLabel}
              data-testid="clarification-collapse-toggle"
              onClick={() => setCollapsed((current) => !current)}
            >
              {collapsed ? (
                <IconChevronUp className="h-4 w-4" />
              ) : (
                <IconChevronDown className="h-4 w-4" />
              )}
            </Button>
          </div>
        )}
        <div
          id={contentId}
          data-testid="clarification-scroll-region"
          className={compact ? "hidden" : "min-h-0 flex-1 overflow-y-auto px-1"}
        >
          <ClarificationInputOverlay
            messages={messages}
            onResolved={onResolved}
            shortcutScopeRef={shortcutScopeRef}
            keyboardShortcutsEnabled={!collapsed}
            onDismiss={() => setCollapsed(true)}
            onCollapse={() => setCollapsed(true)}
            collapseContentId={contentId}
          />
        </div>
      </div>
    </div>
  );
}
