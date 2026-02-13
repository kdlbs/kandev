'use client';

import { useCallback, useEffect, useMemo, useRef, memo } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import { GridSpinner } from '@/components/grid-spinner';
import { SessionPanelContent } from '@kandev/ui/pannel-session';
import type { Message, TaskSessionState } from '@/lib/types/http';
import type { RenderItem } from '@/hooks/use-processed-messages';
import { MessageRenderer } from '@/components/task/chat/message-renderer';
import { TurnGroupMessage } from '@/components/task/chat/messages/turn-group-message';
import { AgentStatus } from '@/components/task/chat/messages/agent-status';
import { useLazyLoadMessages } from '@/hooks/use-lazy-load-messages';

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
  const wasAtBottomRef = useRef(true);
  const previousHeightRef = useRef(0);

  const isInitialLoading = messagesLoading && messages.length === 0;
  const isCreatedSession = sessionState === 'CREATED';
  const showLoadingState = (messagesLoading || isInitialLoading) && !isWorking && !isCreatedSession;
  const { loadMore, hasMore, isLoading: isLoadingMore } = useLazyLoadMessages(sessionId);

  const itemCount = items.length;

  // eslint-disable-next-line react-hooks/incompatible-library
  const virtualizer = useVirtualizer({
    count: itemCount,
    getScrollElement: () => messagesContainerRef.current,
    estimateSize: () => 96,
    overscan: 6,
  });

  // Scroll to a specific message by ID
  const handleScrollToMessage = useCallback((messageId: string) => {
    const index = items.findIndex(item => {
      if (item.type === 'turn_group') {
        return item.messages.some(msg => msg.id === messageId);
      }
      return item.message?.id === messageId;
    });
    if (index >= 0) {
      virtualizer.scrollToIndex(index, { align: 'center' });
    }
  }, [items, virtualizer]);

  // Use ref to track loading state to avoid recreating the scroll handler
  const loadingRef = useRef({ hasMore, isLoadingMore });
  loadingRef.current = { hasMore, isLoadingMore };

  // Combined scroll handler: check if at bottom AND trigger lazy loading at top
  const handleScroll = useCallback(() => {
    const element = messagesContainerRef.current;
    if (!element) return;

    const { scrollTop, scrollHeight, clientHeight } = element;

    // Track if we're at the bottom for auto-scroll behavior (100px threshold)
    // User is considered "at bottom" if within 100px of the bottom
    wasAtBottomRef.current = scrollHeight - scrollTop - clientHeight < 100;

    // Trigger lazy load when scrolled near top
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
  }, [loadMore]);

  useEffect(() => {
    const element = messagesContainerRef.current;
    if (!element) return;
    element.addEventListener('scroll', handleScroll);
    return () => element.removeEventListener('scroll', handleScroll);
  }, [handleScroll]);

  // Maintain scroll position at bottom when container resizes (e.g., chat input grows)
  useEffect(() => {
    const element = messagesContainerRef.current;
    if (!element) return;

    const resizeObserver = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const currentHeight = entry.contentRect.height;
        const previousHeight = previousHeightRef.current;

        // If container got smaller (input grew) and we were at bottom, scroll to bottom
        if (previousHeight > 0 && currentHeight < previousHeight && wasAtBottomRef.current) {
          requestAnimationFrame(() => {
            if (element) {
              element.scrollTop = element.scrollHeight;
            }
          });
        }

        previousHeightRef.current = currentHeight;
      }
    });

    resizeObserver.observe(element);
    return () => resizeObserver.disconnect();
  }, []);

  // Track last message content to detect streaming updates
  const lastMessageContent = useMemo(() => {
    if (items.length === 0) return '';
    const lastItem = items[items.length - 1];
    if (lastItem.type === 'turn_group') {
      const lastMsg = lastItem.messages[lastItem.messages.length - 1];
      return lastMsg?.content ?? '';
    }
    return lastItem.message?.content ?? '';
  }, [items]);

  // Scroll to bottom when new messages arrive or when last message content changes (streaming)
  useEffect(() => {
    if (itemCount === 0) return;
    if (wasAtBottomRef.current) {
      virtualizer.scrollToIndex(itemCount - 1, { align: 'end' });
    }
  }, [itemCount, lastMessageContent, virtualizer]);

  // Scroll to bottom when agent starts running (running indicator appears)
  const isRunning = sessionState === 'CREATED' || sessionState === 'STARTING' || sessionState === 'RUNNING';

  // Find the last turn_group ID to keep it expanded while turn is active
  const lastTurnGroupId = useMemo(() => {
    for (let i = items.length - 1; i >= 0; i--) {
      const item = items[i];
      if (item.type === 'turn_group') {
        return item.id;
      }
    }
    return null;
  }, [items]);
  useEffect(() => {
    if (!isRunning) return;
    const element = messagesContainerRef.current;
    if (!element) return;
    // Scroll to absolute bottom to show the running indicator
    requestAnimationFrame(() => {
      element.scrollTop = element.scrollHeight;
    });
  }, [isRunning]);

  return (
    <SessionPanelContent
      ref={messagesContainerRef}
      className="relative p-4"
    >
      {isLoadingMore && hasMore && (
        <div className="absolute top-2 left-1/2 -translate-x-1/2 text-xs text-muted-foreground">
          Loading older messages...
        </div>
      )}
      {/* Show loading messages spinner when initially loading, hide while agent is being created */}
      {showLoadingState && (
        <div className="flex items-center justify-center py-8 text-muted-foreground">
          <GridSpinner className="text-primary mr-2" />
          <span>Loading messages...</span>
        </div>
      )}
      {/* Show empty state when no messages and no loading */}
      {!messagesLoading && !isInitialLoading && messages.length === 0 && (
        <div className="flex items-center justify-center py-8 text-muted-foreground">
          <span>No messages yet. Start the conversation!</span>
        </div>
      )}
      {/* Render messages */}
      {!isInitialLoading && itemCount > 0 && (
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
                  {item.type === 'turn_group' ? (
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
                      isTaskDescription={item.message.id === 'task-description'}
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
      )}
      {/* Agent status - running indicator or completed turn summary */}
      <AgentStatus
        sessionState={sessionState}
        sessionId={sessionId}
        messages={messages}
      />
    </SessionPanelContent>
  );
});
