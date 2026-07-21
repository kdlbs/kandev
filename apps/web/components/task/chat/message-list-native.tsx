"use client";

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, memo } from "react";
import { SessionPanelContent } from "@kandev/ui/pannel-session";
import { useDockviewStore } from "@/lib/state/dockview-store";
import type { Message } from "@/lib/types/http";
import { AgentStatus } from "@/components/task/chat/messages/agent-status";
import { MessageRenderer } from "@/components/task/chat/message-renderer";
import { useLazyLoadMessages } from "@/hooks/use-lazy-load-messages";
import { useUserMessageNavigation } from "@/hooks/use-message-navigation";
import { cn } from "@/lib/utils";
import {
  type MessageListProps,
  type ProgrammaticNavigation,
  MessageListStatus,
  MessageItem,
  getItemKey,
  getConversationLoadingState,
  getSessionRunningState,
  getLastTurnGroupId,
  getStreamingAgentMessageId,
  findNearestUserMessageId,
  findUserMessageElement,
  getNavigationScrollBehavior,
  replayMessageHighlight,
  waitForUserMessageElement,
} from "./message-list-shared";
import {
  USER_MESSAGE_NAVIGATION_MOBILE_CLEARANCE_CLASS,
  UserMessageNavigationRail,
  usePersistentUserMessageNavigationRail,
} from "./user-message-navigation-rail";

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
    if (!el) return;
    // Skip auto-scroll when a layout rebuild scroll restore is pending
    if (useDockviewStore.getState().pendingChatScrollTop !== null) return;
    if (isNearBottomRef.current) {
      el.scrollTop = el.scrollHeight;
    }
  }, [messages, scrollRef]);
}

const NAVIGATION_SETTLE_ATTEMPTS = 4;

function useViewportOrigin(
  scrollRef: React.RefObject<HTMLDivElement | null>,
  userMessageIds: string[],
  setViewportOrigin: (messageId: string | null) => void,
  programmaticRef: React.RefObject<ProgrammaticNavigation>,
) {
  useEffect(() => {
    const scrollElement = scrollRef.current;
    if (!scrollElement) return;
    let frame = 0;
    const updateOrigin = () => {
      frame = 0;
      const nearestId = findNearestUserMessageId(scrollElement, userMessageIds);
      const programmatic = programmaticRef.current;
      if (programmatic && Date.now() < programmatic.expiresAt) {
        if (nearestId !== programmatic.messageId) return;
      } else if (programmatic) {
        programmaticRef.current = null;
      }
      setViewportOrigin(nearestId);
    };
    const onScroll = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(updateOrigin);
    };
    scrollElement.addEventListener("scroll", onScroll, { passive: true });
    onScroll();
    return () => {
      cancelAnimationFrame(frame);
      scrollElement.removeEventListener("scroll", onScroll);
    };
  }, [programmaticRef, scrollRef, setViewportOrigin, userMessageIds]);
}

function useNativeUserNavigation(args: {
  scrollRef: React.RefObject<HTMLDivElement | null>;
  sessionId: string | null;
  items: MessageListProps["items"];
  hasMore: boolean;
  oldestCursor: string | null;
  loadMore: () => Promise<number>;
}) {
  const programmaticNavigationRef = useRef<ProgrammaticNavigation>(null);
  const mountedRef = useRef(true);
  const sessionIdRef = useRef(args.sessionId);
  sessionIdRef.current = args.sessionId;
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);
  const navigateTo = useCallback(
    async (messageId: string) => {
      const scrollElement = args.scrollRef.current;
      if (!scrollElement || !args.sessionId) return false;
      const actionSessionId = args.sessionId;
      const previousScrollTop = scrollElement.scrollTop;
      programmaticNavigationRef.current = { messageId, expiresAt: Date.now() + 3000 };
      for (let attempt = 0; attempt < NAVIGATION_SETTLE_ATTEMPTS; attempt++) {
        const element = findUserMessageElement(scrollElement, messageId);
        if (!element) break;
        element.scrollIntoView({
          block: "center",
          behavior: attempt === 0 ? getNavigationScrollBehavior() : "auto",
        });
        const settled = await waitForUserMessageElement(
          scrollElement,
          messageId,
          () => mountedRef.current && sessionIdRef.current === actionSessionId,
        );
        if (settled) {
          programmaticNavigationRef.current = { messageId, expiresAt: Date.now() + 500 };
          replayMessageHighlight(settled);
          return true;
        }
      }
      programmaticNavigationRef.current = null;
      if (mountedRef.current && sessionIdRef.current === actionSessionId) {
        scrollElement.scrollTop = previousScrollTop;
      }
      return false;
    },
    [args.scrollRef, args.sessionId],
  );
  const navigation = useUserMessageNavigation({
    sessionId: args.sessionId,
    items: args.items,
    hasOlder: args.hasMore,
    oldestCursor: args.oldestCursor,
    loadOlder: args.loadMore,
    navigateTo,
  });
  useViewportOrigin(
    args.scrollRef,
    navigation.userMessageIds,
    navigation.setViewportOrigin,
    programmaticNavigationRef,
  );
  return navigation;
}

function useInitialScrollToBottom(
  scrollRef: React.RefObject<HTMLDivElement | null>,
  itemCount: number,
) {
  const didInitialScroll = useRef(false);
  useEffect(() => {
    if (didInitialScroll.current || itemCount === 0) return;
    const element = scrollRef.current;
    if (!element) return;
    if (useDockviewStore.getState().pendingChatScrollTop !== null) {
      didInitialScroll.current = true;
      return;
    }
    element.scrollTop = element.scrollHeight;
    didInitialScroll.current = true;
  }, [itemCount, scrollRef]);
}

export const NativeMessageList = memo(function NativeMessageList({
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
}: MessageListProps) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const needsRailClearance = usePersistentUserMessageNavigationRail();

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
  const streamingMessageId = getStreamingAgentMessageId(messages);
  const lastTurnGroupId = useMemo(() => getLastTurnGroupId(items), [items]);
  const navigation = useNativeUserNavigation({
    scrollRef,
    sessionId,
    items,
    hasMore,
    oldestCursor: oldestCursor ?? null,
    loadMore,
  });

  useScrollPositionOnPrepend(scrollRef, items.length);
  const sentinelRef = useLazyLoadSentinel(scrollRef, hasMore, isLoadingMore, loadMore);
  useAutoScroll(scrollRef, messages, isWorking);
  useInitialScrollToBottom(scrollRef, items.length);

  return (
    <div className="group/chat relative flex h-full min-h-0 flex-1">
      <SessionPanelContent
        ref={scrollRef}
        className={cn(
          "relative p-4 chat-message-list",
          needsRailClearance && USER_MESSAGE_NAVIGATION_MOBILE_CLEARANCE_CLASS,
        )}
      >
        {hasMore && <div ref={sentinelRef} className="h-px" />}

        <MessageListStatus
          isLoadingMore={isLoadingMore}
          hasMore={hasMore}
          showLoadingState={showLoadingState}
          messagesLoading={messagesLoading}
          isInitialLoading={isInitialLoading}
          messagesCount={messages.length}
          onLoadMore={loadMore}
        />

        {items.map((item) => {
          const key = getItemKey(item);
          return (
            <div key={key} className="pb-2" style={{ overflowAnchor: "none" }}>
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
        })}

        <AgentStatus sessionState={sessionState} sessionId={sessionId} messages={messages} />
        {(footerActionMessages ?? []).map((msg: Message) => (
          <MessageRenderer key={msg.id} comment={msg} isTaskDescription={false} />
        ))}

        {/* Bottom anchor — browser keeps scroll pinned here when new content appends */}
        <div style={{ overflowAnchor: "auto", height: 1 }} />
      </SessionPanelContent>
      {navigation.userMessageIds.length > 0 && (
        <UserMessageNavigationRail
          canNavigatePrevious={navigation.hasPrevious}
          canNavigateNext={navigation.hasNext}
          isBusy={navigation.isBusy}
          onPrevious={navigation.goPrevious}
          onNext={navigation.goNext}
        />
      )}
    </div>
  );
});
