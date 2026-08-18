"use client";

import { createContext, useContext, useEffect, useId } from "react";

export type ClarificationEscapePredicate = (event: KeyboardEvent) => boolean;

// Test-only holder shape. The production registry stores bare predicates; the
// wrapper lets tests retain a predicate without React treating it as an
// updater function.
export type ClarificationEscapeGuardEntry = { test: ClarificationEscapePredicate } | null;

/**
 * Multiple widgets can be mounted inside the same dialog at once (e.g. a
 * pending clarification carousel and an open suggestion popup, or a pending
 * clarification and an open reverse-search overlay), so the registry holds
 * one predicate per caller -- keyed by a stable per-hook-instance id -- rather
 * than a single slot. A single-slot design would let a second caller's
 * register/unregister silently evict the first's still-active predicate.
 */
export type ClarificationEscapeGuardRegistry = {
  register: (id: string, predicate: ClarificationEscapePredicate) => void;
  unregister: (id: string) => void;
};

const ClarificationEscapeGuardContext = createContext<ClarificationEscapeGuardRegistry | null>(
  null,
);

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
  const registry = useContext(ClarificationEscapeGuardContext);
  const id = useId();
  useEffect(() => {
    if (!registry || !predicate) return;
    registry.register(id, predicate);
    return () => registry.unregister(id);
  }, [registry, id, predicate]);
}
