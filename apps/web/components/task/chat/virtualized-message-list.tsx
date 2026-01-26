'use client';

import { useCallback, useEffect, useMemo, useRef } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import { IconLoader2 } from '@tabler/icons-react';
import { SessionPanelContent } from '@kandev/ui/pannel-session';
import type { Message } from '@/lib/types/http';
import { MessageRenderer } from '@/components/task/chat/message-renderer';
import { TurnSummary } from '@/components/task/chat/messages/turn-summary';
import { useLazyLoadMessages } from '@/hooks/use-lazy-load-messages';

type VirtualizedMessageListProps = {
  messages: Message[];
  permissionsByToolCallId: Map<string, Message>;
  taskId?: string;
  sessionId: string | null;
  messagesLoading: boolean;
  isWorking: boolean;
};

export function VirtualizedMessageList({
  messages,
  permissionsByToolCallId,
  taskId,
  sessionId,
  messagesLoading,
  isWorking,
}: VirtualizedMessageListProps) {
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const wasAtBottomRef = useRef(true);

  const isInitialLoading = messagesLoading && messages.length === 0;
  const showLoadingState = (messagesLoading || isInitialLoading) && !isWorking;
  const { loadMore, hasMore, isLoading: isLoadingMore } = useLazyLoadMessages(sessionId);

  const itemCount = messages.length;

  // eslint-disable-next-line react-hooks/incompatible-library
  const virtualizer = useVirtualizer({
    count: itemCount,
    getScrollElement: () => messagesContainerRef.current,
    estimateSize: () => 96,
    overscan: 6,
  });

  const checkAtBottom = useCallback(() => {
    const element = messagesContainerRef.current;
    if (!element) return;
    const { scrollTop, scrollHeight, clientHeight } = element;
    wasAtBottomRef.current = scrollHeight - scrollTop - clientHeight < 48;
  }, []);

  useEffect(() => {
    const element = messagesContainerRef.current;
    if (!element) return;
    element.addEventListener('scroll', checkAtBottom);
    return () => element.removeEventListener('scroll', checkAtBottom);
  }, [checkAtBottom]);

  // Track last message content to detect streaming updates
  const lastMessageContent = useMemo(() => {
    if (messages.length === 0) return '';
    const lastMsg = messages[messages.length - 1];
    return lastMsg?.content ?? '';
  }, [messages]);

  // Scroll to bottom when new messages arrive or when last message content changes (streaming)
  useEffect(() => {
    if (itemCount === 0) return;
    if (wasAtBottomRef.current) {
      virtualizer.scrollToIndex(itemCount - 1, { align: 'end' });
    }
  }, [itemCount, lastMessageContent, virtualizer]);

  const virtualItems = virtualizer.getVirtualItems();

  // Lazy load older messages when scrolling to top
  useEffect(() => {
    const [firstItem] = virtualItems;
    if (!firstItem) return;
    const element = messagesContainerRef.current;
    if (!element) return;
    if (firstItem.index !== 0 || element.scrollTop > 40) {
      return;
    }
    if (!hasMore || isLoadingMore) {
      return;
    }
    const prevScrollHeight = element.scrollHeight;
    const prevScrollTop = element.scrollTop;
    loadMore().then((added) => {
      if (!added) return;
      requestAnimationFrame(() => {
        const nextScrollHeight = element.scrollHeight;
        element.scrollTop = prevScrollTop + (nextScrollHeight - prevScrollHeight);
      });
    });
  }, [virtualItems, hasMore, isLoadingMore, loadMore]);

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
      {/* Show loading messages spinner when initially loading */}
      {showLoadingState && (
        <div className="flex items-center justify-center py-8 text-muted-foreground">
          <IconLoader2 className="h-5 w-5 animate-spin mr-2" />
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
            const message = messages[virtualRow.index];
            return (
              <div
                key={virtualRow.key}
                ref={virtualizer.measureElement}
                data-index={virtualRow.index}
                className="absolute left-0 top-0 w-full"
                style={{ transform: `translateY(${virtualRow.start}px)` }}
              >
                <div className="pb-2">
                  <MessageRenderer
                    comment={message}
                    isTaskDescription={message.id === 'task-description'}
                    taskId={taskId}
                    permissionsByToolCallId={permissionsByToolCallId}
                  />
                </div>
              </div>
            );
          })}
        </div>
      )}
      {/* Turn summary - shows duration and model after last message */}
      <TurnSummary sessionId={sessionId} />
    </SessionPanelContent>
  );
}
