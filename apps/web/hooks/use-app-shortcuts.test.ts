import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, cleanup } from "@testing-library/react";
import type { StoredShortcutOverrides } from "@/lib/keyboard/shortcut-overrides";

const mockToggleAppSidebar = vi.fn();
let mockShortcuts: StoredShortcutOverrides = {};

function buildState() {
  return {
    userSettings: { keyboardShortcuts: mockShortcuts },
    toggleAppSidebar: mockToggleAppSidebar,
  };
}

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({ getState: () => buildState() }),
}));

import { useAppShortcuts } from "./use-app-shortcuts";

/** Bind TOGGLE_SIDEBAR to Cmd/Ctrl+B (it is unbound by default). */
const CTRL_B_OVERRIDE: StoredShortcutOverrides = {
  TOGGLE_SIDEBAR: { key: "b", modifiers: { ctrlOrCmd: true } },
};

function pressCtrlB(target: EventTarget = window): KeyboardEvent {
  const event = new KeyboardEvent("keydown", {
    key: "b",
    ctrlKey: true,
    bubbles: true,
    cancelable: true,
  });
  target.dispatchEvent(event);
  return event;
}

describe("useAppShortcuts", () => {
  beforeEach(() => {
    mockToggleAppSidebar.mockClear();
    mockShortcuts = {};
  });

  // renderHook attaches a window listener; unmount it between tests so stale
  // listeners from earlier tests don't fire on later dispatches.
  afterEach(() => cleanup());

  it("does nothing by default — TOGGLE_SIDEBAR is unbound", () => {
    renderHook(() => useAppShortcuts());
    pressCtrlB();
    expect(mockToggleAppSidebar).not.toHaveBeenCalled();
  });

  it("toggles the sidebar when the shortcut is bound to Cmd/Ctrl+B", () => {
    mockShortcuts = CTRL_B_OVERRIDE;
    renderHook(() => useAppShortcuts());

    const event = pressCtrlB();
    expect(mockToggleAppSidebar).toHaveBeenCalledTimes(1);
    expect(event.defaultPrevented).toBe(true);
  });

  it("ignores the shortcut while typing in an editable element", () => {
    mockShortcuts = CTRL_B_OVERRIDE;
    renderHook(() => useAppShortcuts());

    const input = document.createElement("input");
    document.body.appendChild(input);
    pressCtrlB(input);
    expect(mockToggleAppSidebar).not.toHaveBeenCalled();
    input.remove();
  });

  it("removes its listener on unmount", () => {
    mockShortcuts = CTRL_B_OVERRIDE;
    const { unmount } = renderHook(() => useAppShortcuts());
    unmount();

    pressCtrlB();
    expect(mockToggleAppSidebar).not.toHaveBeenCalled();
  });
});
