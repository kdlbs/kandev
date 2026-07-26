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

/** Emits the navigate event so a mounted stale-findings banner can expand. */
export function emitNavigateFinding(findingId: string) {
  if (typeof window === "undefined") return;
  window.dispatchEvent(
    new CustomEvent<NavigateFindingEventDetail>(NAVIGATE_FINDING_EVENT, {
      detail: { findingId },
    }),
  );
}

/**
 * The subtree to search for a finding card. The Changes panel behind the review
 * dialog can render the same finding inline, leaving two elements with the same
 * id in the document; scoping to the open dialog keeps navigation from scrolling
 * to (and flashing) the hidden background card. Falls back to the whole document
 * when no dialog is open.
 */
function findingScope(): ParentNode {
  if (typeof document === "undefined") return document;
  const dialogs = document.querySelectorAll<HTMLElement>('[role="dialog"][data-state="open"]');
  return dialogs.length > 0 ? dialogs[dialogs.length - 1] : document;
}

function prefersReducedMotion(): boolean {
  return (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}

/**
 * Scrolls a finding card into view and flashes it, retrying across frames while
 * its file section renders. Resolves true once the card was found, false if it
 * never appeared. Safe to call outside the browser (resolves false).
 *
 * The navigate event is re-emitted on every frame, not just once up front: the
 * target file's section — and a stale finding's collapsed banner — mount lazily
 * after the file is selected, so a single event fired before that render would
 * be missed and the banner would never expand.
 */
export function scrollToFinding(
  findingId: string,
  requestFrame: (cb: () => void) => void = defaultRequestFrame,
): Promise<boolean> {
  if (typeof document === "undefined") return Promise.resolve(false);
  return new Promise((resolve) => {
    let attempts = 0;
    const tick = () => {
      emitNavigateFinding(findingId);
      const el = findingScope().querySelector<HTMLElement>(findingSelector(findingId));
      if (el) {
        el.scrollIntoView({
          behavior: prefersReducedMotion() ? "auto" : "smooth",
          block: "center",
        });
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
 * expands and scrolls into view), then scrolls the finding's card into view and
 * flashes it. `scrollToFinding` re-emits the navigate event each frame so a
 * lazily-mounted stale-findings banner still expands. Resolves false if the
 * card never rendered within the retry budget.
 */
export function navigateToFinding(
  finding: TaskReviewFinding,
  selectFile: (fileKey: string) => void,
): Promise<boolean> {
  selectFile(findingFileKey(finding));
  return scrollToFinding(finding.id);
}
