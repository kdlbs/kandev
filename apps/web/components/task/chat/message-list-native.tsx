"use client";

import { useEffect, useMemo, useRef, memo, forwardRef, useImperativeHandle } from "react";
import { SessionPanelContent } from "@kandev/ui/pannel-session";
import type { Message, TaskSessionState } from "@/lib/types/http";
import type { RenderItem } from "@/hooks/use-processed-messages";
import { OLDER_PAGE_LIMIT, useLazyLoadMessages } from "@/hooks/use-lazy-load-messages";
import { useSessionTurn } from "@/hooks/domains/session/use-session-turn";
import { MessageListFooter } from "./message-list-footer";
import { useNativeScrollManagement } from "./message-list-native-scroll";
import { scheduleAfterPanelRestore, useActivationPending } from "./transcript-auto-scroll";
import { useTranscriptAutoScrollEnabled } from "./use-transcript-auto-scroll-enabled";
import { useDockviewStore } from "@/lib/state/dockview-store";
import {
  type MessageListProps,
  type MessageListHandle,
  type LastPromptEdge,
  MessageListStatus,
  MessageItem,
  UnreadDivider,
  anchoredBarScrollOffsetPx,
  getItemKey,
  getConversationLoadingState,
  getEffectiveActiveTurnId,
  getLastTurnGroupId,
  getStreamingAgentMessageId,
  canReassertDividerScroll,
  resolveLastPromptEdge,
  isElementFullyVisible,
} from "./message-list-shared";

const DIVIDER_SETTLING_WINDOW_MS = 4000;

/** Notifies `onLastPromptEdgeChange`/`onFirstMessageHiddenChange` whenever the
 * last-prompt or first message crosses the container's viewport edges, so
 * the composer's scroll buttons and the anchored-bar affordance know when to
 * show themselves. A single scroll/resize listener drives both checks. */
function useTranscriptEdgeTracking(
  scrollRef: React.RefObject<HTMLDivElement | null>,
  lastPromptMessageId: string | null | undefined,
  onLastPromptEdgeChange: ((edge: LastPromptEdge) => void) | undefined,
  firstMessageId: string | null | undefined,
  onFirstMessageHiddenChange: ((isHidden: boolean) => void) | undefined,
) {
  useEffect(() => {
    const container = scrollRef.current;
    const lastTarget = lastPromptMessageId
      ? document.getElementById(`msg-${lastPromptMessageId}`)
      : null;
    const firstTarget = firstMessageId ? document.getElementById(`msg-${firstMessageId}`) : null;
    if (!container || !lastTarget) onLastPromptEdgeChange?.("visible");
    if (!container || !firstTarget) onFirstMessageHiddenChange?.(false);
    if (!container) return;

    const update = () => {
      if (lastTarget) onLastPromptEdgeChange?.(resolveLastPromptEdge(container, lastTarget));
      if (firstTarget) onFirstMessageHiddenChange?.(!isElementFullyVisible(container, firstTarget));
    };
    update();
    container.addEventListener("scroll", update, { passive: true });
    const resizeObserver = new ResizeObserver(update);
    resizeObserver.observe(container);
    if (lastTarget) resizeObserver.observe(lastTarget);
    if (firstTarget && firstTarget !== lastTarget) resizeObserver.observe(firstTarget);
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
  dividerBeforeItemKey?: string | null;
  anchoredBarHeight?: number;
  /** Initial/refetch loading: the sentinel's hard block. */
  messagesLoading: boolean;
  hasMore: boolean;
  isLoadingMore: boolean;
  loadMore: () => Promise<number>;
  lastPromptMessageId: string | null | undefined;
  onLastPromptEdgeChange: ((edge: LastPromptEdge) => void) | undefined;
  firstMessageId: string | null | undefined;
  onFirstMessageHiddenChange: ((isHidden: boolean) => void) | undefined;
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
};

function useNativeMessageListScroll(params: NativeMessageListScrollParams) {
  const {
    scrollRef,
    ref,
    items,
    messages,
    isWorking,
    sessionId,
    enabled,
    dividerBeforeItemKey,
    anchoredBarHeight,
    messagesLoading,
    hasMore,
    isLoadingMore,
    loadMore,
    lastPromptMessageId,
    onLastPromptEdgeChange,
    firstMessageId,
    onFirstMessageHiddenChange,
    scrollLayoutKey,
    isVisible,
  } = params;
  const {
    handleScrollToMessage,
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
    hasUnreadDivider: Boolean(dividerBeforeItemKey),
    messagesLoading,
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
    scrollLayoutKey,
    enabled,
    sessionId,
    isProgrammaticScrollLocked,
    isVisible,
  });
  useImperativeHandle(ref, () => ({ scrollToMessage: handleScrollToMessage }), [
    handleScrollToMessage,
  ]);
  useTranscriptEdgeTracking(
    scrollRef,
    lastPromptMessageId,
    onLastPromptEdgeChange,
    firstMessageId,
    onFirstMessageHiddenChange,
  );
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
      className="pb-2 scroll-mt-[calc(4rem+env(safe-area-inset-top))] sm:scroll-mt-[var(--anchored-bar-h,0px)]"
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
  hasMore: boolean;
  isLoadingMore: boolean;
  isInitialLoading: boolean;
  showLoadingState: boolean;
  retryLoadMore: () => void;
  showRecovery: boolean;
  sentinelRef: (node: HTMLDivElement | null) => void;
  lastTurnGroupId: string | null;
  activeTurnId: string | null;
  streamingMessageId: string | null;
  onScrollToMessage: (messageId: string, options?: { align?: "start" | "center" }) => void;
  autoScrollEnabled: boolean;
  dividerBeforeItemKey?: string | null;
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
 *   by BOTH of: the reader hasn't started scrolling yet (isUserScrolling
 *   — wheel/touchstart/keydown, since a plain 'scroll' event can't tell
 *   user intent apart from our own programmatic writes), AND still being
 *   within a short settling window since activation (isWithinSettlingWindow).
 *   The window exists so a live message arriving long after the visit has
 *   genuinely settled — with no wheel/touch/key event to catch, e.g. a
 *   scrollbar drag — can never re-trigger a correction; once either gate
 *   trips, it's the user's scroll position to own, same as Slack never
 *   re-snapping you to the unread line once you've started reading.
 * - didScrollToDivider and didInitialScroll are separate latches so the
 *   bottom-fallback firing first (before dividerBeforeItemKey resolves)
 *   doesn't block the divider correction from still applying once it
 *   does. The caller also supplies a layout key so a loading-state transition
 *   with an unchanged item count can re-assert the target. Disabled initial
 *   placement is owned by the native initial-position hook, while previews
 *   keep their existing visible-host behavior without participating in read
 *   tracking.
 */
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
  } = options;
  const { isVisibleRef, activationPendingRef } = useActivationPending(isVisible);
  const isUserScrollingRef = useDividerUserScrolling(scrollRef);

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

  const didInitialScroll = useRef(false);
  const didScrollToDivider = useRef(false);
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
    if (!el || itemCount === 0) return;

    const placeInitialPosition = () => {
      if (!isVisibleRef.current) return;
      const dockviewState = useDockviewStore.getState();
      const hasPendingLayoutRestore = dockviewState.pendingChatScrollTop !== null;
      const hasExplicitScrollTarget =
        sessionId !== null && dockviewState.scrollTarget?.sessionId === sessionId;
      const hasProgrammaticOwner = isProgrammaticScrollLocked();
      if (hasPendingLayoutRestore || hasExplicitScrollTarget || hasProgrammaticOwner) {
        didInitialScroll.current = true;
        activationPendingRef.current = false;
        if (hasExplicitScrollTarget || hasProgrammaticOwner) {
          isUserScrollingRef.current = true;
        }
        return;
      }
      const canReassertDivider = canReassertDividerScroll({
        hasDividerTarget: Boolean(dividerBeforeItemKey),
        didScrollToDivider: didScrollToDivider.current,
        isUserScrolling: isUserScrollingRef.current,
        isWithinSettlingWindow: isWithinSettlingWindow(),
      });
      if (canReassertDivider) {
        const dividerEl = el.querySelector<HTMLElement>(`[id="msg-${dividerBeforeItemKey}"]`);
        if (dividerEl) {
          // scrollIntoView aligns against the viewport, which puts the target
          // behind the fixed mobile session header instead of inside this
          // nested scroll container. Move by the relative geometry instead;
          // the desktop anchored prompt bar still reserves its measured height.
          const containerRect = el.getBoundingClientRect();
          const dividerRect = dividerEl.getBoundingClientRect();
          el.scrollTop += dividerRect.top - containerRect.top - anchoredBarOffsetPx;
          onDividerScroll?.();
          didScrollToDivider.current = true;
          didInitialScroll.current = true;
          activationPendingRef.current = false;
          return;
        }
      }
      if (didInitialScroll.current) return;
      // Disabled initial placement is owned by useInitialScrollPosition,
      // which restores the persisted reader offset (or its bottom fallback).
      // This divider hook only owns the enabled bottom fallback.
      if (!enabled) {
        didInitialScroll.current = true;
        activationPendingRef.current = false;
        return;
      }
      el.scrollTop = el.scrollHeight;
      didInitialScroll.current = true;
      activationPendingRef.current = false;
    };

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
    isVisible,
    scrollRef,
  ]);
}

function useDividerUserScrolling(scrollRef: React.RefObject<HTMLDivElement | null>) {
  const isUserScrollingRef = useRef(false);
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const markUserScrolling = () => {
      isUserScrollingRef.current = true;
    };
    el.addEventListener("wheel", markUserScrolling, { passive: true });
    el.addEventListener("touchstart", markUserScrolling, { passive: true });
    el.addEventListener("keydown", markUserScrolling);
    return () => {
      el.removeEventListener("wheel", markUserScrolling);
      el.removeEventListener("touchstart", markUserScrolling);
      el.removeEventListener("keydown", markUserScrolling);
    };
  }, [scrollRef]);
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
  hasMore,
  isLoadingMore,
  isInitialLoading,
  showLoadingState,
  retryLoadMore,
  showRecovery,
  sentinelRef,
  lastTurnGroupId,
  activeTurnId,
  streamingMessageId,
  onScrollToMessage,
  autoScrollEnabled,
  dividerBeforeItemKey,
}: NativeMessageListBodyProps) {
  return (
    <div className="p-4">
      {/* Sentinel for lazy loading older messages */}
      {hasMore && <div ref={sentinelRef} className="h-px" />}

      <MessageListStatus
        isLoadingMore={isLoadingMore}
        hasMore={hasMore}
        showLoadingState={showLoadingState}
        messagesLoading={messagesLoading}
        isInitialLoading={isInitialLoading}
        messagesCount={messages.length}
        onLoadMore={retryLoadMore}
        showRecovery={showRecovery}
      />

      {items.map((item) => (
        <MessageRow
          key={getItemKey(item)}
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
      ))}

      <MessageListFooter
        sessionState={sessionState}
        sessionId={sessionId}
        messages={messages}
        isWorking={isWorking}
        footerActionMessages={footerActionMessages}
      />

      {/* Bottom anchor keeps the view pinned while auto-scroll is enabled.
          The scroll container disables anchoring entirely while it is off,
          so status/footer updates cannot choose a different anchor and move
          the frozen transcript. */}
      <div style={{ overflowAnchor: autoScrollEnabled ? "auto" : "none", height: 1 }} />
    </div>
  );
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
      isWorking,
      sessionState,
      worktreePath,
      onOpenFile,
      lastPromptMessageId,
      onLastPromptEdgeChange,
      firstMessageId,
      onFirstMessageHiddenChange,
      stickyPromptBar,
      dividerBeforeItemKey,
      anchoredBarHeight,
      isVisible = true,
    }: MessageListProps,
    ref,
  ) {
    const scrollRef = useRef<HTMLDivElement>(null);

    const { isInitialLoading, showLoadingState } = getConversationLoadingState({
      messagesLoading,
      messagesCount: messages.length,
      isWorking,
      sessionState,
    });
    const { loadMore, hasMore, isLoadingMore } = useLazyLoadMessages(sessionId, {
      minTextPartsPerLoad: OLDER_PAGE_LIMIT,
    });
    const { activeTurnId } = useSessionTurn(sessionId);
    const effectiveActiveTurnId = getEffectiveActiveTurnId(activeTurnId, isWorking);
    const streamingMessageId = getStreamingAgentMessageId(messages);
    const lastTurnGroupId = useMemo(() => getLastTurnGroupId(items), [items]);
    const autoScrollEnabled = useTranscriptAutoScrollEnabled(sessionId);
    const { handleScrollToMessage, sentinelRef, retryLoadMore, showRecovery } =
      useNativeMessageListScroll({
        scrollRef,
        ref,
        items,
        messages,
        isWorking,
        sessionId,
        enabled: autoScrollEnabled,
        dividerBeforeItemKey,
        anchoredBarHeight,
        messagesLoading,
        hasMore,
        isLoadingMore,
        loadMore,
        lastPromptMessageId,
        onLastPromptEdgeChange,
        firstMessageId,
        onFirstMessageHiddenChange,
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
        className={`relative chat-message-list p-0 ${
          autoScrollEnabled ? "[overflow-anchor:auto]" : "[overflow-anchor:none]"
        }`}
      >
        {stickyPromptBar}
        <NativeMessageListBody
          items={items}
          messages={messages}
          footerActionMessages={footerActionMessages}
          permissionsByToolCallId={permissionsByToolCallId}
          childrenByParentToolCallId={childrenByParentToolCallId}
          taskId={taskId}
          sessionId={sessionId}
          isWorking={isWorking}
          messagesLoading={messagesLoading}
          sessionState={sessionState}
          worktreePath={worktreePath}
          onOpenFile={onOpenFile}
          hasMore={hasMore}
          isLoadingMore={isLoadingMore}
          isInitialLoading={isInitialLoading}
          showLoadingState={showLoadingState}
          retryLoadMore={retryLoadMore}
          showRecovery={showRecovery}
          sentinelRef={sentinelRef}
          lastTurnGroupId={lastTurnGroupId}
          activeTurnId={effectiveActiveTurnId}
          streamingMessageId={streamingMessageId}
          onScrollToMessage={handleScrollToMessage}
          autoScrollEnabled={autoScrollEnabled}
          dividerBeforeItemKey={dividerBeforeItemKey}
        />
      </SessionPanelContent>
    );
  }),
);
