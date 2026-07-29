"use client";

import type React from "react";
import { useCallback, useEffect, useMemo, useRef, useState, memo } from "react";
import { Virtuoso, type VirtuosoHandle, type StateSnapshot } from "react-virtuoso";
import { SessionPanelContent } from "@kandev/ui/pannel-session";
import { useAppStoreApi } from "@/components/state-provider";
import type { RenderItem } from "@/hooks/use-processed-messages";
import type { Message, TaskSessionState } from "@/lib/types/http";
import { useLazyLoadMessages } from "@/hooks/use-lazy-load-messages";
import { useSessionTurn } from "@/hooks/domains/session/use-session-turn";
import { MessageListFooter } from "./message-list-footer";
import { useTranscriptAutoScrollEnabled } from "./use-transcript-auto-scroll-enabled";
import { resolveFollowOutput, shouldCatchUpOnAutoScrollEnable } from "./transcript-auto-scroll";
import {
  type MessageListProps,
  MessageListStatus,
  MessageItem,
  getItemKey,
  getConversationLoadingState,
  getEffectiveActiveTurnId,
  getLastTurnGroupId,
  getStreamingAgentMessageId,
} from "./message-list-shared";
import { createDebugLogger, isDebug } from "@/lib/debug/log";

const FIRST_INDEX_BASE = 100_000;

const debugVirtuoso = createDebugLogger("chat:virtuoso");
const debugScrollParent = createDebugLogger("chat:virtuoso:scrollParent");
const debugFirstIndex = createDebugLogger("chat:virtuoso:firstIndex");

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

function computeFirstItemIndex(prevKeys: string[], prevIndex: number, keys: string[]): number {
  if (prevKeys.length > 0 && keys.length > prevKeys.length) {
    const oldFirstKey = prevKeys[0];
    const newPos = keys.indexOf(oldFirstKey);
    if (newPos > 0) return prevIndex - newPos;
    if (newPos === -1) {
      for (let i = 0; i < prevKeys.length; i++) {
        const idx = keys.indexOf(prevKeys[i]);
        if (idx >= 0) return prevIndex - (idx - i);
      }
    }
    return prevIndex;
  }
  if (prevKeys.length === 0 && keys.length > 0) {
    return FIRST_INDEX_BASE - keys.length + 1;
  }
  return prevIndex;
}

type IndexState = { keys: string[]; firstItemIndex: number };

function useStableFirstItemIndex(items: RenderItem[]) {
  const keys = useMemo(() => items.map(getItemKey), [items]);

  const [state, setState] = useState<IndexState>(() => {
    const firstItemIndex = FIRST_INDEX_BASE - keys.length + 1;
    if (isDebug()) {
      debugFirstIndex("init", {
        keyCount: keys.length,
        firstItemIndex,
        firstKey: keys[0] ?? "-",
        lastKey: keys[keys.length - 1] ?? "-",
      });
    }
    return { keys, firstItemIndex };
  });

  if (keys !== state.keys) {
    const nextIndex = computeFirstItemIndex(state.keys, state.firstItemIndex, keys);
    if (isDebug()) {
      debugFirstIndex("transition", {
        prevKeyCount: state.keys.length,
        nextKeyCount: keys.length,
        prevIndex: state.firstItemIndex,
        nextIndex,
        delta: nextIndex - state.firstItemIndex,
        prevFirstKey: state.keys[0] ?? "-",
        nextFirstKey: keys[0] ?? "-",
        prevLastKey: state.keys[state.keys.length - 1] ?? "-",
        nextLastKey: keys[keys.length - 1] ?? "-",
      });
    }
    setState({ keys, firstItemIndex: nextIndex });
    return nextIndex;
  }

  return state.firstItemIndex;
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
  const { worktreePath, onOpenFile, lastTurnGroupId, activeTurnId } = props;
  const { hasMore, isLoadingMore, loadMore } = props;
  const virtuosoRef = useRef<VirtuosoHandle>(null);
  const itemCount = items.length;
  const streamingMessageId = getStreamingAgentMessageId(messages);
  const firstItemIndex = useStableFirstItemIndex(items);

  const loadCooldownRef = useRef(false);
  const handleStartReached = useCallback(() => {
    if (hasMore && !isLoadingMore && !loadCooldownRef.current) {
      loadCooldownRef.current = true;
      loadMore().finally(() => {
        setTimeout(() => {
          loadCooldownRef.current = false;
        }, 500);
      });
    }
  }, [hasMore, isLoadingMore, loadMore]);

  const handleScrollToMessage = useCallback(
    (messageId: string) => {
      const idx = items.findIndex((item) => {
        if (item.type === "turn_group") return item.messages.some((m) => m.id === messageId);
        if (item.type === "message") return item.message.id === messageId;
        return false;
      });
      if (idx >= 0)
        virtuosoRef.current?.scrollToIndex({ index: firstItemIndex + idx, align: "center" });
    },
    [items, firstItemIndex],
  );

  const computeItemKey = useCallback(
    (index: number) => {
      const item = items[index - firstItemIndex];
      if (!item) return index;
      return getItemKey(item);
    },
    [items, firstItemIndex],
  );

  const renderItem = useCallback(
    (index: number) => {
      const item = items[index - firstItemIndex];
      if (!item) return <div />;

      return (
        <div className="pb-2">
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
            onScrollToMessage={handleScrollToMessage}
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
      handleScrollToMessage,
    ],
  );

  return { virtuosoRef, itemCount, firstItemIndex, handleStartReached, computeItemKey, renderItem };
}

function VirtuosoBody(props: VirtuosoBodyProps) {
  const { scrollParent, Header, Footer, sessionId, enabled } = props;
  const { virtuosoRef, itemCount, firstItemIndex, handleStartReached, computeItemKey, renderItem } =
    useVirtuosoCallbacks(props);
  const storeApi = useAppStoreApi();

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

  // Virtuoso's own atBottom tracking, reused both to gate `followOutput` and
  // to decide whether re-enabling should catch the view up to the bottom.
  const isAtBottomRef = useRef(true);
  const handleAtBottomStateChange = useCallback((isAtBottom: boolean) => {
    isAtBottomRef.current = isAtBottom;
  }, []);
  const resolvedFollowOutput = useCallback(
    (isAtBottom: boolean) => resolveFollowOutput(enabled, isAtBottom),
    [enabled],
  );

  const captureSnapshot = useCallback(() => {
    if (!sessionId) return;
    virtuosoRef.current?.getState((state) => {
      storeApi.getState().setTranscriptVirtuosoState(sessionId, state);
    });
  }, [sessionId, storeApi, virtuosoRef]);

  // Capture a restorable snapshot on unmount, so a remounted Virtuoso
  // instance (e.g. navigating away and back while disabled) can pick up
  // exactly where the user left off.
  useEffect(() => captureSnapshot, [captureSnapshot]);

  const prevEnabledRef = useRef(enabled);
  useEffect(() => {
    const wasEnabled = prevEnabledRef.current;
    prevEnabledRef.current = enabled;
    if (wasEnabled === enabled) return;
    if (!enabled) {
      // Disabling: snapshot immediately, in addition to the unmount capture
      // above, so a remount that happens without an intervening re-render
      // still restores this exact position.
      captureSnapshot();
      return;
    }
    // Re-enabling: catch up to the bottom only if content progressed past
    // view while disabled.
    if (
      shouldCatchUpOnAutoScrollEnable({
        wasEnabled,
        nowEnabled: enabled,
        isAtBottom: isAtBottomRef.current,
      })
    ) {
      virtuosoRef.current?.scrollToIndex({ index: firstItemIndex + itemCount - 1, align: "end" });
    }
  }, [enabled, captureSnapshot, firstItemIndex, itemCount, virtuosoRef]);

  // Restore the saved position on first mount when disabled. Lazy-initialized
  // so it's read once at mount time, not on every render.
  const [restoreStateFrom] = useState<StateSnapshot | undefined>(() => {
    if (enabled || !sessionId) return undefined;
    return storeApi.getState().transcriptAutoScroll.virtuosoStateBySessionId[sessionId];
  });

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
      followOutput={resolvedFollowOutput}
      atBottomStateChange={handleAtBottomStateChange}
      startReached={handleStartReached}
      increaseViewportBy={200}
      atBottomThreshold={100}
      components={{ Header, Footer }}
    />
  );
}

type VirtuosoSnapshot = {
  branch: string;
  itemCount: number;
  messageCount: number;
  scrollParentReady: boolean;
};

function virtuosoSnapshotChanged(prev: VirtuosoSnapshot | null, next: VirtuosoSnapshot): boolean {
  if (!prev) return true;
  return (
    prev.branch !== next.branch ||
    prev.itemCount !== next.itemCount ||
    prev.messageCount !== next.messageCount ||
    prev.scrollParentReady !== next.scrollParentReady
  );
}

type VirtuosoDebugExtras = {
  sessionId: string | null | undefined;
  messagesLoading: boolean;
  isInitialLoading: boolean;
  showLoadingState: boolean;
  sessionState: string | null | undefined;
  lastItemKey: string;
};

function logVirtuosoSnapshotChange(
  prev: VirtuosoSnapshot | null,
  next: VirtuosoSnapshot,
  extras: VirtuosoDebugExtras,
) {
  debugVirtuoso(prev ? "snapshot-change" : "snapshot-init", {
    sessionId: extras.sessionId ?? "-",
    ...next,
    prevBranch: prev?.branch ?? "-",
    prevItemCount: prev?.itemCount ?? -1,
    prevMessageCount: prev?.messageCount ?? -1,
    prevScrollParentReady: prev?.scrollParentReady ?? false,
    messagesLoading: extras.messagesLoading,
    isInitialLoading: extras.isInitialLoading,
    showLoadingState: extras.showLoadingState,
    sessionState: extras.sessionState ?? "-",
    lastItemKey: extras.lastItemKey,
    initialTopMostItemIndex: next.itemCount - 1,
  });
}

type UseVirtuosoDebugSnapshotArgs = {
  items: RenderItem[];
  messages: { length: number };
  scrollParent: HTMLDivElement | null;
  sessionId: string | null | undefined;
  messagesLoading: boolean;
  isInitialLoading: boolean;
  showLoadingState: boolean;
  sessionState: string | null | undefined;
};

/** Track which render branch fires and how itemCount/messageCount transition. */
function useVirtuosoDebugSnapshot({
  items,
  messages,
  scrollParent,
  sessionId,
  messagesLoading,
  isInitialLoading,
  showLoadingState,
  sessionState,
}: UseVirtuosoDebugSnapshotArgs) {
  const prevSnapshotRef = useRef<VirtuosoSnapshot | null>(null);
  useEffect(() => {
    if (!isDebug()) return;
    const snapshot: VirtuosoSnapshot = {
      branch: isInitialLoading || items.length === 0 ? "fallback" : "virtuoso",
      itemCount: items.length,
      messageCount: messages.length,
      scrollParentReady: Boolean(scrollParent),
    };
    const prev = prevSnapshotRef.current;
    if (!virtuosoSnapshotChanged(prev, snapshot)) return;
    const lastItem = items[items.length - 1];
    logVirtuosoSnapshotChange(prev, snapshot, {
      sessionId,
      messagesLoading,
      isInitialLoading,
      showLoadingState,
      sessionState,
      lastItemKey: lastItem ? getItemKey(lastItem) : "-",
    });
    prevSnapshotRef.current = snapshot;
  }, [
    items,
    messages.length,
    scrollParent,
    sessionId,
    messagesLoading,
    isInitialLoading,
    showLoadingState,
    sessionState,
  ]);
}

/** Defer providing scroll parent to Virtuoso until the element has non-zero size. */
function useVisibleScrollParent() {
  const [scrollParent, setScrollParent] = useState<HTMLDivElement | null>(null);
  const nodeRef = useRef<HTMLDivElement | null>(null);
  const setScrollRef = useCallback((node: HTMLDivElement | null) => {
    nodeRef.current = node;
    if (node && node.offsetHeight > 0) {
      if (isDebug()) {
        debugScrollParent("ref-callback-ready", {
          offsetHeight: node.offsetHeight,
          path: "synchronous",
        });
      }
      setScrollParent(node);
    } else if (isDebug()) {
      debugScrollParent("ref-callback-defer", {
        hasNode: Boolean(node),
        offsetHeight: node?.offsetHeight ?? null,
        reason: !node ? "no-node" : "zero-height",
      });
    }
  }, []);
  useEffect(() => {
    const node = nodeRef.current;
    if (!node || scrollParent) return;
    if (isDebug()) {
      debugScrollParent("ro-attach", {
        initialHeight: node.offsetHeight,
      });
    }
    const ro = new ResizeObserver((entries) => {
      for (const entry of entries) {
        if (entry.contentRect.height > 0) {
          if (isDebug()) {
            debugScrollParent("ro-ready", {
              height: entry.contentRect.height,
            });
          }
          setScrollParent(node);
          ro.disconnect();
          return;
        }
      }
    });
    ro.observe(node);
    return () => ro.disconnect();
  }, [scrollParent]);
  return { scrollParent, setScrollRef };
}

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

export const VirtuosoMessageList = memo(function VirtuosoMessageList(props: MessageListProps) {
  const {
    items,
    messages,
    footerActionMessages,
    sessionId,
    messagesLoading,
    isWorking,
    sessionState,
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

  // Track which render branch fires and how itemCount/messageCount transition.
  // See useVirtuosoDebugSnapshot for details on the remote-executor scroll bug.
  useVirtuosoDebugSnapshot({
    items,
    messages,
    scrollParent,
    sessionId,
    messagesLoading,
    isInitialLoading,
    showLoadingState,
    sessionState,
  });

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
      <SessionPanelContent className="relative p-4 chat-message-list">
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
      </SessionPanelContent>
    );
  }

  return (
    <SessionPanelContent ref={setScrollRef} className="relative p-4 chat-message-list">
      {scrollParent && (
        <VirtuosoBody
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
    </SessionPanelContent>
  );
});
