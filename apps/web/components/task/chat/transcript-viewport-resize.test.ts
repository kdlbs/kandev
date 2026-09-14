import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useTranscriptViewportResize } from "./transcript-viewport-resize";

const observers = new Set<() => void>();
beforeEach(() => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      constructor(private callback: () => void) {}
      observe() {
        observers.add(this.callback);
      }
      disconnect() {
        observers.delete(this.callback);
      }
    },
  );
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  observers.clear();
});

function setup(
  options: { enabled?: boolean; pending?: boolean; locked?: boolean; visible?: boolean } = {},
) {
  const element = document.createElement("div");
  const size = { width: 360, height: 500, content: 2000 };
  Object.defineProperties(element, {
    clientHeight: { get: () => size.height },
    clientWidth: { get: () => size.width },
    scrollHeight: { get: () => size.content },
  });
  element.scrollTop = 1500;
  const hook = renderHook(
    (props) =>
      useTranscriptViewportResize({
        scrollRef: { current: element },
        sessionId: "a",
        enabled: props.enabled ?? true,
        isVisible: props.visible ?? true,
        initialPlacementPending: props.pending ?? false,
        isProgrammaticScrollLocked: () => props.locked ?? false,
      }),
    { initialProps: options },
  );
  return {
    ...hook,
    element,
    size,
    resize: () => act(() => observers.forEach((publish) => publish())),
  };
}

describe("native transcript viewport allocation", () => {
  // @covers AC-UI-THREADS-DECK-005.6
  it("follows the bottom when composer disclosure shrinks the viewport", () => {
    const { element, size, resize } = setup();
    size.height = 300;
    resize();
    expect(element.scrollTop).toBe(1700);
  });

  it("keeps the prior bottom snapshot across a resize-generated scroll event", () => {
    const { element, size, resize } = setup();
    size.height = 300;
    element.dispatchEvent(new Event("scroll"));
    resize();
    expect(element.scrollTop).toBe(1700);
  });

  it("preserves the reader after a real scroll into history", () => {
    const { element, size, resize } = setup();
    element.scrollTop = 600;
    element.dispatchEvent(new Event("scroll"));
    size.height = 300;
    resize();
    expect(element.scrollTop).toBe(600);
    size.height = 500;
    resize();
    expect(element.scrollTop).toBe(600);
  });

  it.each([{ enabled: false }, { pending: true }, { locked: true }, { visible: false }])(
    "respects frozen, initial placement, programmatic navigation, and hidden ownership: %j",
    (options) => {
      const { element, size, resize } = setup(options);
      size.height = 300;
      resize();
      expect(element.scrollTop).toBe(1500);
    },
  );

  it("leaves width reflow to existing message placement and releases observation", () => {
    const { element, size, resize, unmount } = setup();
    size.width = 500;
    size.height = 300;
    resize();
    expect(element.scrollTop).toBe(1500);
    unmount();
    expect(observers.size).toBe(0);
  });
});
