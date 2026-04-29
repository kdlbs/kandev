import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useVisualViewportOffset } from "./use-visual-viewport-offset";

class MockVisualViewport extends EventTarget {
  height = 800;
  width = 400;
  offsetTop = 0;
  offsetLeft = 0;
  pageLeft = 0;
  pageTop = 0;
  scale = 1;
}

function setInnerHeight(h: number) {
  Object.defineProperty(window, "innerHeight", { configurable: true, value: h, writable: true });
}

describe("useVisualViewportOffset", () => {
  let originalVV: VisualViewport | undefined;
  let vv: MockVisualViewport;

  beforeEach(() => {
    originalVV = window.visualViewport ?? undefined;
    vv = new MockVisualViewport();
    Object.defineProperty(window, "visualViewport", {
      configurable: true,
      value: vv,
      writable: true,
    });
    setInnerHeight(800);
  });

  afterEach(() => {
    Object.defineProperty(window, "visualViewport", {
      configurable: true,
      value: originalVV,
      writable: true,
    });
  });

  it("returns zero offset when no keyboard is open", () => {
    const { result } = renderHook(() => useVisualViewportOffset());
    expect(result.current.bottomOffset).toBe(0);
    expect(result.current.keyboardOpen).toBe(false);
    expect(result.current.viewportBottom).toBe(800);
  });

  it("reports keyboard offset when visualViewport shrinks", () => {
    const { result } = renderHook(() => useVisualViewportOffset());

    act(() => {
      vv.height = 500;
      vv.dispatchEvent(new Event("resize"));
    });

    expect(result.current.bottomOffset).toBe(300);
    expect(result.current.keyboardOpen).toBe(true);
    expect(result.current.viewportBottom).toBe(500);
  });

  it("viewportBottom tracks offsetTop + height for top-anchored positioning", () => {
    const { result } = renderHook(() => useVisualViewportOffset());
    act(() => {
      vv.height = 600;
      vv.offsetTop = 50;
      vv.dispatchEvent(new Event("resize"));
    });
    expect(result.current.viewportBottom).toBe(650);
  });

  it("accounts for offsetTop", () => {
    const { result } = renderHook(() => useVisualViewportOffset());

    act(() => {
      vv.height = 600;
      vv.offsetTop = 50;
      vv.dispatchEvent(new Event("resize"));
    });

    // 800 - 600 - 50 = 150
    expect(result.current.bottomOffset).toBe(150);
    expect(result.current.keyboardOpen).toBe(true);
  });

  it("stays below the keyboard-open threshold for small offsets", () => {
    const { result } = renderHook(() => useVisualViewportOffset());

    act(() => {
      vv.height = 750; // 50px offset, under 80 threshold
      vv.dispatchEvent(new Event("resize"));
    });

    expect(result.current.bottomOffset).toBe(50);
    expect(result.current.keyboardOpen).toBe(false);
  });

  it("stops updating after unmount", () => {
    const { result, unmount } = renderHook(() => useVisualViewportOffset());
    unmount();

    act(() => {
      vv.height = 400;
      vv.dispatchEvent(new Event("resize"));
    });

    expect(result.current.bottomOffset).toBe(0);
  });

  it("returns zeros when visualViewport is unavailable", () => {
    Object.defineProperty(window, "visualViewport", {
      configurable: true,
      value: undefined,
      writable: true,
    });
    const { result } = renderHook(() => useVisualViewportOffset());
    expect(result.current.bottomOffset).toBe(0);
    expect(result.current.keyboardOpen).toBe(false);
  });
});
