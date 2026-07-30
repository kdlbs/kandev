"use client";

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, memo } from "react";
import { SessionPanelContent } from "@kandev/ui/pannel-session";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useAppStoreApi } from "@/components/state-provider";
import { getStoredAutoScrollTop } from "@/lib/local-storage";
import type { Message } from "@/lib/types/http";
import type { RenderItem } from "@/hooks/use-processed-messages";
import { useLazyLoadMessages } from "@/hooks/use-lazy-load-messages";
import { useSessionTurn } from "@/hooks/domains/session/use-session-turn";
import { MessageListFooter } from "./message-list-footer";
import { useTranscriptAutoScrollEnabled } from "./use-transcript-auto-scroll-enabled";
import {
  shouldAutoScrollOnMessagesChange,
  shouldAutoScrollOnWorkingStart,
  shouldCatchUpOnAutoScrollEnable,
  hasTranscriptProgressedPastView,
  hasTranscriptAppendedSinceBaseline,
  resolveNativeInitialScrollTop,
  isPrependUpdate,
  createFrameCoalescer,
} from "./transcript-auto-scroll";
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

/**
 * Continuously captures scroll state via scroll listener.
 * On a genuine prepend (older messages loaded above the current view, so the
 * first rendered item's identity changes), restores scroll position so the
 * user stays at the same visual spot. A plain append (new item count grows
 * but the first item is unchanged) is left alone — that's the auto-scroll
 * hook's concern, not this one's.
 */
function useScrollPositionOnPrepend(
  scrollRef: React.RefObject<HTMLDivElement | null>,
  items: RenderItem[],
) {
  const scrollState = useRef({ scrollHeight: 0, scrollTop: 0 });
  const prevItemCountRef = useRef(items.length);
  const prevFirstKeyRef = useRef<string | null>(items.length > 0 ? getItemKey(items[0]) : null);

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
    const nextFirstKey = items.length > 0 ? getItemKey(items[0]) : null;
    const prepend =
      !!el &&
      isPrependUpdate({
        prevItemCount: prevItemCountRef.current,
        nextItemCount: items.length,
        prevFirstKey: prevFirstKeyRef.current,
        nextFirstKey,
      });
    prevItemCountRef.current = items.length;
    prevFirstKeyRef.current = nextFirstKey;
    if (!el || !prepend) return;
    const prev = scrollState.current;
    const delta = el.scrollHeight - prev.scrollHeight;
    if (delta > 0) {
      el.scrollTop = prev.scrollTop + delta;
    }
  }, [items, scrollRef]);
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
 * or when the agent starts working (isWorking transitions to true) — unless
 * auto-scroll is disabled for this session, in which case both are
 * suppressed and the current position is continuously persisted so it
 * survives a dockview panel remount. Re-enabling catches the view up to the
 * bottom if the transcript progressed past it while disabled.
 *
 * Returns `isNearBottomRef` so the caller's initial-scroll effect can keep it
 * in sync after applying the resolved initial scrollTop.
 */
function useAutoScroll(
  scrollRef: React.RefObject<HTMLDivElement | null>,
  messages: Message[],
  isWorking: boolean,
  sessionId: string | null,
  enabled: boolean,
) {
  const storeApi = useAppStoreApi();
  const isNearBottomRef = useRef(true);
  const prevIsWorkingRef = useRef(isWorking);

  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const captureScrollTop = () => {
      if (sessionId) storeApi.getState().setTranscriptScrollTop(sessionId, el.scrollTop);
    };
    // Coalesce persisted writes to at most one per animation frame — native
    // scroll events can fire far more often than that, and each write is a
    // synchronous sessionStorage.setItem plus a store update.
    const coalescer = createFrameCoalescer(captureScrollTop);
    const onScroll = () => {
      isNearBottomRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 100;
      coalescer.schedule();
    };
    el.addEventListener("scroll", onScroll, { passive: true });
    return () => {
      el.removeEventListener("scroll", onScroll);
      // Final capture on unmount so a disabled session's exact position
      // survives a dockview panel teardown/remount (e.g. navigating away
      // and back), even if no scroll event fired right before it, and even
      // if a coalesced write above was still pending.
      coalescer.flush();
    };
  }, [scrollRef, sessionId]);

  // When isWorking transitions to true, force scroll to bottom (unless
  // disabled, or a layout rebuild scroll restore is pending — same guard
  // as the messages-change effect below).
  useEffect(() => {
    if (isWorking && !prevIsWorkingRef.current && shouldAutoScrollOnWorkingStart(enabled)) {
      const el = scrollRef.current;
      if (el && useDockviewStore.getState().pendingChatScrollTop === null) {
        el.scrollTop = el.scrollHeight;
        isNearBottomRef.current = true;
      }
    }
    prevIsWorkingRef.current = isWorking;
  }, [isWorking, scrollRef, enabled]);

  // Auto-scroll on new messages if near bottom (unless disabled)
  useLayoutEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    // Skip auto-scroll when a layout rebuild scroll restore is pending
    if (useDockviewStore.getState().pendingChatScrollTop !== null) return;
    if (shouldAutoScrollOnMessagesChange(enabled, isNearBottomRef.current)) {
      el.scrollTop = el.scrollHeight;
    }
  }, [messages, scrollRef, enabled]);

  useCatchUpOnReEnable(scrollRef, messages, enabled, isNearBottomRef);

  return isNearBottomRef;
}

/**
 * Catches the view up to the bottom when the user re-enables auto-scroll,
 * but only if the transcript actually appended content while disabled —
 * never on a manual scroll or a prepend, which don't change baselineRef's
 * identity. A one-time mount-time init covers panels that remount already
 * disabled (no live transition to capture a baseline from).
 */
function useCatchUpOnReEnable(
  scrollRef: React.RefObject<HTMLDivElement | null>,
  messages: Message[],
  enabled: boolean,
  isNearBottomRef: React.RefObject<boolean>,
) {
  const prevEnabledRef = useRef(enabled);
  const baselineRef = useRef<{
    count: number;
    lastId: string | null;
    lastUpdatedAt: string | undefined;
  } | null>(null);
  const hasInitializedBaselineRef = useRef(false);
  useEffect(() => {
    const wasEnabled = prevEnabledRef.current;
    prevEnabledRef.current = enabled;
    const captureBaseline = () => {
      const last = messages[messages.length - 1];
      baselineRef.current = {
        count: messages.length,
        lastId: last?.id ?? null,
        lastUpdatedAt: last?.updated_at,
      };
    };

    // First run: if the panel mounted already disabled (e.g. a session
    // opened, or remounted via a dockview rebuild, with a persisted
    // disabled preference), there was no in-process disable transition to
    // capture a baseline from — establish one now from whatever the
    // transcript looks like at mount, so a later re-enable can still detect
    // genuine progression instead of never catching up.
    if (!hasInitializedBaselineRef.current) {
      hasInitializedBaselineRef.current = true;
      if (!enabled) captureBaseline();
      return;
    }

    if (wasEnabled === enabled) return;
    if (!enabled) {
      // Transitioning to disabled: capture the baseline used to detect real
      // progression (a new row, or the trailing row streaming more content)
      // once re-enabled.
      captureBaseline();
      return;
    }
    // Transitioning to enabled.
    const el = scrollRef.current;
    const baseline = baselineRef.current;
    baselineRef.current = null;
    if (!el || !baseline) return;
    const lastNow = messages[messages.length - 1];
    const appendedSinceDisable = hasTranscriptAppendedSinceBaseline({
      baselineCount: baseline.count,
      currentCount: messages.length,
      baselineLastId: baseline.lastId,
      currentLastId: lastNow?.id ?? null,
      baselineLastUpdatedAt: baseline.lastUpdatedAt,
      currentLastUpdatedAt: lastNow?.updated_at,
    });
    const isAtBottom = !hasTranscriptProgressedPastView({
      scrollTop: el.scrollTop,
      scrollHeight: el.scrollHeight,
      clientHeight: el.clientHeight,
    });
    if (
      shouldCatchUpOnAutoScrollEnable({
        wasEnabled,
        nowEnabled: enabled,
        appendedSinceDisable,
        isAtBottom,
      })
    ) {
      el.scrollTop = el.scrollHeight;
      isNearBottomRef.current = true;
    }
  }, [enabled, scrollRef, messages]);
}

/**
 * Applies the initial scrollTop once items are available: bottom when
 * enabled, or the last captured offset for this session when disabled (see
 * resolveNativeInitialScrollTop). Skips while a dockview layout-rebuild
 * restore is pending — that separate mechanism owns the position.
 */
function useInitialScrollPosition(
  scrollRef: React.RefObject<HTMLDivElement | null>,
  itemCount: number,
  sessionId: string | null,
  enabled: boolean,
  isNearBottomRef: React.RefObject<boolean>,
) {
  const storeApi = useAppStoreApi();
  const didInitialScroll = useRef(false);
  useEffect(() => {
    if (didInitialScroll.current || itemCount === 0) return;
    const el = scrollRef.current;
    if (!el) return;
    const hasPendingLayoutRestore = useDockviewStore.getState().pendingChatScrollTop !== null;
    const savedScrollTop = sessionId
      ? (storeApi.getState().transcriptAutoScroll.scrollTopBySessionId[sessionId] ??
        getStoredAutoScrollTop(sessionId) ??
        undefined)
      : undefined;
    const scrollTop = resolveNativeInitialScrollTop({
      enabled,
      hasPendingLayoutRestore,
      savedScrollTop,
      scrollHeight: el.scrollHeight,
    });
    if (scrollTop !== null) {
      el.scrollTop = scrollTop;
      isNearBottomRef.current = !hasTranscriptProgressedPastView({
        scrollTop,
        scrollHeight: el.scrollHeight,
        clientHeight: el.clientHeight,
      });
    }
    didInitialScroll.current = true;
  }, [itemCount, sessionId, enabled, isNearBottomRef, storeApi]);
}

/** Returns a stable callback that smooth-scrolls a given message into the
 *  center of the viewport, given its rendered `msg-<id>` element. */
function useScrollToMessage() {
  return useCallback((messageId: string) => {
    const el = document.getElementById(`msg-${messageId}`);
    el?.scrollIntoView({ block: "center", behavior: "smooth" });
  }, []);
}

/**
 * Renders the transcript as plain DOM nodes with `overflow-anchor` for
 * scroll pinning. Wires together lazy-loading of older messages, the
 * scroll-position-on-prepend fix-up, and the session's auto-scroll toggle
 * (freeze/resume/catch-up) via the hooks defined above.
 */
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
  const autoScrollEnabled = useTranscriptAutoScrollEnabled(sessionId);

  useScrollPositionOnPrepend(scrollRef, items);
  const sentinelRef = useLazyLoadSentinel(scrollRef, hasMore, isLoadingMore, loadMore);
  const isNearBottomRef = useAutoScroll(
    scrollRef,
    messages,
    isWorking,
    sessionId,
    autoScrollEnabled,
  );
  useInitialScrollPosition(scrollRef, items.length, sessionId, autoScrollEnabled, isNearBottomRef);

  return (
    <SessionPanelContent
      ref={scrollRef}
      className={`relative p-4 chat-message-list ${
        autoScrollEnabled ? "[overflow-anchor:auto]" : "[overflow-anchor:none]"
      }`}
    >
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

      {/* Bottom anchor keeps the view pinned while auto-scroll is enabled.
          The scroll container disables anchoring entirely while it is off,
          so status/footer updates cannot choose a different anchor and move
          the frozen transcript. */}
      <div style={{ overflowAnchor: autoScrollEnabled ? "auto" : "none", height: 1 }} />
    </SessionPanelContent>
  );
});
