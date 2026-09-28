import { afterEach, describe, expect, it, vi } from "vitest";
import type { MouseEvent, RefObject } from "react";
import { routePanelClick, routePanelMouseDown } from "./route-panel-mouse-down";

function eventWithTarget(target: { closest: ReturnType<typeof vi.fn> }) {
  return { target } as unknown as MouseEvent<HTMLDivElement>;
}

function panelRef() {
  const focus = vi.fn();
  const element = { focus } as unknown as HTMLDivElement;
  return {
    element,
    focus,
    ref: { current: element } as unknown as RefObject<HTMLDivElement | null>,
  };
}

afterEach(() => vi.unstubAllGlobals());

describe("Quick Chat route panel focus", () => {
  it("focuses immediately on a non-interactive mouse down and preserves controls", () => {
    const { focus, ref } = panelRef();
    const content = eventWithTarget({ closest: vi.fn().mockReturnValue(null) });
    routePanelMouseDown(content, ref);
    expect(focus).toHaveBeenCalledWith({ preventScroll: true });

    focus.mockClear();
    const control = eventWithTarget({ closest: vi.fn().mockReturnValue({}) });
    routePanelMouseDown(control, ref);
    expect(focus).not.toHaveBeenCalled();
  });

  it("defers non-interactive click focus and preserves controls", () => {
    const callbacks: FrameRequestCallback[] = [];
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callbacks.push(callback);
      return callbacks.length;
    });
    const { focus, ref } = panelRef();
    const content = eventWithTarget({ closest: vi.fn().mockReturnValue(null) });
    routePanelClick(content, ref);
    expect(focus).not.toHaveBeenCalled();
    callbacks[0]?.(0);
    expect(focus).toHaveBeenCalledWith({ preventScroll: true });

    focus.mockClear();
    const control = eventWithTarget({ closest: vi.fn().mockReturnValue({}) });
    routePanelClick(control, ref);
    expect(callbacks).toHaveLength(1);
    expect(focus).not.toHaveBeenCalled();
  });
});
