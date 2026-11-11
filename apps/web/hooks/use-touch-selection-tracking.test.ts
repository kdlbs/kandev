import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { useTouchSelectionTracking } from "./use-touch-selection-tracking";

let root: HTMLDivElement;
let outside: HTMLParagraphElement;
let dock: HTMLButtonElement;
let frames: FrameRequestCallback[];

function select(node: Node, start: number, end: number) {
  const range = document.createRange();
  range.setStart(node, start);
  range.setEnd(node, end);
  const selection = window.getSelection()!;
  selection.removeAllRanges();
  selection.addRange(range);
  document.dispatchEvent(new Event("selectionchange"));
}

function flushFrames() {
  const pending = frames;
  frames = [];
  pending.forEach((frame) => frame(0));
}

beforeEach(() => {
  frames = [];
  vi.stubGlobal("requestAnimationFrame", (frame: FrameRequestCallback) => frames.push(frame));
  vi.stubGlobal("cancelAnimationFrame", () => {});
  root = document.createElement("div");
  root.textContent = "Selectable agent reply.";
  outside = document.createElement("p");
  outside.textContent = "Outside text.";
  const dockHost = document.createElement("div");
  dockHost.setAttribute("data-selection-action-dock", "");
  dock = document.createElement("button");
  dockHost.appendChild(dock);
  document.body.append(root, outside, dockHost);
});

afterEach(() => {
  window.getSelection()?.removeAllRanges();
  document.body.innerHTML = "";
  vi.unstubAllGlobals();
});

describe("useTouchSelectionTracking", () => {
  it("reports selections inside the root once per frame", () => {
    const onChange = vi.fn();
    renderHook(() => useTouchSelectionTracking({ current: root }, true, onChange));

    select(root.firstChild!, 0, 4);
    select(root.firstChild!, 0, 10);
    expect(onChange).not.toHaveBeenCalled();
    flushFrames();

    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange.mock.calls[0][0]?.toString()).toBe("Selectable");
  });

  it("reports null for collapsed, whitespace-only, and outside selections", () => {
    const onChange = vi.fn();
    renderHook(() => useTouchSelectionTracking({ current: root }, true, onChange));

    select(outside.firstChild!, 0, 7);
    flushFrames();
    select(root.firstChild!, 10, 11);
    flushFrames();
    select(root.firstChild!, 2, 2);
    flushFrames();

    expect(onChange.mock.calls.map(([value]) => value)).toEqual([null, null, null]);
  });

  it("keeps the current target while the selection action dock is pressed", () => {
    const onChange = vi.fn();
    renderHook(() => useTouchSelectionTracking({ current: root }, true, onChange));

    dock.dispatchEvent(new Event("pointerdown", { bubbles: true }));
    window.getSelection()!.removeAllRanges();
    document.dispatchEvent(new Event("selectionchange"));
    flushFrames();
    expect(onChange).not.toHaveBeenCalled();

    outside.dispatchEvent(new Event("pointerdown", { bubbles: true }));
    document.dispatchEvent(new Event("selectionchange"));
    flushFrames();
    expect(onChange).toHaveBeenCalledWith(null);
  });

  it("does not listen while disabled", () => {
    const onChange = vi.fn();
    renderHook(() => useTouchSelectionTracking({ current: root }, false, onChange));

    select(root.firstChild!, 0, 4);
    flushFrames();

    expect(onChange).not.toHaveBeenCalled();
  });
});
