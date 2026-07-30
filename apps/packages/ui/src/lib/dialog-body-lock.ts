/**
 * Recovery for a stuck modal body lock.
 *
 * Radix sets `pointer-events: none` on <body> while a modal dialog is open and
 * releases it when the content unmounts. Presence defers that unmount until the
 * exit animation fires `animationend` — so anything that stops that event from
 * arriving leaves every control on the page dead, with no visible dialog to
 * close. Two ways in, both observed:
 *
 *   - the dialog unmounts mid-close (onOpenChange(false) then router.push), so
 *     Radix's own cleanup never runs;
 *   - the document is not being rendered when the dialog closes, which freezes
 *     CSS animation timelines. `document.timeline.currentTime` stays at 0 and a
 *     100ms exit animation reports playState "running" indefinitely.
 *
 * The fix is not to remove the animation — it is to stop treating an animation
 * callback as the only path back to a usable page.
 */

/** Grace period after a close before sweeping. Must exceed the exit duration. */
export const DIALOG_BODY_LOCK_SWEEP_MS = 400;

/** Slot suffix shared by every content element that can hold the modal lock. */
const OPEN_MODAL_CONTENT = '[data-state="open"][data-slot$="content"]';

/**
 * Release the body pointer-events / scroll lock, but only when nothing still
 * needs it. Returns true when a stuck lock was cleared.
 *
 * Deliberately conservative: if any modal content is still open the lock is
 * legitimate and is left alone. Failing to release leaves the status quo;
 * releasing while a real modal is open would break that modal.
 */
export function releaseStuckDialogBodyLock(doc: Document): boolean {
  const locked =
    doc.body.style.pointerEvents === "none" || doc.body.hasAttribute("data-scroll-locked");
  if (!locked) return false;
  if (doc.querySelector(OPEN_MODAL_CONTENT)) return false;
  doc.body.style.removeProperty("pointer-events");
  doc.body.removeAttribute("data-scroll-locked");
  return true;
}
