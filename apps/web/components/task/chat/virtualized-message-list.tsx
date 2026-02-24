"use client";

import { useCallback, useEffect, useMemo, useRef, memo } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import type { Virtualizer } from "@tanstack/react-virtual";
import { GridSpinner } from "@/components/grid-spinner";
import { SessionPanelContent } from "@kandev/ui/pannel-session";
import type { Message, TaskSessionState } from "@/lib/types/http";
import type { RenderItem } from "@/hooks/use-processed-messages";
import { MessageRenderer } from "@/components/task/chat/message-renderer";
import { TurnGroupMessage } from "@/components/task/chat/messages/turn-group-message";
import { AgentStatus } from "@/components/task/chat/messages/agent-status";
import { PrepareProgress } from "@/components/session/prepare-progress";
import { useLazyLoadMessages } from "@/hooks/use-lazy-load-messages";

type ContainerResizeContext = {
  isHiddenRef: React.RefObject<boolean>;
  previousHeightRef: React.RefObject<number>;
  previousWidthRef: React.RefObject<number>;
  wasAtBottomRef: React.RefObject<boolean>;
  virtualizerRef: React.RefObject<MessageVirtualizer>;
  scrollSnapshot: React.RefObject<{ wasAtBottom: boolean; firstVisibleIndex: number }>;
};

type MessageVirtualizer = Virtualizer<HTMLDivElement, Element>;

function handleContainerResize(
  entry: ResizeObserverEntry,
  element: HTMLDivElement,
  ctx: ContainerResizeContext,
): void {
  const {
    isHiddenRef,
    previousHeightRef,
    previousWidthRef,
    wasAtBottomRef,
    virtualizerRef,
    scrollSnapshot,
  } = ctx;
  const currentHeight = entry.contentRect.height;
  const currentWidth = entry.contentRect.width;
  const previousHeight = previousHeightRef.current;
  const previousWidth = previousWidthRef.current;

  if (currentWidth === 0 || currentHeight === 0) {
    isHiddenRef.current = true;
    return;
  }

  if (isHiddenRef.current) {
    isHiddenRef.current = false;
    previousHeightRef.current = currentHeight;
    previousWidthRef.current = currentWidth;
    const virt = virtualizerRef.current;
    const snapshot = scrollSnapshot.current;
    virt.measure();
    requestAnimationFrame(() => {
      if (snapshot.wasAtBottom) {
        const count = virt.options.count;
        if (count > 0) virt.scrollToIndex(count - 1, { align: "end" });
      } else {
        virt.scrollToIndex(snapshot.firstVisibleIndex, { align: "start" });
      }
    });
    return;
  }

  if (previousWidth > 0 && Math.abs(currentWidth - previousWidth) > 20)
    virtualizerRef.current.measure();
  if (previousHeight > 0 && currentHeight < previousHeight && wasAtBottomRef.current) {
    requestAnimationFrame(() => {
      element.scrollTop = element.scrollHeight;
    });
  }
  previousHeightRef.current = currentHeight;
  previousWidthRef.current = currentWidth;
}

function useVirtualListScrolling(
  sessionId: string | null,
  virtualizer: MessageVirtualizer,
  messagesContainerRef: React.RefObject<HTMLDivElement | null>,
) {
  const wasAtBottomRef = useRef(true);
  const savedScrollTopRef = useRef(0);
  const previousHeightRef = useRef(0);
  const previousWidthRef = useRef(0);
  const isHiddenRef = useRef(false);
  const scrollSnapshot = useRef<{ wasAtBottom: boolean; firstVisibleIndex: number }>({
    wasAtBottom: true,
    firstVisibleIndex: 0,
  });
  const { loadMore, hasMore, isLoading: isLoadingMore } = useLazyLoadMessages(sessionId);
  const loadingRef = useRef({ hasMore, isLoadingMore });
  loadingRef.current = { hasMore, isLoadingMore };
  const virtualizerRef = useRef(virtualizer);
  virtualizerRef.current = virtualizer;

  const handleScroll = useCallback(() => {
    const element = messagesContainerRef.current;
    if (!element || isHiddenRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = element;
    wasAtBottomRef.current = scrollHeight - scrollTop - clientHeight < 100;
    savedScrollTopRef.current = scrollTop;
    const virt = virtualizerRef.current;
    const visibleItems = virt.getVirtualItems();
    scrollSnapshot.current = {
      wasAtBottom: wasAtBottomRef.current,
      firstVisibleIndex: visibleItems.length > 0 ? visibleItems[0].index : 0,
    };
    if (scrollTop < 40 && loadingRef.current.hasMore && !loadingRef.current.isLoadingMore) {
      const prevScrollHeight = scrollHeight;
      const prevScrollTop = scrollTop;
      loadMore().then((added) => {
        if (!added) return;
        requestAnimationFrame(() => {
          const nextScrollHeight = element.scrollHeight;
          element.scrollTop = prevScrollTop + (nextScrollHeight - prevScrollHeight);
        });
      });
    }
  }, [loadMore, messagesContainerRef]);

  useEffect(() => {
    const element = messagesContainerRef.current;
    if (!element) return;
    element.addEventListener("scroll", handleScroll);
    return () => element.removeEventListener("scroll", handleScroll);
  }, [handleScroll, messagesContainerRef]);

  useEffect(() => {
    const element = messagesContainerRef.current;
    if (!element) return;
    const resizeCtx: ContainerResizeContext = {
      isHiddenRef,
      previousHeightRef,
      previousWidthRef,
      wasAtBottomRef,
      virtualizerRef,
      scrollSnapshot,
    };
    const resizeObserver = new ResizeObserver((entries) => {
      for (const entry of entries) handleContainerResize(entry, element, resizeCtx);
    });
    resizeObserver.observe(element);
    return () => resizeObserver.disconnect();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return { wasAtBottomRef, hasMore, isLoadingMore };
}

type VirtualizedMessageListProps = {
  items: RenderItem[];
  messages: Message[];
  permissionsByToolCallId: Map<string, Message>;
  childrenByParentToolCallId: Map<string, Message[]>;
  taskId?: string;
  sessionId: string | null;
  messagesLoading: boolean;
  isWorking: boolean;
  sessionState?: TaskSessionState;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
};

type MessageListBodyProps = {
  items: RenderItem[];
  messages: Message[];
  permissionsByToolCallId: Map<string, Message>;
  childrenByParentToolCallId: Map<string, Message[]>;
  taskId?: string;
  sessionId: string | null;
  sessionState?: TaskSessionState;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
  isInitialLoading: boolean;
  virtualizer: MessageVirtualizer;
  lastTurnGroupId: string | null;
  isRunning: boolean;
  handleScrollToMessage: (messageId: string) => void;
};

function MessageListBody({
  items,
  messages,
  permissionsByToolCallId,
  childrenByParentToolCallId,
  taskId,
  sessionId,
  sessionState,
  worktreePath,
  onOpenFile,
  isInitialLoading,
  virtualizer,
  lastTurnGroupId,
  isRunning,
  handleScrollToMessage,
}: MessageListBodyProps) {
  const itemCount = items.length;
  if (isInitialLoading || itemCount === 0) return null;
  return (
    <div className="relative w-full" style={{ height: `${virtualizer.getTotalSize()}px` }}>
      {virtualizer.getVirtualItems().map((virtualRow) => {
        const item = items[virtualRow.index];
        return (
          <div
            key={virtualRow.key}
            ref={virtualizer.measureElement}
            data-index={virtualRow.index}
            className="absolute left-0 top-0 w-full"
            style={{ transform: `translateY(${virtualRow.start}px)` }}
          >
            <div className="pb-2">
              {item.type === "turn_group" ? (
                <TurnGroupMessage
                  group={item}
                  sessionId={sessionId}
                  permissionsByToolCallId={permissionsByToolCallId}
                  childrenByParentToolCallId={childrenByParentToolCallId}
                  taskId={taskId}
                  worktreePath={worktreePath}
                  onOpenFile={onOpenFile}
                  isLastGroup={item.id === lastTurnGroupId}
                  isTurnActive={isRunning}
                  allMessages={messages}
                  onScrollToMessage={handleScrollToMessage}
                />
              ) : (
                <MessageRenderer
                  comment={item.message}
                  isTaskDescription={item.message.id === "task-description"}
                  sessionState={sessionState}
                  taskId={taskId}
                  permissionsByToolCallId={permissionsByToolCallId}
                  childrenByParentToolCallId={childrenByParentToolCallId}
                  worktreePath={worktreePath}
                  sessionId={sessionId ?? undefined}
                  onOpenFile={onOpenFile}
                  allMessages={messages}
                  onScrollToMessage={handleScrollToMessage}
                />
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
}

function getSessionRunningState(sessionState: string | null | undefined) {
  return sessionState === "CREATED" || sessionState === "STARTING" || sessionState === "RUNNING";
}

function getLastTurnGroupId(items: RenderItem[]) {
  for (let i = items.length - 1; i >= 0; i--) {
    const item = items[i];
    if (item.type === "turn_group") return item.id;
  }
  return null;
}

function MessageListStatus({
  isLoadingMore,
  hasMore,
  showLoadingState,
  messagesLoading,
  isInitialLoading,
  messagesCount,
}: {
  isLoadingMore: boolean;
  hasMore: boolean;
  showLoadingState: boolean;
  messagesLoading: boolean;
  isInitialLoading: boolean;
  messagesCount: number;
}) {
  return (
    <>
      {isLoadingMore && hasMore && (
        <div className="absolute top-2 left-1/2 -translate-x-1/2 text-xs text-muted-foreground">
          Loading older messages...
        </div>
      )}
      {showLoadingState && (
        <div className="flex items-center justify-center py-8 text-muted-foreground">
          <GridSpinner className="text-primary mr-2" />
          <span>Loading messages...</span>
        </div>
      )}
      {!messagesLoading && !isInitialLoading && messagesCount === 0 && (
        <div className="flex items-center justify-center py-8 text-muted-foreground">
          <span>No messages yet. Start the conversation!</span>
        </div>
      )}
    </>
  );
}

export const VirtualizedMessageList = memo(function VirtualizedMessageList({
  items,
  messages,
  permissionsByToolCallId,
  childrenByParentToolCallId,
  taskId,
  sessionId,
  messagesLoading,
  isWorking,
  sessionState,
  worktreePath,
  onOpenFile,
}: VirtualizedMessageListProps) {
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const isInitialLoading = messagesLoading && messages.length === 0;
  const isNonLoadableSession =
    !sessionState || ["CREATED", "FAILED", "COMPLETED", "CANCELLED"].includes(sessionState);
  const showLoadingState =
    (messagesLoading || isInitialLoading) && !isWorking && !isNonLoadableSession;
  const itemCount = items.length;

  // eslint-disable-next-line react-hooks/incompatible-library
  const virtualizer = useVirtualizer({
    count: itemCount,
    getScrollElement: () => messagesContainerRef.current,
    estimateSize: () => 96,
    overscan: 6,
  });
  const { wasAtBottomRef, hasMore, isLoadingMore } = useVirtualListScrolling(
    sessionId,
    virtualizer,
    messagesContainerRef,
  );

  const handleScrollToMessage = useCallback(
    (messageId: string) => {
      const index = items.findIndex((item) => {
        if (item.type === "turn_group") return item.messages.some((msg) => msg.id === messageId);
        return item.message?.id === messageId;
      });
      if (index >= 0) virtualizer.scrollToIndex(index, { align: "center" });
    },
    [items, virtualizer],
  );

  const lastMessageContent = useMemo(() => {
    if (items.length === 0) return "";
    const lastItem = items[items.length - 1];
    if (lastItem.type === "turn_group")
      return lastItem.messages[lastItem.messages.length - 1]?.content ?? "";
    return lastItem.message?.content ?? "";
  }, [items]);

  useEffect(() => {
    if (itemCount === 0) return;
    if (wasAtBottomRef.current) virtualizer.scrollToIndex(itemCount - 1, { align: "end" });
  }, [itemCount, lastMessageContent, virtualizer, wasAtBottomRef]);

  const isRunning = getSessionRunningState(sessionState);
  const lastTurnGroupId = useMemo(() => getLastTurnGroupId(items), [items]);

  useEffect(() => {
    if (!isRunning) return;
    const element = messagesContainerRef.current;
    if (!element) return;
    requestAnimationFrame(() => {
      element.scrollTop = element.scrollHeight;
    });
  }, [isRunning]);

  return (
    <SessionPanelContent ref={messagesContainerRef} className="relative p-4 chat-message-list">
      <MessageListStatus
        isLoadingMore={isLoadingMore}
        hasMore={hasMore}
        showLoadingState={showLoadingState}
        messagesLoading={messagesLoading}
        isInitialLoading={isInitialLoading}
        messagesCount={messages.length}
      />
      <MessageListBody
        items={items}
        messages={messages}
        permissionsByToolCallId={permissionsByToolCallId}
        childrenByParentToolCallId={childrenByParentToolCallId}
        taskId={taskId}
        sessionId={sessionId}
        sessionState={sessionState}
        worktreePath={worktreePath}
        onOpenFile={onOpenFile}
        isInitialLoading={isInitialLoading}
        virtualizer={virtualizer}
        lastTurnGroupId={lastTurnGroupId}
        isRunning={isRunning}
        handleScrollToMessage={handleScrollToMessage}
      />
      {sessionId && <PrepareProgress sessionId={sessionId} />}
      <AgentStatus sessionState={sessionState} sessionId={sessionId} messages={messages} />
    </SessionPanelContent>
  );
});
