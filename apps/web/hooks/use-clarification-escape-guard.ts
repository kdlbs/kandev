"use client";

import { createContext, useContext, useEffect } from "react";

type ClarificationEscapeGuardSetter = (guarded: boolean) => void;

const ClarificationEscapeGuardContext = createContext<ClarificationEscapeGuardSetter | null>(null);

export const ClarificationEscapeGuardProvider = ClarificationEscapeGuardContext.Provider;

/**
 * Tells an ancestor dialog (via ClarificationEscapeGuardProvider) to swallow
 * its Escape-closes-dialog default while `guarded` is true, so the first
 * Escape collapses this panel instead of dismissing the surrounding modal.
 * No-ops outside a provider, so callers like the non-modal task chat panel
 * can invoke this unconditionally.
 */
export function useClarificationEscapeGuard(guarded: boolean) {
  const setGuarded = useContext(ClarificationEscapeGuardContext);
  useEffect(() => {
    if (!setGuarded) return;
    setGuarded(guarded);
    return () => setGuarded(false);
  }, [guarded, setGuarded]);
}
