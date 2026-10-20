"use client";

/* eslint-disable max-lines -- native transcript composition owns scrolling. */

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  memo,
  forwardRef,
  useImperativeHandle,
} from "react";
import { cancelChatScrollMotion, listenForScrollIntent } from "./chat-scroll-motion";
import { useChatMotion } from "@/hooks/use-chat-motion";
import { ChatMotionProvider } from "./chat-motion";
import { SessionPanelContent } from "@kandev/ui/pannel-session";
import type { Message, TaskSessionState } from "@/lib/types/http";
import { parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";
import { useAppStore } from "@/components/state-provider";
import { useSessionTurnChanges } from "@/hooks/domains/session/use-turn-changes";
import { TurnChangedFilesCard } from "./turn-changed-files-card";
import { projectTurnChangeSummary } from "@/lib/turn-changes/projection";
import type { RenderItem } from "@/hooks/use-processed-messages";
import { OLDER_PAGE_LIMIT, useLazyLoadMessages } from "@/hooks/use-lazy-load-messages";
import { useSessionTurn } from "@/hooks/domains/session/use-session-turn";
import { MessageListFooter } from "./message-list-footer";
import { findMessageRow, useNativeScrollManagement } from "./message-list-native-scroll";
import { scheduleAfterPanelRestore, useActivationPending } from "./transcript-auto-scroll";
import { useTranscriptAutoScrollEnabled } from "./use-transcript-auto-scroll-enabled";
import { useDockviewStore } from "@/lib/state/dockview-store";
import {
  type MessageListProps,
  type MessageListHandle,
  type LastPromptEdge,
  type PromptNeighbors,
  MessageListStatus,
  MessageItem,
  UnreadDivider,
  anchoredBarScrollOffsetPx,
  getItemKey,
  getConversationLoadingState,
  getEffectiveActiveTurnId,
  getLastTurnGroupId,
  getStreamingAgentMessageId,
  hasTranscriptContentBelowViewport,
  canReassertDividerScroll,
  filterLaunchErrorItems,
  filterLaunchErrorMessages,
  resolveLastPromptEdge,
  findPromptNeighbors,
  resolveUnloadedPromptEdge,
  isElementFullyVisible,
} from "./message-list-shared";

const DIVIDER_SETTLING_WINDOW_MS = 4000;

type PromptEdgeTrackingOptions = {
  lastPromptMessageId: string | null | undefined;
  onLastPromptEdgeChange: ((edge: LastPromptEdge) => void) | undefined;
  firstMessageId: string | null | undefined;
  onFirstMessageHiddenChange: ((isHidden: boolean) => void) | undefined;
  hasContent: boolean;
  onLatestVisibilityChange: ((isVisible: boolean) => void) | undefined;
  prompt?: Message | null;
  unloaded?: boolean;
  items: RenderItem[];
};

function resolveTrackedPromptEdge(
  container: HTMLElement,
  messageId: string | null | undefined,
  neighbors: PromptNeighbors | null,
  olderRow: HTMLElement | null,
  newerRow: HTMLElement | null,
): LastPromptEdge {
  const row = findMessageRow(container, messageId ?? null);
  if (row) return resolveLastPromptEdge(container, row);
  if (!neighbors || (neighbors.unordered && (!neighbors.olderKey || !neighbors.newerKey)))
    return "visible";
  return resolveUnloadedPromptEdge(container, olderRow, newerRow);
}

function observeTrackedRows(
  observer: ResizeObserver,
  last: HTMLElement | null,
  older: HTMLElement | null,
  newer: HTMLElement | null,
  first: HTMLElement | null,
) {
  if (last) observer.observe(last);
  if (older && older !== last) observer.observe(older);
  if (newer && newer !== older && newer !== last) observer.observe(newer);
  if (first && first !== last && first !== older && first !== newer) observer.observe(first);
}

/** Tracks prompt edges and scroll-position affordances within this transcript. */
export function useTranscriptEdgeTracking(
  scrollRef: React.RefObject<HTMLDivElement | null>,
  {
    lastPromptMessageId,
    onLastPromptEdgeChange,
    firstMessageId,
    onFirstMessageHiddenChange,
    hasContent,
    onLatestVisibilityChange,
    prompt,
    unloaded,
    items,
  }: PromptEdgeTrackingOptions,
) {
  useEffect(() => {
    const container = scrollRef.current;
    if (!container) {
      onLastPromptEdgeChange?.("visible");
      onFirstMessageHiddenChange?.(false);
      onLatestVisibilityChange?.(false);
      return;
    }
    const lastTarget = findMessageRow(container, lastPromptMessageId ?? null);
    const firstTarget = findMessageRow(container, firstMessageId ?? null);
    const neighbors = unloaded && prompt && !lastTarget ? findPromptNeighbors(items, prompt) : null;
    const olderRow = findMessageRow(container, neighbors?.olderKey ?? null);
    const newerRow = findMessageRow(container, neighbors?.newerKey ?? null);
    const update = () => {
      onLastPromptEdgeChange?.(
        resolveTrackedPromptEdge(container, lastPromptMessageId, neighbors, olderRow, newerRow),
      );
      onFirstMessageHiddenChange?.(
        firstTarget ? !isElementFullyVisible(container, firstTarget) : false,
      );
      onLatestVisibilityChange?.(
        hasTranscriptContentBelowViewport({
          hasContent,
          scrollTop: container.scrollTop,
          scrollHeight: container.scrollHeight,
          clientHeight: container.clientHeight,
        }),
      );
    };
    update();
    container.addEventListener("scroll", update, { passive: true });
    const resizeObserver = new ResizeObserver(update);
    resizeObserver.observe(container);
    const content = container.querySelector<HTMLElement>("[data-chat-content]");
    if (content) resizeObserver.observe(content);
    observeTrackedRows(resizeObserver, lastTarget, olderRow, newerRow, firstTarget);
    return () => {
      container.removeEventListener("scroll", update);
      resizeObserver.disconnect();
    };
  }, [
    scrollRef,
    lastPromptMessageId,
    onLastPromptEdgeChange,
    firstMessageId,
    onFirstMessageHiddenChange,
    prompt,
    unloaded,
    items,
    hasContent,
    onLatestVisibilityChange,
  ]);
}

type NativeMessageListScrollParams = {
  scrollRef: React.RefObject<HTMLDivElement | null>;
  ref: React.ForwardedRef<MessageListHandle>;
  items: RenderItem[];
  messages: Message[];
  isWorking: boolean;
  sessionId: string | null;
  enabled: boolean;
  motionEnabled: boolean;
  dividerBeforeItemKey?: string | null;
  anchoredBarHeight?: number;
  /** Initial/refetch loading: the sentinel's hard block. */
  messagesLoading: boolean;
  historyRefreshPending: boolean;
  hasMore: boolean;
  isLoadingMore: boolean;
  loadMore: () => Promise<number>;
  lastPromptMessageId: string | null | undefined;
  lastPromptMessage: Message | null | undefined;
  lastPromptUnloaded: boolean | undefined;
  onLastPromptEdgeChange: ((edge: LastPromptEdge) => void) | undefined;
  firstMessageId: string | null | undefined;
  onFirstMessageHiddenChange: ((isHidden: boolean) => void) | undefined;
  onLatestVisibilityChange: ((isVisible: boolean) => void) | undefined;
  /** Changes when transcript status rows can add/remove space above messages. */
  scrollLayoutKey: string;
  isVisible: boolean;
};

type ScrollToDividerOptions = {
  onDividerScroll?: () => void;
  scrollLayoutKey?: string;
  enabled?: boolean;
  sessionId?: string | null;
  isProgrammaticScrollLocked?: () => boolean;
  isVisible?: boolean;
  historyRefreshPending?: boolean;
  onUserScrollIntent?: () => void;
  isReaderPositionClaimed?: () => boolean;
};

function hasCompetingInitialPositionOwner(
  hasPendingLayoutRestore: boolean,
  hasExplicitScrollTarget: boolean,
  hasProgrammaticOwner: boolean,
) {
  return hasPendingLayoutRestore || hasExplicitScrollTarget || hasProgrammaticOwner;
}

function applyInitialDividerPosition(params: {
  element: HTMLDivElement;
  isVisibleRef: { current: boolean };
  sessionId: string | null;
  isProgrammaticScrollLocked: () => boolean;
  didInitialScroll: { current: boolean };
  activationPendingRef: { current: boolean };
  isUserScrollingRef: { current: boolean };
  isReaderPositionClaimed?: () => boolean;
  dividerBeforeItemKey: string | null | undefined;
  didScrollToDivider: { current: boolean };
  isWithinSettlingWindow: () => boolean;
  anchoredBarOffsetPx: number;
  onDividerScroll?: () => void;
  enabled: boolean;
}) {
  const {
    element,
    isVisibleRef,
    sessionId,
    isProgrammaticScrollLocked,
    didInitialScroll,
    activationPendingRef,
    isUserScrollingRef,
    isReaderPositionClaimed,
    dividerBeforeItemKey,
    didScrollToDivider,
    isWithinSettlingWindow,
    anchoredBarOffsetPx,
    onDividerScroll,
    enabled,
  } = params;
  if (!isVisibleRef.current) return;
  const dockviewState = useDockviewStore.getState();
  const hasPendingLayoutRestore = dockviewState.pendingChatScrollTop !== null;
  const hasExplicitScrollTarget =
    sessionId !== null && dockviewState.scrollTarget?.sessionId === sessionId;
  const hasProgrammaticOwner = isProgrammaticScrollLocked();
  if (
    hasCompetingInitialPositionOwner(
      hasPendingLayoutRestore,
      hasExplicitScrollTarget,
      hasProgrammaticOwner,
    )
  ) {
    didInitialScroll.current = true;
    activationPendingRef.current = false;
    if (hasExplicitScrollTarget || hasProgrammaticOwner) {
      isUserScrollingRef.current = true;
    }
    return;
  }
  if (isReaderPositionClaimed?.()) {
    didInitialScroll.current = true;
    activationPendingRef.current = false;
    return;
  }
  const canReassertDivider = canReassertDividerScroll({
    hasDividerTarget: Boolean(dividerBeforeItemKey),
    didScrollToDivider: didScrollToDivider.current,
    isUserScrolling: isUserScrollingRef.current,
    isWithinSettlingWindow: isWithinSettlingWindow(),
  });
  if (canReassertDivider) {
    const dividerElement = element.querySelector<HTMLElement>(`[id="msg-${dividerBeforeItemKey}"]`);
    if (dividerElement) {
      // Align within the transcript container so fixed mobile headers and the
      // desktop anchored prompt bar both keep the divider visible.
      const containerRect = element.getBoundingClientRect();
      const dividerRect = dividerElement.getBoundingClientRect();
      cancelChatScrollMotion(element);
      element.scrollTop += dividerRect.top - containerRect.top - anchoredBarOffsetPx;
      onDividerScroll?.();
      didScrollToDivider.current = true;
      didInitialScroll.current = true;
      activationPendingRef.current = false;
      return;
    }
  }
  if (didInitialScroll.current) return;
  if (!enabled) {
    didInitialScroll.current = true;
    activationPendingRef.current = false;
    return;
  }
  cancelChatScrollMotion(element);
  element.scrollTop = element.scrollHeight;
  didInitialScroll.current = true;
  activationPendingRef.current = false;
}

function useNativeMessageListScroll(params: NativeMessageListScrollParams) {
  const {
    scrollRef,
    ref,
    items,
    messages,
    isWorking,
    sessionId,
    enabled,
    motionEnabled,
    dividerBeforeItemKey,
    anchoredBarHeight,
    messagesLoading,
    historyRefreshPending,
    hasMore,
    isLoadingMore,
    loadMore,
    lastPromptMessageId,
    lastPromptMessage,
    lastPromptUnloaded,
    onLastPromptEdgeChange,
    firstMessageId,
    onFirstMessageHiddenChange,
    onLatestVisibilityChange,
    scrollLayoutKey,
    isVisible,
  } = params;
  const {
    handleScrollToMessage,
    claimReaderPosition,
    isReaderPositionClaimed,
    scrollToLatest: handleScrollToLatest,
    sentinelRef,
    markNotNearBottom,
    isProgrammaticScrollLocked,
    retryLoadMore,
    showRecovery,
  } = useNativeScrollManagement({
    scrollRef,
    items,
    messages,
    isWorking,
    sessionId,
    enabled,
    motionEnabled,
    hasUnreadDivider: Boolean(dividerBeforeItemKey),
    messagesLoading,
    historyRefreshPending,
    hasMore,
    isLoadingMore,
    loadMore,
    isVisible,
  });
  const anchoredBarOffsetPx = anchoredBarScrollOffsetPx(anchoredBarHeight);
  useEffect(() => {
    scrollRef.current?.style.setProperty("--anchored-bar-h", `${anchoredBarOffsetPx}px`);
  }, [anchoredBarOffsetPx]);
  useScrollToDividerOrBottom(scrollRef, items.length, dividerBeforeItemKey, anchoredBarOffsetPx, {
    onDividerScroll: markNotNearBottom,
    onUserScrollIntent: claimReaderPosition,
    isReaderPositionClaimed,
    scrollLayoutKey,
    enabled,
    sessionId,
    isProgrammaticScrollLocked,
    isVisible,
    historyRefreshPending,
  });
  useImperativeHandle(
    ref,
    () => ({
      scrollToMessage: handleScrollToMessage,
      scrollToLatest: handleScrollToLatest,
      claimReaderPosition,
    }),
    [claimReaderPosition, handleScrollToMessage, handleScrollToLatest],
  );
  useTranscriptEdgeTracking(scrollRef, {
    lastPromptMessageId,
    onLastPromptEdgeChange,
    firstMessageId,
    onFirstMessageHiddenChange,
    prompt: lastPromptMessage,
    unloaded: lastPromptUnloaded,
    items,
    hasContent: items.length > 0,
    onLatestVisibilityChange,
  });
  return { handleScrollToMessage, sentinelRef, retryLoadMore, showRecovery };
}

type MessageRowProps = {
  item: RenderItem;
  sessionId: string | null;
  permissionsByToolCallId: Map<string, Message>;
  childrenByParentToolCallId: Map<string, Message[]>;
  taskId?: string;
  worktreePath?: string;
  onOpenFile?: (path: string, repo?: string) => void;
  isLastGroup: boolean;
  activeTurnId: string | null;
  streamingMessageId: string | null;
  onScrollToMessage: (messageId: string, options?: { align?: "start" | "center" }) => void;
  dividerBeforeItemKey?: string | null;
};

function getItemTurnId(item: RenderItem): string | undefined {
  if (item.type === "turn_group") return item.turnId ?? undefined;
  if (item.type === "message") return item.message.turn_id ?? undefined;
  return undefined;
}

/** One transcript row, keyed and DOM-id'd by `getItemKey` so the scroll
 * affordances (and `scrollToMessage`) can locate it directly. */
function MessageRow({
  item,
  sessionId,
  permissionsByToolCallId,
  childrenByParentToolCallId,
  taskId,
  worktreePath,
  onOpenFile,
  isLastGroup,
  activeTurnId,
  streamingMessageId,
  onScrollToMessage,
  dividerBeforeItemKey,
}: MessageRowProps) {
  const key = getItemKey(item);
  return (
    <div
      id={`msg-${key}`}
      data-turn-id={getItemTurnId(item)}
      tabIndex={-1}
      className="pb-2 scroll-mt-[calc(4rem+env(safe-area-inset-top))] md:scroll-mt-[var(--anchored-bar-h,0px)]"
      style={{ overflowAnchor: "none" }}
    >
      {dividerBeforeItemKey === key && <UnreadDivider />}
      <MessageItem
        item={item}
        sessionId={sessionId}
        permissionsByToolCallId={permissionsByToolCallId}
        childrenByParentToolCallId={childrenByParentToolCallId}
        taskId={taskId}
        worktreePath={worktreePath}
        onOpenFile={onOpenFile}
        isLastGroup={isLastGroup}
        activeTurnId={activeTurnId}
        streamingMessageId={streamingMessageId}
        onScrollToMessage={onScrollToMessage}
      />
    </div>
  );
}

type NativeMessageListBodyProps = {
  items: RenderItem[];
  messages: Message[];
  footerActionMessages?: Message[];
  permissionsByToolCallId: Map<string, Message>;
  childrenByParentToolCallId: Map<string, Message[]>;
  taskId?: string;
  sessionId: string | null;
  isWorking: boolean;
  messagesLoading: boolean;
  sessionState?: TaskSessionState;
  worktreePath?: string;
  onOpenFile?: (path: string, repo?: string) => void;
  onOpenHistoricalDiff?: MessageListProps["onOpenHistoricalDiff"];
  turnChangeSummaries: NonNullable<MessageListProps["turnChangeSummaries"]>;
  messageHistoryHasMore: boolean;
  showTurnChangedFiles: boolean;
  hasMore: boolean;
  isLoadingMore: boolean;
  isInitialLoading: boolean;
  showLoadingState: boolean;
  historyStatus: MessageListProps["historyStatus"];
  historyError: MessageListProps["historyError"];
  onRetryHistory: MessageListProps["onRetryHistory"];
  retryLoadMore: () => void;
  showRecovery: boolean;
  sentinelRef: (node: HTMLDivElement | null) => void;
  lastTurnGroupId: string | null;
  activeTurnId: string | null;
  streamingMessageId: string | null;
  onScrollToMessage: (messageId: string, options?: { align?: "start" | "center" }) => void;
  autoScrollEnabled: boolean;
  dividerBeforeItemKey?: string | null;
  launchErrorOwned: boolean;
  launchErrorStamp?: string;
  launchErrorOccurredAt?: string;
};

/**
 * Scroll to bottom on initial load — or, if this visit's Slack-style "New"
 * boundary lands on a currently-loaded item, straight to the divider
 * instead (mirrors Slack drawing the line where you left off rather than
 * always jumping to the newest message).
 *
 * - dividerBeforeItemKey is derived from usePanelActive (Dockview's
 *   active-tab signal), backed by useSyncExternalStore, which only
 *   resolves true on a render *after* this component's own mount — so on
 *   the very first run here it's still null even for a session that does
 *   have an unread divider.
 * - The initial messages fetch can itself arrive in more than one wave
 *   (e.g. a WebSocket-delivered backfill continuing after this
 *   component's first commit, unrelated to user-triggered pagination),
 *   which can also retroactively shift where useScrollPositionOnPrepend
 *   lands the scroll. Rather than trying to classify every wave as
 *   "prepend" or "append" up front, the correction below keeps
 *   re-asserting the divider's position on every relevant change, bounded
 *   by BOTH of: the reader hasn't directed the transcript toward older
 *   content (a plain 'scroll' event can't distinguish reader input from our
 *   own programmatic writes), AND the visit is still within a short settling
 *   window. Once either gate trips, the reader owns the position.
 * - didScrollToDivider and didInitialScroll are separate latches so the
 *   bottom-fallback firing first (before dividerBeforeItemKey resolves)
 *   doesn't block the divider correction from still applying once it
 *   does. The caller also supplies a layout key so a loading-state transition
 *   with an unchanged item count can re-assert the target. Disabled initial
 *   placement is owned by the native initial-position hook, while previews
 *   keep their existing visible-host behavior without participating in read
 *   tracking.
 */
// eslint-disable-next-line max-lines-per-function -- divider and initial placement share one scroll lifecycle.
export function useScrollToDividerOrBottom(
  scrollRef: React.RefObject<HTMLDivElement | null>,
  itemCount: number,
  dividerBeforeItemKey: string | null | undefined,
  anchoredBarOffsetPx: number,
  options: ScrollToDividerOptions = {},
) {
  const {
    onDividerScroll,
    scrollLayoutKey = "",
    enabled = true,
    sessionId = null,
    isProgrammaticScrollLocked = () => false,
    isVisible = true,
    historyRefreshPending = false,
    onUserScrollIntent,
    isReaderPositionClaimed,
  } = options;
  const { isVisibleRef, activationPendingRef } = useActivationPending(isVisible);
  const didInitialScroll = useRef(false);
  const didScrollToDivider = useRef(false);
  const markReaderPosition = useCallback(() => {
    didInitialScroll.current = true;
    activationPendingRef.current = false;
    onUserScrollIntent?.();
  }, [activationPendingRef, onUserScrollIntent]);
  const isUserScrollingRef = useDividerUserScrolling(
    scrollRef,
    enabled ? undefined : markReaderPosition,
  );

  // Bounds how long the divider correction below can keep re-asserting
  // itself after activation, independent of user interaction: a scrollbar drag
  // (no wheel/touch/key event) or a live message arriving long after the
  // visit has settled must never be able to re-trigger it. 4s comfortably
  // covers the slowest observed multi-wave initial load (WS backfill
  // continuing after the REST fetch) without lingering into the range
  // where the user has plausibly started reading and scrolling normally.
  const previousVisibleForSettlingRef = useRef(isVisible);
  const settlingDeadlineRef = useRef<number | null>(null);
  const isWithinSettlingWindow = () =>
    settlingDeadlineRef.current !== null && Date.now() < settlingDeadlineRef.current;

  useEffect(() => {
    const becameVisible = isVisible && !previousVisibleForSettlingRef.current;
    previousVisibleForSettlingRef.current = isVisible;
    if (!isVisible) {
      settlingDeadlineRef.current = null;
      return;
    }
    if (settlingDeadlineRef.current === null || becameVisible) {
      settlingDeadlineRef.current = Date.now() + DIVIDER_SETTLING_WINDOW_MS;
    }
    const el = scrollRef.current;
    if (!el || historyRefreshPending) return;
    if (itemCount === 0) return;

    const placeInitialPosition = () =>
      applyInitialDividerPosition({
        element: el,
        isVisibleRef,
        sessionId,
        isProgrammaticScrollLocked,
        didInitialScroll,
        activationPendingRef,
        isUserScrollingRef,
        isReaderPositionClaimed,
        dividerBeforeItemKey,
        didScrollToDivider,
        isWithinSettlingWindow,
        anchoredBarOffsetPx,
        onDividerScroll,
        enabled,
      });

    if (activationPendingRef.current) {
      return scheduleAfterPanelRestore(placeInitialPosition);
    }
    placeInitialPosition();
  }, [
    itemCount,
    dividerBeforeItemKey,
    anchoredBarOffsetPx,
    onDividerScroll,
    scrollLayoutKey,
    enabled,
    sessionId,
    isProgrammaticScrollLocked,
    isReaderPositionClaimed,
    isVisible,
    historyRefreshPending,
    scrollRef,
  ]);
}

function useDividerUserScrolling(
  scrollRef: React.RefObject<HTMLDivElement | null>,
  onUserScrollIntent?: () => void,
) {
  const isUserScrollingRef = useRef(false);
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const markUserScrolling = () => {
      isUserScrollingRef.current = true;
      onUserScrollIntent?.();
    };
    return listenForScrollIntent(el, markUserScrolling);
  }, [onUserScrollIntent, scrollRef]);
  return isUserScrollingRef;
}

/** Sentinel, status/footer, and transcript rows — everything below the
 * (optional) anchored prompt bar inside the scroll container. */
function NativeMessageListBody({
  items,
  messages,
  footerActionMessages,
  permissionsByToolCallId,
  childrenByParentToolCallId,
  taskId,
  sessionId,
  isWorking,
  messagesLoading,
  sessionState,
  worktreePath,
  onOpenFile,
  onOpenHistoricalDiff,
  turnChangeSummaries,
  showTurnChangedFiles,
  hasMore,
  isLoadingMore,
  isInitialLoading,
  showLoadingState,
  historyStatus,
  historyError,
  onRetryHistory,
  retryLoadMore,
  showRecovery,
  sentinelRef,
  lastTurnGroupId,
  activeTurnId,
  streamingMessageId,
  onScrollToMessage,
  autoScrollEnabled,
  dividerBeforeItemKey,
  launchErrorOwned,
  launchErrorStamp,
  launchErrorOccurredAt,
}: NativeMessageListBodyProps) {
  return (
    <div className="p-4" data-chat-content>
      {/* Sentinel for lazy loading older messages */}
      {hasMore && <div ref={sentinelRef} className="h-px" />}

      <MessageListStatus
        isLoadingMore={isLoadingMore}
        hasMore={hasMore}
        showLoadingState={showLoadingState}
        messagesLoading={messagesLoading}
        isInitialLoading={isInitialLoading}
        messagesCount={messages.length}
        sessionId={sessionId}
        sessionState={sessionState}
        historyStatus={historyStatus}
        historyError={historyError}
        onRetryHistory={onRetryHistory}
        onLoadMore={retryLoadMore}
        showRecovery={showRecovery}
      />

      <TranscriptMessageItems
        items={items}
        permissionsByToolCallId={permissionsByToolCallId}
        childrenByParentToolCallId={childrenByParentToolCallId}
        taskId={taskId}
        sessionId={sessionId}
        worktreePath={worktreePath}
        onOpenFile={onOpenFile}
        onOpenHistoricalDiff={onOpenHistoricalDiff}
        turnChangeSummaries={turnChangeSummaries}
        messageHistoryHasMore={hasMore}
        showTurnChangedFiles={showTurnChangedFiles}
        lastTurnGroupId={lastTurnGroupId}
        activeTurnId={activeTurnId}
        streamingMessageId={streamingMessageId}
        onScrollToMessage={onScrollToMessage}
        dividerBeforeItemKey={dividerBeforeItemKey}
      />

      <NativeMessageListFooter
        sessionState={sessionState}
        sessionId={sessionId}
        messages={messages}
        isWorking={isWorking}
        footerActionMessages={footerActionMessages}
        launchErrorOwned={launchErrorOwned}
        launchErrorStamp={launchErrorStamp}
        launchErrorOccurredAt={launchErrorOccurredAt}
      />

      <TranscriptAutoScrollAnchor enabled={autoScrollEnabled} />
    </div>
  );
}

// The anchor owns the transcript position only while auto-scroll is enabled.
function TranscriptAutoScrollAnchor({ enabled }: { enabled: boolean }) {
  return <div style={{ overflowAnchor: enabled ? "auto" : "none", height: 1 }} />;
}

type TranscriptMessageItemsProps = Pick<
  NativeMessageListBodyProps,
  | "items"
  | "permissionsByToolCallId"
  | "childrenByParentToolCallId"
  | "taskId"
  | "sessionId"
  | "worktreePath"
  | "onOpenFile"
  | "onOpenHistoricalDiff"
  | "turnChangeSummaries"
  | "messageHistoryHasMore"
  | "showTurnChangedFiles"
  | "lastTurnGroupId"
  | "activeTurnId"
  | "streamingMessageId"
  | "onScrollToMessage"
  | "dividerBeforeItemKey"
>;

function TranscriptMessageItems({
  items,
  permissionsByToolCallId,
  childrenByParentToolCallId,
  taskId,
  sessionId,
  worktreePath,
  onOpenFile,
  onOpenHistoricalDiff,
  turnChangeSummaries,
  messageHistoryHasMore,
  showTurnChangedFiles,
  lastTurnGroupId,
  activeTurnId,
  streamingMessageId,
  onScrollToMessage,
  dividerBeforeItemKey,
}: TranscriptMessageItemsProps) {
  const fallbackByItem = new Map<number, NonNullable<MessageListProps["turnChangeSummaries"]>>();
  if (showTurnChangedFiles && sessionId) {
    turnChangeSummaries.forEach((summary) => {
      if (!projectTurnChangeSummary(summary).visible) return;
      if (
        !shouldRenderTurnChangeFallback(
          summary.final_assistant_message_id,
          items,
          messageHistoryHasMore,
        )
      )
        return;
      const index = turnChangeFallbackIndex(items, summary.turn_id, summary.terminal_at);
      if (index < 0) return;
      const bucket = fallbackByItem.get(index) ?? [];
      bucket.push(summary);
      fallbackByItem.set(index, bucket);
    });
  }
  return items.map((item, itemIndex) => (
    <div key={getItemKey(item)}>
      <MessageRow
        item={item}
        sessionId={sessionId}
        permissionsByToolCallId={permissionsByToolCallId}
        childrenByParentToolCallId={childrenByParentToolCallId}
        taskId={taskId}
        worktreePath={worktreePath}
        onOpenFile={onOpenFile}
        isLastGroup={item.type === "turn_group" && item.id === lastTurnGroupId}
        activeTurnId={activeTurnId}
        streamingMessageId={streamingMessageId}
        onScrollToMessage={onScrollToMessage}
        dividerBeforeItemKey={dividerBeforeItemKey}
      />
      <TurnChangedFilesAfterMessage
        item={item}
        sessionId={sessionId}
        summaries={turnChangeSummaries}
        enabled={showTurnChangedFiles}
        onOpenDiff={onOpenHistoricalDiff}
      />
      {(fallbackByItem.get(itemIndex) ?? []).map((summary) => (
        <div
          key={`turn-change-fallback-${summary.id}`}
          data-transcript-fallback-anchor={summary.fallback_anchor}
        >
          <TurnChangedFilesCard
            sessionId={sessionId!}
            summary={summary}
            onOpenDiff={onOpenHistoricalDiff ?? NOOP_HISTORICAL_DIFF}
          />
        </div>
      ))}
    </div>
  ));
}

export function shouldRenderTurnChangeFallback(
  finalAssistantMessageID: string | undefined,
  items: RenderItem[],
  messageHistoryHasMore: boolean,
): boolean {
  if (
    finalAssistantMessageID &&
    items.some((item) => item.type === "message" && item.message.id === finalAssistantMessageID)
  )
    return false;
  // The anchor may be in an older message page. Wait until history is complete
  // before treating its absence as a genuinely reply-less terminal turn.
  return !messageHistoryHasMore;
}

export function turnChangeFallbackIndex(
  items: RenderItem[],
  turnId: string,
  terminalAt?: string,
): number {
  let matchingTurnIndex = -1;
  items.forEach((item, index) => {
    if (item.type === "turn_group" && item.turnId === turnId) matchingTurnIndex = index;
    else if (item.type === "message" && item.message.turn_id === turnId) matchingTurnIndex = index;
  });
  if (matchingTurnIndex >= 0) return matchingTurnIndex;
  const terminalTime = parseTurnTimestamp(terminalAt);
  if (terminalTime === null) return items.length - 1;
  let lastBeforeTerminal = -1;
  items.forEach((item, index) => {
    const latestTime = messagesInRenderItem(item).reduce<bigint | null>((latest, message) => {
      const timestamp = parseTurnTimestamp(message.created_at);
      if (timestamp === null) return latest;
      return latest === null || timestamp > latest ? timestamp : latest;
    }, null);
    if (latestTime !== null && latestTime <= terminalTime) lastBeforeTerminal = index;
  });
  return lastBeforeTerminal;
}

function messagesInRenderItem(item: RenderItem): Message[] {
  if (item.type === "message") return [item.message];
  if (item.type === "turn_group") return item.messages;
  return [];
}

function NativeMessageListFooter(props: {
  sessionState?: TaskSessionState;
  sessionId: string | null;
  messages: Message[];
  isWorking: boolean;
  footerActionMessages?: Message[];
  launchErrorOwned: boolean;
  launchErrorStamp?: string;
  launchErrorOccurredAt?: string;
}) {
  return <MessageListFooter {...props} />;
}

type TurnChangedFilesTranscriptProps = {
  sessionId: string | null;
  summaries: NonNullable<MessageListProps["turnChangeSummaries"]>;
  enabled: boolean;
  onOpenDiff: MessageListProps["onOpenHistoricalDiff"];
};

const NOOP_HISTORICAL_DIFF = () => {};

function TurnChangedFilesAfterMessage({
  item,
  sessionId,
  summaries,
  enabled,
  onOpenDiff,
}: TurnChangedFilesTranscriptProps & { item: RenderItem }) {
  if (!enabled || !sessionId || item.type !== "message" || item.message.author_type !== "agent") {
    return null;
  }
  return summaries
    .filter(
      (summary) =>
        summary.final_assistant_message_id === item.message.id &&
        projectTurnChangeSummary(summary).visible,
    )
    .map((summary) => (
      <TurnChangedFilesCard
        key={summary.id}
        sessionId={sessionId}
        summary={summary}
        onOpenDiff={onOpenDiff ?? NOOP_HISTORICAL_DIFF}
      />
    ));
}

/**
 * Renders the transcript as plain DOM nodes with `overflow-anchor` for
 * scroll pinning. Wires together lazy-loading of older messages, the
 * scroll-position-on-prepend fix-up, last-prompt/first-message edge
 * tracking, and the session's auto-scroll toggle (freeze/resume/catch-up)
 * via {@link useNativeScrollManagement}.
 */
export const NativeMessageList = memo(
  // eslint-disable-next-line max-lines-per-function -- native transcript composition owns scrolling.
  forwardRef<MessageListHandle, MessageListProps>(function NativeMessageList(
    {
      items,
      messages,
      footerActionMessages,
      permissionsByToolCallId,
      childrenByParentToolCallId,
      taskId,
      sessionId,
      messagesLoading,
      historyRefreshPending = false,
      historyStatus = "ready",
      historyError = null,
      onRetryHistory,
      isWorking,
      sessionState,
      worktreePath,
      onOpenFile,
      onOpenHistoricalDiff,
      lastPromptMessageId,
      lastPromptMessage,
      lastPromptUnloaded,
      onLastPromptEdgeChange,
      firstMessageId,
      onFirstMessageHiddenChange,
      onLatestVisibilityChange,
      stickyPromptBar,
      dividerBeforeItemKey,
      anchoredBarHeight,
      isVisible = true,
      launchErrorOwned = false,
      launchErrorStamp,
      launchErrorOccurredAt,
    }: MessageListProps,
    ref,
  ) {
    const scrollRef = useRef<HTMLDivElement>(null);
    const showTurnChangedFiles = useAppStore((state) => state.userSettings.showTurnChangedFiles);
    const turnChanges = useSessionTurnChanges(sessionId);

    const visibleItems = useMemo(
      () =>
        filterLaunchErrorItems(items, launchErrorOwned, launchErrorStamp, launchErrorOccurredAt),
      [items, launchErrorOwned, launchErrorStamp, launchErrorOccurredAt],
    );
    const visibleMessages = useMemo(
      () =>
        filterLaunchErrorMessages(
          messages,
          launchErrorOwned,
          launchErrorStamp,
          launchErrorOccurredAt,
        ),
      [messages, launchErrorOwned, launchErrorStamp, launchErrorOccurredAt],
    );
    useEffect(() => {
      const visibleTurnIds = visibleItems.flatMap((item) => {
        if (item.type === "turn_group") return item.turnId ? [item.turnId] : [];
        if (item.type === "message") return item.message.turn_id ? [item.message.turn_id] : [];
        return [];
      });
      void turnChanges.ensureTurnsLoaded(visibleTurnIds);
    }, [visibleItems, turnChanges.ensureTurnsLoaded, turnChanges.summaries]);
    const visibleFooterActionMessages = useMemo(
      () =>
        filterLaunchErrorMessages(
          footerActionMessages ?? [],
          launchErrorOwned,
          launchErrorStamp,
          launchErrorOccurredAt,
        ),
      [footerActionMessages, launchErrorOwned, launchErrorStamp, launchErrorOccurredAt],
    );

    const { isInitialLoading, showLoadingState } = getConversationLoadingState({
      messagesLoading,
      messagesCount: visibleMessages.length,
      isWorking,
      sessionState,
    });
    const { loadMore, hasMore, isLoadingMore } = useLazyLoadMessages(sessionId, {
      minTextPartsPerLoad: OLDER_PAGE_LIMIT,
    });
    const { activeTurnId } = useSessionTurn(sessionId);
    const effectiveActiveTurnId = getEffectiveActiveTurnId(activeTurnId, isWorking);
    const streamingMessageId = getStreamingAgentMessageId(visibleMessages);
    const lastTurnGroupId = useMemo(() => getLastTurnGroupId(visibleItems), [visibleItems]);
    const autoScrollEnabled = useTranscriptAutoScrollEnabled(sessionId);
    const motionEnabled = useChatMotion();
    const { handleScrollToMessage, sentinelRef, retryLoadMore, showRecovery } =
      useNativeMessageListScroll({
        scrollRef,
        ref,
        items: visibleItems,
        messages: visibleMessages,
        isWorking,
        sessionId,
        enabled: autoScrollEnabled,
        motionEnabled,
        dividerBeforeItemKey,
        anchoredBarHeight,
        messagesLoading,
        historyRefreshPending,
        hasMore,
        isLoadingMore,
        loadMore,
        lastPromptMessageId,
        lastPromptMessage,
        lastPromptUnloaded,
        onLastPromptEdgeChange,
        firstMessageId,
        onFirstMessageHiddenChange,
        onLatestVisibilityChange,
        scrollLayoutKey: [
          messagesLoading,
          isInitialLoading,
          showLoadingState,
          isLoadingMore,
          hasMore,
          isWorking,
        ].join(":"),
        isVisible,
      });

    return (
      <SessionPanelContent
        ref={scrollRef}
        sessionId={sessionId ?? undefined}
        tabIndex={-1}
        className={`relative chat-message-list p-0 ${
          autoScrollEnabled &&
          (!motionEnabled || messagesLoading || historyRefreshPending || isLoadingMore)
            ? "[overflow-anchor:auto]"
            : "[overflow-anchor:none]"
        }`}
      >
        {stickyPromptBar}
        <ChatMotionProvider
          sessionId={sessionId}
          messages={visibleMessages}
          live={isVisible && !messagesLoading && !historyRefreshPending && !isLoadingMore}
        >
          <NativeMessageListBody
            items={visibleItems}
            messages={visibleMessages}
            footerActionMessages={visibleFooterActionMessages}
            permissionsByToolCallId={permissionsByToolCallId}
            childrenByParentToolCallId={childrenByParentToolCallId}
            taskId={taskId}
            sessionId={sessionId}
            isWorking={isWorking}
            messagesLoading={messagesLoading}
            sessionState={sessionState}
            worktreePath={worktreePath}
            onOpenFile={onOpenFile}
            onOpenHistoricalDiff={onOpenHistoricalDiff}
            turnChangeSummaries={turnChanges.summaries.filter(
              (summary) => projectTurnChangeSummary(summary).visible,
            )}
            messageHistoryHasMore={hasMore}
            showTurnChangedFiles={showTurnChangedFiles}
            hasMore={hasMore}
            isLoadingMore={isLoadingMore}
            isInitialLoading={isInitialLoading}
            showLoadingState={showLoadingState}
            historyStatus={historyStatus}
            historyError={historyError}
            onRetryHistory={onRetryHistory}
            retryLoadMore={retryLoadMore}
            showRecovery={showRecovery}
            sentinelRef={sentinelRef}
            lastTurnGroupId={lastTurnGroupId}
            activeTurnId={effectiveActiveTurnId}
            streamingMessageId={streamingMessageId}
            onScrollToMessage={handleScrollToMessage}
            autoScrollEnabled={autoScrollEnabled}
            dividerBeforeItemKey={dividerBeforeItemKey}
            launchErrorOwned={launchErrorOwned}
            launchErrorStamp={launchErrorStamp}
            launchErrorOccurredAt={launchErrorOccurredAt}
          />
        </ChatMotionProvider>
      </SessionPanelContent>
    );
  }),
);
