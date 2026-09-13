import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useIsTitleTruncated } from "./use-is-title-truncated";

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

function setGeometry(
  element: Element,
  overrides: { scrollHeight?: number; clientHeight?: number },
) {
  if (overrides.scrollHeight !== undefined) {
    Object.defineProperty(element, "scrollHeight", {
      configurable: true,
      value: overrides.scrollHeight,
    });
  }
  if (overrides.clientHeight !== undefined) {
    Object.defineProperty(element, "clientHeight", {
      configurable: true,
      value: overrides.clientHeight,
    });
  }
}

/** Fires the captured ResizeObserver callback recorded for the given element. */
function fireResize(element: Element) {
  for (const entry of observerEntries) {
    if (entry.element === element) {
      act(() => entry.callback([], {} as ResizeObserver));
    }
  }
}

const TRUNCATED_ATTR = "data-truncated";

function TestTitle({ text }: { text?: string } = {}) {
  const { ref, isTruncated } = useIsTitleTruncated<HTMLSpanElement>(text);
  return <span ref={ref} data-testid="title" data-truncated={isTruncated} />;
}

beforeEach(() => {
  observerEntries.length = 0;
  vi.stubGlobal("ResizeObserver", CapturingResizeObserver);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("useIsTitleTruncated", () => {
  it("reports not truncated when scrollHeight does not exceed clientHeight", () => {
    render(<TestTitle />);
    const el = screen.getByTestId("title");
    setGeometry(el, { scrollHeight: 18, clientHeight: 18 });
    fireResize(el);

    expect(el.getAttribute(TRUNCATED_ATTR)).toBe("false");
  });

  it("reports truncated when scrollHeight exceeds clientHeight (line-clamp clips vertically, not horizontally)", () => {
    render(<TestTitle />);
    const el = screen.getByTestId("title");
    setGeometry(el, { scrollHeight: 88, clientHeight: 18 });
    fireResize(el);

    expect(el.getAttribute(TRUNCATED_ATTR)).toBe("true");
  });

  it("flips back to not truncated when the element widens enough to fit on one line", () => {
    render(<TestTitle />);
    const el = screen.getByTestId("title");
    setGeometry(el, { scrollHeight: 88, clientHeight: 18 });
    fireResize(el);
    expect(el.getAttribute(TRUNCATED_ATTR)).toBe("true");

    setGeometry(el, { scrollHeight: 18, clientHeight: 18 });
    fireResize(el);

    expect(el.getAttribute(TRUNCATED_ATTR)).toBe("false");
  });

  it("recomputes when the title text changes without a resize event (fixed line-height keeps the box size constant)", () => {
    const { rerender } = render(<TestTitle text="Short" />);
    const el = screen.getByTestId("title");
    // The box's own size never changes: only the content clips differently.
    setGeometry(el, { scrollHeight: 18, clientHeight: 18 });
    fireResize(el);
    expect(el.getAttribute(TRUNCATED_ATTR)).toBe("false");

    setGeometry(el, { scrollHeight: 88, clientHeight: 18 });
    rerender(<TestTitle text="A much longer title that now clips at the same box size" />);

    expect(el.getAttribute(TRUNCATED_ATTR)).toBe("true");
  });

  it("disconnects the observer on unmount", () => {
    const disconnect = vi.fn();
    class TrackingResizeObserver extends CapturingResizeObserver {
      disconnect() {
        disconnect();
      }
    }
    vi.stubGlobal("ResizeObserver", TrackingResizeObserver);

    const { unmount } = render(<TestTitle />);
    unmount();

    expect(disconnect).toHaveBeenCalledTimes(1);
  });
});
