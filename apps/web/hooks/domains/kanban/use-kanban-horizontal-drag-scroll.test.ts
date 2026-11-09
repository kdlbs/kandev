import { act, cleanup, fireEvent, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { DndMonitorListener, DragStartEvent } from "@dnd-kit/core";
import { useKanbanHorizontalDragScroll } from "./use-kanban-horizontal-drag-scroll";

const monitor = vi.hoisted(() => ({ current: {} as DndMonitorListener }));
vi.mock("@dnd-kit/core", () => ({
  useDndMonitor: (listener: DndMonitorListener) => {
    monitor.current = listener;
  },
}));

function startDrag(x = 300) {
  act(() =>
    monitor.current.onDragStart?.({
      active: {
        id: "task",
        data: { current: {} },
        rect: { current: { initial: null, translated: null } },
      },
      activatorEvent: new PointerEvent("pointerdown", { clientX: x, clientY: 150 }),
    } as DragStartEvent),
  );
}

let time = 0;
let nextFrame = 0;
const frames = new Map<number, FrameRequestCallback>();

function frame(elapsed = 16) {
  time += elapsed;
  const pending = [...frames.values()];
  frames.clear();
  act(() => pending.forEach((callback) => callback(time)));
}

function board(contentWidth = 800) {
  const scroll = document.createElement("div");
  const grid = document.createElement("div");
  scroll.append(grid);
  document.body.append(scroll);
  const geometry = { width: 400, height: 300, contentWidth };
  Object.defineProperty(scroll, "clientWidth", { get: () => geometry.width });
  Object.defineProperty(scroll, "scrollWidth", { get: () => geometry.contentWidth + 400 });
  vi.spyOn(scroll, "getBoundingClientRect").mockImplementation(
    () => new DOMRect(100, 50, geometry.width, geometry.height),
  );
  vi.spyOn(grid, "getBoundingClientRect").mockImplementation(
    () => new DOMRect(100 - scroll.scrollLeft, 50, geometry.contentWidth, geometry.height),
  );
  const view = renderHook(
    ({ isDragging }) =>
      useKanbanHorizontalDragScroll({
        isDragging,
        scrollWindowRef: { current: scroll },
        laneGridRef: { current: grid },
      }),
    { initialProps: { isDragging: true } },
  );
  startDrag();
  return { scroll, geometry, ...view };
}

function pointer(x: number, y = 150) {
  fireEvent.pointerMove(document, { clientX: x, clientY: y, isPrimary: true });
}

beforeEach(() => {
  time = 0;
  nextFrame = 0;
  vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
    frames.set(++nextFrame, callback);
    return nextFrame;
  });
  vi.stubGlobal("cancelAnimationFrame", (id: number) => frames.delete(id));
});

afterEach(() => {
  cleanup();
  frames.clear();
  document.body.replaceChildren();
  vi.unstubAllGlobals();
});

describe("desktop horizontal drag scrolling", () => {
  it("uses the capture-phase edge position when one move activates dragging", () => {
    const { scroll, result } = board();
    act(() =>
      result.current({
        nativeEvent: new PointerEvent("pointermove", {
          clientX: 499,
          clientY: 150,
          isPrimary: true,
        }),
      }),
    );
    startDrag(300);
    frame();
    expect(scroll.scrollLeft).toBeGreaterThan(0);
  });

  // @covers AC-UI-ADAPTIVE-KANBAN-001.4, AC-UI-ADAPTIVE-KANBAN-003.9
  it("starts edge travel from the accepted activation event before another move", () => {
    const { scroll } = board();
    startDrag(499);
    frame();
    expect(scroll.scrollLeft).toBeGreaterThan(0);
  });

  it("does not enter reserve space on a fitting board", () => {
    const { scroll } = board(400);
    pointer(499);
    for (let i = 0; i < 10; i++) frame();
    expect(scroll.scrollLeft).toBe(0);
  });

  it("reaches actual content bounds and reverses within the same drag", () => {
    const { scroll } = board(430);
    pointer(499);
    for (let i = 0; i < 5; i++) frame();
    expect(scroll.scrollLeft).toBe(30);
    pointer(101);
    frame();
    expect(scroll.scrollLeft).toBe(0);
  });

  it("preserves an anchor beyond content until leftward edge input", () => {
    const { scroll } = board(430);
    scroll.scrollLeft = 200;
    frame();
    expect(scroll.scrollLeft).toBe(200);
    pointer(499);
    frame();
    expect(scroll.scrollLeft).toBe(200);
    pointer(101);
    frame();
    expect(scroll.scrollLeft).toBeGreaterThan(30);
    expect(scroll.scrollLeft).toBeLessThan(200);
  });

  it("reads live viewport and content dimensions", () => {
    const { scroll, geometry } = board(430);
    pointer(499);
    frame();
    expect(scroll.scrollLeft).toBeGreaterThan(0);
    geometry.contentWidth = 500;
    geometry.width = 300;
    pointer(399);
    for (let i = 0; i < 10; i++) frame();
    expect(scroll.scrollLeft).toBe(200);
  });

  it("ignores a zero-width viewport and input outside the lane", () => {
    const { scroll, geometry } = board();
    pointer(499, 400);
    frame();
    expect(scroll.scrollLeft).toBe(0);
    pointer(550);
    frame();
    expect(scroll.scrollLeft).toBe(0);
    geometry.width = 0;
    pointer(100);
    frame();
    expect(scroll.scrollLeft).toBe(0);
  });

  it("caps elapsed time after a suspended frame", () => {
    const { scroll } = board(2000);
    pointer(499);
    frame();
    const before = scroll.scrollLeft;
    frame(10_000);
    expect(scroll.scrollLeft - before).toBeGreaterThan(0);
    expect(scroll.scrollLeft - before).toBeLessThanOrEqual(64);
  });
});

describe("desktop drag input lifecycle", () => {
  it("uses touch coordinates for an accepted desktop touch drag", () => {
    const { scroll } = board();
    fireEvent.touchMove(document, { touches: [{ clientX: 499, clientY: 150 }] });
    frame();
    expect(scroll.scrollLeft).toBeGreaterThan(0);
    fireEvent.touchEnd(document);
    expect(frames.size).toBe(0);
  });

  it.each(["pointerup", "pointercancel", "touchend", "touchcancel"])(
    "stops pending travel on %s",
    (type) => {
      const { scroll } = board();
      pointer(499);
      frame();
      const before = scroll.scrollLeft;
      document.dispatchEvent(new Event(type));
      pointer(499);
      frame();
      expect(scroll.scrollLeft).toBe(before);
      expect(frames.size).toBe(0);
    },
  );

  it("disposes frames and listeners on drag end, branch change, and unmount", () => {
    const { scroll, rerender, unmount } = board();
    pointer(499);
    frame();
    rerender({ isDragging: false });
    const before = scroll.scrollLeft;
    pointer(499);
    frame();
    expect(scroll.scrollLeft).toBe(before);
    expect(frames.size).toBe(0);
    rerender({ isDragging: true });
    startDrag();
    pointer(499);
    frame();
    expect(scroll.scrollLeft).toBeGreaterThan(before);
    unmount();
    const final = scroll.scrollLeft;
    pointer(499);
    frame();
    expect(scroll.scrollLeft).toBe(final);
    expect(frames.size).toBe(0);
  });
});
