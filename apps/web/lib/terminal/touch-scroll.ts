import type { Terminal } from "@xterm/xterm";

/**
 * Translate touch swipes on an xterm.js container into scrollback navigation.
 *
 * xterm.js renders to a canvas under `.xterm-screen`; that canvas has the
 * default `pointer-events: auto` and absorbs touchstart/touchmove before they
 * can bubble to any ancestor scroll container. xterm itself does not wire
 * touch events to its scrollback API (`scrollLines`), so on a mobile viewport
 * the user has no way to review prior output.
 *
 * This module bridges that gap with a small touch handler that maps single-
 * finger vertical drag distance onto `terminal.scrollLines(n)`.
 */

export type AttachTouchScrollOptions = {
  /** Minimum vertical drag (px) before we start treating the gesture as a
   *  scroll. Below this, taps still reach xterm (focus / selection). */
  threshold?: number;
  /** Override row-height resolution for tests. Defaults to
   *  container.clientHeight / terminal.rows. */
  rowHeightFn?: () => number;
};

const DEFAULT_THRESHOLD_PX = 6;

/**
 * Compute the number of lines to scroll for a given vertical drag delta.
 *
 * Sign convention matches xterm's `scrollLines`: positive = scroll down toward
 * newer output, negative = scroll up into scrollback. A downward finger drag
 * (deltaY positive) reveals older lines, so we return a negative count.
 *
 * Sub-row drags round to 0 so the viewport doesn't flicker on micro-motion.
 */
export function computeScrollLines(deltaY: number, rowHeight: number): number {
  if (!Number.isFinite(deltaY) || !Number.isFinite(rowHeight) || rowHeight <= 0) {
    return 0;
  }
  // Drag down (positive deltaY) → reveal older lines → negative scrollLines.
  // `|| 0` normalises -0 from `-Math.trunc(0.x)` to a plain 0 so equality
  // checks (and callers' `lines !== 0` short-circuits) behave intuitively.
  return -Math.trunc(deltaY / rowHeight) || 0;
}

type TouchScrollMinimalTerminal = Pick<Terminal, "scrollLines" | "rows">;

function defaultRowHeight(container: HTMLElement, terminal: TouchScrollMinimalTerminal): number {
  const rows = terminal.rows;
  if (!rows || rows <= 0) return 0;
  return container.clientHeight / rows;
}

/**
 * Attach touch listeners to `container` that scroll `terminal` on vertical
 * drag. Returns a cleanup function.
 *
 * Behavior:
 * - Single-finger drag only — multi-touch (pinch/zoom) is ignored.
 * - Vertical-dominant only — if |dx| > |dy| we yield to the surrounding UI
 *   (e.g. horizontal swipes on a sibling key-bar).
 * - `preventDefault` fires only after the drag passes `threshold` so a pure
 *   tap still focuses xterm (via xterm's own click handler).
 * - Drag distance is quantized to whole rows; intermediate motion accumulates
 *   in `lastY` so a slow drag still scrolls eventually.
 */
export function attachTouchScroll(
  container: HTMLElement,
  terminal: TouchScrollMinimalTerminal,
  opts: AttachTouchScrollOptions = {},
): () => void {
  const threshold = opts.threshold ?? DEFAULT_THRESHOLD_PX;
  const rowHeightFn = opts.rowHeightFn ?? (() => defaultRowHeight(container, terminal));

  let startX = 0;
  let startY = 0;
  let lastY = 0;
  let active = false;
  let crossedThreshold = false;

  const onTouchStart = (event: TouchEvent) => {
    if (event.touches.length !== 1) {
      active = false;
      return;
    }
    const touch = event.touches[0];
    startX = touch.clientX;
    startY = touch.clientY;
    lastY = touch.clientY;
    active = true;
    crossedThreshold = false;
  };

  const onTouchMove = (event: TouchEvent) => {
    if (!active) return;
    if (event.touches.length !== 1) {
      active = false;
      return;
    }
    const touch = event.touches[0];
    const dx = touch.clientX - startX;
    const dy = touch.clientY - startY;

    if (!crossedThreshold) {
      if (Math.abs(dy) < threshold) return;
      // Horizontal-dominant — yield to sibling gestures (e.g. key-bar swipes).
      if (Math.abs(dx) > Math.abs(dy)) {
        active = false;
        return;
      }
      crossedThreshold = true;
    }

    event.preventDefault();
    const rowHeight = rowHeightFn();
    const lines = computeScrollLines(touch.clientY - lastY, rowHeight);
    if (lines !== 0) {
      terminal.scrollLines(lines);
      // Consume the portion of the drag we just applied so the next move
      // event is relative to a fresh row boundary. We scrolled `lines` xterm
      // rows, which corresponds to `-lines * rowHeight` px of finger motion
      // (downward drag → negative lines → positive px consumed).
      lastY -= lines * rowHeight;
    }
  };

  const onTouchEnd = () => {
    active = false;
    crossedThreshold = false;
  };

  container.addEventListener("touchstart", onTouchStart, { passive: true });
  container.addEventListener("touchmove", onTouchMove, { passive: false });
  container.addEventListener("touchend", onTouchEnd, { passive: true });
  container.addEventListener("touchcancel", onTouchEnd, { passive: true });

  return () => {
    container.removeEventListener("touchstart", onTouchStart);
    container.removeEventListener("touchmove", onTouchMove);
    container.removeEventListener("touchend", onTouchEnd);
    container.removeEventListener("touchcancel", onTouchEnd);
  };
}
