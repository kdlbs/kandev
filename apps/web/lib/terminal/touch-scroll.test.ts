import { afterEach, describe, expect, it, vi } from "vitest";
import { attachTouchScroll, computeScrollLines } from "./touch-scroll";

afterEach(() => {
  // Each test appends a fresh container to document.body. Without this, jsdom
  // accumulates orphan divs across the suite and a future test that queries
  // document.body could pick up containers from prior runs.
  document.body.innerHTML = "";
});

describe("computeScrollLines", () => {
  it("returns negative line count for a downward drag (reveal older)", () => {
    // 30px down with 10px per row → finger moved 3 rows → scroll 3 lines up.
    expect(computeScrollLines(30, 10)).toBe(-3);
  });

  it("returns positive line count for an upward drag (reveal newer)", () => {
    expect(computeScrollLines(-25, 10)).toBe(2);
  });

  it("rounds sub-row drags toward zero", () => {
    expect(computeScrollLines(9, 10)).toBe(0);
    expect(computeScrollLines(-9, 10)).toBe(0);
  });

  it("returns 0 for non-finite or non-positive row heights", () => {
    expect(computeScrollLines(50, 0)).toBe(0);
    expect(computeScrollLines(50, -10)).toBe(0);
    expect(computeScrollLines(Number.NaN, 10)).toBe(0);
    expect(computeScrollLines(10, Number.NaN)).toBe(0);
  });
});

type TerminalStub = { scrollLines: ReturnType<typeof vi.fn>; rows: number };

function makeContainer(): HTMLElement {
  const el = document.createElement("div");
  document.body.appendChild(el);
  return el;
}

function makeTouch(clientX: number, clientY: number): Touch {
  // jsdom doesn't ship a Touch constructor; the structural shape is enough
  // for the handler, which only reads clientX/clientY/identifier.
  return { clientX, clientY, identifier: 1 } as unknown as Touch;
}

function makeTouchEvent(type: string, touches: Touch[]): TouchEvent {
  const event = new Event(type, { bubbles: true, cancelable: true }) as TouchEvent;
  Object.defineProperty(event, "touches", { value: touches, configurable: true });
  Object.defineProperty(event, "changedTouches", { value: touches, configurable: true });
  return event;
}

function fire(container: HTMLElement, type: string, touches: Touch[]): TouchEvent {
  const event = makeTouchEvent(type, touches);
  container.dispatchEvent(event);
  return event;
}

describe("attachTouchScroll", () => {
  it("does not scroll or preventDefault for a single-finger drag below threshold", () => {
    const container = makeContainer();
    const terminal: TerminalStub = { scrollLines: vi.fn(), rows: 24 };
    const cleanup = attachTouchScroll(container, terminal, {
      threshold: 10,
      rowHeightFn: () => 10,
    });

    fire(container, "touchstart", [makeTouch(50, 100)]);
    const move = fire(container, "touchmove", [makeTouch(50, 104)]);

    expect(terminal.scrollLines).not.toHaveBeenCalled();
    expect(move.defaultPrevented).toBe(false);

    cleanup();
  });

  it("scrolls into scrollback (negative lines) for a downward drag past threshold", () => {
    const container = makeContainer();
    const terminal: TerminalStub = { scrollLines: vi.fn(), rows: 24 };
    const cleanup = attachTouchScroll(container, terminal, {
      threshold: 6,
      rowHeightFn: () => 10,
    });

    fire(container, "touchstart", [makeTouch(50, 100)]);
    // Drag 30px down — should scroll 3 lines into scrollback (-3).
    const move = fire(container, "touchmove", [makeTouch(50, 130)]);

    expect(terminal.scrollLines).toHaveBeenCalledTimes(1);
    expect(terminal.scrollLines).toHaveBeenCalledWith(-3);
    expect(move.defaultPrevented).toBe(true);

    cleanup();
  });

  it("scrolls toward newer output (positive lines) for an upward drag", () => {
    const container = makeContainer();
    const terminal: TerminalStub = { scrollLines: vi.fn(), rows: 24 };
    const cleanup = attachTouchScroll(container, terminal, {
      threshold: 6,
      rowHeightFn: () => 10,
    });

    fire(container, "touchstart", [makeTouch(50, 200)]);
    fire(container, "touchmove", [makeTouch(50, 170)]);

    expect(terminal.scrollLines).toHaveBeenCalledWith(3);

    cleanup();
  });

  it("accumulates partial-row motion across successive move events", () => {
    const container = makeContainer();
    const terminal: TerminalStub = { scrollLines: vi.fn(), rows: 24 };
    const cleanup = attachTouchScroll(container, terminal, {
      threshold: 1,
      rowHeightFn: () => 10,
    });

    fire(container, "touchstart", [makeTouch(50, 100)]);
    fire(container, "touchmove", [makeTouch(50, 115)]); // dy=15 → -1 line (5 px residual)
    fire(container, "touchmove", [makeTouch(50, 121)]); // 6 + 5 residual = 11 → -1 line
    fire(container, "touchmove", [makeTouch(50, 124)]); // residual + 3 → 4 → 0

    expect(terminal.scrollLines).toHaveBeenNthCalledWith(1, -1);
    expect(terminal.scrollLines).toHaveBeenNthCalledWith(2, -1);
    expect(terminal.scrollLines).toHaveBeenCalledTimes(2);

    cleanup();
  });

  it("ignores multi-touch (pinch) gestures", () => {
    const container = makeContainer();
    const terminal: TerminalStub = { scrollLines: vi.fn(), rows: 24 };
    const cleanup = attachTouchScroll(container, terminal, {
      threshold: 6,
      rowHeightFn: () => 10,
    });

    fire(container, "touchstart", [makeTouch(50, 100), makeTouch(80, 100)]);
    const move = fire(container, "touchmove", [makeTouch(50, 130), makeTouch(80, 130)]);

    expect(terminal.scrollLines).not.toHaveBeenCalled();
    expect(move.defaultPrevented).toBe(false);

    cleanup();
  });

  it("yields to horizontal-dominant drags", () => {
    const container = makeContainer();
    const terminal: TerminalStub = { scrollLines: vi.fn(), rows: 24 };
    const cleanup = attachTouchScroll(container, terminal, {
      threshold: 6,
      rowHeightFn: () => 10,
    });

    fire(container, "touchstart", [makeTouch(50, 100)]);
    // |dx|=40, |dy|=10 — horizontal dominates; no scroll, no preventDefault.
    const move = fire(container, "touchmove", [makeTouch(90, 110)]);

    expect(terminal.scrollLines).not.toHaveBeenCalled();
    expect(move.defaultPrevented).toBe(false);

    cleanup();
  });

  it("cleanup removes the listeners", () => {
    const container = makeContainer();
    const terminal: TerminalStub = { scrollLines: vi.fn(), rows: 24 };
    const cleanup = attachTouchScroll(container, terminal, {
      threshold: 6,
      rowHeightFn: () => 10,
    });

    cleanup();

    fire(container, "touchstart", [makeTouch(50, 100)]);
    fire(container, "touchmove", [makeTouch(50, 200)]);

    expect(terminal.scrollLines).not.toHaveBeenCalled();
  });
});
