"use client";

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, memo } from "react";
import { SessionPanelContent } from "@kandev/ui/pannel-session";
import { useDockviewStore } from "@/lib/state/dockview-store";
import type { Message } from "@/lib/types/http";
import { useLazyLoadMessages } from "@/hooks/use-lazy-load-messages";
import { useSessionTurn } from "@/hooks/domains/session/use-session-turn";
import { MessageListFooter } from "./message-list-footer";
import {
  type MessageListProps,
  MessageListStatus,
  MessageItem,
  UnreadDivider,
  getItemKey,
  getConversationLoadingState,
  getEffectiveActiveTurnId,
  getLastTurnGroupId,
  getStreamingAgentMessageId,
  canReassertDividerScroll,
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
  // The very first "messages changed" layout effect run below coincides
  // with mount — deliberately skipped here because initial positioning
  // (scroll to bottom, or to the Slack-style "New" divider instead) is
  // fully owned by the caller's own didInitialScroll/didScrollToDivider
  // effect. Without this guard, this layout effect's unconditional
  // isNearBottomRef.current === true default raced that effect on mount
  // and clobbered a just-applied divider scroll back to the bottom, since
  // both are layout/effect timing siblings on the same commit and the
  // native "scroll" event that would otherwise flip isNearBottomRef to
  // false hasn't fired yet by the time this one reads it.
  const hasHandledMountRef = useRef(false);

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
    if (!hasHandledMountRef.current) {
      hasHandledMountRef.current = true;
      return;
    }
    // Skip auto-scroll when a layout rebuild scroll restore is pending
    if (useDockviewStore.getState().pendingChatScrollTop !== null) return;
    if (isNearBottomRef.current) {
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
 *   within a short settling window since mount (isWithinSettlingWindow).
 *   The window exists so a live message arriving long after the visit has
 *   genuinely settled — with no wheel/touch/key event to catch, e.g. a
 *   scrollbar drag — can never re-trigger a correction; once either gate
 *   trips, it's the user's scroll position to own, same as Slack never
 *   re-snapping you to the unread line once you've started reading.
 * - didScrollToDivider and didInitialScroll are separate latches so the
 *   bottom-fallback firing first (before dividerBeforeItemKey resolves)
 *   doesn't block the divider correction from still applying once it
 *   does. Embedded, always-invisible previews (isVisible hardcoded false,
 *   see TaskChatPanel) never resolve a divider, so they keep the
 *   original, unconditional scroll-to-bottom-on-mount behavior untouched.
 */
function useScrollToDividerOrBottom(
  scrollRef: React.RefObject<HTMLDivElement | null>,
  itemCount: number,
  dividerBeforeItemKey: string | null | undefined,
) {
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

  // Bounds how long the divider correction below can keep re-asserting
  // itself after mount, independent of user interaction: a scrollbar drag
  // (no wheel/touch/key event) or a live message arriving long after the
  // visit has settled must never be able to re-trigger it. 4s comfortably
  // covers the slowest observed multi-wave initial load (WS backfill
  // continuing after the REST fetch) without lingering into the range
  // where the user has plausibly started reading and scrolling normally.
  const mountedAtRef = useRef<number | null>(null);
  if (mountedAtRef.current === null) mountedAtRef.current = Date.now();
  const isWithinSettlingWindow = () => Date.now() - (mountedAtRef.current ?? 0) < 4000;

  const didInitialScroll = useRef(false);
  const didScrollToDivider = useRef(false);
  useEffect(() => {
    const el = scrollRef.current;
    if (!el || itemCount === 0) return;
    const canReassertDivider = canReassertDividerScroll({
      hasDividerTarget: Boolean(dividerBeforeItemKey),
      didScrollToDivider: didScrollToDivider.current,
      isUserScrolling: isUserScrollingRef.current,
      isWithinSettlingWindow: isWithinSettlingWindow(),
    });
    if (canReassertDivider) {
      if (useDockviewStore.getState().pendingChatScrollTop === null) {
        const dividerEl = el.querySelector<HTMLElement>(`[id="msg-${dividerBeforeItemKey}"]`);
        if (dividerEl) {
          dividerEl.scrollIntoView({ block: "start" });
          didScrollToDivider.current = true;
          didInitialScroll.current = true;
          return;
        }
      }
    }
    if (didInitialScroll.current) return;
    // If a layout rebuild scroll restore is pending, skip initial scroll
    // (the restore handler will set the correct position)
    if (useDockviewStore.getState().pendingChatScrollTop !== null) {
      didInitialScroll.current = true;
      return;
    }
    el.scrollTop = el.scrollHeight;
    didInitialScroll.current = true;
  }, [itemCount, dividerBeforeItemKey]);
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
  dividerBeforeItemKey,
}: MessageListProps) {
  const scrollRef = useRef<HTMLDivElement>(null);

  const { isInitialLoading, showLoadingState } = getConversationLoadingState({
    messagesLoading,
    messagesCount: messages.length,
    isWorking,
    sessionState,
  });
  const { loadMore, hasMore, isLoading: isLoadingMore } = useLazyLoadMessages(sessionId);
  const { activeTurnId } = useSessionTurn(sessionId);
  const effectiveActiveTurnId = getEffectiveActiveTurnId(activeTurnId, isWorking);
  const streamingMessageId = getStreamingAgentMessageId(messages);
  const lastTurnGroupId = useMemo(() => getLastTurnGroupId(items), [items]);
  const handleScrollToMessage = useScrollToMessage();

  useScrollPositionOnPrepend(scrollRef, items.length);
  const sentinelRef = useLazyLoadSentinel(scrollRef, hasMore, isLoadingMore, loadMore);
  useAutoScroll(scrollRef, messages, isWorking);
  useScrollToDividerOrBottom(scrollRef, items.length, dividerBeforeItemKey);

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
        onLoadMore={loadMore}
      />

      {items.map((item) => {
        const key = getItemKey(item);
        return (
          <div
            key={key}
            id={`msg-${key}`}
            className="pb-2 scroll-mt-[calc(4rem+env(safe-area-inset-top))] sm:scroll-mt-0"
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
              isLastGroup={item.type === "turn_group" && item.id === lastTurnGroupId}
              activeTurnId={effectiveActiveTurnId}
              streamingMessageId={streamingMessageId}
              onScrollToMessage={handleScrollToMessage}
            />
          </div>
        );
      })}

      <MessageListFooter
        sessionState={sessionState}
        sessionId={sessionId}
        messages={messages}
        isWorking={isWorking}
        footerActionMessages={footerActionMessages}
      />

      {/* Bottom anchor — browser keeps scroll pinned here when new content appends */}
      <div style={{ overflowAnchor: "auto", height: 1 }} />
    </SessionPanelContent>
  );
});
