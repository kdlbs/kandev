import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render } from "@testing-library/react";
import type { IDockviewPanelProps } from "dockview-react";
import { panelPortalManager } from "./panel-portal-manager";
import { releasePortalScrollRestoration, usePortalSlot } from "./panel-portal-host";

const resizeCallbacks: Array<() => void> = [];
beforeEach(() => {
  resizeCallbacks.length = 0;
  vi.stubGlobal(
    "ResizeObserver",
    class {
      constructor(callback: () => void) {
        resizeCallbacks.push(callback);
      }
      observe() {}
      disconnect() {}
    },
  );
});

function Slot() {
  const ref = usePortalSlot({
    api: {
      id: "scroll-test",
      component: "sidebar",
      onDidParametersChange: () => ({ dispose() {} }),
    },
    params: {},
  } as unknown as IDockviewPanelProps);
  return <div ref={ref} />;
}

afterEach(() => {
  panelPortalManager.release("scroll-test");
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

function attachScrollablePanel() {
  const view = render(<Slot />);
  const entry = panelPortalManager.get("scroll-test")!;
  const scroller = document.createElement("div");
  Object.defineProperties(scroller, {
    scrollHeight: { value: 100, configurable: true },
    clientHeight: { value: 100 },
  });
  entry.element.appendChild(scroller);
  scroller.scrollTop = 80;
  scroller.dispatchEvent(new Event("scroll"));
  view.unmount();
  scroller.scrollTop = 0;
  return scroller;
}

describe("portal scroll restoration", () => {
  it("releases a clamped restore before explicit upward navigation", () => {
    const scroller = attachScrollablePanel();
    const view = render(<Slot />);
    expect(scroller.scrollTop).toBe(80);
    releasePortalScrollRestoration(scroller);
    scroller.scrollTop = 10;
    Object.defineProperty(scroller, "scrollHeight", { value: 200 });
    resizeCallbacks.forEach((callback) => callback());
    expect(scroller.scrollTop).toBe(10);
    view.unmount();
  });

  it("does not apply an imminent restore after explicit navigation", () => {
    const scroller = attachScrollablePanel();
    scroller.scrollTop = 10;
    releasePortalScrollRestoration(scroller);
    scroller.scrollTop = 0;
    const view = render(<Slot />);
    expect(scroller.scrollTop).toBe(0);
    view.unmount();
  });

  it("restores a snapshot after the explicit navigation window expires", () => {
    vi.useFakeTimers();
    const scroller = attachScrollablePanel();
    releasePortalScrollRestoration(scroller);
    scroller.scrollTop = 80;
    releasePortalScrollRestoration(scroller);
    scroller.scrollTop = 0;
    vi.advanceTimersByTime(2001);
    const view = render(<Slot />);
    expect(scroller.scrollTop).toBe(80);
    view.unmount();
  });
});
