import { useCallback, useEffect, useRef } from "react";

export type LazyLoadSentinelOptions = {
  /** IntersectionObserver rootMargin. Defaults to the transcript's
   * "200px 0px 0px 0px" (the sentinel sits at the TOP of a scroll-up list). */
  rootMargin?: string;
  /** After a positive load, explicitly re-observe the still-intersecting
   * sentinel so the next page auto-loads without a scroll-away/scroll-back.
   * Defaults to false (the transcript's no-automatic-re-arm behavior). */
  rearmWhileIntersecting?: boolean;
  /** Called after a positive page commits, before a still-intersecting
   * sentinel starts another page. Return false to stop and require an
   * observed exit/re-entry or a later user-gesture retry. Defaults to true
   * when re-arm is enabled. */
  shouldContinueWhileIntersecting?: () => boolean;
  /** Reports one terminal outcome for each request that started. The
   * continuation value reflects the final firing guards, not only the
   * caller's boundary decision. */
  onLoadSettled?: (result: LazyLoadSentinelSettleResult) => void;
  /** Fire (and join) even while an older-page request is in flight. Never
   * bypasses `blocked`. Defaults to false. */
  joinInFlightWhileLoading?: boolean;
  /** After a positive load, if the user is pinned at the bottom of the scroll
   * container, scroll it back to the new bottom so the re-armed sentinel stays
   * in view and the next page keeps loading without a scroll-away/scroll-back.
   * Appended rows otherwise push the sentinel below the viewport while the
   * user waits at the bottom. Defaults to false (the transcript's scroll-up
   * list never sticks). */
  stickToBottomWhileLoading?: boolean;
};

export type LazyLoadSentinelContinuation =
  | "continued"
  | "caller-stopped"
  | "sentinel-left-preload"
  | "disarmed"
  | "no-more"
  | "blocked"
  | "stale"
  | "not-rearmed"
  | "no-progress"
  | "rejected";

export type LazyLoadSentinelSettleResult = {
  count: number;
  rejected: boolean;
  continuation: LazyLoadSentinelContinuation;
};

/** How close to the scroll container's bottom counts as "pinned". */
const STICK_BOTTOM_TOLERANCE_PX = 24;

/** Mutable state shared by the sentinel state-machine helpers. */
type SentinelMutableRefs = {
  stateRef: React.MutableRefObject<{
    hasMore: boolean;
    blocked: boolean;
    isLoadingMore: boolean;
  }>;
  optionsRef: React.MutableRefObject<{
    rearmWhileIntersecting: boolean;
    joinInFlightWhileLoading: boolean;
    stickToBottomWhileLoading: boolean;
    shouldContinueWhileIntersecting?: () => boolean;
    onLoadSettled?: (result: LazyLoadSentinelSettleResult) => void;
  }>;
  observerRef: React.MutableRefObject<IntersectionObserver | null>;
  sentinelNodeRef: React.MutableRefObject<HTMLDivElement | null>;
  mountedRef: React.MutableRefObject<boolean>;
  disarmedRef: React.MutableRefObject<boolean>;
  intersectingRef: React.MutableRefObject<boolean>;
  fireLoadRef: React.MutableRefObject<(() => void) | null>;
  loadInFlightRef: React.MutableRefObject<boolean>;
  continuationScheduledRef: React.MutableRefObject<boolean>;
  continuationFrameRef: React.MutableRefObject<number | null>;
  pendingContinuationSettleRef: React.MutableRefObject<(() => void) | null>;
};

/** Creates and destroys the IntersectionObserver over the scroll container.
 * The callback updates the intersection/disarm state and fires `fireLoad` when
 * eligible; the sentinel is observed eagerly if it already mounted. */
function useSentinelObserver(opts: {
  scrollRef: React.RefObject<HTMLDivElement | null>;
  rootMargin: string;
  fireLoad: () => void;
  refs: SentinelMutableRefs;
}) {
  const fireLoadRef = useRef(opts.fireLoad);
  fireLoadRef.current = opts.fireLoad;
  const observerRootRef = useRef<Element | Document | null>(null);
  const observerRootMarginRef = useRef<string | null>(null);
  const observerFireLoadRef = useRef<(() => void) | null>(null);

  useEffect(() => {
    const root = opts.scrollRef.current;
    const currentObserver = opts.refs.observerRef.current;
    if (
      currentObserver &&
      observerRootRef.current === root &&
      observerRootMarginRef.current === opts.rootMargin &&
      observerFireLoadRef.current === opts.fireLoad
    ) {
      return;
    }
    currentObserver?.disconnect();
    opts.refs.observerRef.current = null;
    observerRootRef.current = root;
    observerRootMarginRef.current = opts.rootMargin;
    observerFireLoadRef.current = opts.fireLoad;
    if (!root) return;
    const observer = new IntersectionObserver(
      (entries) => {
        const entry = entries[0];
        if (!entry) return;
        opts.refs.intersectingRef.current = entry.isIntersecting;
        if (opts.refs.disarmedRef.current) {
          // Ignore the current intersection; arm only after an observed exit.
          if (!entry.isIntersecting) opts.refs.disarmedRef.current = false;
          return;
        }
        const { hasMore, blocked, isLoadingMore } = opts.refs.stateRef.current;
        const { joinInFlightWhileLoading } = opts.refs.optionsRef.current;
        if (
          !entry.isIntersecting ||
          !hasMore ||
          blocked ||
          (isLoadingMore && !joinInFlightWhileLoading)
        ) {
          return;
        }
        void fireLoadRef.current();
      },
      { root, rootMargin: opts.rootMargin },
    );
    opts.refs.observerRef.current = observer;
    if (opts.refs.sentinelNodeRef.current) {
      observer.observe(opts.refs.sentinelNodeRef.current);
    }
  });

  useEffect(
    () => () => {
      opts.refs.observerRef.current?.disconnect();
      opts.refs.observerRef.current = null;
      observerRootRef.current = null;
      observerRootMarginRef.current = null;
      observerFireLoadRef.current = null;
    },
    [],
  );
}

/** Post-load settle: re-arm after a positive result, disarm after a rejected
 * or zero-result load. Stale completions (unmount, observer cleanup or
 * replacement, sentinel replacement) never re-arm or disarm: the settle is
 * applied only when the observer that STARTED the request is still current. */
function useSentinelSettle(opts: {
  scrollRef: React.RefObject<HTMLDivElement | null>;
  isPinned: () => boolean;
  refs: SentinelMutableRefs;
}) {
  return useCallback(
    (
      node: HTMLDivElement,
      observer: IntersectionObserver,
      outcome: { count: number; rejected: boolean },
      onLoadSettled: ((result: LazyLoadSentinelSettleResult) => void) | undefined,
    ) => {
      const notify = (continuation: LazyLoadSentinelContinuation) => {
        onLoadSettled?.({ ...outcome, continuation });
      };
      if (
        !opts.refs.mountedRef.current ||
        opts.refs.observerRef.current !== observer ||
        node !== opts.refs.sentinelNodeRef.current
      ) {
        notify("stale");
        return;
      }
      if (outcome.count > 0 && !outcome.rejected) {
        // Positive progress: re-arm and re-observe when enabled. A caller can
        // stop the deferred continuation after committed content is visible.
        // A gesture retry that succeeded while disarmed must arm again so the
        // next eligible page can load without another gesture.
        opts.refs.disarmedRef.current = false;
        if (opts.refs.optionsRef.current.rearmWhileIntersecting) {
          opts.refs.observerRef.current.observe(node);
          // Browsers do not consistently emit a new entry when an already
          // intersecting target is observed again after its layout changes.
          // Yield one browser turn so prepend layout effects and the resulting
          // intersection state settle before deciding whether the sentinel is
          // still in the preload region.
          if (!opts.refs.continuationScheduledRef.current) {
            opts.refs.continuationScheduledRef.current = true;
            opts.refs.pendingContinuationSettleRef.current = () => notify("stale");
            opts.refs.continuationFrameRef.current = requestAnimationFrame(() => {
              opts.refs.pendingContinuationSettleRef.current = null;
              opts.refs.continuationFrameRef.current = null;
              opts.refs.continuationScheduledRef.current = false;
              if (
                !opts.refs.mountedRef.current ||
                opts.refs.observerRef.current !== observer ||
                opts.refs.sentinelNodeRef.current !== node
              ) {
                notify("stale");
                return;
              }
              if (!opts.refs.intersectingRef.current) {
                notify("sentinel-left-preload");
                return;
              }
              if (opts.refs.disarmedRef.current) {
                notify("disarmed");
                return;
              }
              const { hasMore, blocked } = opts.refs.stateRef.current;
              if (!hasMore) {
                notify("no-more");
                return;
              }
              if (blocked) {
                notify("blocked");
                return;
              }
              const shouldContinue =
                opts.refs.optionsRef.current.shouldContinueWhileIntersecting?.() ?? true;
              if (!shouldContinue) {
                // The loaded page added a visible boundary. A stale true
                // intersection must not chain another page; wait for fresh
                // upward movement or an observed exit/re-entry.
                opts.refs.disarmedRef.current = true;
                notify("caller-stopped");
                return;
              }
              // The just-completed request owns this re-arm. `loadMore` still
              // applies its own cursor/in-flight guard, so do not reject this
              // hand-off on a stale loading render from the request that has
              // already settled.
              notify("continued");
              opts.refs.fireLoadRef.current?.();
            });
          }
        }
        // Appended rows push the sentinel below the viewport while the user
        // waits at the bottom, so the re-armed observer never fires. Scroll
        // back to the new bottom to keep it in view and the next page loading.
        if (
          opts.refs.optionsRef.current.stickToBottomWhileLoading &&
          opts.isPinned() &&
          opts.scrollRef.current
        ) {
          opts.scrollRef.current.scrollTop = opts.scrollRef.current.scrollHeight;
        }
        if (!opts.refs.optionsRef.current.rearmWhileIntersecting) notify("not-rearmed");
        return;
      }
      // Rejected or zero-result load: re-observe disarmed. The current
      // intersection is ignored; arming waits for an observed exit, and the
      // next true re-entry (or onUserGesture) can retry.
      opts.refs.disarmedRef.current = true;
      if (opts.refs.optionsRef.current.rearmWhileIntersecting) {
        opts.refs.observerRef.current.observe(node);
      }
      notify(outcome.rejected ? "rejected" : "no-progress");
    },
    [opts.isPinned, opts.scrollRef],
  );
}

/** Tracks whether the user is pinned at the scroll container's bottom. The
 * pin is updated by scroll events and on demand via `refreshPinned` (called
 * before each fresh load so late-arriving content or a session switch cannot
 * leave it stale); content growth alone never clears it, so it survives rows
 * being appended beneath the viewport while a load runs. The scroll listener
 * attaches at most once per scroller node, so re-renders do not churn it. */
function useScrollPinnedToBottom(scrollRef: React.RefObject<HTMLDivElement | null>): {
  isPinned: () => boolean;
  refreshPinned: () => void;
} {
  const pinnedRef = useRef(false);
  const attachedScrollerRef = useRef<HTMLDivElement | null>(null);

  const refreshPinned = useCallback(() => {
    const scroller = scrollRef.current;
    pinnedRef.current = Boolean(
      scroller &&
      scroller.scrollTop + scroller.clientHeight >=
        scroller.scrollHeight - STICK_BOTTOM_TOLERANCE_PX,
    );
  }, [scrollRef]);
  const isPinned = useCallback(() => pinnedRef.current, []);

  // Run after every commit because the ref object stays stable while its DOM
  // node can appear or change between panel branches.
  useEffect(() => {
    const previousScroller = attachedScrollerRef.current;
    const scroller = scrollRef.current;
    if (previousScroller === scroller) return;
    previousScroller?.removeEventListener("scroll", refreshPinned);
    attachedScrollerRef.current = scroller;
    if (!scroller) return;
    refreshPinned();
    scroller.addEventListener("scroll", refreshPinned, { passive: true });
  });

  useEffect(
    () => () => {
      attachedScrollerRef.current?.removeEventListener("scroll", refreshPinned);
      attachedScrollerRef.current = null;
    },
    [refreshPinned],
  );

  return { isPinned, refreshPinned };
}

/** Retries an eligible, still-visible sentinel after a firing guard clears. */
function shouldRetrySentinel(
  previous: { hasMore: boolean; blocked: boolean; isLoadingMore: boolean },
  current: { hasMore: boolean; blocked: boolean; isLoadingMore: boolean },
  refs: SentinelMutableRefs,
): boolean {
  const { optionsRef, intersectingRef, disarmedRef } = refs;
  if (!optionsRef.current.rearmWhileIntersecting) return false;
  if (!intersectingRef.current || disarmedRef.current) return false;
  const becameEligible =
    (!previous.hasMore && current.hasMore) ||
    (previous.blocked && !current.blocked) ||
    (previous.isLoadingMore && !current.isLoadingMore);
  if (!becameEligible || !current.hasMore || current.blocked) return false;
  if (current.isLoadingMore && !optionsRef.current.joinInFlightWhileLoading) return false;
  return !refs.loadInFlightRef.current && !refs.continuationScheduledRef.current;
}

function useRetryWhenSentinelBecomesEligible({
  hasMore,
  blocked,
  isLoadingMore,
  fireLoad,
  refs,
}: {
  hasMore: boolean;
  blocked: boolean;
  isLoadingMore: boolean;
  fireLoad: () => void;
  refs: SentinelMutableRefs;
}) {
  const previousStateRef = useRef({ hasMore, blocked, isLoadingMore });

  useEffect(() => {
    const previous = previousStateRef.current;
    const current = { hasMore, blocked, isLoadingMore };
    previousStateRef.current = current;
    if (!shouldRetrySentinel(previous, current, refs)) return;
    void fireLoad();
  }, [hasMore, blocked, isLoadingMore, fireLoad, refs]);
}

/**
 * Observes a sentinel element to trigger older-message lazy loading, shared by
 * the native transcript (top-of-list sentinel, no automatic re-arm) and the
 * prompt-history panel (bottom-of-list sentinel with re-arm and join).
 *
 * Uses a callback ref so the observer reconnects when the sentinel remounts.
 * The callback-ref/stateRef bridging and observer lifecycle match the
 * transcript's original implementation; the panel enables re-arm and join via
 * options.
 *
 * Firing rule: `hasMore && !blocked && (!isLoadingMore || joinInFlightWhileLoading)`.
 * `blocked` is the hard initial/refetch loading flag — it never fires and
 * never joins (a refetch is not a cursor request). With re-arm enabled, the
 * current sentinel is unobserved before awaiting `loadMore()` and re-observed
 * only after a positive result while the hook is still mounted, the observer
 * identity is current, and the sentinel node is still current. After a
 * rejected or zero-result load the node is re-observed disarmed: the current
 * intersection is ignored, arming happens only after an observed exit, and a
 * retry fires on the next true re-entry — or via `onUserGesture` while still
 * intersecting (the panel's wheel/touch path when short content prevents a
 * scroll-away). If an eligible sentinel was observed while blocked or while
 * another request was in flight, an eligibility transition retries it while
 * it remains intersecting; this closes the gap where IntersectionObserver does
 * not emit a second entry after the blocked state changes. Stale completions
 * (unmount, observer cleanup, sentinel replacement) never re-arm.
 */
// eslint-disable-next-line max-params, max-lines-per-function -- plan-mandated sentinel state machine; eligibility retry and deferred re-arm stay coordinated with the extracted observer/settle/pin helpers
export function useLazyLoadSentinel(
  scrollRef: React.RefObject<HTMLDivElement | null>,
  hasMore: boolean,
  blocked: boolean,
  isLoadingMore: boolean,
  loadMore: () => Promise<number>,
  options?: LazyLoadSentinelOptions,
): {
  sentinelRef: (node: HTMLDivElement | null) => void;
  onUserGesture: () => void;
} {
  const {
    rootMargin = "200px 0px 0px 0px",
    rearmWhileIntersecting = false,
    joinInFlightWhileLoading = false,
    stickToBottomWhileLoading = false,
    shouldContinueWhileIntersecting,
    onLoadSettled,
  } = options ?? {};

  const stateRef = useRef({ hasMore, blocked, isLoadingMore });
  useEffect(() => {
    stateRef.current = { hasMore, blocked, isLoadingMore };
  }, [hasMore, blocked, isLoadingMore]);
  const optionsRef = useRef({
    rearmWhileIntersecting,
    joinInFlightWhileLoading,
    stickToBottomWhileLoading,
    shouldContinueWhileIntersecting,
    onLoadSettled,
  });
  useEffect(() => {
    optionsRef.current = {
      rearmWhileIntersecting,
      joinInFlightWhileLoading,
      stickToBottomWhileLoading,
      shouldContinueWhileIntersecting,
      onLoadSettled,
    };
  }, [
    rearmWhileIntersecting,
    joinInFlightWhileLoading,
    stickToBottomWhileLoading,
    shouldContinueWhileIntersecting,
    onLoadSettled,
  ]);

  const observerRef = useRef<IntersectionObserver | null>(null);
  const sentinelNodeRef = useRef<HTMLDivElement | null>(null);
  const mountedRef = useRef(true);
  const fireLoadRef = useRef<(() => void) | null>(null);
  const loadInFlightRef = useRef(false);
  const continuationScheduledRef = useRef(false);
  const continuationFrameRef = useRef<number | null>(null);
  const pendingContinuationSettleRef = useRef<(() => void) | null>(null);
  /** When true, ignore intersections until an observed exit arms the hook. */
  const disarmedRef = useRef(false);
  const intersectingRef = useRef(false);
  const { isPinned, refreshPinned } = useScrollPinnedToBottom(scrollRef);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      if (continuationFrameRef.current !== null) {
        cancelAnimationFrame(continuationFrameRef.current);
        continuationFrameRef.current = null;
      }
      pendingContinuationSettleRef.current?.();
      pendingContinuationSettleRef.current = null;
      continuationScheduledRef.current = false;
    };
  }, []);

  const refs: SentinelMutableRefs = {
    stateRef,
    optionsRef,
    observerRef,
    sentinelNodeRef,
    mountedRef,
    disarmedRef,
    intersectingRef,
    fireLoadRef,
    loadInFlightRef,
    continuationScheduledRef,
    continuationFrameRef,
    pendingContinuationSettleRef,
  };
  const settleLoad = useSentinelSettle({ scrollRef, isPinned, refs });

  const fireLoad = useCallback(async () => {
    const node = sentinelNodeRef.current;
    const observer = observerRef.current;
    if (!node || !observer || loadInFlightRef.current || continuationScheduledRef.current) {
      return;
    }
    loadInFlightRef.current = true;
    // Refresh the pin from the CURRENT geometry before the request: the pin
    // otherwise reflects only the initial mount or the last user scroll, which
    // can be stale after a session switch or late-arriving content. Preserving
    // it during the in-flight load keeps rows appended beneath the viewport
    // from clearing it.
    refreshPinned();
    if (optionsRef.current.rearmWhileIntersecting) {
      // Unobserve the current sentinel before awaiting so a still-intersecting
      // node cannot re-fire mid-load.
      observer.unobserve(node);
    }
    let count = 0;
    let rejected = false;
    const onLoadSettled = optionsRef.current.onLoadSettled;
    try {
      count = await loadMore();
    } catch {
      rejected = true;
    } finally {
      settleLoad(node, observer, { count, rejected }, onLoadSettled);
      loadInFlightRef.current = false;
    }
  }, [loadMore, refreshPinned, settleLoad]);
  fireLoadRef.current = fireLoad;

  useSentinelObserver({ scrollRef, rootMargin, fireLoad, refs });
  useRetryWhenSentinelBecomesEligible({
    hasMore,
    blocked,
    isLoadingMore,
    fireLoad,
    refs,
  });

  // Callback ref — stores the node and observes if the observer already
  // exists. Only the sentinel is ever observed, so a remount swaps the
  // observation with unobserve+observe rather than disconnect(), which would
  // silently drop any other target the observer might hold.
  const sentinelRef = useCallback((node: HTMLDivElement | null) => {
    const previous = sentinelNodeRef.current;
    sentinelNodeRef.current = node;
    const observer = observerRef.current;
    if (!observer) return;
    if (previous && previous !== node) {
      observer.unobserve(previous);
    }
    if (node) {
      observer.observe(node);
    }
  }, []);

  // Panel wheel/touch retry path: when short content prevents a scroll-away
  // (the sentinel cannot exit), a user gesture retries while disarmed,
  // intersecting, and eligible. Never retries the same failure automatically.
  const onUserGesture = useCallback(() => {
    if (!disarmedRef.current || !intersectingRef.current) return;
    const { hasMore, blocked, isLoadingMore } = stateRef.current;
    const { joinInFlightWhileLoading } = optionsRef.current;
    if (!hasMore || blocked || (isLoadingMore && !joinInFlightWhileLoading)) return;
    void fireLoad();
  }, [fireLoad]);

  return { sentinelRef, onUserGesture };
}
