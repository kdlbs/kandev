import { afterEach, describe, expect, it, vi } from "vitest";
import type { MouseEvent, RefObject } from "react";
import { routePanelClick, routePanelMouseDown } from "./route-panel-mouse-down";

function eventWithTarget(target: { closest: ReturnType<typeof vi.fn> }) {
  return { target } as unknown as MouseEvent<HTMLDivElement>;
}

function panelRef() {
  const focus = vi.fn();
  const element = { focus, contains: vi.fn().mockReturnValue(true) } as unknown as HTMLDivElement;
  return {
    element,
    focus,
    ref: { current: element } as unknown as RefObject<HTMLDivElement | null>,
  };
}

afterEach(() => vi.unstubAllGlobals());

describe("Quick Chat route panel focus", () => {
  it("preserves focus for portal events that bubble through the React panel", () => {
    const panel = document.createElement("div");
    const portal = document.createElement("div");
    const optionLabel = document.createElement("span");
    portal.append(optionLabel);
    const focus = vi.spyOn(panel, "focus");
    const frame = vi.fn();
    vi.stubGlobal("requestAnimationFrame", frame);
    const event = { target: optionLabel } as unknown as MouseEvent<HTMLDivElement>;
    const ref = { current: panel };

    routePanelMouseDown(event, ref);
    routePanelClick(event, ref);

    expect(focus).not.toHaveBeenCalled();
    expect(frame).not.toHaveBeenCalled();
  });

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
