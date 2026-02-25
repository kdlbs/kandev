"use client";

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, memo } from "react";
import { SessionPanelContent } from "@kandev/ui/pannel-session";
import type { Message } from "@/lib/types/http";
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

/**
 * Continuously captures scroll state via scroll listener.
 * On prepend (itemCount increases), restores scroll position so the user
 * stays at the same visual spot.
 */
function useScrollPositionOnPrepend(
  scrollRef: React.RefObject<HTMLDivElement | null>,
  itemCount: number,
) {
  const scrollState = useRef({ scrollHeight: 0, scrollTop: 0 });
  const prevItemCount = useRef(itemCount);

  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const onScroll = () => {
      scrollState.current.scrollHeight = el.scrollHeight;
      scrollState.current.scrollTop = el.scrollTop;
    };
    onScroll();
    el.addEventListener("scroll", onScroll, { passive: true });
    return () => el.removeEventListener("scroll", onScroll);
  }, [scrollRef]);

  useLayoutEffect(() => {
    const el = scrollRef.current;
    if (!el || itemCount <= prevItemCount.current) {
      prevItemCount.current = itemCount;
      return;
    }
    const prev = scrollState.current;
    const delta = el.scrollHeight - prev.scrollHeight;
    if (delta > 0) {
      el.scrollTop = prev.scrollTop + delta;
    }
    prevItemCount.current = itemCount;
  }, [itemCount, scrollRef]);
}

/**
 * Observes a sentinel element at the top of the list to trigger lazy loading.
 * Uses a callback ref so the observer reconnects when the sentinel remounts.
 *
 * Handles the timing issue where the sentinel DOM node mounts (callback ref fires)
 * before the useEffect creates the IntersectionObserver. The sentinelNodeRef bridges
 * the gap: the callback ref stores the node, and the effect observes it if present.
 */
function useLazyLoadSentinel(
  scrollRef: React.RefObject<HTMLDivElement | null>,
  hasMore: boolean,
  isLoadingMore: boolean,
  loadMore: () => Promise<number>,
) {
  const stateRef = useRef({ hasMore, isLoadingMore });
  useEffect(() => {
    stateRef.current = { hasMore, isLoadingMore };
  }, [hasMore, isLoadingMore]);

  const observerRef = useRef<IntersectionObserver | null>(null);
  const sentinelNodeRef = useRef<HTMLDivElement | null>(null);

  // Create/destroy observer when scroll container changes
  useEffect(() => {
    const root = scrollRef.current;
    if (!root) return;
    const observer = new IntersectionObserver(
      (entries) => {
        const { hasMore, isLoadingMore } = stateRef.current;
        const isIntersecting = entries[0]?.isIntersecting;
        if (isIntersecting && hasMore && !isLoadingMore) {
          loadMore();
        }
      },
      { root, rootMargin: "200px 0px 0px 0px" },
    );
    observerRef.current = observer;
    // If sentinel already mounted before this effect ran, observe it now
    if (sentinelNodeRef.current) {
      observer.observe(sentinelNodeRef.current);
    }
    return () => {
      observer.disconnect();
      observerRef.current = null;
    };
  }, [scrollRef, loadMore]);

  // Callback ref — stores node and observes if observer already exists
  const sentinelRef = useCallback((node: HTMLDivElement | null) => {
    sentinelNodeRef.current = node;
    const observer = observerRef.current;
    if (observer) {
      observer.disconnect();
      if (node) {
        observer.observe(node);
      }
    }
  }, []);

  return sentinelRef;
}

/**
 * Auto-scrolls to bottom when new messages arrive (if user is near bottom)
 * or when the agent starts working (isWorking transitions to true).
 */
function useAutoScroll(
  scrollRef: React.RefObject<HTMLDivElement | null>,
  messages: Message[],
  isWorking: boolean,
) {
  const isNearBottomRef = useRef(true);
  const prevIsWorkingRef = useRef(isWorking);

  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const onScroll = () => {
      isNearBottomRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 100;
    };
    el.addEventListener("scroll", onScroll, { passive: true });
    return () => el.removeEventListener("scroll", onScroll);
  }, [scrollRef]);

  // When isWorking transitions to true, force scroll to bottom
  useEffect(() => {
    if (isWorking && !prevIsWorkingRef.current) {
      const el = scrollRef.current;
      if (el) {
        el.scrollTop = el.scrollHeight;
        isNearBottomRef.current = true;
      }
    }
    prevIsWorkingRef.current = isWorking;
  }, [isWorking, scrollRef]);

  // Auto-scroll on new messages if near bottom
  useLayoutEffect(() => {
    const el = scrollRef.current;
    if (el && isNearBottomRef.current) {
      el.scrollTop = el.scrollHeight;
    }
  }, [messages, scrollRef]);
}

function useScrollToMessage() {
  return useCallback((messageId: string) => {
    const el = document.getElementById(`msg-${messageId}`);
    el?.scrollIntoView({ block: "center", behavior: "smooth" });
  }, []);
}

export const NativeMessageList = memo(function NativeMessageList({
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
}: MessageListProps) {
  const scrollRef = useRef<HTMLDivElement>(null);

  const isInitialLoading = messagesLoading && messages.length === 0;
  const isNonLoadableSession =
    !sessionState || ["CREATED", "FAILED", "COMPLETED", "CANCELLED"].includes(sessionState);
  const showLoadingState = isInitialLoading && !isWorking && !isNonLoadableSession;
  const { loadMore, hasMore, isLoading: isLoadingMore } = useLazyLoadMessages(sessionId);
  const isRunning = getSessionRunningState(sessionState);
  const lastTurnGroupId = useMemo(() => getLastTurnGroupId(items), [items]);
  const handleScrollToMessage = useScrollToMessage();

  useScrollPositionOnPrepend(scrollRef, items.length);
  const sentinelRef = useLazyLoadSentinel(scrollRef, hasMore, isLoadingMore, loadMore);
  useAutoScroll(scrollRef, messages, isWorking);

  // Scroll to bottom on initial load
  const didInitialScroll = useRef(false);
  useEffect(() => {
    if (didInitialScroll.current || items.length === 0) return;
    const el = scrollRef.current;
    if (el) {
      el.scrollTop = el.scrollHeight;
      didInitialScroll.current = true;
    }
  }, [items.length]);

  return (
    <SessionPanelContent ref={scrollRef} className="relative p-4 chat-message-list">
      {/* Sentinel for lazy loading older messages */}
      {hasMore && <div ref={sentinelRef} className="h-px" />}

      <MessageListStatus
        isLoadingMore={isLoadingMore}
        hasMore={hasMore}
        showLoadingState={showLoadingState}
        messagesLoading={messagesLoading}
        isInitialLoading={isInitialLoading}
        messagesCount={messages.length}
      />

      {items.map((item) => {
        const key = getItemKey(item);
        return (
          <div key={key} id={`msg-${key}`} className="pb-2" style={{ overflowAnchor: "none" }}>
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
              onScrollToMessage={handleScrollToMessage}
            />
          </div>
        );
      })}

      {sessionId && <PrepareProgress sessionId={sessionId} />}
      <AgentStatus sessionState={sessionState} sessionId={sessionId} messages={messages} />

      {/* Bottom anchor — browser keeps scroll pinned here when new content appends */}
      <div style={{ overflowAnchor: "auto", height: 1 }} />
    </SessionPanelContent>
  );
});
