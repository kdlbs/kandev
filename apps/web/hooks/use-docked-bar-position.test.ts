import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { resolveDockedBarPosition, useDockedBarPosition } from "./use-docked-bar-position";

class MockVisualViewport extends EventTarget {
  height = 800;
  width = 400;
  offsetTop = 0;
  offsetLeft = 0;
  pageLeft = 0;
  pageTop = 0;
  scale = 1;
}

function containerWithBounds(bounds: { left: number; width: number; bottom: number }) {
  const element = document.createElement("div");
  element.getBoundingClientRect = () =>
    ({ ...bounds, top: 0, right: bounds.left + bounds.width, height: bounds.bottom }) as DOMRect;
  return element;
}

describe("resolveDockedBarPosition", () => {
  it("docks to the container bottom while the keyboard is closed", () => {
    expect(
      resolveDockedBarPosition({
        keyboardOpen: false,
        viewportBottom: 800,
        barHeight: 48,
        container: { left: 10, width: 300, bottom: 600 },
      }),
    ).toEqual({ top: 552, left: 10, width: 300 });
  });

  it("docks above the keyboard when it covers the container bottom", () => {
    expect(
      resolveDockedBarPosition({
        keyboardOpen: true,
        viewportBottom: 450,
        barHeight: 48,
        container: { left: 0, width: 400, bottom: 700 },
      }),
    ).toEqual({ top: 402, left: 0, width: 400 });
  });

  it("clamps a container that extends below the visible viewport", () => {
    expect(
      resolveDockedBarPosition({
        keyboardOpen: false,
        viewportBottom: 800,
        barHeight: 48,
        container: { left: 0, width: 400, bottom: 1200 },
      }),
    ).toEqual({ top: 752, left: 0, width: 400 });
  });

  it("falls back to the visual viewport without a container", () => {
    expect(
      resolveDockedBarPosition({ keyboardOpen: true, viewportBottom: 450, barHeight: 48 }),
    ).toEqual({ top: 402 });
    expect(
      resolveDockedBarPosition({ keyboardOpen: false, viewportBottom: 800, barHeight: 48 }),
    ).toEqual({ bottom: "env(safe-area-inset-bottom, 0px)" });
  });
});

describe("useDockedBarPosition", () => {
  let originalVV: VisualViewport | undefined;
  let vv: MockVisualViewport;

  beforeEach(() => {
    originalVV = window.visualViewport ?? undefined;
    vv = new MockVisualViewport();
    Object.defineProperty(window, "visualViewport", { configurable: true, value: vv });
    Object.defineProperty(window, "innerHeight", { configurable: true, value: 800 });
  });

  afterEach(() => {
    Object.defineProperty(window, "visualViewport", { configurable: true, value: originalVV });
  });

  it("tracks the container and follows the keyboard", () => {
    const container = containerWithBounds({ left: 0, width: 400, bottom: 700 });
    const containerRef = { current: container };
    const { result } = renderHook(() => useDockedBarPosition({ containerRef, barHeight: 48 }));
    expect(result.current).toEqual({ top: 652, left: 0, width: 400 });

    act(() => {
      vv.height = 450;
      vv.dispatchEvent(new Event("resize"));
    });
    expect(result.current).toEqual({ top: 402, left: 0, width: 400 });
  });
});
