"use client";

import { useEffect } from "react";
import {
  useClarificationEscapeGuard,
  type ClarificationEscapePredicate,
} from "@/hooks/use-clarification-escape-guard";

// Stable reference (module scope, not re-created per render/call) so the
// guard registry sees the same predicate identity across renders while a
// suggestion menu stays open, instead of re-registering on every render.
const CLAIM_ANY_ESCAPE: ClarificationEscapePredicate = () => true;

export type SuggestionEscapeFallbackArgs = {
  /** True while any ProseMirror suggestion popup (mention, slash, entity
   *  reference) is open for this composer instance. */
  isSuggestionMenuOpen: boolean;
  mentionMenuOpen: boolean;
  slashMenuOpen: boolean;
  entityReferenceMenuOpen: boolean;
  closeMentionMenu: () => void;
  closeSlashMenu: () => void;
  closeEntityReferenceMenu: () => void;
};

/**
 * On Quick Chat, Radix's Dialog calls `event.preventDefault()` on Escape from
 * a document-CAPTURE listener before the keydown ever reaches the editor.
 * prosemirror-view's own keydown handling refuses any event whose
 * `defaultPrevented` is already true, so the Suggestion plugins' Escape
 * handling -- which would otherwise close an open mention/slash/entity-
 * reference popup and call stopPropagation() -- never runs at all on that
 * surface, and the popup stays open while Quick Chat's clarification-collapse
 * listener (bound on `window`) still sees the event.
 *
 * Worse, unless something already called `preventDefault()` during that same
 * capture phase, Radix's DismissableLayer treats the Escape as unclaimed and
 * dismisses the whole dialog right then -- before this hook's own bubble-phase
 * listener (below) ever gets a turn. `useClarificationEscapeGuard` is the only
 * hook point that runs early enough (Quick Chat's `onEscapeKeyDown` prop,
 * invoked synchronously from within DismissableLayer's own capture handler) to
 * prevent that: registering `CLAIM_ANY_ESCAPE` while a suggestion popup is open
 * tells the dialog this Escape is spoken for, so it stays open instead of
 * closing out from under the popup. No-ops on the main task chat panel, where
 * there is no ClarificationEscapeGuardProvider.
 *
 * With the dialog's own auto-dismiss suppressed, a plain document-BUBBLE
 * listener still runs after the target regardless of `defaultPrevented` (that
 * flag only suppresses default browser behavior, not propagation), so it still
 * sees the keydown. It only acts while a suggestion popup is open, closes it,
 * then calls stopPropagation() so the window-level carousel listener never
 * sees the same keydown either -- mirroring what the Suggestion plugins' own
 * onKeyDown does when it runs normally.
 *
 * On the main task chat panel there is no competing capture listener, so the
 * Suggestion plugin's own bubble-phase stopPropagation() halts the event at
 * the editor node first and this listener never fires -- verified in
 * use-suggestion-escape-fallback.test.ts.
 */
export function useSuggestionEscapeFallback({
  isSuggestionMenuOpen,
  mentionMenuOpen,
  slashMenuOpen,
  entityReferenceMenuOpen,
  closeMentionMenu,
  closeSlashMenu,
  closeEntityReferenceMenu,
}: SuggestionEscapeFallbackArgs) {
  useClarificationEscapeGuard(isSuggestionMenuOpen ? CLAIM_ANY_ESCAPE : null);
  useEffect(() => {
    if (!isSuggestionMenuOpen) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      if (mentionMenuOpen) closeMentionMenu();
      if (slashMenuOpen) closeSlashMenu();
      if (entityReferenceMenuOpen) closeEntityReferenceMenu();
      event.preventDefault();
      event.stopPropagation();
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [
    isSuggestionMenuOpen,
    mentionMenuOpen,
    slashMenuOpen,
    entityReferenceMenuOpen,
    closeMentionMenu,
    closeSlashMenu,
    closeEntityReferenceMenu,
  ]);
}
