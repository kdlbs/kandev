"use client";

import type React from "react";
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  memo,
  forwardRef,
  useImperativeHandle,
} from "react";
import { Virtuoso, type VirtuosoHandle } from "react-virtuoso";
import { SessionPanelContent } from "@kandev/ui/pannel-session";
import type { RenderItem } from "@/hooks/use-processed-messages";
import type { Message, TaskSessionState } from "@/lib/types/http";
import { useLazyLoadMessages } from "@/hooks/use-lazy-load-messages";
import { useSessionTurn } from "@/hooks/domains/session/use-session-turn";
import { MessageListFooter } from "./message-list-footer";
import { useTranscriptAutoScrollEnabled } from "./use-transcript-auto-scroll-enabled";
import { useVirtuosoAutoScrollLifecycle } from "./message-list-virtuoso-auto-scroll";
import {
  findMessageItemIndex,
  resolveVirtuosoEdgeState,
  rangePositionToLastPromptEdge,
} from "./message-list-virtuoso-edges";
import {
  useGuardedFollowOutput,
  useProgrammaticScrollLock,
} from "./message-list-virtuoso-scroll-lock";
import { useStableFirstItemIndex } from "./message-list-virtuoso-index";
import { useVisibleScrollParent } from "./message-list-virtuoso-scroll-parent";
import {
  type MessageListProps,
  type MessageListHandle,
  type LastPromptEdge,
  MessageListStatus,
  MessageItem,
  UnreadDivider,
  anchoredBarScrollOffsetPx,
  canReassertDividerScroll,
  getItemKey,
  getConversationLoadingState,
  getEffectiveActiveTurnId,
  getStreamingAgentMessageId,
  getLastTurnGroupId,
  isElementFullyVisible,
  resolveLastPromptEdge,
} from "./message-list-shared";
import { createDebugLogger, isDebug } from "@/lib/debug/log";

const debugVirtuoso = createDebugLogger("chat:virtuoso");

type VirtuosoBodyProps = MessageListProps & {
  scrollParent: HTMLDivElement;
  activeTurnId: string | null;
  lastTurnGroupId: string | null;
  hasMore: boolean;
  isLoadingMore: boolean;
  loadMore: () => Promise<number>;
  Header: () => React.ReactNode;
  Footer: () => React.ReactNode;
  /** Whether the transcript auto-scroll toggle is enabled for this session. */
  enabled: boolean;
};

/** Virtuoso windowing can unmount an off-screen prompt. While the prompt's
 * row remains mounted, use native geometry so the bar opens only when no
 * prompt content remains visible; once unmounted, fall back to range position. */
type TranscriptRangeEdges = {
  lastPromptMessageId: string | null | undefined;
  onLastPromptEdgeChange: ((edge: LastPromptEdge) => void) | undefined;
  firstMessageId: string | null | undefined;
  onFirstMessageHiddenChange: ((isHidden: boolean) => void) | undefined;
};

function useTranscriptRangeTracking(
  items: RenderItem[],
  firstItemIndex: number,
  edges: TranscriptRangeEdges,
  scrollParent: HTMLDivElement,
) {
  const {
    lastPromptMessageId,
    onLastPromptEdgeChange,
    firstMessageId,
    onFirstMessageHiddenChange,
  } = edges;
  const lastPromptItemIndex = useMemo(
    () => findMessageItemIndex(items, lastPromptMessageId),
    [items, lastPromptMessageId],
  );
  const firstMessageItemIndex = useMemo(
    () => findMessageItemIndex(items, firstMessageId),
    [items, firstMessageId],
  );
  const rangeRef = useRef<{ startIndex: number; endIndex: number } | null>(null);
  const updateTranscriptEdges = useCallback(
    (range: { startIndex: number; endIndex: number }) => {
      const renderedRange = {
        start: range.startIndex - firstItemIndex,
        end: range.endIndex - firstItemIndex,
      };
      const lastPromptRow = scrollParent.querySelector<HTMLElement>(
        '[data-last-prompt-row="true"]',
      );
      onLastPromptEdgeChange?.(
        resolveVirtuosoEdgeState(lastPromptRow, scrollParent, lastPromptItemIndex, renderedRange, {
          geometryCheck: resolveLastPromptEdge,
          fromRangePosition: rangePositionToLastPromptEdge,
        }),
      );
      const firstMessageRow = scrollParent.querySelector<HTMLElement>(
        '[data-first-message-row="true"]',
      );
      onFirstMessageHiddenChange?.(
        resolveVirtuosoEdgeState(
          firstMessageRow,
          scrollParent,
          firstMessageItemIndex,
          renderedRange,
          {
            geometryCheck: (container, row) => !isElementFullyVisible(container, row),
            fromRangePosition: (position) => position !== "within",
          },
        ),
      );
    },
    [
      firstItemIndex,
      firstMessageItemIndex,
      lastPromptItemIndex,
      onFirstMessageHiddenChange,
      onLastPromptEdgeChange,
      scrollParent,
    ],
  );

  useEffect(() => {
    const handleScroll = () => {
      if (rangeRef.current) updateTranscriptEdges(rangeRef.current);
    };
    scrollParent.addEventListener("scroll", handleScroll, { passive: true });
    return () => scrollParent.removeEventListener("scroll", handleScroll);
  }, [scrollParent, updateTranscriptEdges]);

  return useCallback(
    (range: { startIndex: number; endIndex: number }) => {
      rangeRef.current = range;
      requestAnimationFrame(() => updateTranscriptEdges(range));
    },
    [updateTranscriptEdges],
  );
}

type RenderItemArgs = {
  items: RenderItem[];
  firstItemIndex: number;
  sessionId: string | null;
  permissionsByToolCallId: Map<string, Message>;
  childrenByParentToolCallId: Map<string, Message[]>;
  taskId?: string;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
  lastTurnGroupId: string | null;
  lastPromptMessageId: string | null | undefined;
  firstMessageId: string | null | undefined;
  activeTurnId: string | null;
  streamingMessageId: string | null;
  onScrollToMessage: (messageId: string, options?: { align?: "start" | "center" }) => void;
  dividerBeforeItemKey?: string | null;
};

/** Renders one transcript row for a given (rebased) Virtuoso item index. */
function useVirtuosoRenderItem(args: RenderItemArgs) {
  const {
    items,
    firstItemIndex,
    sessionId,
    permissionsByToolCallId,
    childrenByParentToolCallId,
    taskId,
    worktreePath,
    onOpenFile,
    lastTurnGroupId,
    activeTurnId,
    streamingMessageId,
    lastPromptMessageId,
    firstMessageId,
    onScrollToMessage,
    dividerBeforeItemKey,
  } = args;
  return useCallback(
    (index: number) => {
      const item = items[index - firstItemIndex];
      if (!item) return <div />;
      const isLastPromptRow =
        item.type === "turn_group"
          ? item.messages.some((message) => message.id === lastPromptMessageId)
          : item.type === "message" && item.message.id === lastPromptMessageId;
      const isFirstMessageRow =
        item.type === "turn_group"
          ? item.messages.some((message) => message.id === firstMessageId)
          : item.type === "message" && item.message.id === firstMessageId;

      return (
        <div
          id={`msg-${getItemKey(item)}`}
          className="pb-2"
          data-last-prompt-row={isLastPromptRow || undefined}
          data-first-message-row={isFirstMessageRow || undefined}
        >
          {dividerBeforeItemKey === getItemKey(item) && <UnreadDivider />}
          <MessageItem
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
          />
        </div>
      );
    },
    [
      items,
      firstItemIndex,
      sessionId,
      permissionsByToolCallId,
      childrenByParentToolCallId,
      taskId,
      worktreePath,
      onOpenFile,
      lastTurnGroupId,
      activeTurnId,
      streamingMessageId,
      lastPromptMessageId,
      firstMessageId,
      onScrollToMessage,
      dividerBeforeItemKey,
    ],
  );
}

/**
 * Corrects the initial scroll position to this visit's Slack-style unread
 * divider (see findUnreadDividerItemId) once it resolves, mirroring Slack
 * drawing the line where you left off instead of always landing on the
 * newest message.
 *
 * Deliberately NOT done via Virtuoso's own `initialTopMostItemIndex` prop
 * (which only takes effect on Virtuoso's first render): dividerBeforeItemKey
 * is derived from usePanelActive (Dockview's active-tab signal), which is
 * backed by useSyncExternalStore and typically only resolves true one
 * render *after* this component's own mount — so on Virtuoso's actual first
 * render it's still null even for a session that does have an unread
 * divider, and a value frozen at that first render could never be
 * corrected. Scrolling here imperatively, gated on a change to
 * dividerBeforeItemKey rather than mount, catches it whenever it resolves.
 *
 * `anchoredBar.offsetPx` negatively offsets the placement (see Virtuoso's
 * documented sticky-header pattern) so the divider lands below the
 * anchored last-prompt bar's pinned overlay instead of underneath it. The
 * bar's real height typically arrives an async tick after mount, after
 * the first placement already ran with offset 0 — {@link
 * canReassertDividerScroll} allows exactly one more correction once it
 * does, under the same settling-window/no-user-scroll gate
 * `useScrollToDividerOrBottom` (message-list-native.tsx) uses for its
 * own multi-wave corrections.
 *
 * The actual `scrollToIndex` call runs through `runLocked` (see
 * `useProgrammaticScrollLock`) so `followOutput` can't fight a reassertion
 * that fires while a live message is streaming in — without the lock, a
 * `followOutput` re-evaluation triggered by that same message could snap
 * the transcript back to the bottom and silently cancel the correction.
 */
export function useScrollToDividerOnceResolved(
  virtuosoRef: React.RefObject<VirtuosoHandle | null>,
  items: RenderItem[],
  firstItemIndex: number,
  dividerBeforeItemKey: string | null | undefined,
  anchoredBar: {
    offsetPx: number;
    scrollParent: HTMLDivElement;
    runLocked: (performScroll: () => void) => void;
  },
) {
  const { offsetPx: anchoredBarOffsetPx, scrollParent, runLocked } = anchoredBar;
  const isUserScrollingRef = useRef(false);
  useEffect(() => {
    const markUserScrolling = () => {
      isUserScrollingRef.current = true;
    };
    scrollParent.addEventListener("wheel", markUserScrolling, { passive: true });
    scrollParent.addEventListener("touchstart", markUserScrolling, { passive: true });
    scrollParent.addEventListener("keydown", markUserScrolling);
    return () => {
      scrollParent.removeEventListener("wheel", markUserScrolling);
      scrollParent.removeEventListener("touchstart", markUserScrolling);
      scrollParent.removeEventListener("keydown", markUserScrolling);
    };
  }, [scrollParent]);

  // Mirrors useScrollToDividerOrBottom's 4s settling window: bounds how
  // long the correction below can keep re-asserting after mount,
  // independent of user interaction.
  const mountedAtRef = useRef<number | null>(null);
  if (mountedAtRef.current === null) mountedAtRef.current = Date.now();
  const isWithinSettlingWindow = () => Date.now() - (mountedAtRef.current ?? 0) < 4000;

  const didScrollRef = useRef(false);
  useEffect(() => {
    if (!dividerBeforeItemKey) return;
    const dividerIndex = items.findIndex((item) => getItemKey(item) === dividerBeforeItemKey);
    if (dividerIndex < 0) return;
    const canReassert = canReassertDividerScroll({
      hasDividerTarget: true,
      didScrollToDivider: didScrollRef.current,
      isUserScrolling: isUserScrollingRef.current,
      isWithinSettlingWindow: isWithinSettlingWindow(),
    });
    if (!canReassert) return;
    runLocked(() => {
      virtuosoRef.current?.scrollToIndex({
        index: firstItemIndex + dividerIndex,
        align: "start",
        offset: -anchoredBarOffsetPx || 0,
      });
    });
    didScrollRef.current = true;
  }, [
    virtuosoRef,
    items,
    firstItemIndex,
    dividerBeforeItemKey,
    anchoredBarOffsetPx,
    scrollParent,
    runLocked,
  ]);
}

/** Debounced `startReached` handler for lazy-loading older messages: a
 * 500ms cooldown after each `loadMore()` settles prevents Virtuoso from
 * re-firing `startReached` (e.g. while the prepended page is still
 * settling scroll position) into a runaway fetch loop. */
function useLoadOlderOnStartReached(
  hasMore: boolean,
  isLoadingMore: boolean,
  loadMore: () => Promise<number>,
) {
  const loadCooldownRef = useRef(false);
  return useCallback(() => {
    if (hasMore && !isLoadingMore && !loadCooldownRef.current) {
      loadCooldownRef.current = true;
      loadMore().finally(() => {
        setTimeout(() => {
          loadCooldownRef.current = false;
        }, 500);
      });
    }
  }, [hasMore, isLoadingMore, loadMore]);
}

function useVirtuosoCallbacks(props: VirtuosoBodyProps) {
  const {
    items,
    messages,
    sessionId,
    permissionsByToolCallId,
    childrenByParentToolCallId,
    taskId,
  } = props;
  const { worktreePath, onOpenFile, lastTurnGroupId, activeTurnId, dividerBeforeItemKey } = props;
  const { hasMore, isLoadingMore, loadMore } = props;
  const virtuosoRef = useRef<VirtuosoHandle>(null);
  const itemCount = items.length;
  const streamingMessageId = getStreamingAgentMessageId(messages);
  const firstItemIndex = useStableFirstItemIndex(items);
  const { isLocked, runLocked } = useProgrammaticScrollLock(props.scrollParent);
  useScrollToDividerOnceResolved(virtuosoRef, items, firstItemIndex, dividerBeforeItemKey, {
    offsetPx: anchoredBarScrollOffsetPx(props.anchoredBarHeight),
    scrollParent: props.scrollParent,
    runLocked,
  });
  const handleStartReached = useLoadOlderOnStartReached(hasMore, isLoadingMore, loadMore);

  const handleScrollToMessage = useCallback(
    (messageId: string, options?: { align?: "start" | "center" }) => {
      const idx = findMessageItemIndex(items, messageId);
      if (idx >= 0)
        runLocked(() => {
          virtuosoRef.current?.scrollToIndex({
            index: firstItemIndex + idx,
            align: options?.align === "start" ? "start" : "center",
          });
        });
    },
    [items, firstItemIndex, runLocked],
  );

  const followOutput = useGuardedFollowOutput(isLocked, props.enabled);

  const { handleAtBottomStateChange, restoreStateFrom } = useVirtuosoAutoScrollLifecycle({
    sessionId,
    enabled: props.enabled,
    messages,
    virtuosoRef,
    firstItemIndex,
    itemCount,
  });

  const handleRangeChanged = useTranscriptRangeTracking(
    items,
    firstItemIndex,
    {
      lastPromptMessageId: props.lastPromptMessageId,
      onLastPromptEdgeChange: props.onLastPromptEdgeChange,
      firstMessageId: props.firstMessageId,
      onFirstMessageHiddenChange: props.onFirstMessageHiddenChange,
    },
    props.scrollParent,
  );

  const computeItemKey = useCallback(
    (index: number) => {
      const item = items[index - firstItemIndex];
      if (!item) return index;
      return getItemKey(item);
    },
    [items, firstItemIndex],
  );

  const renderItem = useVirtuosoRenderItem({
    items,
    firstItemIndex,
    sessionId,
    permissionsByToolCallId,
    childrenByParentToolCallId,
    taskId,
    worktreePath,
    onOpenFile,
    lastTurnGroupId,
    activeTurnId,
    streamingMessageId,
    lastPromptMessageId: props.lastPromptMessageId,
    firstMessageId: props.firstMessageId,
    onScrollToMessage: handleScrollToMessage,
    dividerBeforeItemKey,
  });

  return {
    virtuosoRef,
    itemCount,
    firstItemIndex,
    handleStartReached,
    computeItemKey,
    renderItem,
    handleScrollToMessage,
    handleRangeChanged,
    followOutput,
    handleAtBottomStateChange,
    restoreStateFrom,
  };
}

/**
 * Renders the `<Virtuoso>` element once a scroll parent is available,
 * wiring together the stable item index, render callbacks, last-prompt/
 * first-message range tracking, and the auto-scroll lifecycle
 * (follow/catch-up/restore) from {@link useVirtuosoAutoScrollLifecycle}.
 */
const VirtuosoBody = forwardRef<MessageListHandle, VirtuosoBodyProps>(
  function VirtuosoBody(props, ref) {
    const { scrollParent, Header, Footer } = props;
    const {
      virtuosoRef,
      itemCount,
      firstItemIndex,
      handleStartReached,
      computeItemKey,
      renderItem,
      handleScrollToMessage,
      handleRangeChanged,
      followOutput,
      handleAtBottomStateChange,
      restoreStateFrom,
    } = useVirtuosoCallbacks(props);
    useImperativeHandle(ref, () => ({ scrollToMessage: handleScrollToMessage }), [
      handleScrollToMessage,
    ]);

    // Captured once on mount — `initialTopMostItemIndex` only takes effect on
    // Virtuoso's first render, so logging it here tells us which item Virtuoso
    // anchored on for that lifecycle.
    const mountSnapshotRef = useRef<{ itemCount: number; firstItemIndex: number } | null>(null);
    useEffect(() => {
      if (!isDebug()) return;
      if (mountSnapshotRef.current) return;
      mountSnapshotRef.current = { itemCount, firstItemIndex };
      debugVirtuoso("mount", {
        itemCount,
        firstItemIndex,
        initialTopMostItemIndex: itemCount - 1,
        hasMore: props.hasMore,
        activeTurnId: props.activeTurnId,
        lastTurnGroupId: props.lastTurnGroupId ?? "-",
      });
    }, [itemCount, firstItemIndex, props.hasMore, props.activeTurnId, props.lastTurnGroupId]);

    return (
      <Virtuoso
        ref={virtuosoRef}
        /* Suppress Virtuoso's verbose internal logging in all environments */
        logLevel={Number.MAX_SAFE_INTEGER}
        customScrollParent={scrollParent}
        totalCount={itemCount}
        firstItemIndex={firstItemIndex}
        initialTopMostItemIndex={itemCount - 1}
        restoreStateFrom={restoreStateFrom}
        computeItemKey={computeItemKey}
        itemContent={renderItem}
        followOutput={followOutput}
        atBottomStateChange={handleAtBottomStateChange}
        startReached={handleStartReached}
        rangeChanged={handleRangeChanged}
        increaseViewportBy={200}
        atBottomThreshold={100}
        components={{ Header, Footer }}
      />
    );
  },
);

type HeaderFooterArgs = {
  isLoadingMore: boolean;
  hasMore: boolean;
  showLoadingState: boolean;
  messagesLoading: boolean;
  isInitialLoading: boolean;
  messages: Message[];
  loadMore: () => Promise<number>;
  sessionState?: TaskSessionState;
  sessionId: string | null;
  isWorking: boolean;
  footerActionMessages?: Message[];
};

/** Memoized Virtuoso Header (load-more status) and Footer (agent status + actions). */
function useVirtuosoHeaderFooter(args: HeaderFooterArgs) {
  const { isLoadingMore, hasMore, showLoadingState, messagesLoading, isInitialLoading } = args;
  const { messages, loadMore, sessionState, sessionId, isWorking, footerActionMessages } = args;
  const footerActions = useMemo(() => footerActionMessages ?? [], [footerActionMessages]);

  const Header = useCallback(
    () => (
      <MessageListStatus
        isLoadingMore={isLoadingMore}
        hasMore={hasMore}
        showLoadingState={showLoadingState}
        messagesLoading={messagesLoading}
        isInitialLoading={isInitialLoading}
        messagesCount={messages.length}
        onLoadMore={loadMore}
      />
    ),
    [
      isLoadingMore,
      hasMore,
      showLoadingState,
      messagesLoading,
      isInitialLoading,
      messages.length,
      loadMore,
    ],
  );

  const Footer = useCallback(
    () => (
      <MessageListFooter
        sessionState={sessionState}
        sessionId={sessionId}
        messages={messages}
        isWorking={isWorking}
        footerActionMessages={footerActions}
      />
    ),
    [sessionId, sessionState, messages, isWorking, footerActions],
  );

  return { Header, Footer, footerActions };
}

/**
 * Windowed transcript renderer for very long conversations (1000+ messages),
 * backed by react-virtuoso. Shows the loading/empty state until the scroll
 * parent and initial page are ready, then delegates to {@link VirtuosoBody}.
 */
export const VirtuosoMessageList = memo(
  forwardRef<MessageListHandle, MessageListProps>(function VirtuosoMessageList(props, ref) {
    const {
      items,
      messages,
      footerActionMessages,
      sessionId,
      messagesLoading,
      isWorking,
      sessionState,
      stickyPromptBar,
    } = props;
    const { scrollParent, setScrollRef } = useVisibleScrollParent();
    const { isInitialLoading, showLoadingState } = getConversationLoadingState({
      messagesLoading,
      messagesCount: messages.length,
      isWorking,
      sessionState,
    });
    const { loadMore, hasMore, isLoading: isLoadingMore } = useLazyLoadMessages(sessionId);
    const { activeTurnId } = useSessionTurn(sessionId);
    const effectiveActiveTurnId = getEffectiveActiveTurnId(activeTurnId, isWorking);
    const lastTurnGroupId = useMemo(() => getLastTurnGroupId(items), [items]);
    const autoScrollEnabled = useTranscriptAutoScrollEnabled(sessionId);

    const { Header, Footer, footerActions } = useVirtuosoHeaderFooter({
      isLoadingMore,
      hasMore,
      showLoadingState,
      messagesLoading,
      isInitialLoading,
      messages,
      loadMore,
      sessionState,
      sessionId,
      isWorking,
      footerActionMessages,
    });

    if (isInitialLoading || items.length === 0) {
      return (
        <SessionPanelContent className="relative chat-message-list p-0">
          {stickyPromptBar}
          <div className="p-4">
            <MessageListStatus
              isLoadingMore={isLoadingMore}
              hasMore={hasMore}
              showLoadingState={showLoadingState}
              messagesLoading={messagesLoading}
              isInitialLoading={isInitialLoading}
              messagesCount={messages.length}
              onLoadMore={loadMore}
            />
            <MessageListFooter
              sessionState={sessionState}
              sessionId={sessionId}
              messages={messages}
              isWorking={isWorking}
              footerActionMessages={footerActions}
            />
          </div>
        </SessionPanelContent>
      );
    }

    return (
      <SessionPanelContent ref={setScrollRef} className="relative chat-message-list p-0">
        {stickyPromptBar}
        <div className="p-4">
          {scrollParent && (
            <VirtuosoBody
              ref={ref}
              {...props}
              scrollParent={scrollParent}
              activeTurnId={effectiveActiveTurnId}
              lastTurnGroupId={lastTurnGroupId}
              hasMore={hasMore}
              isLoadingMore={isLoadingMore}
              loadMore={loadMore}
              Header={Header}
              Footer={Footer}
              enabled={autoScrollEnabled}
            />
          )}
        </div>
      </SessionPanelContent>
    );
  }),
);
