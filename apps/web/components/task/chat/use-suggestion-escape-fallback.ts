"use client";

import { useCallback, useEffect, type RefObject } from "react";
import { useClarificationEscapeGuard } from "@/hooks/use-clarification-escape-guard";

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
  /** This composer instance's own editor wrapper element. Escape only closes
   *  THIS composer's popups when the keydown's target or the currently
   *  focused element is inside it -- see the ownership note below. */
  containerRef: RefObject<HTMLElement | null>;
};

/** True while `node` (the keydown's target, or document.activeElement) sits
 *  inside `container`. Keyboard events target the focused element, so this
 *  is the same check whichever of the two callers pass. */
function isOwnedByContainer(container: HTMLElement | null, node: Node | null): boolean {
  return !!container && !!node && container.contains(node);
}

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
 * prevent that: registering an ownership-aware predicate while a suggestion
 * popup is open tells the dialog this Escape is spoken for -- but only when
 * the keydown actually targets (or focus sits inside) this composer's own
 * `containerRef`, mirroring the bubble-phase listener's own ownership check
 * below. Without that check, a menu left open in a backgrounded composer (see
 * OWNERSHIP below) would claim every Escape regardless of where focus
 * actually is, and if focus later leaves this composer entirely (e.g. Tab
 * navigates out while an empty-results popup is still open, which does not
 * consume Tab) the claim would stay unconditionally true forever -- the dialog
 * would never dismiss AND the bubble listener would correctly refuse to act
 * (wrong owner), leaving Escape completely dead. No-ops on the main task chat
 * panel, where there is no ClarificationEscapeGuardProvider.
 *
 * With the dialog's own auto-dismiss suppressed, a plain document-BUBBLE
 * listener still runs after the target regardless of `defaultPrevented` (that
 * flag only suppresses default browser behavior, not propagation), so it still
 * sees the keydown. It only acts while a suggestion popup is open, closes it,
 * then calls stopPropagation() so the window-level carousel listener never
 * sees the same keydown either -- mirroring what the Suggestion plugins' own
 * onKeyDown does when it runs normally.
 *
 * OWNERSHIP: neither this document-level bubble listener nor the capture-phase
 * guard predicate above is scoped by React tree -- both see every Escape
 * keydown from *any* mounted composer instance (the guard predicate, via
 * whatever the registry's Escape handler is called with; this listener, via
 * `document`). Quick Chat is a modal layered over a still-mounted task page,
 * so both composers can be alive at once, and `popup-menu.tsx` closes a menu
 * only on outside `pointerdown` (no blur handler), so a menu left open in a
 * backgrounded composer stays open indefinitely. Without an ownership check on
 * both, whichever instance happens to run first "wins" the event regardless of
 * which composer the user is actually typing into -- closing the wrong
 * (background) menu, or claiming an Escape it has no way to act on.
 * `containerRef` is this composer's own editor wrapper; both the predicate and
 * the listener bail unless the keydown's target or the currently focused
 * element sits inside it, so only the composer that actually owns the
 * keystroke reacts to it.
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
  containerRef,
}: SuggestionEscapeFallbackArgs) {
  const isOwnEscape = useCallback(
    (event: KeyboardEvent) => {
      const container = containerRef.current;
      return (
        isOwnedByContainer(container, event.target as Node | null) ||
        isOwnedByContainer(container, document.activeElement)
      );
    },
    [containerRef],
  );
  useClarificationEscapeGuard(isSuggestionMenuOpen ? isOwnEscape : null);
  useEffect(() => {
    if (!isSuggestionMenuOpen) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      const container = containerRef.current;
      const owned =
        isOwnedByContainer(container, event.target as Node | null) ||
        isOwnedByContainer(container, document.activeElement);
      if (!owned) return;
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
    containerRef,
  ]);
}
