import { findingFileKey } from "@/lib/review/findings";
import type { TaskReviewFinding } from "@/lib/types/review";

/**
 * DOM attribute stamped on the wrapper of every rendered finding (inline in the
 * diff and inside the stale-findings banner). It is the hook `scrollToFinding`
 * uses to locate a finding's card once its file section has rendered.
 */
export const FINDING_DOM_ATTR = "data-review-finding-id";

/**
 * Window event carrying a finding id the user asked to jump to. The stale
 * findings banner listens for it so it can expand itself when the target is one
 * of its own — otherwise a stale finding could never be scrolled to because its
 * card is collapsed out of the DOM.
 */
export const NAVIGATE_FINDING_EVENT = "review:navigate-finding";

/** Class that briefly highlights a finding card after it is scrolled into view. */
export const FINDING_FLASH_CLASS = "review-finding-flash";

export type NavigateFindingEventDetail = { findingId: string };

/** CSS selector for a rendered finding card by id. */
export function findingSelector(findingId: string): string {
  return `[${FINDING_DOM_ATTR}="${CSS.escape(findingId)}"]`;
}

const FLASH_DURATION_MS = 1400;
// A file section renders lazily once selected + scrolled into view; poll a few
// animation frames so navigation still lands after that render, then give up
// rather than leak a running loop.
const MAX_SCROLL_ATTEMPTS = 60;

/**
 * Scrolls a finding card into view and flashes it, retrying across frames while
 * its file section renders. Resolves true once the card was found, false if it
 * never appeared. Safe to call outside the browser (resolves false).
 */
export function scrollToFinding(
  findingId: string,
  requestFrame: (cb: () => void) => void = defaultRequestFrame,
): Promise<boolean> {
  if (typeof document === "undefined") return Promise.resolve(false);
  return new Promise((resolve) => {
    let attempts = 0;
    const tick = () => {
      const el = document.querySelector<HTMLElement>(findingSelector(findingId));
      if (el) {
        el.scrollIntoView({ behavior: "smooth", block: "center" });
        flashFinding(el);
        resolve(true);
        return;
      }
      attempts += 1;
      if (attempts >= MAX_SCROLL_ATTEMPTS) {
        resolve(false);
        return;
      }
      requestFrame(tick);
    };
    tick();
  });
}

function defaultRequestFrame(cb: () => void) {
  if (typeof requestAnimationFrame === "function") requestAnimationFrame(cb);
  else setTimeout(cb, 16);
}

/** Retriggers the flash animation by clearing and re-adding the class. */
function flashFinding(el: HTMLElement) {
  el.classList.remove(FINDING_FLASH_CLASS);
  // Force a reflow so re-adding the class restarts the animation even when the
  // same finding is navigated to twice in a row.
  void el.offsetWidth;
  el.classList.add(FINDING_FLASH_CLASS);
  window.setTimeout(() => el.classList.remove(FINDING_FLASH_CLASS), FLASH_DURATION_MS);
}

/**
 * Jumps to a finding in the review diff: selects its file (so the section
 * expands and scrolls into view), notifies the stale banner in case it must
 * expand, then scrolls the finding's card into view and flashes it.
 */
export function navigateToFinding(
  finding: TaskReviewFinding,
  selectFile: (fileKey: string) => void,
): Promise<boolean> {
  selectFile(findingFileKey(finding));
  if (typeof window !== "undefined") {
    window.dispatchEvent(
      new CustomEvent<NavigateFindingEventDetail>(NAVIGATE_FINDING_EVENT, {
        detail: { findingId: finding.id },
      }),
    );
  }
  return scrollToFinding(finding.id);
}
