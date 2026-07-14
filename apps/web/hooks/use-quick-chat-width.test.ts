import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import {
  clampQuickChatWidth,
  QUICK_CHAT_WIDTH_LIMITS,
  useQuickChatWidth,
} from "./use-quick-chat-width";

function mouseDown(clientX: number, dialog: HTMLElement) {
  const handle = document.createElement("button");
  dialog.appendChild(handle);
  return {
    clientX,
    currentTarget: handle,
    preventDefault: () => {},
  } as unknown as React.MouseEvent;
}

function mouseMove(clientX: number) {
  document.dispatchEvent(new MouseEvent("mousemove", { clientX, bubbles: true }));
}

function mouseUp() {
  document.dispatchEvent(new MouseEvent("mouseup", { bubbles: true }));
}

describe("quick chat dialog width", () => {
  beforeEach(() => {
    window.localStorage.clear();
    Object.defineProperty(window, "innerWidth", { configurable: true, value: 1200 });
    document.body.style.cursor = "";
    document.body.style.userSelect = "";
  });

  it("defaults to 80 percent of the viewport", () => {
    const { result } = renderHook(() => useQuickChatWidth());

    expect(result.current.width).toBe(960);
  });

  it("restores and clamps a persisted width", () => {
    window.localStorage.setItem(QUICK_CHAT_WIDTH_LIMITS.storageKey, JSON.stringify(5000));
    const { result } = renderHook(() => useQuickChatWidth());

    expect(result.current.width).toBe(1200 - QUICK_CHAT_WIDTH_LIMITS.viewportMargin);
  });

  it("grows symmetrically when the right edge moves right", () => {
    const { result } = renderHook(() => useQuickChatWidth());
    const dialog = document.createElement("div");

    act(() => result.current.rightResizeHandleProps.onMouseDown(mouseDown(900, dialog)));
    act(() => mouseMove(950));

    expect(dialog.style.getPropertyValue("--quick-chat-width")).toBe("1060px");
    expect(result.current.width).toBe(960);
    act(() => mouseUp());
    expect(result.current.width).toBe(1060);
  });

  it("grows symmetrically when the left edge moves left", () => {
    const { result } = renderHook(() => useQuickChatWidth());
    const dialog = document.createElement("div");

    act(() => result.current.leftResizeHandleProps.onMouseDown(mouseDown(300, dialog)));
    act(() => mouseMove(250));

    expect(dialog.style.getPropertyValue("--quick-chat-width")).toBe("1060px");
    expect(result.current.width).toBe(960);
    act(() => mouseUp());
    expect(result.current.width).toBe(1060);
  });

  it("persists the final width on mouseup and restores it after remount", () => {
    const first = renderHook(() => useQuickChatWidth());
    const dialog = document.createElement("div");
    act(() => first.result.current.rightResizeHandleProps.onMouseDown(mouseDown(900, dialog)));
    act(() => mouseMove(950));
    act(() => mouseUp());
    first.unmount();

    const second = renderHook(() => useQuickChatWidth());
    expect(second.result.current.width).toBe(1060);
  });

  it("clamps to the minimum and current viewport", () => {
    expect(clampQuickChatWidth(100, 1200)).toBe(QUICK_CHAT_WIDTH_LIMITS.minWidth);
    expect(clampQuickChatWidth(5000, 900)).toBe(900 - QUICK_CHAT_WIDTH_LIMITS.viewportMargin);
  });
});
