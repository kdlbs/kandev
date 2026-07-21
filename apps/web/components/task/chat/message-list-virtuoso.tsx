"use client";

import type React from "react";
import { useCallback, useEffect, useMemo, useRef, useState, memo } from "react";
import { Virtuoso, type VirtuosoHandle } from "react-virtuoso";
import { SessionPanelContent } from "@kandev/ui/pannel-session";
import type { RenderItem } from "@/hooks/use-processed-messages";
import type { Message, TaskSessionState } from "@/lib/types/http";
import { AgentStatus } from "@/components/task/chat/messages/agent-status";
import { MessageRenderer } from "@/components/task/chat/message-renderer";
import { useLazyLoadMessages } from "@/hooks/use-lazy-load-messages";
import {
  type MessageListProps,
  MessageListStatus,
  MessageItem,
  getItemKey,
  getConversationLoadingState,
  getSessionRunningState,
  getLastTurnGroupId,
  getStreamingAgentMessageId,
} from "./message-list-shared";
import { UserMessageNavigationProvider } from "./user-message-navigation-context";
import { VirtuosoMessageListFallback } from "./message-list-virtuoso-fallback";
import { useVirtuosoUserNavigation } from "./use-virtuoso-user-navigation";
import { createDebugLogger, isDebug } from "@/lib/debug/log";

const debugVirtuoso = createDebugLogger("chat:virtuoso");
const debugScrollParent = createDebugLogger("chat:virtuoso:scrollParent");

type VirtuosoBodyProps = MessageListProps & {
  scrollParent: HTMLDivElement;
  isRunning: boolean;
  lastTurnGroupId: string | null;
  hasMore: boolean;
  isLoadingMore: boolean;
  loadMore: () => Promise<number>;
  Header: () => React.ReactNode;
  Footer: () => React.ReactNode;
  firstItemIndex: number;
  virtuosoRef: React.RefObject<VirtuosoHandle | null>;
  suspendFollowOutput: boolean;
};

function useVirtuosoCallbacks(props: VirtuosoBodyProps) {
  const {
    items,
    messages,
    sessionId,
    permissionsByToolCallId,
    childrenByParentToolCallId,
    taskId,
  } = props;
  const { worktreePath, onOpenFile, lastTurnGroupId, isRunning } = props;
  const { hasMore, isLoadingMore, loadMore } = props;
  const { virtuosoRef, firstItemIndex } = props;
  const itemCount = items.length;
  const streamingMessageId = getStreamingAgentMessageId(messages);

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
            isTurnActive={isRunning}
            streamingMessageId={streamingMessageId}
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
      isRunning,
      streamingMessageId,
    ],
  );

  return { virtuosoRef, itemCount, firstItemIndex, handleStartReached, computeItemKey, renderItem };
}

const FOLLOW_SMOOTH = "smooth" as const;
const followOutput = (isAtBottom: boolean) => (isAtBottom ? FOLLOW_SMOOTH : false);

function VirtuosoBody(props: VirtuosoBodyProps) {
  const { scrollParent, Header, Footer } = props;
  const { virtuosoRef, itemCount, firstItemIndex, handleStartReached, computeItemKey, renderItem } =
    useVirtuosoCallbacks(props);

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
      isRunning: props.isRunning,
      lastTurnGroupId: props.lastTurnGroupId ?? "-",
    });
  }, [itemCount, firstItemIndex, props.hasMore, props.isRunning, props.lastTurnGroupId]);

  return (
    <Virtuoso
      ref={virtuosoRef}
      /* Suppress Virtuoso's verbose internal logging in all environments */
      logLevel={Number.MAX_SAFE_INTEGER}
      customScrollParent={scrollParent}
      totalCount={itemCount}
      firstItemIndex={firstItemIndex}
      initialTopMostItemIndex={itemCount - 1}
      computeItemKey={computeItemKey}
      itemContent={renderItem}
      followOutput={props.suspendFollowOutput ? false : followOutput}
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
  footerActionMessages?: Message[];
};

/** Memoized Virtuoso Header (load-more status) and Footer (agent status + actions). */
function useVirtuosoHeaderFooter(args: HeaderFooterArgs) {
  const { isLoadingMore, hasMore, showLoadingState, messagesLoading, isInitialLoading } = args;
  const { messages, loadMore, sessionState, sessionId, footerActionMessages } = args;
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
      <>
        <AgentStatus sessionState={sessionState} sessionId={sessionId} messages={messages} />
        {footerActions.map((msg) => (
          <MessageRenderer key={msg.id} comment={msg} isTaskDescription={false} />
        ))}
      </>
    ),
    [sessionId, sessionState, messages, footerActions],
  );

  return { Header, Footer, footerActions };
}

type VirtuosoMessageViewportProps = Omit<
  VirtuosoBodyProps,
  "scrollParent" | "suspendFollowOutput"
> & {
  scrollParent: HTMLDivElement | null;
  navigation: ReturnType<typeof useVirtuosoUserNavigation>["navigation"];
  setScrollRef: (node: HTMLDivElement | null) => void;
};

function VirtuosoMessageViewport({
  navigation,
  setScrollRef,
  scrollParent,
  ...bodyProps
}: VirtuosoMessageViewportProps) {
  return (
    <UserMessageNavigationProvider value={navigation}>
      <SessionPanelContent ref={setScrollRef} className="relative p-4 chat-message-list">
        {scrollParent && (
          <VirtuosoBody
            {...bodyProps}
            scrollParent={scrollParent}
            suspendFollowOutput={navigation.isBusy}
          />
        )}
      </SessionPanelContent>
    </UserMessageNavigationProvider>
  );
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
  const {
    loadMore,
    hasMore,
    isLoading: isLoadingMore,
    oldestCursor,
  } = useLazyLoadMessages(sessionId);
  const isRunning = getSessionRunningState(sessionState);
  const lastTurnGroupId = useMemo(() => getLastTurnGroupId(items), [items]);
  const { navigation, firstItemIndex, virtuosoRef } = useVirtuosoUserNavigation({
    items,
    sessionId,
    scrollParent,
    hasMore,
    oldestCursor: oldestCursor ?? null,
    loadMore,
  });

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
    footerActionMessages,
  });

  if (isInitialLoading || items.length === 0) {
    return (
      <VirtuosoMessageListFallback
        isLoadingMore={isLoadingMore}
        hasMore={hasMore}
        showLoadingState={showLoadingState}
        messagesLoading={messagesLoading}
        isInitialLoading={isInitialLoading}
        messages={messages}
        loadMore={loadMore}
        sessionState={sessionState}
        sessionId={sessionId}
        footerActions={footerActions}
      />
    );
  }

  return (
    <VirtuosoMessageViewport
      {...props}
      scrollParent={scrollParent}
      isRunning={isRunning}
      lastTurnGroupId={lastTurnGroupId}
      hasMore={hasMore}
      isLoadingMore={isLoadingMore}
      loadMore={loadMore}
      Header={Header}
      Footer={Footer}
      firstItemIndex={firstItemIndex}
      virtuosoRef={virtuosoRef}
      navigation={navigation}
      setScrollRef={setScrollRef}
    />
  );
});
