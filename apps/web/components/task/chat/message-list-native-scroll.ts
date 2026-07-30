import { useCallback, useEffect, useLayoutEffect, useRef } from "react";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useAppStoreApi } from "@/components/state-provider";
import { getStoredAutoScrollTop } from "@/lib/local-storage";
import type { Message } from "@/lib/types/http";
import type { RenderItem } from "@/hooks/use-processed-messages";
import { getItemKey, shouldAutoScrollToBottom } from "./message-list-shared";
import {
  isPrependUpdate,
  hasTranscriptProgressedPastView,
  hasTranscriptAppendedSinceBaseline,
  shouldCatchUpOnAutoScrollEnable,
  resolveNativeInitialScrollTop,
  createFrameCoalescer,
} from "./transcript-auto-scroll";

/**
 * Continuously captures scroll state via scroll listener.
 * On a genuine prepend (older messages loaded above the current view, so the
 * first rendered item's identity changes), restores scroll position so the
 * user stays at the same visual spot. A plain append (new item count grows
 * but the first item is unchanged) is left alone — that's the auto-scroll
 * hook's concern, not this one's. Skipped while a user-initiated programmatic
 * scroll (scroll-to-start / scroll-to-last-prompt) is in flight — otherwise
 * writing a stale captured `scrollTop` mid-animation interrupts/cancels the
 * user's smooth scroll and can leave the transcript at the wrong position.
 */
function useScrollPositionOnPrepend(
  scrollRef: React.RefObject<HTMLDivElement | null>,
  items: RenderItem[],
  isProgrammaticScrollLocked: () => boolean,
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
    if (!el || !prepend || isProgrammaticScrollLocked()) return;
    const prev = scrollState.current;
    const delta = el.scrollHeight - prev.scrollHeight;
    if (delta > 0) {
      el.scrollTop = prev.scrollTop + delta;
    }
  }, [items, scrollRef, isProgrammaticScrollLocked]);
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

/** Duration a programmatic scroll's guard stays held if the browser never
 * reports `scrollend` (Safari lacks it as of writing; some scroll targets
 * fire it inconsistently). Comfortably longer than a `scrollIntoView`
 * smooth-scroll's typical animation so the guard doesn't outlive it in the
 * common case, but still bounded so a missed event can't wedge auto-scroll
 * off indefinitely. */
const PROGRAMMATIC_SCROLL_GUARD_MS = 1000;

/**
 * Auto-scrolls to bottom when new messages arrive (if user is near bottom)
 * or when the agent starts working (isWorking transitions to true) — unless
 * auto-scroll is disabled for this session, in which case both are
 * suppressed and the current position is continuously persisted so it
 * survives a dockview panel remount. Re-enabling catches the view up to the
 * bottom if the transcript progressed past it while disabled (see
 * `useCatchUpOnReEnable`).
 *
 * `isProgrammaticScrollLocked` suppresses both triggers while a user-initiated
 * scroll-to-start/scroll-to-last-prompt animation (see
 * `useProgrammaticScrollGuard`) is still in flight — otherwise a message
 * streaming in mid-animation can silently snap the transcript back to the
 * bottom, cancelling the user's action.
 */
function useAutoScroll(params: {
  scrollRef: React.RefObject<HTMLDivElement | null>;
  messages: Message[];
  isWorking: boolean;
  sessionId: string | null;
  enabled: boolean;
  isProgrammaticScrollLocked: () => boolean;
}) {
  const { scrollRef, messages, isWorking, sessionId, enabled, isProgrammaticScrollLocked } = params;
  const storeApi = useAppStoreApi();
  const isNearBottomRef = useRef(true);
  const prevIsWorkingRef = useRef(isWorking);

  const resyncIsNearBottom = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    isNearBottomRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 100;
  }, [scrollRef]);

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
      resyncIsNearBottom();
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
  }, [scrollRef, sessionId, storeApi, resyncIsNearBottom]);

  // When isWorking transitions to true, force scroll to bottom (unless
  // disabled, locked, or a layout rebuild scroll restore is pending).
  useEffect(() => {
    if (
      isWorking &&
      !prevIsWorkingRef.current &&
      shouldAutoScrollToBottom({
        isNearBottom: true,
        isProgrammaticScrollLocked: isProgrammaticScrollLocked(),
        hasPendingLayoutRestore: useDockviewStore.getState().pendingChatScrollTop !== null,
      }) &&
      enabled
    ) {
      const el = scrollRef.current;
      if (el) {
        el.scrollTop = el.scrollHeight;
        isNearBottomRef.current = true;
      }
    }
    prevIsWorkingRef.current = isWorking;
  }, [isWorking, scrollRef, enabled, isProgrammaticScrollLocked]);

  // Auto-scroll on new messages if near bottom (unless disabled, locked, or a
  // layout rebuild scroll restore is pending).
  useLayoutEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    if (
      shouldAutoScrollToBottom({
        isNearBottom: isNearBottomRef.current,
        isProgrammaticScrollLocked: isProgrammaticScrollLocked(),
        hasPendingLayoutRestore: useDockviewStore.getState().pendingChatScrollTop !== null,
      }) &&
      enabled
    ) {
      el.scrollTop = el.scrollHeight;
    }
  }, [messages, scrollRef, enabled, isProgrammaticScrollLocked]);

  useCatchUpOnReEnable(scrollRef, messages, enabled, isNearBottomRef);

  return { isNearBottomRef, resyncIsNearBottom };
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
 * Guards a user-initiated smooth scroll (scroll-to-start / scroll-to-last-
 * prompt) against `useAutoScroll`'s follow-bottom behavior firing mid-flight
 * — e.g. the agent streams a new message while the scroll is still animating,
 * which would otherwise snap the transcript back to the bottom and silently
 * cancel the user's action. Held until the scroll settles (native `scrollend`
 * where supported, a bounded timeout fallback otherwise), then resyncs the
 * near-bottom state from the ACTUAL scroll position — never assumed — before
 * releasing.
 */
function useProgrammaticScrollGuard(
  scrollRef: React.RefObject<HTMLDivElement | null>,
  lockedRef: React.RefObject<boolean>,
  resyncIsNearBottom: () => void,
) {
  const cleanupRef = useRef<(() => void) | null>(null);

  const release = useCallback(() => {
    cleanupRef.current?.();
    cleanupRef.current = null;
    lockedRef.current = false;
    resyncIsNearBottom();
  }, [lockedRef, resyncIsNearBottom]);

  const runGuardedScroll = useCallback(
    (performScroll: () => void) => {
      cleanupRef.current?.();
      lockedRef.current = true;
      performScroll();

      const timeoutId = window.setTimeout(release, PROGRAMMATIC_SCROLL_GUARD_MS);
      const el = scrollRef.current;
      el?.addEventListener("scrollend", release, { once: true });
      cleanupRef.current = () => {
        window.clearTimeout(timeoutId);
        el?.removeEventListener("scrollend", release);
      };
    },
    [scrollRef, lockedRef, release],
  );

  useEffect(() => () => cleanupRef.current?.(), []);

  return runGuardedScroll;
}

function useScrollToMessage(runGuardedScroll: (performScroll: () => void) => void) {
  return useCallback(
    (messageId: string, options?: { align?: "start" | "center" }) => {
      const el = document.getElementById(`msg-${messageId}`);
      if (!el) return;
      runGuardedScroll(() => {
        el.scrollIntoView({
          block: options?.align === "start" ? "start" : "center",
          behavior: "smooth",
        });
      });
    },
    [runGuardedScroll],
  );
}

/**
 * Applies the initial scrollTop once items are available: bottom when
 * enabled, or the last captured offset for this session when disabled (see
 * `resolveNativeInitialScrollTop`). Skips while a dockview layout-rebuild
 * restore is pending — that separate mechanism owns the position instead.
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

/**
 * Composes every native-renderer scroll behavior — auto-follow-bottom (honoring
 * the session's auto-scroll toggle, with position persistence and re-enable
 * catch-up), the programmatic-scroll guard that protects scroll-to-start/
 * scroll-to-last-prompt from streaming-content races, prepend scroll
 * preservation, lazy-load sentinel observation, and the initial scroll
 * position — behind one call so `NativeMessageList` only wires a scroll
 * container, item list, and the auto-scroll preference through it.
 */
export function useNativeScrollManagement(params: {
  scrollRef: React.RefObject<HTMLDivElement | null>;
  items: RenderItem[];
  messages: Message[];
  isWorking: boolean;
  sessionId: string | null;
  enabled: boolean;
  hasMore: boolean;
  isLoadingMore: boolean;
  loadMore: () => Promise<number>;
}) {
  const {
    scrollRef,
    items,
    messages,
    isWorking,
    sessionId,
    enabled,
    hasMore,
    isLoadingMore,
    loadMore,
  } = params;
  const programmaticScrollLockRef = useRef(false);
  const isProgrammaticScrollLocked = useCallback(() => programmaticScrollLockRef.current, []);
  const { isNearBottomRef, resyncIsNearBottom } = useAutoScroll({
    scrollRef,
    messages,
    isWorking,
    sessionId,
    enabled,
    isProgrammaticScrollLocked,
  });
  const runGuardedScroll = useProgrammaticScrollGuard(
    scrollRef,
    programmaticScrollLockRef,
    resyncIsNearBottom,
  );
  const handleScrollToMessage = useScrollToMessage(runGuardedScroll);
  useScrollPositionOnPrepend(scrollRef, items, isProgrammaticScrollLocked);
  const sentinelRef = useLazyLoadSentinel(scrollRef, hasMore, isLoadingMore, loadMore);
  useInitialScrollPosition(scrollRef, items.length, sessionId, enabled, isNearBottomRef);

  return { handleScrollToMessage, sentinelRef };
}
