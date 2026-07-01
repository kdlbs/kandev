import * as React from "react";

/**
 * Enter-to-confirm for dialogs.
 *
 * Pressing Enter inside a dialog should execute its semantically focused
 * action — the destructive/primary button — regardless of which control
 * currently holds DOM focus (Radix, for example, focuses the Cancel button of
 * an AlertDialog by default). This module resolves that button from the dialog
 * content and exposes a keydown handler the base Dialog/AlertDialog content
 * components attach.
 */

function isActionable(el: HTMLElement | null): el is HTMLElement {
  if (!el) return false;
  if (el.hasAttribute("disabled")) return false;
  if (el.getAttribute("aria-disabled") === "true") return false;
  if (el.getAttribute("data-disabled") !== null) return false;
  // Hidden elements (e.g. inside a collapsed section) have no layout box.
  if (el.offsetParent === null && el.getAttribute("aria-hidden") === "true") return false;
  return true;
}

/**
 * Resolve the button that Enter should activate inside a dialog, or null when
 * there is no unambiguous semantic action.
 *
 * Resolution order:
 *  1. `[data-slot="alert-dialog-action"]` — the AlertDialog primary action.
 *  2. `[data-dialog-default-action]` — an explicit opt-in marker a generic
 *     Dialog can place on its primary button (useful when a footer has several
 *     action buttons).
 *  3. A `type="submit"` button inside the dialog footer.
 *  4. The single non-cancel action button in the dialog footer, identified by
 *     its `data-variant` (`default`/`destructive`). If the footer has zero or
 *     more than one such candidate we return null and do nothing, rather than
 *     guess and fire the wrong (possibly destructive) action.
 */
export function resolveDialogDefaultAction(content: HTMLElement): HTMLElement | null {
  const alertAction = content.querySelector<HTMLElement>('[data-slot="alert-dialog-action"]');
  if (alertAction) return isActionable(alertAction) ? alertAction : null;

  const explicit = content.querySelector<HTMLElement>("[data-dialog-default-action]");
  if (explicit) return isActionable(explicit) ? explicit : null;

  const footer = content.querySelector<HTMLElement>('[data-slot="dialog-footer"]');
  if (!footer) return null;

  const buttons = Array.from(footer.querySelectorAll<HTMLElement>("button")).filter(isActionable);

  const submit = buttons.find((b) => b.getAttribute("type") === "submit");
  if (submit) return submit;

  const primaries = buttons.filter((b) => {
    const variant = b.getAttribute("data-variant");
    return variant === "default" || variant === "destructive";
  });
  return primaries.length === 1 ? primaries[0] : null;
}

const TEXT_ENTRY_TAGS = new Set(["TEXTAREA"]);

/** Enter in a multi-line text field means "newline", not "confirm". */
function isTextEntry(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false;
  if (TEXT_ENTRY_TAGS.has(el.tagName)) return true;
  if (el.isContentEditable) return true;
  return false;
}

/**
 * Keydown handler for dialog content. Attach to the Radix `*Content` element so
 * `event.currentTarget` is the dialog content root. On a plain Enter it
 * activates the resolved semantic action; everything else falls through
 * untouched.
 */
export function handleDialogDefaultActionKeyDown(event: React.KeyboardEvent<HTMLElement>): void {
  if (event.key !== "Enter") return;
  if (event.shiftKey || event.metaKey || event.ctrlKey || event.altKey) return;
  if (event.defaultPrevented) return;
  if (event.nativeEvent.isComposing) return; // mid-IME composition
  if (isTextEntry(event.target)) return;

  const action = resolveDialogDefaultAction(event.currentTarget);
  if (!action) return;

  event.preventDefault();
  action.click();
}
