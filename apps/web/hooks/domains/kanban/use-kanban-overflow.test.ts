import { act, fireEvent, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useKanbanOverflow } from "./use-kanban-overflow";

type Geometry = {
  clientHeight?: number;
  clientWidth?: number;
  offsetHeight?: number;
  offsetWidth?: number;
  scrollHeight?: number;
  scrollLeft?: number;
  scrollTop?: number;
  scrollWidth?: number;
};

const observerCallbacks: ResizeObserverCallback[] = [];
const observerDisconnects: Array<() => void> = [];

function scrollElement(geometry: Geometry = {}) {
  const element = document.createElement("div");
  Object.defineProperties(element, {
    clientHeight: { configurable: true, value: geometry.clientHeight ?? 200 },
    clientWidth: { configurable: true, value: geometry.clientWidth ?? 200 },
    offsetHeight: { configurable: true, value: geometry.offsetHeight ?? 0 },
    offsetWidth: { configurable: true, value: geometry.offsetWidth ?? 0 },
    scrollHeight: { configurable: true, value: geometry.scrollHeight ?? 200 },
    scrollLeft: { configurable: true, value: geometry.scrollLeft ?? 0, writable: true },
    scrollTop: { configurable: true, value: geometry.scrollTop ?? 0, writable: true },
    scrollWidth: { configurable: true, value: geometry.scrollWidth ?? 200 },
  });
  return element;
}

function flushFrame() {
  act(() => {
    vi.advanceTimersByTime(0);
  });
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) =>
    setTimeout(() => callback(0), 0),
  );
  vi.stubGlobal("cancelAnimationFrame", (id: number) => clearTimeout(id));
  vi.stubGlobal(
    "ResizeObserver",
    class {
      constructor(callback: ResizeObserverCallback) {
        observerCallbacks.push(callback);
      }
      observe() {}
      disconnect() {
        observerDisconnects.push(() => {});
      }
    },
  );
});

afterEach(() => {
  observerCallbacks.length = 0;
  observerDisconnects.length = 0;
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("useKanbanOverflow", () => {
  it("reports vertical start, middle, and end edges with a one-pixel tolerance", () => {
    // @covers AC-UI-ADAPTIVE-KANBAN-003.2, AC-UI-ADAPTIVE-KANBAN-003.3
    const scroll = scrollElement({ scrollHeight: 600 });
    const scrollRef = { current: scroll };
    const contentRef = { current: scrollElement() };
    const { result } = renderHook(() =>
      useKanbanOverflow(scrollRef, { axis: "vertical", contentRef }),
    );

    flushFrame();
    expect(result.current).toMatchObject({ canScrollTop: false, canScrollBottom: true });

    act(() => {
      scroll.scrollTop = 200;
      fireEvent.scroll(scroll);
    });
    flushFrame();
    expect(result.current).toMatchObject({ canScrollTop: true, canScrollBottom: true });

    act(() => {
      scroll.scrollTop = 399;
      fireEvent.scroll(scroll);
    });
    flushFrame();
    expect(result.current).toMatchObject({ canScrollTop: true, canScrollBottom: false });
  });

  it("reports horizontal edges without treating vertical dimensions as overflow", () => {
    // @covers AC-UI-ADAPTIVE-KANBAN-003.1, AC-UI-ADAPTIVE-KANBAN-003.2
    const scroll = scrollElement({
      clientWidth: 200,
      offsetWidth: 210,
      scrollWidth: 700,
      scrollHeight: 400,
    });
    const scrollRef = { current: scroll };
    const { result } = renderHook(() => useKanbanOverflow(scrollRef, { axis: "horizontal" }));

    flushFrame();
    expect(result.current).toMatchObject({
      canScrollLeft: false,
      canScrollRight: true,
      canScrollTop: false,
      canScrollBottom: false,
    });

    act(() => {
      scroll.scrollLeft = 250;
      fireEvent.scroll(scroll);
    });
    flushFrame();
    expect(result.current).toMatchObject({ canScrollLeft: true, canScrollRight: true });

    act(() => {
      scroll.scrollLeft = 498;
      fireEvent.scroll(scroll);
    });
    flushFrame();
    expect(result.current).toMatchObject({ canScrollLeft: true, canScrollRight: true });

    act(() => {
      scroll.scrollLeft = 499;
      fireEvent.scroll(scroll);
    });
    flushFrame();
    expect(result.current).toMatchObject({ canScrollLeft: true, canScrollRight: false });
  });

  it("uses the real content extent when the scroll owner includes a drag reserve", () => {
    const scroll = scrollElement({ clientWidth: 200, scrollWidth: 700 });
    const content = scrollElement({ clientWidth: 200, scrollWidth: 200 });
    const scrollRef = { current: scroll };
    const contentRef = { current: content };
    const { result } = renderHook(() =>
      useKanbanOverflow(scrollRef, { axis: "horizontal", contentRef }),
    );

    flushFrame();
    expect(result.current).toMatchObject({ canScrollLeft: false, canScrollRight: false });
  });
});

describe("useKanbanOverflow interaction state", () => {
  it("reveals the active-scroll state and clears it after the idle window", () => {
    // @covers AC-UI-ADAPTIVE-KANBAN-003.4
    const scroll = scrollElement({ scrollHeight: 600 });
    const scrollRef = { current: scroll };
    const { result } = renderHook(() => useKanbanOverflow(scrollRef, { axis: "vertical" }));
    flushFrame();

    act(() => {
      scroll.scrollTop = 100;
      fireEvent.scroll(scroll);
    });
    flushFrame();
    expect(result.current.isScrolling).toBe(true);

    act(() => vi.advanceTimersByTime(799));
    expect(result.current.isScrolling).toBe(true);
    act(() => vi.advanceTimersByTime(1));
    expect(result.current.isScrolling).toBe(false);
  });

  it("rechecks content changes and disconnects observers on unmount", () => {
    // @covers AC-UI-ADAPTIVE-KANBAN-003.2, AC-UI-ADAPTIVE-KANBAN-003.8
    const scroll = scrollElement();
    const content = scrollElement();
    const scrollRef = { current: scroll };
    const contentRef = { current: content };
    const { result, unmount } = renderHook(() =>
      useKanbanOverflow(scrollRef, { axis: "vertical", contentRef, revision: 1 }),
    );
    flushFrame();
    expect(result.current.canScrollBottom).toBe(false);

    Object.defineProperty(scroll, "scrollHeight", { configurable: true, value: 500 });
    act(() => observerCallbacks[0]?.([], {} as ResizeObserver));
    flushFrame();
    expect(result.current.canScrollBottom).toBe(true);

    unmount();
    expect(observerDisconnects).toHaveLength(1);
  });
});
