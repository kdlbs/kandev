import { act, cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SessionPanelContent } from "@kandev/ui/pannel-session";

let resizeCallback: ResizeObserverCallback | undefined;
const frameCallbacks: FrameRequestCallback[] = [];

class ControllableResizeObserver {
  constructor(callback: ResizeObserverCallback) {
    resizeCallback = callback;
  }

  observe() {}
  disconnect() {}
  unobserve() {}
  takeRecords(): ResizeObserverEntry[] {
    return [];
  }
}

function emitResize(width: number, height: number) {
  if (!resizeCallback) throw new Error("ResizeObserver callback did not register");
  const entry = {
    contentRect: { width, height },
  } as ResizeObserverEntry;
  act(() => resizeCallback?.([entry], {} as ResizeObserver));
}

function flushFrame() {
  const callback = frameCallbacks.shift();
  if (!callback) throw new Error("requestAnimationFrame callback did not register");
  act(() => callback(0));
}

function renderScrollContainer() {
  const result = render(
    <SessionPanelContent>
      <div />
    </SessionPanelContent>,
  );
  const scrollContainer = result.container.querySelector<HTMLElement>(".overflow-y-auto");
  if (!scrollContainer) throw new Error("SessionPanelContent scroll container did not render");
  return { ...result, scrollContainer };
}

beforeEach(() => {
  resizeCallback = undefined;
  frameCallbacks.length = 0;
  vi.stubGlobal("ResizeObserver", ControllableResizeObserver);
  vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
    frameCallbacks.push(callback);
    return frameCallbacks.length;
  });
});

afterEach(() => {
  cleanup();
  resizeCallback = undefined;
  frameCallbacks.length = 0;
  vi.unstubAllGlobals();
});

describe("SessionPanelContent scroll restoration", () => {
  it("restores the saved offset when the browser leaves scrollTop unchanged", () => {
    const { scrollContainer } = renderScrollContainer();
    scrollContainer.scrollTop = 184;
    act(() => scrollContainer.dispatchEvent(new Event("scroll")));

    emitResize(0, 0);
    scrollContainer.scrollTop = 0;
    emitResize(100, 100);
    flushFrame();

    expect(scrollContainer.scrollTop).toBe(184);
  });

  it("does not overwrite a newer app or user scroll before the restore frame", () => {
    const { scrollContainer } = renderScrollContainer();
    scrollContainer.scrollTop = 184;
    act(() => scrollContainer.dispatchEvent(new Event("scroll")));

    emitResize(0, 0);
    scrollContainer.scrollTop = 0;
    emitResize(100, 100);

    // Model a newer synchronous scroll owner. It deliberately does not emit
    // another event, so the stale-restore guard is the behavior under test.
    scrollContainer.scrollTop = 263;
    flushFrame();

    expect(scrollContainer.scrollTop).toBe(263);
  });
});
