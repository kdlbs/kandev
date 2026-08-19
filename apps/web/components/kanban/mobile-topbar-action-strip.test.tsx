import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MobileTopbarActionStrip } from "./mobile-topbar-action-strip";

const observers: Array<{ trigger: () => void }> = [];
const VIEWPORT_TEST_ID = "mobile-topbar-action-strip-viewport";
const LEFT_FADE_TEST_ID = "mobile-topbar-action-strip-left-fade";
const RIGHT_FADE_TEST_ID = "mobile-topbar-action-strip-right-fade";

class MockResizeObserver {
  private readonly callback: ResizeObserverCallback;

  constructor(callback: ResizeObserverCallback) {
    this.callback = callback;
    observers.push({ trigger: () => this.callback([], this as unknown as ResizeObserver) });
  }

  observe() {}

  disconnect() {}
}

function setScrollMetrics(
  element: HTMLElement,
  metrics: { clientWidth: number; scrollWidth: number; scrollLeft: number },
) {
  Object.defineProperties(element, {
    clientWidth: { configurable: true, value: metrics.clientWidth },
    scrollWidth: { configurable: true, value: metrics.scrollWidth },
    scrollLeft: { configurable: true, value: metrics.scrollLeft, writable: true },
  });
}

describe("MobileTopbarActionStrip", () => {
  beforeEach(() => {
    observers.length = 0;
    vi.stubGlobal("ResizeObserver", MockResizeObserver);
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("does not render directional fades when all actions fit", () => {
    render(
      <MobileTopbarActionStrip>
        <span>Actions</span>
      </MobileTopbarActionStrip>,
    );

    const viewport = screen.getByTestId(VIEWPORT_TEST_ID);
    expect((viewport.firstElementChild as HTMLElement).className).toContain("justify-end");
    setScrollMetrics(viewport, { clientWidth: 200, scrollWidth: 200, scrollLeft: 0 });
    act(() => observers[0]?.trigger());

    expect(screen.queryByTestId(LEFT_FADE_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(RIGHT_FADE_TEST_ID)).toBeNull();
  });

  it("shows only the right fade when content overflows at the initial position", () => {
    render(
      <MobileTopbarActionStrip>
        <span>Actions</span>
      </MobileTopbarActionStrip>,
    );

    const viewport = screen.getByTestId(VIEWPORT_TEST_ID);
    setScrollMetrics(viewport, { clientWidth: 100, scrollWidth: 240, scrollLeft: 0 });
    act(() => observers[0]?.trigger());

    expect(screen.queryByTestId(LEFT_FADE_TEST_ID)).toBeNull();
    expect(screen.getByTestId(RIGHT_FADE_TEST_ID)).toBeTruthy();
  });

  it("updates both fades while scrolling through overflowing actions", () => {
    render(
      <MobileTopbarActionStrip>
        <span>Actions</span>
      </MobileTopbarActionStrip>,
    );

    const viewport = screen.getByTestId(VIEWPORT_TEST_ID);
    setScrollMetrics(viewport, { clientWidth: 100, scrollWidth: 240, scrollLeft: 0 });
    act(() => observers[0]?.trigger());

    act(() => {
      viewport.scrollLeft = 60;
      fireEvent.scroll(viewport);
    });
    expect(screen.getByTestId(LEFT_FADE_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId(RIGHT_FADE_TEST_ID)).toBeTruthy();

    act(() => {
      viewport.scrollLeft = 140;
      fireEvent.scroll(viewport);
    });
    expect(screen.getByTestId(LEFT_FADE_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId(RIGHT_FADE_TEST_ID)).toBeNull();
  });
});
