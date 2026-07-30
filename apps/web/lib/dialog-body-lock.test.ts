import { describe, it, expect, beforeEach, vi } from "vitest";
import { finishClosingDialogAnimations } from "@kandev/ui/lib/dialog-body-lock";

/**
 * Unit coverage for the modal body-lock recovery used by the base Dialog.
 *
 * Radix releases `pointer-events: none` on <body> when the dialog content
 * unmounts, and Presence defers that unmount until the exit animation fires
 * `animationend`. When that event never arrives — the document was hidden while
 * closing, so animation timelines were frozen, or the dialog unmounted mid-close
 * — the lock outlives its dialog and every control on the page stops responding
 * with nothing visible to close. These tests pin the two halves of the contract:
 * recovery terminates only closing modal animations and leaves Radix to release
 * its own body-lock bookkeeping when those modal owners unmount.
 */
const SCROLL_LOCKED = "data-scroll-locked";
const DIALOG_CONTENT = "dialog-content";

function lockBody() {
  document.body.style.pointerEvents = "none";
  document.body.setAttribute(SCROLL_LOCKED, "1");
}

function isLocked() {
  return document.body.style.pointerEvents === "none" || document.body.hasAttribute(SCROLL_LOCKED);
}

function animation() {
  return { finish: vi.fn() } as unknown as Animation;
}

/** Mirrors modal/content primitives so selectors see realistic markup. */
function mountContent(
  state: "open" | "closed",
  slot = DIALOG_CONTENT,
  animations: Animation[] = [],
) {
  const el = document.createElement("div");
  el.setAttribute("role", "dialog");
  el.setAttribute("data-slot", slot);
  el.setAttribute("data-state", state);
  el.getAnimations = () => animations;
  document.body.appendChild(el);
  return el;
}

beforeEach(() => {
  document.body.innerHTML = "";
  document.body.removeAttribute(SCROLL_LOCKED);
  document.body.style.removeProperty("pointer-events");
});

describe("finishClosingDialogAnimations", () => {
  it("finishes a closing dialog animation without mutating its owner's body lock", () => {
    const exit = animation();
    mountContent("closed", DIALOG_CONTENT, [exit]);
    lockBody();

    expect(finishClosingDialogAnimations(document)).toBe(true);
    expect(exit.finish).toHaveBeenCalledOnce();
    expect(isLocked()).toBe(true);
  });

  it("finishes a closing overlay so an invisible portal cannot intercept clicks", () => {
    const exit = animation();
    mountContent("closed", "dialog-overlay", [exit]);

    expect(finishClosingDialogAnimations(document)).toBe(true);
    expect(exit.finish).toHaveBeenCalledOnce();
  });

  it("ignores open dialog animations", () => {
    const open = animation();
    mountContent("open", DIALOG_CONTENT, [open]);
    lockBody();

    expect(finishClosingDialogAnimations(document)).toBe(false);
    expect(open.finish).not.toHaveBeenCalled();
    expect(isLocked()).toBe(true);
  });

  it("finishes the closing owner while preserving a second open modal's lock", () => {
    const closing = animation();
    const open = animation();
    mountContent("closed", DIALOG_CONTENT, [closing]);
    mountContent("open", "alert-dialog-content", [open]);
    lockBody();

    expect(finishClosingDialogAnimations(document)).toBe(true);
    expect(closing.finish).toHaveBeenCalledOnce();
    expect(open.finish).not.toHaveBeenCalled();
    expect(isLocked()).toBe(true);
  });

  it("does not let unrelated open content suppress dialog recovery", () => {
    const closing = animation();
    mountContent("closed", DIALOG_CONTENT, [closing]);
    mountContent("open", "accordion-content");
    mountContent("open", "collapsible-content");
    lockBody();

    expect(finishClosingDialogAnimations(document)).toBe(true);
    expect(closing.finish).toHaveBeenCalledOnce();
    expect(isLocked()).toBe(true);
  });

  it("reports nothing to do without a closing dialog animation", () => {
    expect(finishClosingDialogAnimations(document)).toBe(false);
  });
});
