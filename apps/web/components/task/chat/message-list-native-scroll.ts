/* eslint-disable max-lines -- pagination, scroll anchoring, and retry state share one boundary. */

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useAppStoreApi } from "@/components/state-provider";
import { getStoredAutoScrollTop } from "@/lib/local-storage";
import {
  useLazyLoadSentinel as useSharedLazyLoadSentinel,
  type LazyLoadSentinelSettleResult,
} from "@/hooks/use-lazy-load-sentinel";
import type { Message } from "@/lib/types/http";
import { TASK_DESCRIPTION_SYNTHETIC_ID, type RenderItem } from "@/hooks/use-processed-messages";
import { getItemKey, shouldAutoScrollToBottom } from "./message-list-shared";
import { getOldestVisibleBoundaryKey } from "./message-list-native-boundary";
import {
  isPrependUpdate,
  hasTranscriptProgressedPastView,
  hasTranscriptAppendedSinceBaseline,
  shouldCatchUpOnAutoScrollEnable,
  resolveNativeInitialScrollTop,
  createFrameCoalescer,
} from "./transcript-auto-scroll";
import { scheduleClampedScrollRestore } from "./clamped-scroll-restore";
import { createDebugLogger, isDebug } from "@/lib/debug/log";

const paginationDebug = createDebugLogger("messages:pagination");
// i18n-exempt: IntersectionObserver root-margin configuration, not user-facing copy.
export const TRANSCRIPT_SENTINEL_ROOT_MARGIN = "200px 0px 0px 0px";

// INT32_MAX: WebKit resolves Number.MAX_SAFE_INTEGER to 0 (not bottom).
const NATIVE_BOTTOM_SCROLL_TOP = 2_147_483_647;

/** Writes a clamped maximum so the browser resolves the native bottom without
 * forcing a synchronous scrollHeight layout read. */
export function scrollNativeToBottom(element: HTMLElement): void {
  element.scrollTop = NATIVE_BOTTOM_SCROLL_TOP;
}

type PaginationRequest = {
  boundaryBefore: string | null;
  sessionEpoch: number;
  debug?: {
    generation: number;
    scrollTopBefore: number | null;
    scrollHeightBefore: number | null;
  };
};

type PaginationStopReason =
  | "visible-boundary-unchanged"
  | "visible-boundary-added"
  | "exhausted"
  | "no-progress"
  | "sentinel-left-preload"
  | "disarmed"
  | "blocked"
  | "stale"
  | "not-rearmed";

export function resolvePaginationStopReason(
  boundaryUnchanged: boolean,
  hasMore: boolean,
): PaginationStopReason {
  if (!hasMore) return "exhausted";
  return boundaryUnchanged ? "visible-boundary-unchanged" : "visible-boundary-added";
}

function resolvePaginationSettleReason(
  result: LazyLoadSentinelSettleResult,
  boundaryUnchanged: boolean,
  hasMore: boolean,
  sentinelInPreload: boolean,
): PaginationStopReason | null {
  switch (result.continuation) {
    case "continued":
      return null;
    case "rejected":
    case "no-progress":
      return "no-progress";
    case "caller-stopped":
    case "no-more":
      if (result.continuation === "caller-stopped" && !sentinelInPreload) {
        return "sentinel-left-preload";
      }
      return resolvePaginationStopReason(boundaryUnchanged, hasMore);
    default:
      return result.continuation;
  }
}

type RootMarginPixels = {
  top: number;
  right: number;
  bottom: number;
  left: number;
};

function parseRootMarginPixels(rootMargin: string): RootMarginPixels {
  const values = rootMargin
    .trim()
    .split(/\s+/)
    .map((value) => Number.parseFloat(value))
    .map((value) => (Number.isFinite(value) ? value : 0));
  const [top = 0, right = top, bottom = top, left = right] = values;
  return { top, right, bottom, left };
}

/** Returns whether the current sentinel geometry is inside the observer's
 * preload rectangle. This is intentionally measured after a page commit,
 * rather than inferred from the last IntersectionObserver entry. */
export function isElementInPreloadRegion(
  scrollRoot: HTMLElement,
  sentinel: HTMLElement,
  rootMargin = TRANSCRIPT_SENTINEL_ROOT_MARGIN,
): boolean {
  const rootRect = scrollRoot.getBoundingClientRect();
  const sentinelRect = sentinel.getBoundingClientRect();
  const margin = parseRootMarginPixels(rootMargin);
  return (
    sentinelRect.bottom >= rootRect.top - margin.top &&
    sentinelRect.top <= rootRect.bottom + margin.bottom &&
    sentinelRect.right >= rootRect.left - margin.left &&
    sentinelRect.left <= rootRect.right + margin.right
  );
}

function resolveRecoveryVisibility(
  result: LazyLoadSentinelSettleResult,
  hasMore: boolean,
): boolean | null {
  if (result.continuation === "stale") return null;
  if (hasMore && (result.continuation === "rejected" || result.continuation === "no-progress")) {
    return true;
  }
  if (result.count > 0 || !hasMore) return false;
  return null;
}

function usePaginationRecovery(sessionId: string | null, hasMore: boolean) {
  const [showRecovery, setShowRecovery] = useState(false);
  const recoverySessionRef = useRef(sessionId);

  useEffect(() => {
    if (recoverySessionRef.current !== sessionId || !sessionId || !hasMore) {
      setShowRecovery(false);
    }
    recoverySessionRef.current = sessionId;
  }, [hasMore, sessionId]);

  const reportRecovery = useCallback(
    (result: LazyLoadSentinelSettleResult) => {
      const nextVisibility = resolveRecoveryVisibility(result, hasMore);
      if (nextVisibility !== null) setShowRecovery(nextVisibility);
    },
    [hasMore],
  );

  return { showRecovery, reportRecovery };
}

function useSessionEpoch(sessionId: string | null) {
  const epochRef = useRef(0);
  const previousSessionIdRef = useRef(sessionId);
  if (previousSessionIdRef.current !== sessionId) {
    previousSessionIdRef.current = sessionId;
    epochRef.current += 1;
  }
  return epochRef;
}

function reportPaginationSettleDebug(params: {
  result: LazyLoadSentinelSettleResult;
  sessionId: string | null;
  request: PaginationRequest | null;
  items: RenderItem[];
  scrollRoot: HTMLElement | null;
  sentinel: HTMLElement | null;
  hasMore: boolean;
}) {
  const request = params.request;
  if (!isDebug() || !request?.debug) return;
  const { result, scrollRoot, sentinel } = params;
  const requestDebug = request.debug;
  const boundaryAfter = getOldestVisibleBoundaryKey(params.items);
  const boundaryUnchanged = request.boundaryBefore === boundaryAfter;
  const sentinelInPreload = Boolean(
    scrollRoot &&
    sentinel &&
    isElementInPreloadRegion(scrollRoot, sentinel, TRANSCRIPT_SENTINEL_ROOT_MARGIN),
  );
  paginationDebug("older page settled", {
    sessionId: params.sessionId,
    trigger: "top-intersection",
    generation: requestDebug.generation,
    loadedCount: result.count,
    boundaryBefore: request.boundaryBefore,
    boundaryAfter,
    scrollTopBefore: requestDebug.scrollTopBefore,
    scrollTopAfter: scrollRoot?.scrollTop ?? null,
    scrollHeightBefore: requestDebug.scrollHeightBefore,
    scrollHeightAfter: scrollRoot?.scrollHeight ?? null,
    continued: result.continuation === "continued",
    stopReason: resolvePaginationSettleReason(
      result,
      boundaryUnchanged,
      params.hasMore,
      sentinelInPreload,
    ),
    continuation: result.continuation,
  });
}

/**
 * Continuously captures scroll state via scroll listener.
 * On a genuine prepend (older messages loaded above the current view, so the
 * oldest non-synthetic item's identity changes), restores scroll position so
 * the user stays at the same visual spot. A plain append (new item count grows
 * but the oldest real item is unchanged) is left alone — that's the
 * auto-scroll hook's concern, not this one's. Skipped while a user-initiated
 * programmatic scroll (scroll-to-start / scroll-to-last-prompt) is in flight
 * — otherwise writing a stale captured `scrollTop` mid-animation
 * interrupts/cancels the user's smooth scroll and can leave the transcript at
 * the wrong position.
 */
function useScrollPositionOnPrepend(
  scrollRef: React.RefObject<HTMLDivElement | null>,
  items: RenderItem[],
  isLoadingMore: boolean,
  isProgrammaticScrollLocked: () => boolean,
) {
  const scrollState = useRef<{
    scrollHeight: number;
    scrollTop: number;
    anchorKey: string | null;
    anchorTop: number | null;
  }>({ scrollHeight: 0, scrollTop: 0, anchorKey: null, anchorTop: null });
  const isLoadingMoreRef = useRef(isLoadingMore);
  const olderLoadPendingRef = useRef(false);
  const newestItemKeyRef = useRef<string | null>(getNewestNonSyntheticItemKey(items));
  isLoadingMoreRef.current = isLoadingMore;
  if (isLoadingMore) olderLoadPendingRef.current = true;
  newestItemKeyRef.current = getNewestNonSyntheticItemKey(items);
  const prevItemCountRef = useRef(items.length);
  const prevFirstKeyRef = useRef<string | null>(getOldestNonSyntheticItemKey(items));

  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    /** Captures the container's current scrollHeight/scrollTop so a later
     * prepend can restore the visual position. */
    const onScroll = () => {
      // Native overflow anchoring can adjust scrollTop while the older page is
      // being inserted. Keep the pre-request baseline until our layout effect
      // has restored the visual position explicitly.
      if (isLoadingMoreRef.current) return;
      scrollState.current = capturePrependScrollState(el, newestItemKeyRef.current);
    };
    onScroll();
    el.addEventListener("scroll", onScroll, { passive: true });
    return () => el.removeEventListener("scroll", onScroll);
  }, [scrollRef]);

  useLayoutEffect(() => {
    const el = scrollRef.current;
    const nextFirstKey = getOldestNonSyntheticItemKey(items);
    const identityPrepend =
      !!el &&
      isPrependUpdate({
        prevItemCount: prevItemCountRef.current,
        nextItemCount: items.length,
        prevFirstKey: prevFirstKeyRef.current,
        nextFirstKey,
      });
    const olderLoadSettled = olderLoadPendingRef.current && !isLoadingMore;
    const prepend = olderLoadSettled || (!olderLoadPendingRef.current && identityPrepend);
    prevItemCountRef.current = items.length;
    prevFirstKeyRef.current = nextFirstKey;
    if (olderLoadSettled) olderLoadPendingRef.current = false;
    if (!el || !prepend || isProgrammaticScrollLocked()) return;
    const prev = scrollState.current;
    const anchor = findMessageRow(el, prev.anchorKey);
    if (anchor && prev.anchorTop !== null) {
      el.scrollTop += anchor.getBoundingClientRect().top - prev.anchorTop;
    } else {
      const delta = el.scrollHeight - prev.scrollHeight;
      if (delta > 0) el.scrollTop = prev.scrollTop + delta;
    }
    scrollState.current = capturePrependScrollState(el, newestItemKeyRef.current);
  }, [items, scrollRef, isLoadingMore, isProgrammaticScrollLocked]);
}

function getOldestNonSyntheticItemKey(items: RenderItem[]): string | null {
  const oldestRealItem = items.find((item) => {
    if (item.type === "prepare_progress" || item.type === "agent_error_notice") return false;
    return item.type !== "message" || item.message.id !== TASK_DESCRIPTION_SYNTHETIC_ID;
  });
  return oldestRealItem ? getItemKey(oldestRealItem) : null;
}

function getNewestNonSyntheticItemKey(items: RenderItem[]): string | null {
  for (let index = items.length - 1; index >= 0; index -= 1) {
    const item = items[index];
    if (item.type === "prepare_progress" || item.type === "agent_error_notice") continue;
    if (item.type === "message" && item.message.id === TASK_DESCRIPTION_SYNTHETIC_ID) continue;
    return getItemKey(item);
  }
  return null;
}

function findMessageRow(scrollRoot: HTMLElement, itemKey: string | null): HTMLElement | null {
  if (!itemKey) return null;
  const expectedId = `msg-${itemKey}`;
  return (
    Array.from(scrollRoot.querySelectorAll<HTMLElement>("[id^='msg-']")).find(
      (candidate) => candidate.id === expectedId,
    ) ?? null
  );
}

function capturePrependScrollState(scrollRoot: HTMLElement, anchorKey: string | null) {
  const anchor = findMessageRow(scrollRoot, anchorKey);
  return {
    scrollHeight: scrollRoot.scrollHeight,
    scrollTop: scrollRoot.scrollTop,
    anchorKey,
    anchorTop: anchor?.getBoundingClientRect().top ?? null,
  };
}

/**
 * Observes a sentinel element at the top of the list to trigger lazy loading.
 * Re-arms only while the committed sentinel remains inside the preload
 * region, which crosses both collapsed activity and standalone messages. The
 * transcript does not join an in-flight request. The explicit button is
 * rendered only as the recovery path for errors and no-op pages.
 */
// eslint-disable-next-line max-lines-per-function -- request epoch, geometry, and recovery must stay synchronized here.
function useLazyLoadSentinel(params: {
  scrollRef: React.RefObject<HTMLDivElement | null>;
  items: RenderItem[];
  sessionId: string | null;
  hasMore: boolean;
  blocked: boolean;
  isLoadingMore: boolean;
  loadMore: () => Promise<number>;
}) {
  const { scrollRef, items, sessionId, hasMore, blocked, isLoadingMore, loadMore } = params;
  const itemsRef = useRef(items);
  itemsRef.current = items;
  const hasMoreRef = useRef(hasMore);
  hasMoreRef.current = hasMore;
  const sentinelNodeRef = useRef<HTMLDivElement | null>(null);
  const requestGenerationRef = useRef(0);
  const requestRef = useRef<PaginationRequest | null>(null);
  const sessionEpochRef = useSessionEpoch(sessionId);
  const { showRecovery, reportRecovery } = usePaginationRecovery(sessionId, hasMore);

  const reportSettle = useCallback(
    (result: LazyLoadSentinelSettleResult) => {
      const request = requestRef.current;
      const staleSession = request !== null && request.sessionEpoch !== sessionEpochRef.current;
      if (!staleSession) reportRecovery(result);
      reportPaginationSettleDebug({
        result,
        sessionId,
        request,
        items: itemsRef.current,
        scrollRoot: scrollRef.current,
        sentinel: sentinelNodeRef.current,
        hasMore: hasMoreRef.current,
      });
    },
    [reportRecovery, scrollRef, sessionId],
  );

  const loadPage = useCallback(async () => {
    const request: PaginationRequest = {
      boundaryBefore: getOldestVisibleBoundaryKey(itemsRef.current),
      sessionEpoch: sessionEpochRef.current,
    };
    requestRef.current = request;
    if (isDebug()) {
      const scroller = scrollRef.current;
      request.debug = {
        generation: ++requestGenerationRef.current,
        scrollTopBefore: scroller?.scrollTop ?? null,
        scrollHeightBefore: scroller?.scrollHeight ?? null,
      };
      paginationDebug("older page started", {
        sessionId,
        trigger: "top-intersection",
        generation: request.debug.generation,
        boundaryBefore: request.boundaryBefore,
        scrollTopBefore: request.debug.scrollTopBefore,
        scrollHeightBefore: request.debug.scrollHeightBefore,
      });
    }
    return loadMore();
  }, [loadMore, scrollRef, sessionId]);

  const isRequestCurrent = useCallback(() => {
    const request = requestRef.current;
    return request === null || request.sessionEpoch === sessionEpochRef.current;
  }, []);

  const shouldContinueWhileIntersecting = useCallback(() => {
    const scrollRoot = scrollRef.current;
    const sentinel = sentinelNodeRef.current;
    return Boolean(
      hasMoreRef.current &&
      scrollRoot &&
      sentinel &&
      isElementInPreloadRegion(scrollRoot, sentinel, TRANSCRIPT_SENTINEL_ROOT_MARGIN),
    );
  }, [scrollRef]);

  const sharedSentinel = useSharedLazyLoadSentinel(
    scrollRef,
    hasMore,
    blocked,
    isLoadingMore,
    loadPage,
    {
      rootMargin: TRANSCRIPT_SENTINEL_ROOT_MARGIN,
      rearmWhileIntersecting: true,
      shouldContinueWhileIntersecting,
      onLoadSettled: reportSettle,
      isRequestCurrent,
    },
  );
  const sentinelRef = useCallback(
    (node: HTMLDivElement | null) => {
      sentinelNodeRef.current = node;
      sharedSentinel.sentinelRef(node);
    },
    [sharedSentinel.sentinelRef],
  );

  return {
    sentinelRef,
    onUserGesture: sharedSentinel.onUserGesture,
    retry: sharedSentinel.retry,
    showRecovery,
  };
}

/**
 * Treats an actual upward movement as fresh pagination intent. This is
 * modality-independent (wheel, keyboard, scrollbar, or touch) and only calls
 * the shared sentinel's guarded retry path; normal armed scrolling remains
 * owned by IntersectionObserver.
 */
function useRetryPaginationOnUpwardScroll(
  scrollRef: React.RefObject<HTMLDivElement | null>,
  onUserGesture: () => void,
  isProgrammaticScrollLocked: () => boolean,
) {
  useEffect(() => {
    const scroller = scrollRef.current;
    if (!scroller) return;
    let previousScrollTop = scroller.scrollTop;
    const onScroll = () => {
      const nextScrollTop = scroller.scrollTop;
      const movedUp = nextScrollTop < previousScrollTop;
      previousScrollTop = nextScrollTop;
      if (movedUp && !isProgrammaticScrollLocked()) onUserGesture();
    };
    scroller.addEventListener("scroll", onScroll, { passive: true });
    return () => scroller.removeEventListener("scroll", onScroll);
  }, [isProgrammaticScrollLocked, onUserGesture, scrollRef]);
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
export function useAutoScroll(params: {
  scrollRef: React.RefObject<HTMLDivElement | null>;
  messages: Message[];
  isWorking: boolean;
  sessionId: string | null;
  enabled: boolean;
  hasUnreadDivider: boolean;
  isProgrammaticScrollLocked: () => boolean;
}) {
  const {
    scrollRef,
    messages,
    isWorking,
    sessionId,
    enabled,
    hasUnreadDivider,
    isProgrammaticScrollLocked,
  } = params;
  const storeApi = useAppStoreApi();
  const isNearBottomRef = useRef(true);
  const prevIsWorkingRef = useRef(isWorking);
  const prevEnabledRef = useRef(enabled);
  const frozenScrollTopRef = useRef<number | null>(null);

  const resyncIsNearBottom = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    isNearBottomRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 100;
  }, [scrollRef]);
  const markNotNearBottom = useCallback(() => {
    isNearBottomRef.current = false;
  }, []);

  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    /** Persists the container's current scrollTop for the session (used when
     * auto-scroll is disabled). */
    const captureScrollTop = () => {
      if (sessionId) storeApi.getState().setTranscriptScrollTop(sessionId, el.scrollTop);
    };
    // Coalesce persisted writes to at most one per animation frame — native
    // scroll events can fire far more often than that, and each write is a
    // synchronous sessionStorage.setItem plus a store update.
    const coalescer = createFrameCoalescer(captureScrollTop);
    /** Scroll listener: resyncs the near-bottom flag and schedules a
     * coalesced persistence of the scroll position. */
    const onScroll = () => {
      resyncIsNearBottom();
      if (!enabled) frozenScrollTopRef.current = el.scrollTop;
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
  }, [scrollRef, sessionId, storeApi, resyncIsNearBottom, enabled]);

  // Own the disabled offset across every transcript layout update. Sending a
  // prompt can briefly shrink the scroll range before the new message row is
  // committed, which makes the browser clamp scrollTop even with
  // overflow-anchor disabled. Keep the pre-update offset and reapply it after
  // each message/working-state render so that transient clamp cannot move the
  // reader. Real user scroll events update the owned offset above.
  useLayoutEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const wasEnabled = prevEnabledRef.current;
    prevEnabledRef.current = enabled;
    if (enabled) {
      frozenScrollTopRef.current = null;
      return;
    }
    if (wasEnabled || frozenScrollTopRef.current === null) {
      frozenScrollTopRef.current = el.scrollTop;
      return;
    }
    el.scrollTop = frozenScrollTopRef.current;
  }, [enabled, isWorking, messages, scrollRef]);

  // When isWorking transitions to true, force scroll to bottom (unless
  // disabled, locked, or a layout rebuild scroll restore is pending).
  useEffect(() => {
    if (
      isWorking &&
      !prevIsWorkingRef.current &&
      shouldAutoScrollToBottom({
        isNearBottom: !hasUnreadDivider,
        isProgrammaticScrollLocked: isProgrammaticScrollLocked(),
        hasPendingLayoutRestore: useDockviewStore.getState().pendingChatScrollTop !== null,
      }) &&
      enabled
    ) {
      const el = scrollRef.current;
      if (el) {
        scrollNativeToBottom(el);
        isNearBottomRef.current = true;
      }
    }
    prevIsWorkingRef.current = isWorking;
  }, [hasUnreadDivider, isWorking, scrollRef, enabled, isProgrammaticScrollLocked]);

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
      scrollNativeToBottom(el);
    }
  }, [messages, scrollRef, enabled, isProgrammaticScrollLocked]);

  useCatchUpOnReEnable(scrollRef, messages, enabled, isNearBottomRef);

  return { isNearBottomRef, resyncIsNearBottom, markNotNearBottom };
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
    /** Snapshots the current transcript tail (count, last id, last updated
     * timestamp) as the disable-time baseline for detecting progression. */
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
      scrollNativeToBottom(el);
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

/**
 * Returns a `scrollToMessage(messageId, options?)` callback that scrolls the
 * message's row into view (start- or center-aligned) under the programmatic
 * scroll guard, then watches the animation and force-lands the alignment if
 * the browser settles misaligned. Returns false when the row isn't rendered
 * yet; a superseding request invalidates in-flight verification. When the
 * requested alignment exceeds the scroll range, the nearest reachable
 * position is accepted.
 */
export function useScrollToMessage(
  scrollRef: React.RefObject<HTMLDivElement | null>,
  runGuardedScroll: (performScroll: () => void) => void,
) {
  // Bumped on every scrollToMessage call; in-flight verifiers of a superseded
  // request bail on the next frame so stale work can never land the
  // transcript on an older prompt after a newer one consumed.
  const generationRef = useRef(0);
  return useCallback(
    (messageId: string, options?: { align?: "start" | "center"; behavior?: "smooth" | "auto" }) => {
      // Advance the generation BEFORE the lookup: a superseding request whose
      // row is not rendered yet still returns false, but must invalidate any
      // in-flight verifier so it can never force-land on a stale prompt.
      const generation = ++generationRef.current;
      const selector = `[id="msg-${CSS.escape(messageId)}"]`;
      const el = scrollRef.current?.querySelector<HTMLElement>(selector);
      if (!el) return false;
      const alignStart = options?.align === "start";
      runGuardedScroll(() => {
        el.scrollIntoView({
          block: alignStart ? "start" : "center",
          behavior: options?.behavior ?? "smooth",
        });
        const container = scrollRef.current;
        if (!container) return;
        const margin = parseFloat(getComputedStyle(el).scrollMarginTop) || 0;
        // A dockview panel re-show (the prompt-history jump activates the
        // chat) makes SessionPanelContent restore its saved scrollTop in a
        // rAF that can cancel the scroll, and some runtimes no-op a smooth
        // scrollIntoView entirely. Watch a bounded frame window: follow an
        // in-progress animation toward the target, and force-land the
        // alignment whenever the container settles misaligned (movement
        // stopped short or never started). Bail immediately if a newer
        // scroll request superseded this one.
        let frames = 0;
        let lastAbsDelta = Infinity;
        /** Frame-watch verifier: follows an in-progress animation toward the
         * target and force-lands the alignment once the container settles
         * misaligned; bails when superseded or the nodes disconnect. */
        const verify = () => {
          frames += 1;
          if (frames > 30 || !container.isConnected || !el.isConnected) return;
          if (generationRef.current !== generation) return; // superseded
          const elementRect = el.getBoundingClientRect();
          const containerRect = container.getBoundingClientRect();
          const delta = alignStart
            ? elementRect.top - containerRect.top - margin
            : elementRect.top +
              elementRect.height / 2 -
              (containerRect.top + containerRect.height / 2) -
              margin / 2;
          const absDelta = Math.abs(delta);
          if (absDelta <= 2) return; // aligned
          if (absDelta < lastAbsDelta) {
            // Animation still moving toward the target — keep watching.
            lastAbsDelta = absDelta;
            requestAnimationFrame(verify);
            return;
          }
          // Settled (or never moved): land the requested alignment. Browsers
          // clamp scrollTop at the scroll range, so accept that boundary as
          // the nearest reachable position instead of retrying forever.
          const hasScrollMetrics = container.scrollHeight > 0 || container.clientHeight > 0;
          const maxScrollTop = Math.max(0, container.scrollHeight - container.clientHeight);
          const desiredScrollTop = container.scrollTop + delta;
          const nextScrollTop = hasScrollMetrics
            ? Math.min(maxScrollTop, Math.max(0, desiredScrollTop))
            : desiredScrollTop;
          if (hasScrollMetrics && nextScrollTop === container.scrollTop) return;
          container.scrollTop = nextScrollTop;
          lastAbsDelta = Infinity;
          requestAnimationFrame(verify);
        };
        requestAnimationFrame(verify);
      });
      return true;
    },
    [runGuardedScroll, scrollRef],
  );
}

/**
 * Applies the initial scrollTop once items are available: bottom when
 * enabled, or the last captured offset for this session when disabled (see
 * `resolveNativeInitialScrollTop`). Skips while a dockview layout-rebuild
 * restore is pending — that separate mechanism owns the position instead.
 *
 * When restoring a concrete saved offset (auto-scroll disabled), the first
 * write can be clamped short: on a dockview remount the container's content is
 * still laying out, so `scrollHeight - clientHeight` momentarily sits below
 * the target and the browser caps the write. The content grows to full height
 * a frame or two later, so the offset is re-applied across a bounded run of
 * animation frames until it lands (or the budget runs out). That re-apply loop
 * is deliberately NOT torn down on unmount: dockview detaches (rather than
 * destroys) the panel's DOM as it finalizes the layout, so the component that
 * seeded the restore unmounts while its scroll element stays on screen — the
 * loop keeps a direct reference to that element and self-terminates once the
 * offset lands, reaches its frame budget, or the element leaves the document.
 * The enabled "scroll to bottom" path needs no retry and settles in a single
 * write.
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
    didInitialScroll.current = true;
    if (scrollTop === null) return;

    /** Derives and stores the near-bottom flag from the applied scrollTop
     * against the container's current dimensions. */
    const syncNearBottom = () => {
      isNearBottomRef.current = !hasTranscriptProgressedPastView({
        scrollTop,
        scrollHeight: el.scrollHeight,
        clientHeight: el.clientHeight,
      });
    };

    el.scrollTop = scrollTop;
    syncNearBottom();

    // Only a concrete disabled-restore offset can be clamped by a not-yet-
    // settled layout; the enabled bottom target lands in one write, and a
    // write that already reached its target needs no follow-up.
    if (enabled || scrollTop <= 0 || el.scrollTop >= scrollTop - 1) return;

    scheduleClampedScrollRestore({
      element: el,
      targetScrollTop: scrollTop,
      onApply: syncNearBottom,
    });
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
  hasUnreadDivider: boolean;
  /** Initial/refetch loading: the sentinel's hard block (never fires, never joins). */
  messagesLoading: boolean;
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
    hasUnreadDivider,
    messagesLoading,
    hasMore,
    isLoadingMore,
    loadMore,
  } = params;
  const programmaticScrollLockRef = useRef(false);
  const isProgrammaticScrollLocked = useCallback(() => programmaticScrollLockRef.current, []);
  const { isNearBottomRef, resyncIsNearBottom, markNotNearBottom } = useAutoScroll({
    scrollRef,
    messages,
    isWorking,
    sessionId,
    enabled,
    hasUnreadDivider,
    isProgrammaticScrollLocked,
  });
  const runGuardedScroll = useProgrammaticScrollGuard(
    scrollRef,
    programmaticScrollLockRef,
    resyncIsNearBottom,
  );
  const handleScrollToMessage = useScrollToMessage(scrollRef, runGuardedScroll);
  useScrollPositionOnPrepend(scrollRef, items, isLoadingMore, isProgrammaticScrollLocked);
  const {
    sentinelRef,
    onUserGesture,
    retry: retryLoadMore,
    showRecovery,
  } = useLazyLoadSentinel({
    scrollRef,
    items,
    sessionId,
    hasMore,
    blocked: messagesLoading,
    isLoadingMore,
    loadMore,
  });
  useRetryPaginationOnUpwardScroll(scrollRef, onUserGesture, isProgrammaticScrollLocked);
  useInitialScrollPosition(scrollRef, items.length, sessionId, enabled, isNearBottomRef);

  return {
    handleScrollToMessage,
    sentinelRef,
    resyncIsNearBottom,
    markNotNearBottom,
    retryLoadMore,
    showRecovery,
  };
}
