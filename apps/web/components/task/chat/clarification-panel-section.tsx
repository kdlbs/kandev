"use client";

import { useEffect, useId, useRef, useState, type Ref, type RefObject } from "react";
import { IconChevronDown, IconChevronUp, IconMessageQuestion } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { ClarificationInputOverlay } from "./clarification-input-overlay";
import { useComposerDisclosureContext } from "./composer-disclosure";
import { ResizeHandle } from "./resize-handle";
import { useResizableClarificationOverlay } from "@/hooks/use-resizable-clarification-overlay";
import type { ClarificationRequestMetadata, Message } from "@/lib/types/http";
import type { ClarificationOutcome } from "@/hooks/domains/session/use-clarification-group";
import type {
  LateClarificationSnapshot,
  LateClarificationState,
} from "@/hooks/use-late-clarification-message";
import type { MessageAdmissionOutcome } from "@/hooks/use-message-handler";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useClarificationResizeState } from "./clarification-draft-context";

const MIN_PANEL_HEIGHT = 120;
const EMPTY_HOST_REFS: readonly RefObject<HTMLElement | null>[] = [];

type ClarificationPanelSectionProps = {
  pending: boolean;
  messages: readonly Message[] | null | undefined;
  onResolved: () => void;
  shortcutScopeRef: RefObject<HTMLElement | null>;
  /**
   * True when the session no longer has a live clarification waiter.
   * Detached requests remain answerable even if their row refresh lags.
   */
  agentDisconnected?: boolean;
  initialMaxHeightVh: number;
  allocationRef?: RefObject<HTMLElement | null>;
  anchorRef?: RefObject<HTMLElement | null>;
  reservedSiblingRefs?: readonly RefObject<HTMLElement | null>[];
  reservedHeightPx?: number;
  // Shared late-message state keeps the recovery surface available when a
  // host unmounts and the transcript mounts a new form for the same bundle.
  onOutcome?: (outcome: ClarificationOutcome) => void;
  onLateAnswer?: (snapshot: LateClarificationSnapshot) => Promise<MessageAdmissionOutcome>;
  lateAnswerState?: LateClarificationState;
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
  agentDisconnected = false,
  initialMaxHeightVh,
  allocationRef,
  anchorRef,
  reservedSiblingRefs = EMPTY_HOST_REFS,
  reservedHeightPx = 0,
  onOutcome,
  onLateAnswer,
  lateAnswerState,
}: ClarificationPanelSectionProps) {
  const { t } = useTranslation();
  const disclosure = useComposerDisclosureContext();
  const { isMobile } = useResponsiveBreakpoint();
  const pendingId = pendingIdFromMessages(messages);
  const sizingState = useClarificationResizeState(pendingId);
  const [collapsed, setCollapsed] = useCollapsedForBundle(pendingId);
  const contentId = useId();
  const {
    height,
    maxHeight,
    maxHostHeight,
    minHeight,
    canExpand,
    canReset,
    containerRef,
    expandHeight,
    resetHeight,
    resetAllHeights,
    resizeHandleProps,
  } = useResizableClarificationOverlay({
    initialMaxHeightVh,
    allocationRef: disclosure?.clarificationAllocationRef ?? allocationRef,
    anchorRef,
    reservedSiblingRefs,
    reservedHeightPx: disclosure?.clarificationAllocationRef
      ? (disclosure.clarificationReservedHeightPx ?? reservedHeightPx)
      : reservedHeightPx,
    isPhone: isMobile,
    sizingState,
  });

  // A newly opened clarification starts expanded and auto-sized. Collapsing
  // an active one leaves its form state and user-selected height intact.
  useEffect(() => {
    if (!pending) resetAllHeights();
  }, [pending, resetAllHeights]);

  if (!pending) return null;

  const questionCount = messages?.length ?? 0;
  const actionLabel = collapsed ? t("chat:expandClarification") : t("chat:collapseClarification");

  return (
    <ClarificationPanelContent
      actionLabel={actionLabel}
      agentDisconnected={agentDisconnected}
      collapsed={collapsed}
      containerRef={containerRef}
      contentId={contentId}
      disclosure={disclosure}
      height={height}
      maxHeight={maxHeight}
      maxHostHeight={maxHostHeight}
      minHeight={minHeight}
      canExpand={canExpand}
      canReset={canReset}
      onExpand={expandHeight}
      onReset={resetHeight}
      messages={messages}
      onCollapse={() => setCollapsed(true)}
      onOutcome={onOutcome}
      onLateAnswer={onLateAnswer}
      lateAnswerState={lateAnswerState}
      onResolved={onResolved}
      onToggleCollapse={() => setCollapsed((current) => !current)}
      questionCount={questionCount}
      resizeHandleProps={resizeHandleProps}
      shortcutScopeRef={shortcutScopeRef}
    />
  );
}

type ClarificationPanelContentProps = {
  actionLabel: string;
  agentDisconnected: boolean;
  collapsed: boolean;
  containerRef: Ref<HTMLDivElement>;
  contentId: string;
  disclosure: ReturnType<typeof useComposerDisclosureContext>;
  height: number | null;
  maxHeight: number;
  maxHostHeight: number | null;
  minHeight: number;
  canExpand: boolean;
  canReset: boolean;
  messages: readonly Message[] | null | undefined;
  onCollapse: () => void;
  onOutcome?: (outcome: ClarificationOutcome) => void;
  onLateAnswer?: (snapshot: LateClarificationSnapshot) => Promise<MessageAdmissionOutcome>;
  lateAnswerState?: LateClarificationState;
  onResolved: () => void;
  onToggleCollapse: () => void;
  onExpand: () => void;
  onReset: () => void;
  questionCount: number;
  resizeHandleProps: Parameters<typeof ResizeHandle>[0];
  shortcutScopeRef: RefObject<HTMLElement | null>;
};

function clarificationContainerClassName(compact: boolean, footerScrollFallback: boolean): string {
  if (compact) return "h-11";
  if (footerScrollFallback) return "flex min-h-0 flex-col";
  return "flex min-h-[7.5rem] flex-col overflow-hidden overscroll-contain";
}

function clarificationScrollRegionClassName(
  compact: boolean,
  footerScrollFallback: boolean,
): string {
  if (compact) return "hidden";
  if (footerScrollFallback) return "min-h-0 flex-1 overflow-visible px-1";
  return "min-h-0 flex-1 overflow-y-auto px-1";
}

function ClarificationCollapsedHeader({
  actionLabel,
  collapsed,
  contentId,
  onToggleCollapse,
  questionCount,
}: Pick<
  ClarificationPanelContentProps,
  "actionLabel" | "collapsed" | "contentId" | "onToggleCollapse" | "questionCount"
>) {
  const { t } = useTranslation();
  return (
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
        onClick={onToggleCollapse}
      >
        {collapsed ? (
          <IconChevronUp className="h-4 w-4" />
        ) : (
          <IconChevronDown className="h-4 w-4" />
        )}
      </Button>
    </div>
  );
}

function ClarificationPanelContent({
  actionLabel,
  agentDisconnected,
  collapsed,
  containerRef,
  contentId,
  disclosure,
  height,
  maxHeight,
  maxHostHeight,
  minHeight,
  canExpand,
  canReset,
  messages,
  onCollapse,
  onOutcome,
  onLateAnswer,
  lateAnswerState,
  onResolved,
  onToggleCollapse,
  onExpand,
  onReset,
  questionCount,
  resizeHandleProps,
  shortcutScopeRef,
}: ClarificationPanelContentProps) {
  const compact = collapsed || questionCount === 0;
  const footerScrollFallback =
    Boolean(disclosure) && maxHostHeight !== null && maxHostHeight < MIN_PANEL_HEIGHT;
  const containerStyle = compact
    ? undefined
    : {
        minHeight: `${minHeight}px`,
        ...(footerScrollFallback ? {} : { maxHeight: `${maxHeight}px` }),
        // Threads owns the outer vertical scroll boundary in its bounded footer.
        overscrollBehaviorY: disclosure ? ("auto" as const) : undefined,
        ...(height !== null && !footerScrollFallback ? { height } : {}),
      };

  return (
    <div className="relative flex-shrink-0 border-t border-sky-400/30 bg-card">
      {!compact && <ResizeHandle {...resizeHandleProps} />}
      <div
        ref={containerRef}
        data-testid="clarification-overlay-container"
        data-host-max-height={maxHostHeight ?? ""}
        data-scroll-owner={footerScrollFallback ? "thread-footer" : "clarification-panel"}
        className={clarificationContainerClassName(compact, footerScrollFallback)}
        style={containerStyle}
      >
        {compact && (
          <ClarificationCollapsedHeader
            actionLabel={actionLabel}
            collapsed={collapsed}
            contentId={contentId}
            onToggleCollapse={onToggleCollapse}
            questionCount={questionCount}
          />
        )}
        <div
          id={contentId}
          data-testid="clarification-scroll-region"
          className={clarificationScrollRegionClassName(compact, footerScrollFallback)}
        >
          <ClarificationInputOverlay
            messages={messages}
            onResolved={onResolved}
            onOutcome={onOutcome}
            onLateAnswer={onLateAnswer}
            lateAnswerState={lateAnswerState}
            shortcutScopeRef={shortcutScopeRef}
            keyboardShortcutsEnabled={!collapsed}
            previewsEnabled={!collapsed}
            agentDisconnected={agentDisconnected}
            onDismiss={onCollapse}
            onCollapse={onCollapse}
            onExpandPanel={onExpand}
            onResetPanel={onReset}
            canExpandPanel={canExpand}
            canResetPanel={canReset}
            collapseContentId={contentId}
          />
        </div>
      </div>
    </div>
  );
}
