import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import {
  PREVIEW_TOOLBAR_POSITION_KEY,
  toolbarFractions,
  toolbarPixels,
  usePreviewToolbarPosition,
} from "./use-preview-toolbar-position";

const frame = { width: 1000, height: 600 };
const toolbar = { width: 400, height: 40 };

function element(width: number, height: number) {
  const node = document.createElement("div");
  Object.defineProperty(node, "clientWidth", { value: width, configurable: true });
  Object.defineProperty(node, "clientHeight", { value: height, configurable: true });
  Object.defineProperty(node, "offsetWidth", { value: width, configurable: true });
  Object.defineProperty(node, "offsetHeight", { value: height, configurable: true });
  return node;
}

function mouse(type: string, x: number, y: number) {
  document.dispatchEvent(new MouseEvent(type, { clientX: x, clientY: y, bubbles: true }));
}

afterEach(() => window.localStorage.clear());

describe("toolbar position geometry", () => {
  it("places the default at the top center, inside an 8 pixel margin", () => {
    expect(toolbarPixels({ x: 0.5, y: 0 }, frame, toolbar)).toEqual({ left: 300, top: 8 });
    expect(toolbarPixels({ x: 1, y: 1 }, frame, toolbar)).toEqual({ left: 592, top: 552 });
  });

  it("clamps a dragged position to the frame", () => {
    expect(toolbarFractions({ left: -50, top: 900 }, frame, toolbar)).toEqual({ x: 0, y: 1 });
    expect(toolbarFractions({ left: 300, top: 8 }, frame, toolbar)).toEqual({ x: 0.5, y: 0 });
  });

  it("keeps a toolbar wider than the frame at the left margin", () => {
    expect(toolbarPixels({ x: 1, y: 0 }, { width: 300, height: 600 }, toolbar)).toEqual({
      left: 8,
      top: 8,
    });
  });
});

describe("usePreviewToolbarPosition", () => {
  function setup() {
    const frameRef = { current: element(frame.width, frame.height) };
    const toolbarRef = { current: element(toolbar.width, toolbar.height) };
    return renderHook(() => usePreviewToolbarPosition(frameRef, toolbarRef));
  }

  function press(result: ReturnType<typeof setup>["result"], detail = 1) {
    act(() =>
      result.current.handleProps.onMouseDown({
        clientX: 0,
        clientY: 0,
        detail,
        preventDefault: () => undefined,
      } as React.MouseEvent),
    );
  }

  function drag(result: ReturnType<typeof setup>["result"], dx: number, dy: number) {
    press(result);
    act(() => mouse("mousemove", dx, dy));
    act(() => mouse("mouseup", dx, dy));
  }

  it("drags the toolbar and remembers where it was left", () => {
    const { result, unmount } = setup();
    expect(result.current.style).toEqual({ left: 300, top: 8 });

    drag(result, 200, 100);
    expect(result.current.style).toEqual({ left: 500, top: 108 });
    expect(JSON.parse(window.localStorage.getItem(PREVIEW_TOOLBAR_POSITION_KEY) ?? "{}")).toEqual({
      x: 492 / 584,
      y: 100 / 544,
    });
    unmount();

    expect(setup().result.current.style).toEqual({ left: 500, top: 108 });
  });

  it("resets on double-click and moves with arrow keys", () => {
    const { result } = setup();
    drag(result, 200, 100);
    press(result, 2);
    expect(result.current.style).toEqual({ left: 300, top: 8 });

    act(() =>
      result.current.handleProps.onKeyDown({
        key: "ArrowDown",
        preventDefault: () => undefined,
      } as React.KeyboardEvent),
    );
    expect(result.current.style).toEqual({ left: 300, top: 24 });
  });

  it("ignores a malformed stored position", () => {
    window.localStorage.setItem(PREVIEW_TOOLBAR_POSITION_KEY, '{"x":"left"}');
    expect(setup().result.current.style).toEqual({ left: 300, top: 8 });
  });
});
