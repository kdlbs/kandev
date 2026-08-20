import { useCallback, useEffect, useRef } from "react";

export type LazyLoadSentinelOptions = {
  /** IntersectionObserver rootMargin. Defaults to the transcript's
   * "200px 0px 0px 0px" (the sentinel sits at the TOP of a scroll-up list). */
  rootMargin?: string;
  /** After a positive load, explicitly re-observe the still-intersecting
   * sentinel so the next page auto-loads without a scroll-away/scroll-back.
   * Defaults to false (the transcript's no-automatic-re-arm behavior). */
  rearmWhileIntersecting?: boolean;
  /** Fire (and join) even while an older-page request is in flight. Never
   * bypasses `blocked`. Defaults to false. */
  joinInFlightWhileLoading?: boolean;
};

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
 * scroll-away). Stale completions (unmount, observer cleanup, sentinel
 * replacement) never re-arm.
 */
// eslint-disable-next-line max-params, max-lines-per-function -- plan-mandated unified sentinel state machine (scrollRef, hasMore, blocked, isLoadingMore, loadMore, options); splitting would fragment the re-arm/disarm/join invariants
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
  } = options ?? {};

  const stateRef = useRef({ hasMore, blocked, isLoadingMore });
  useEffect(() => {
    stateRef.current = { hasMore, blocked, isLoadingMore };
  }, [hasMore, blocked, isLoadingMore]);
  const optionsRef = useRef({ rearmWhileIntersecting, joinInFlightWhileLoading });
  useEffect(() => {
    optionsRef.current = { rearmWhileIntersecting, joinInFlightWhileLoading };
  }, [rearmWhileIntersecting, joinInFlightWhileLoading]);

  const observerRef = useRef<IntersectionObserver | null>(null);
  const sentinelNodeRef = useRef<HTMLDivElement | null>(null);
  const mountedRef = useRef(true);
  /** When true, ignore intersections until an observed exit arms the hook. */
  const disarmedRef = useRef(false);
  const intersectingRef = useRef(false);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  /** Post-load settle: re-arm after a positive result, disarm after a rejected
   * or zero-result load. Stale completions (unmount, observer cleanup or
   * replacement, sentinel replacement) never re-arm or disarm: the settle is
   * applied only when the observer that STARTED the request is still current. */
  const settleLoad = useCallback(
    (
      node: HTMLDivElement,
      observer: IntersectionObserver,
      outcome: { count: number; rejected: boolean },
    ) => {
      if (!mountedRef.current || observerRef.current !== observer) return;
      if (node !== sentinelNodeRef.current) return;
      if (outcome.count > 0 && !outcome.rejected) {
        // Positive progress: re-arm and re-observe (re-arm only when enabled;
        // the transcript leaves observation untouched). A gesture retry that
        // succeeded while disarmed must arm again so the next page can load
        // without another gesture.
        disarmedRef.current = false;
        if (optionsRef.current.rearmWhileIntersecting) {
          observerRef.current.observe(node);
        }
        return;
      }
      // Rejected or zero-result load: re-observe disarmed. The current
      // intersection is ignored; arming waits for an observed exit, and the
      // next true re-entry (or onUserGesture) can retry.
      disarmedRef.current = true;
      if (optionsRef.current.rearmWhileIntersecting) {
        observerRef.current.observe(node);
      }
    },
    [],
  );

  const fireLoad = useCallback(async () => {
    const node = sentinelNodeRef.current;
    const observer = observerRef.current;
    if (!node || !observer) return;
    if (optionsRef.current.rearmWhileIntersecting) {
      // Unobserve the current sentinel before awaiting so a still-intersecting
      // node cannot re-fire mid-load.
      observer.unobserve(node);
    }
    let count = 0;
    let rejected = false;
    try {
      count = await loadMore();
    } catch {
      rejected = true;
    }
    settleLoad(node, observer, { count, rejected });
  }, [loadMore, settleLoad]);

  // Create/destroy observer when the scroll container changes
  useEffect(() => {
    const root = scrollRef.current;
    if (!root) return;
    const observer = new IntersectionObserver(
      (entries) => {
        const entry = entries[0];
        if (!entry) return;
        intersectingRef.current = entry.isIntersecting;
        if (disarmedRef.current) {
          // Ignore the current intersection; arm only after an observed exit.
          if (!entry.isIntersecting) disarmedRef.current = false;
          return;
        }
        const { hasMore, blocked, isLoadingMore } = stateRef.current;
        const { joinInFlightWhileLoading } = optionsRef.current;
        if (
          !entry.isIntersecting ||
          !hasMore ||
          blocked ||
          (isLoadingMore && !joinInFlightWhileLoading)
        ) {
          return;
        }
        void fireLoad();
      },
      { root, rootMargin },
    );
    observerRef.current = observer;
    // If the sentinel already mounted before this effect ran, observe it now
    if (sentinelNodeRef.current) {
      observer.observe(sentinelNodeRef.current);
    }
    return () => {
      observer.disconnect();
      observerRef.current = null;
    };
  }, [scrollRef, loadMore, rootMargin, fireLoad]);

  // Callback ref — stores the node and observes if the observer already exists
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
