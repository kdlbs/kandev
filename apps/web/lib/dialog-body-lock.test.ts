import { describe, it, expect, beforeEach } from "vitest";
import { releaseStuckDialogBodyLock } from "@kandev/ui/lib/dialog-body-lock";

/**
 * Unit coverage for the modal body-lock recovery used by the base Dialog.
 *
 * Radix releases `pointer-events: none` on <body> when the dialog content
 * unmounts, and Presence defers that unmount until the exit animation fires
 * `animationend`. When that event never arrives — the document was hidden while
 * closing, so animation timelines were frozen, or the dialog unmounted mid-close
 * — the lock outlives its dialog and every control on the page stops responding
 * with nothing visible to close. These tests pin the two halves of the contract:
 * a stranded lock is cleared, and a lock a live modal still needs is not.
 */
const SCROLL_LOCKED = "data-scroll-locked";

function lockBody() {
  document.body.style.pointerEvents = "none";
  document.body.setAttribute(SCROLL_LOCKED, "1");
}

function isLocked() {
  return document.body.style.pointerEvents === "none" || document.body.hasAttribute(SCROLL_LOCKED);
}

/** Mirrors what DialogContent renders, so the guard sees realistic markup. */
function mountContent(state: "open" | "closed", slot = "dialog-content") {
  const el = document.createElement("div");
  el.setAttribute("role", "dialog");
  el.setAttribute("data-slot", slot);
  el.setAttribute("data-state", state);
  document.body.appendChild(el);
  return el;
}

beforeEach(() => {
  document.body.innerHTML = "";
  document.body.removeAttribute(SCROLL_LOCKED);
  document.body.style.removeProperty("pointer-events");
});

describe("releaseStuckDialogBodyLock", () => {
  it("clears a lock stranded by a dialog that closed but never unmounted", () => {
    mountContent("closed");
    lockBody();

    expect(releaseStuckDialogBodyLock(document)).toBe(true);
    expect(isLocked()).toBe(false);
  });

  it("clears a lock stranded by a dialog that unmounted mid-close", () => {
    lockBody(); // no content in the DOM at all

    expect(releaseStuckDialogBodyLock(document)).toBe(true);
    expect(isLocked()).toBe(false);
  });

  it("leaves the lock alone while a dialog is still open", () => {
    mountContent("open");
    lockBody();

    expect(releaseStuckDialogBodyLock(document)).toBe(false);
    expect(isLocked()).toBe(true);
  });

  it("leaves the lock alone when a second modal is open behind a closed one", () => {
    mountContent("closed");
    mountContent("open", "alert-dialog-content");
    lockBody();

    expect(releaseStuckDialogBodyLock(document)).toBe(false);
    expect(isLocked()).toBe(true);
  });

  it("clears a scroll lock even when pointer-events was already released", () => {
    mountContent("closed");
    document.body.setAttribute(SCROLL_LOCKED, "1");

    expect(releaseStuckDialogBodyLock(document)).toBe(true);
    expect(document.body.hasAttribute(SCROLL_LOCKED)).toBe(false);
  });

  it("reports nothing to do on an unlocked body", () => {
    expect(releaseStuckDialogBodyLock(document)).toBe(false);
  });
});
