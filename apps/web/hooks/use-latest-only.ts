import { useCallback, useRef } from "react";

/**
 * Ordering guard for async work that can overlap (a WS trigger firing
 * again before a prior GET resolves, ordinary network jitter). `begin()`
 * stamps a new generation before starting the work; `isCurrent(token)`
 * reports whether no newer call has started since — even if this call's
 * response arrives after a more recent call's response, so callers can
 * discard it instead of overwriting fresher state with stale data.
 */
export function useLatestOnly() {
  const generationRef = useRef(0);
  const begin = useCallback(() => ++generationRef.current, []);
  const isCurrent = useCallback((token: number) => token === generationRef.current, []);
  return { begin, isCurrent };
}
