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
 * Recovery finishes only closing dialog animations. The resulting
 * `animationend` event lets Radix Presence unmount the closing owner and release
 * its own pointer and scroll-lock bookkeeping.
 */

/** Grace period after a close before sweeping. Must exceed the exit duration. */
export const DIALOG_CLOSE_RECOVERY_MS = 400;

const CLOSING_DIALOG_PARTS = [
  '[data-state="closed"][data-slot="dialog-content"]',
  '[data-state="closed"][data-slot="dialog-overlay"]',
].join(",");

type VisibilitySubscription = {
  subscribers: number;
  onVisibilityChange: () => void;
};

const visibilitySubscriptions = new WeakMap<Document, VisibilitySubscription>();

/**
 * Ask Radix Presence to finish closing dialog owners through its normal
 * `animationend` path. Returns true when at least one animation was finished.
 */
export function finishClosingDialogAnimations(doc: Document): boolean {
  let finished = false;
  for (const element of doc.querySelectorAll<HTMLElement>(CLOSING_DIALOG_PARTS)) {
    for (const animation of element.getAnimations()) {
      animation.finish();
      finished = true;
    }
  }
  return finished;
}

/** Share one document-level foreground recovery listener across dialog roots. */
export function subscribeDialogCloseRecovery(doc: Document): () => void {
  let subscription = visibilitySubscriptions.get(doc);
  if (!subscription) {
    const onVisibilityChange = () => {
      if (doc.visibilityState === "visible") finishClosingDialogAnimations(doc);
    };
    subscription = { subscribers: 0, onVisibilityChange };
    visibilitySubscriptions.set(doc, subscription);
    doc.addEventListener("visibilitychange", onVisibilityChange);
  }
  subscription.subscribers += 1;

  let subscribed = true;
  return () => {
    if (!subscribed) return;
    subscribed = false;
    subscription.subscribers -= 1;
    if (subscription.subscribers > 0) return;
    doc.removeEventListener("visibilitychange", subscription.onVisibilityChange);
    visibilitySubscriptions.delete(doc);
  };
}
