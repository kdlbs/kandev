import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { RAIL_DEFAULT_WIDTH, RAIL_MIN_WIDTH, useRailWidth } from "./use-rail-width";

const STORAGE_KEY = "kandev.runsRailWidth";

function drag(onResizeStart: (event: React.MouseEvent) => void, from: number, to: number) {
  act(() => {
    onResizeStart({ preventDefault: () => {}, clientX: from } as React.MouseEvent);
  });
  act(() => {
    window.dispatchEvent(new MouseEvent("mousemove", { clientX: to }));
  });
  act(() => {
    window.dispatchEvent(new MouseEvent("mouseup"));
  });
}

beforeEach(() => {
  window.localStorage.clear();
  window.innerWidth = 1600;
});

afterEach(() => {
  window.localStorage.clear();
});

describe("useRailWidth", () => {
  it("starts at the default width", () => {
    const { result } = renderHook(() => useRailWidth());

    expect(result.current.width).toBe(RAIL_DEFAULT_WIDTH);
  });

  it("widens when the edge is dragged left", () => {
    // The rail is on the right, so leftward travel makes it bigger — the
    // inverse of the sidebar's handle.
    const { result } = renderHook(() => useRailWidth());

    drag(result.current.onResizeStart, 1000, 900);

    expect(result.current.width).toBe(RAIL_DEFAULT_WIDTH + 100);
  });

  it("narrows when dragged right", () => {
    const { result } = renderHook(() => useRailWidth());

    drag(result.current.onResizeStart, 1000, 1050);

    expect(result.current.width).toBe(RAIL_DEFAULT_WIDTH - 50);
  });

  it("will not collapse past the point where a run row is readable", () => {
    const { result } = renderHook(() => useRailWidth());

    drag(result.current.onResizeStart, 1000, 2000);

    expect(result.current.width).toBe(RAIL_MIN_WIDTH);
  });

  it("will not let the switcher crowd out the transcript", () => {
    // The transcript is the point of the page; the rail beside it is not.
    const { result } = renderHook(() => useRailWidth());

    drag(result.current.onResizeStart, 1000, -5000);

    expect(result.current.width).toBeLessThanOrEqual(Math.floor(window.innerWidth * 0.4));
  });

  it("remembers the width across sessions", async () => {
    const first = renderHook(() => useRailWidth());
    drag(first.result.current.onResizeStart, 1000, 940);
    await waitFor(() => expect(window.localStorage.getItem(STORAGE_KEY)).toBe("348"));

    first.unmount();
    const second = renderHook(() => useRailWidth());

    await waitFor(() => expect(second.result.current.width).toBe(348));
  });

  it("reports while a drag is in flight, so the edge can skip its transition", () => {
    const { result } = renderHook(() => useRailWidth());

    act(() => {
      result.current.onResizeStart({
        preventDefault: () => {},
        clientX: 1000,
      } as React.MouseEvent);
    });
    expect(result.current.resizing).toBe(true);

    act(() => {
      window.dispatchEvent(new MouseEvent("mouseup"));
    });
    expect(result.current.resizing).toBe(false);
  });

  it("ignores a stored value that is not a width", async () => {
    window.localStorage.setItem(STORAGE_KEY, "not-a-number");

    const { result } = renderHook(() => useRailWidth());

    await waitFor(() => expect(result.current.width).toBe(RAIL_DEFAULT_WIDTH));
  });

  // Only mouseup used to detach the drag, and it is not guaranteed to arrive:
  // switching workspace mid-drag redirects the detail page to the list. The
  // listeners then outlived the hook for the life of the document, still
  // setting state on it.
  it("detaches the drag listeners when it unmounts mid-drag", () => {
    const { result, unmount } = renderHook(() => useRailWidth());

    act(() => {
      result.current.onResizeStart({
        preventDefault: () => {},
        clientX: 1000,
      } as React.MouseEvent);
    });

    const removed: string[] = [];
    const realRemove = window.removeEventListener.bind(window);
    window.removeEventListener = ((type: string, ...rest: unknown[]) => {
      removed.push(type);
      return (realRemove as (...args: unknown[]) => void)(type, ...rest);
    }) as typeof window.removeEventListener;

    try {
      unmount();
    } finally {
      window.removeEventListener = realRemove;
    }

    expect(removed).toContain("mousemove");
    expect(removed).toContain("mouseup");
  });

  // A drag that never sees its mouseup — pointer released over an iframe, or
  // the browser stealing capture — would otherwise strand its listeners when
  // the next drag starts. The width stays right either way (both handlers
  // compute the same absolute value), so the leak is only visible as the
  // listener that never comes off.
  it("detaches a previous drag before starting another", () => {
    const { result } = renderHook(() => useRailWidth());
    const start = () =>
      act(() => {
        result.current.onResizeStart({
          preventDefault: () => {},
          clientX: 1000,
        } as React.MouseEvent);
      });

    start();

    const removed: string[] = [];
    const realRemove = window.removeEventListener.bind(window);
    window.removeEventListener = ((type: string, ...rest: unknown[]) => {
      removed.push(type);
      return (realRemove as (...args: unknown[]) => void)(type, ...rest);
    }) as typeof window.removeEventListener;

    try {
      start();
    } finally {
      window.removeEventListener = realRemove;
    }

    expect(removed).toContain("mousemove");
    expect(removed).toContain("mouseup");
  });
});
