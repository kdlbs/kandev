import { useEffect, useRef } from "react";

/**
 * Pure decision logic for the native transcript auto-scroll toggle, plus small
 * visibility lifecycle primitives. The renderer owns all DOM calls and just
 * consults these functions or refs.
 */

/**
 * Distinguishes a genuine prepend (older messages loaded above the current
 * view, e.g. via lazy-loading) from a plain append (a new message arriving
 * at the end) when the rendered item count grows. A one-for-one replacement
 * of the synthetic task-description row with the stored user prompt is also
 * a prepend layout update even though its item count stays equal.
 * `useScrollPositionOnPrepend` only needs to compensate scrollTop for the
 * former — compensating an append
 * too would drag a scrolled-away user's view down by the height of every new
 * message, fighting the auto-scroll toggle's "stay put while disabled"
 * contract (and, independently of that toggle, jittering anyone who has
 * scrolled up to read older messages while new ones arrive).
 */
export function isPrependUpdate(params: {
  prevItemCount: number;
  nextItemCount: number;
  prevFirstKey: string | null;
  nextFirstKey: string | null;
}): boolean {
  if (params.nextItemCount < params.prevItemCount) return false;
  if (params.prevFirstKey === null) return false;
  if (params.nextFirstKey === null) return false;
  return params.nextFirstKey !== params.prevFirstKey;
}

// Matches the settle tolerance used elsewhere for "has this scrolled past
// view" checks (see the last-prompt scroll-nav feature) — sub-pixel jitter
// from layout rounding shouldn't count as "progressed".
const SETTLE_TOLERANCE_PX = 2;

/**
 * Whether the transcript's content bottom currently sits meaningfully below
 * the visible viewport bottom, i.e. there is unseen content past the current
 * scroll position.
 */
export function hasTranscriptProgressedPastView(params: {
  scrollTop: number;
  scrollHeight: number;
  clientHeight: number;
}): boolean {
  return params.scrollHeight - params.scrollTop - params.clientHeight > SETTLE_TOLERANCE_PX;
}

/**
 * Whether the transcript actually appended new content since a baseline
 * captured at disable time — i.e. a genuinely new message row, or the same
 * trailing message streaming in more content. Derived purely from message
 * identity and `updated_at` (never scrollTop/scrollHeight), so it can't be
 * fooled by:
 *  - the user scrolling further away themselves while disabled (no message
 *    data changed, so this stays false), or
 *  - a prepend (older messages loaded above): count grows but the LAST
 *    message's id and `updated_at` are untouched, so this stays false.
 *
 * `updated_at` is the backend's documented "authoritative per-message change
 * signal; advances on every content/metadata update" (see Message in
 * lib/types/http.ts) — it catches in-place streaming growth of the last row
 * even when no new row was appended.
 */
export function hasTranscriptAppendedSinceBaseline(params: {
  baselineCount: number;
  currentCount: number;
  baselineLastId: string | null;
  currentLastId: string | null;
  baselineLastUpdatedAt: string | undefined;
  currentLastUpdatedAt: string | undefined;
}): boolean {
  if (params.currentLastId === null) return false;
  const newRowAppended =
    params.currentCount > params.baselineCount && params.currentLastId !== params.baselineLastId;
  const sameRowChanged =
    params.currentLastId === params.baselineLastId &&
    params.currentLastUpdatedAt !== undefined &&
    params.currentLastUpdatedAt !== params.baselineLastUpdatedAt;
  return newRowAppended || sameRowChanged;
}

/**
 * Whether re-enabling auto-scroll (the user flips the toggle back on) should
 * immediately catch the view up to the bottom. Only fires on the disabled ->
 * enabled transition, and only when the transcript genuinely appended new
 * content while disabled (see `hasTranscriptAppendedSinceBaseline`) AND that
 * content isn't already in view. Resuming auto-follow with no new content —
 * whether the user simply never scrolled back down, or scrolled further away
 * themselves while disabled — never forces a scroll here.
 */
export function shouldCatchUpOnAutoScrollEnable(params: {
  wasEnabled: boolean;
  nowEnabled: boolean;
  appendedSinceDisable: boolean;
  isAtBottom: boolean;
}): boolean {
  if (params.wasEnabled || !params.nowEnabled) return false;
  return params.appendedSinceDisable && !params.isAtBottom;
}

/**
 * Resolves the scrollTop the native list should apply right after mount
 * (session opened, or the panel remounted via a dockview layout rebuild).
 *
 * - A pending dockview layout-rebuild restore always wins; that separate
 *   mechanism owns the position for maximize/un-maximize and runs
 *   independently of this feature.
 * - When auto-scroll is disabled, restore whatever offset was captured the
 *   last time this session's transcript was visible — falling back to the
 *   bottom only if nothing was ever captured (first-time disable, or a
 *   session that has never been scrolled).
 * - When enabled, the pre-existing behavior (scroll to bottom) applies.
 *
 * Returns `null` when the caller should skip applying any scrollTop.
 */
export function resolveNativeInitialScrollTop(params: {
  enabled: boolean;
  hasPendingLayoutRestore: boolean;
  savedScrollTop: number | undefined;
  scrollHeight: number;
}): number | null {
  if (params.hasPendingLayoutRestore) return null;
  if (!params.enabled) return params.savedScrollTop ?? params.scrollHeight;
  return params.scrollHeight;
}

/**
 * Coalesces frequent `schedule()` calls into at most one `run` execution per
 * animation frame — e.g. persisting scroll position on every native `scroll`
 * event is far more expensive (sync storage write + store update) than the
 * events actually need. `flush` cancels any pending frame and runs `run`
 * once immediately, guaranteeing a final call on cleanup even mid-frame.
 */
export function createFrameCoalescer(run: () => void): {
  schedule: () => void;
  flush: () => void;
} {
  let frameId: number | null = null;
  return {
    schedule: () => {
      if (frameId !== null) return;
      frameId = requestAnimationFrame(() => {
        frameId = null;
        run();
      });
    },
    flush: () => {
      if (frameId !== null) {
        cancelAnimationFrame(frameId);
        frameId = null;
      }
      run();
    },
  };
}

/**
 * Tracks a hidden-to-visible transition without consuming it until the caller
 * completes its activation work. This keeps every activation owner aligned on
 * the same visibility and pending-transition contract.
 */
export function useActivationPending(isVisible: boolean) {
  const isVisibleRef = useRef(isVisible);
  const previousVisibleRef = useRef(isVisible);
  const activationPendingRef = useRef(false);
  isVisibleRef.current = isVisible;

  useEffect(() => {
    const becameVisible = !previousVisibleRef.current && isVisible;
    previousVisibleRef.current = isVisible;
    activationPendingRef.current = isVisible
      ? becameVisible || activationPendingRef.current
      : false;
  }, [isVisible]);

  return { isVisibleRef, activationPendingRef };
}

/**
 * Schedules work after the generic Dockview panel restore's current one-frame
 * write. The generic restore also guards stale offsets, so this is a bounded
 * coordination window rather than the sole correctness mechanism.
 */
export function scheduleAfterPanelRestore(run: () => void): () => void {
  let cancelled = false;
  let secondFrame: number | null = null;
  const firstFrame = requestAnimationFrame(() => {
    if (cancelled) return;
    secondFrame = requestAnimationFrame(() => {
      if (!cancelled) run();
    });
  });
  return () => {
    cancelled = true;
    cancelAnimationFrame(firstFrame);
    if (secondFrame !== null) cancelAnimationFrame(secondFrame);
  };
}
