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
 *
 * Note: a plain single-line `<input>` is intentionally *not* exempt — dialogs
 * such as a "type the name to confirm" delete flow expect Enter in the input to
 * fire the primary action. Only text-entry surfaces where Enter means "newline"
 * (`textarea`, contenteditable) are skipped. Interactive controls with their own
 * Enter semantics (other buttons, `<select>`, comboboxes) keep that behavior;
 * see `focusKeepsEnter`.
 */

function isActionable(el: HTMLElement | null): el is HTMLElement {
  if (!el) return false;
  if (el.hasAttribute("disabled")) return false;
  if (el.getAttribute("aria-disabled") === "true") return false;
  if (el.getAttribute("data-disabled") !== null) return false;
  return true;
}

function isPrimaryCandidate(button: HTMLElement): boolean {
  if (button.getAttribute("type") === "submit") return true;
  const variant = button.getAttribute("data-variant");
  return variant === "default" || variant === "destructive";
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
 *  3. The single primary action button in the dialog footer — a `type="submit"`
 *     button or one with `data-variant` `default`/`destructive`.
 *
 * Primary candidates are counted *including disabled ones*: a footer with two
 * competing actions where one is temporarily disabled (e.g. "Migrate & Delete"
 * pending a selection, next to an enabled "Delete & Archive") is ambiguous, so
 * we return null and do nothing rather than fire the wrong destructive action.
 */
export function resolveDialogDefaultAction(content: HTMLElement): HTMLElement | null {
  const alertAction = content.querySelector<HTMLElement>('[data-slot="alert-dialog-action"]');
  if (alertAction) return isActionable(alertAction) ? alertAction : null;

  const explicit = content.querySelector<HTMLElement>("[data-dialog-default-action]");
  if (explicit) return isActionable(explicit) ? explicit : null;

  const footer = content.querySelector<HTMLElement>('[data-slot="dialog-footer"]');
  if (!footer) return null;

  const primaries = Array.from(footer.querySelectorAll<HTMLElement>("button")).filter(
    isPrimaryCandidate,
  );
  if (primaries.length !== 1) return null;

  const only = primaries[0];
  return isActionable(only) ? only : null;
}

const TEXT_ENTRY_TAGS = new Set(["TEXTAREA"]);

/** Enter in a multi-line text field means "newline", not "confirm". */
function isTextEntry(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false;
  if (TEXT_ENTRY_TAGS.has(el.tagName)) return true;
  if (el.isContentEditable) return true;
  return false;
}

const DISMISS_VARIANTS = new Set(["outline", "ghost", "secondary", "link"]);

/**
 * When Enter fires from a focused interactive control that owns its own Enter
 * behavior — another action button, a `<select>`, a combobox — let that control
 * handle it instead of hijacking Enter for the footer action. The one exception
 * is a dismiss/secondary control (Cancel, the close "X"): overriding those is
 * the whole point (Radix focuses Cancel on an AlertDialog by default), so Enter
 * still fires the primary action there.
 */
function focusKeepsEnter(target: EventTarget | null, action: HTMLElement): boolean {
  if (!(target instanceof HTMLElement)) return false;
  if (target === action) return true; // let native activation click it (no double-fire)

  const role = target.getAttribute("role");
  const interactive =
    target.tagName === "BUTTON" ||
    target.tagName === "A" ||
    target.tagName === "SELECT" ||
    role === "button" ||
    role === "combobox" ||
    role === "listbox" ||
    role === "menu";
  if (!interactive) return false;

  const slot = target.getAttribute("data-slot");
  if (slot === "alert-dialog-cancel" || slot === "dialog-close") return false;
  const variant = target.getAttribute("data-variant");
  if (variant && DISMISS_VARIANTS.has(variant)) return false;

  return true;
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
  if (event.repeat) return; // ignore auto-repeat while Enter is held down
  if (event.defaultPrevented) return;
  if (event.nativeEvent.isComposing) return; // mid-IME composition
  if (isTextEntry(event.target)) return;

  const action = resolveDialogDefaultAction(event.currentTarget);
  if (!action) return;
  if (focusKeepsEnter(event.target, action)) return;

  event.preventDefault();
  action.click();
}
