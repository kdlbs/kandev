"use client";

import type React from "react";
import { useCallback, useEffect, useMemo, useRef, useState, memo } from "react";
import { Virtuoso, type VirtuosoHandle } from "react-virtuoso";
import { SessionPanelContent } from "@kandev/ui/pannel-session";
import type { RenderItem } from "@/hooks/use-processed-messages";
import { AgentStatus } from "@/components/task/chat/messages/agent-status";
import { PrepareProgress } from "@/components/session/prepare-progress";
import { useLazyLoadMessages } from "@/hooks/use-lazy-load-messages";
import {
  type MessageListProps,
  MessageListStatus,
  MessageItem,
  getItemKey,
  getSessionRunningState,
  getLastTurnGroupId,
} from "./message-list-shared";

const FIRST_INDEX_BASE = 100_000;

type VirtuosoBodyProps = MessageListProps & {
  scrollParent: HTMLDivElement;
  isRunning: boolean;
  lastTurnGroupId: string | null;
  hasMore: boolean;
  isLoadingMore: boolean;
  loadMore: () => Promise<number>;
  Header: () => React.ReactNode;
  Footer: () => React.ReactNode;
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

  const [state, setState] = useState<IndexState>(() => ({
    keys,
    firstItemIndex: FIRST_INDEX_BASE - keys.length + 1,
  }));

  if (keys !== state.keys) {
    const nextIndex = computeFirstItemIndex(state.keys, state.firstItemIndex, keys);
    setState({ keys, firstItemIndex: nextIndex });
    return nextIndex;
  }

  return state.firstItemIndex;
}

function useVirtuosoCallbacks(props: VirtuosoBodyProps) {
  const { items, sessionId, permissionsByToolCallId, childrenByParentToolCallId, taskId } = props;
  const {
    worktreePath,
    onOpenFile,
    lastTurnGroupId,
    isRunning,
    messages,
    sessionState,
    taskState,
  } = props;
  const { hasMore, isLoadingMore, loadMore } = props;
  const virtuosoRef = useRef<VirtuosoHandle>(null);
  const itemCount = items.length;
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
        return item.message?.id === messageId;
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
            isTurnActive={isRunning}
            messages={messages}
            sessionState={sessionState}
            taskState={taskState}
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
      isRunning,
      messages,
      sessionState,
      taskState,
      handleScrollToMessage,
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
      followOutput={followOutput}
      startReached={handleStartReached}
      increaseViewportBy={200}
      atBottomThreshold={100}
      components={{ Header, Footer }}
    />
  );
}

/** Defer providing scroll parent to Virtuoso until the element has non-zero size. */
function useVisibleScrollParent() {
  const [scrollParent, setScrollParent] = useState<HTMLDivElement | null>(null);
  const nodeRef = useRef<HTMLDivElement | null>(null);
  const setScrollRef = useCallback((node: HTMLDivElement | null) => {
    nodeRef.current = node;
    if (node && node.offsetHeight > 0) setScrollParent(node);
  }, []);
  useEffect(() => {
    const node = nodeRef.current;
    if (!node || scrollParent) return;
    const ro = new ResizeObserver((entries) => {
      for (const entry of entries) {
        if (entry.contentRect.height > 0) {
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

export const VirtuosoMessageList = memo(function VirtuosoMessageList(props: MessageListProps) {
  const { items, messages, sessionId, messagesLoading, isWorking, sessionState } = props;
  const { scrollParent, setScrollRef } = useVisibleScrollParent();
  const isInitialLoading = messagesLoading && messages.length === 0;
  const isNonLoadableSession =
    !sessionState || ["CREATED", "FAILED", "COMPLETED", "CANCELLED"].includes(sessionState);
  const showLoadingState =
    (messagesLoading || isInitialLoading) && !isWorking && !isNonLoadableSession;
  const { loadMore, hasMore, isLoading: isLoadingMore } = useLazyLoadMessages(sessionId);
  const isRunning = getSessionRunningState(sessionState);
  const lastTurnGroupId = useMemo(() => getLastTurnGroupId(items), [items]);

  const Header = useCallback(
    () => (
      <MessageListStatus
        isLoadingMore={isLoadingMore}
        hasMore={hasMore}
        showLoadingState={showLoadingState}
        messagesLoading={messagesLoading}
        isInitialLoading={isInitialLoading}
        messagesCount={messages.length}
      />
    ),
    [isLoadingMore, hasMore, showLoadingState, messagesLoading, isInitialLoading, messages.length],
  );

  const Footer = useCallback(
    () => (
      <>
        {sessionId && <PrepareProgress sessionId={sessionId} />}
        <AgentStatus sessionState={sessionState} sessionId={sessionId} messages={messages} />
      </>
    ),
    [sessionId, sessionState, messages],
  );

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
        />
        {sessionId && <PrepareProgress sessionId={sessionId} />}
        <AgentStatus sessionState={sessionState} sessionId={sessionId} messages={messages} />
      </SessionPanelContent>
    );
  }

  return (
    <SessionPanelContent ref={setScrollRef} className="relative p-4 chat-message-list">
      {scrollParent && (
        <VirtuosoBody
          {...props}
          scrollParent={scrollParent}
          isRunning={isRunning}
          lastTurnGroupId={lastTurnGroupId}
          hasMore={hasMore}
          isLoadingMore={isLoadingMore}
          loadMore={loadMore}
          Header={Header}
          Footer={Footer}
        />
      )}
    </SessionPanelContent>
  );
});
