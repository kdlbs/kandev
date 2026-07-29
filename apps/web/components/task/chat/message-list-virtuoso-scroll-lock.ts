import { useCallback, useEffect, useRef } from "react";
import { resolveFollowOutput } from "./transcript-auto-scroll";

export const FOLLOW_SMOOTH = "smooth" as const;

/** Duration a programmatic scroll's lock stays held if the browser never
 * reports `scrollend` on the scroll parent — bounded so a missed event can't
 * wedge follow-bottom off indefinitely. */
const PROGRAMMATIC_SCROLL_LOCK_MS = 1000;

/** Guards a user-initiated `scrollToIndex` (scroll-to-start / scroll-to-last-
 * prompt) against Virtuoso's own `followOutput` follow-bottom behavior firing
 * mid-flight — e.g. the agent streams a new message while the scroll is
 * still animating, which would otherwise snap the transcript back to the
 * bottom and silently cancel the user's action. Held until the scroll
 * settles (native `scrollend` on the real scroll-parent element where
 * supported, a bounded timeout fallback otherwise); Virtuoso re-evaluates
 * `followOutput` on its own subsequent range/bottom updates once released,
 * so no manual resync is needed here (unlike the native renderer's
 * DOM-scroll-listener-driven near-bottom tracking). */
export function useProgrammaticScrollLock(scrollParent: HTMLDivElement) {
  const lockedRef = useRef(false);
  const cleanupRef = useRef<(() => void) | null>(null);

  const release = useCallback(() => {
    cleanupRef.current?.();
    cleanupRef.current = null;
    lockedRef.current = false;
  }, []);

  const runLocked = useCallback(
    (performScroll: () => void) => {
      cleanupRef.current?.();
      lockedRef.current = true;
      performScroll();

      const timeoutId = window.setTimeout(release, PROGRAMMATIC_SCROLL_LOCK_MS);
      scrollParent.addEventListener("scrollend", release, { once: true });
      cleanupRef.current = () => {
        window.clearTimeout(timeoutId);
        scrollParent.removeEventListener("scrollend", release);
      };
    },
    [scrollParent, release],
  );

  useEffect(() => () => cleanupRef.current?.(), []);

  return { isLocked: useCallback(() => lockedRef.current, []), runLocked };
}

/** Wraps Virtuoso's `followOutput` so it never re-engages while a
 * programmatic scroll (see `useProgrammaticScrollLock`) is in flight, and
 * respects the transcript auto-scroll toggle the same way the native
 * renderer's `shouldAutoScrollToBottom` does. */
export function useGuardedFollowOutput(isLocked: () => boolean, enabled: boolean) {
  return useCallback(
    (isAtBottom: boolean) => (isLocked() ? false : resolveFollowOutput(enabled, isAtBottom)),
    [isLocked, enabled],
  );
}
