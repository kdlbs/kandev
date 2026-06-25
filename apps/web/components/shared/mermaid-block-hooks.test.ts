import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { useMermaidScale, useMermaidViewportWidth } from "./mermaid-block-hooks";

function setClientWidth(element: HTMLElement, width: number) {
  Object.defineProperty(element, "clientWidth", { value: width, configurable: true });
}

function disableResizeObserver() {
  Object.defineProperty(window, "ResizeObserver", {
    configurable: true,
    value: undefined,
    writable: true,
  });
}

describe("useMermaidViewportWidth", () => {
  const originalResizeObserver = window.ResizeObserver;

  afterEach(() => {
    Object.defineProperty(window, "ResizeObserver", {
      configurable: true,
      value: originalResizeObserver,
      writable: true,
    });
  });

  it("measures the scroll region content width on mount", () => {
    disableResizeObserver();
    const el = document.createElement("div");
    el.style.paddingLeft = "12px";
    el.style.paddingRight = "8px";
    setClientWidth(el, 300);
    document.body.appendChild(el);

    try {
      const { result } = renderHook(() => useMermaidViewportWidth(el));

      expect(result.current).toBe(280);
    } finally {
      el.remove();
    }
  });

  it("uses the window resize fallback when ResizeObserver is unavailable", () => {
    disableResizeObserver();
    const el = document.createElement("div");
    setClientWidth(el, 300);
    document.body.appendChild(el);
    const { result } = renderHook(() => useMermaidViewportWidth(el));

    act(() => {
      setClientWidth(el, 240);
      window.dispatchEvent(new Event("resize"));
    });

    expect(result.current).toBe(240);
    el.remove();
  });

  it("does not replace the last width while the scroll region is hidden", () => {
    disableResizeObserver();
    const el = document.createElement("div");
    setClientWidth(el, 300);
    document.body.appendChild(el);
    const { result } = renderHook(() => useMermaidViewportWidth(el));

    act(() => {
      el.style.display = "none";
      setClientWidth(el, 0);
      window.dispatchEvent(new Event("resize"));
    });

    expect(result.current).toBe(300);
    el.remove();
  });

  it("observes a replacement scroll region after the element changes", () => {
    window.ResizeObserver = class {
      private readonly callback: ResizeObserverCallback;
      observed: Element | null = null;

      constructor(callback: ResizeObserverCallback) {
        this.callback = callback;
      }

      observe(element: Element) {
        this.observed = element;
      }

      disconnect() {
        this.observed = null;
      }

      unobserve() {
        this.observed = null;
      }

      trigger() {
        this.callback([], this);
      }
    };

    const first = document.createElement("div");
    const second = document.createElement("div");
    setClientWidth(first, 200);
    setClientWidth(second, 320);
    document.body.append(first, second);

    try {
      const { result, rerender } = renderHook(({ element }) => useMermaidViewportWidth(element), {
        initialProps: { element: first },
      });

      expect(result.current).toBe(200);

      rerender({ element: second });

      expect(result.current).toBe(320);
    } finally {
      first.remove();
      second.remove();
    }
  });
});

describe("useMermaidScale", () => {
  it("preserves manual zoom across viewport changes and reset returns to the latest fit", () => {
    const { result, rerender } = renderHook(
      ({ viewportWidth }) => useMermaidScale({ w: 1200, h: 400 }, viewportWidth),
      { initialProps: { viewportWidth: 600 } },
    );

    expect(result.current.scale).toBe(0.5);

    act(() => result.current.zoomIn());
    expect(result.current.scale).toBe(0.6);

    rerender({ viewportWidth: 300 });
    expect(result.current.scale).toBe(0.6);

    act(() => result.current.zoomReset());
    expect(result.current.scale).toBe(0.25);
  });

  it("resetAutoScale returns subsequent measurements to auto-fit behavior", () => {
    const { result, rerender } = renderHook(
      ({ viewportWidth }) => useMermaidScale({ w: 1200, h: 400 }, viewportWidth),
      { initialProps: { viewportWidth: 600 } },
    );

    act(() => result.current.zoomOut());
    expect(result.current.scale).toBe(0.4);

    act(() => result.current.resetAutoScale());
    rerender({ viewportWidth: 300 });

    expect(result.current.scale).toBe(0.25);
  });
});
