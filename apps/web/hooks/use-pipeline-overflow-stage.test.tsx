import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { usePipelineOverflowStage } from "./use-pipeline-overflow-stage";

const observerEntries: Array<{ element: Element; callback: ResizeObserverCallback }> = [];

class CapturingResizeObserver {
  private readonly callback: ResizeObserverCallback;

  constructor(callback: ResizeObserverCallback) {
    this.callback = callback;
  }

  observe(element: Element) {
    observerEntries.push({ element, callback: this.callback });
  }

  disconnect() {}
  unobserve() {}
}

function setGeometry(element: Element, overrides: { scrollWidth?: number; clientWidth?: number }) {
  if (overrides.scrollWidth !== undefined) {
    Object.defineProperty(element, "scrollWidth", {
      configurable: true,
      value: overrides.scrollWidth,
    });
  }
  if (overrides.clientWidth !== undefined) {
    Object.defineProperty(element, "clientWidth", {
      configurable: true,
      value: overrides.clientWidth,
    });
  }
}

/** Fires every captured ResizeObserver callback recorded for the given element. */
function fireResize(element: Element) {
  for (const entry of observerEntries) {
    if (entry.element === element) {
      act(() => entry.callback([], {} as ResizeObserver));
    }
  }
}

function TestRow() {
  const { outerRef, stripRef, atTerminus } = usePipelineOverflowStage<
    HTMLDivElement,
    HTMLDivElement
  >();
  return (
    <div ref={outerRef} data-testid="outer" data-at-terminus={atTerminus}>
      <div ref={stripRef} data-testid="strip" />
    </div>
  );
}

beforeEach(() => {
  observerEntries.length = 0;
  vi.stubGlobal("ResizeObserver", CapturingResizeObserver);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function readAtTerminus(outer: Element): string | null {
  return outer.getAttribute("data-at-terminus");
}

describe("usePipelineOverflowStage", () => {
  it("stays out of the terminus while the status strip fits within the combined region", () => {
    render(<TestRow />);
    const outer = screen.getByTestId("outer");
    const strip = screen.getByTestId("strip");
    setGeometry(outer, { clientWidth: 200 });
    setGeometry(strip, { scrollWidth: 150 });
    fireResize(outer);

    expect(readAtTerminus(outer)).toBe("false");
  });

  it("enters the terminus once the status strip alone no longer fits the combined region", () => {
    render(<TestRow />);
    const outer = screen.getByTestId("outer");
    const strip = screen.getByTestId("strip");
    setGeometry(outer, { clientWidth: 100 });
    setGeometry(strip, { scrollWidth: 150 });
    fireResize(outer);

    expect(readAtTerminus(outer)).toBe("true");
  });

  it("leaves the terminus again once more room becomes available", () => {
    render(<TestRow />);
    const outer = screen.getByTestId("outer");
    const strip = screen.getByTestId("strip");
    setGeometry(outer, { clientWidth: 100 });
    setGeometry(strip, { scrollWidth: 150 });
    fireResize(outer);
    expect(readAtTerminus(outer)).toBe("true");

    setGeometry(outer, { clientWidth: 200 });
    fireResize(outer);

    expect(readAtTerminus(outer)).toBe("false");
  });

  it("disconnects the observer on unmount", () => {
    const disconnect = vi.fn();
    class TrackingResizeObserver extends CapturingResizeObserver {
      disconnect() {
        disconnect();
      }
    }
    vi.stubGlobal("ResizeObserver", TrackingResizeObserver);

    const { unmount } = render(<TestRow />);
    unmount();

    expect(disconnect).toHaveBeenCalledTimes(1);
  });
});
