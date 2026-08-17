"use client";

import { createContext, useContext, useEffect } from "react";

export type ClarificationEscapePredicate = (event: KeyboardEvent) => boolean;

// Wrapped in an object so storing a function via useState's setter is never
// mistaken for React's "updater function" overload (setState(fn) calls fn
// instead of storing it).
export type ClarificationEscapeGuardEntry = { test: ClarificationEscapePredicate } | null;

type ClarificationEscapeGuardSetter = (entry: ClarificationEscapeGuardEntry) => void;

const ClarificationEscapeGuardContext = createContext<ClarificationEscapeGuardSetter | null>(null);

export const ClarificationEscapeGuardProvider = ClarificationEscapeGuardContext.Provider;

/**
 * Tells an ancestor dialog (via ClarificationEscapeGuardProvider) to swallow
 * its Escape-closes-dialog default and let this widget's own Escape handler
 * run instead -- but only while `predicate(event)` reports that this exact
 * keydown is one the widget will actually act on. Radix's DismissableLayer
 * intercepts Escape on `document` in the capture phase, before this widget's
 * own bubble-phase `window` listener runs, so the dialog cannot wait to see
 * whether the widget handles the key -- it must ask a predicate that mirrors
 * the widget's own enabled/scope/modifier checks exactly, rather than derive
 * a separate approximation. Otherwise Escape can be swallowed with nothing
 * left to handle it.
 * No-ops outside a provider, so callers like the non-modal task chat panel
 * can invoke this unconditionally.
 */
export function useClarificationEscapeGuard(predicate: ClarificationEscapePredicate | null) {
  const setEntry = useContext(ClarificationEscapeGuardContext);
  useEffect(() => {
    if (!setEntry) return;
    setEntry(predicate ? { test: predicate } : null);
    return () => setEntry(null);
  }, [predicate, setEntry]);
}
