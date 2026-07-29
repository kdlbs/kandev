import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, cleanup } from "@testing-library/react";
import type { StoredShortcutOverrides } from "@/lib/keyboard/shortcut-overrides";

const mockToggleAppSidebar = vi.fn();
const mockOpenMode = vi.fn();
let mockShortcuts: StoredShortcutOverrides = {};
let mockPathname = "/t/task-1";
let mockActiveTaskId: string | null = "task-1";
let mockActiveSessionId: string | null = "session-1";
let mockSessionTaskId: string | null = "task-1";

function buildState() {
  return {
    userSettings: { keyboardShortcuts: mockShortcuts },
    toggleAppSidebar: mockToggleAppSidebar,
    tasks: {
      activeTaskId: mockActiveTaskId,
      activeSessionId: mockActiveSessionId,
    },
    taskSessions: {
      items:
        mockActiveSessionId && mockSessionTaskId
          ? { [mockActiveSessionId]: { task_id: mockSessionTaskId } }
          : {},
    },
  };
}

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({ getState: () => buildState() }),
}));

vi.mock("@/lib/commands/command-registry", () => ({
  useCommandPanelOpen: () => ({ openMode: mockOpenMode }),
}));

vi.mock("@/lib/routing/client-router", () => ({
  usePathname: () => mockPathname,
}));

import { useAppShortcuts } from "./use-app-shortcuts";

/** Bind TOGGLE_SIDEBAR to Cmd/Ctrl+B (it is unbound by default). */
const CTRL_B_OVERRIDE: StoredShortcutOverrides = {
  TOGGLE_SIDEBAR: { key: "b", modifiers: { ctrlOrCmd: true } },
};

function pressB(modifier: "ctrlKey" | "metaKey", target: EventTarget = window): KeyboardEvent {
  const init: KeyboardEventInit = { key: "b", bubbles: true, cancelable: true };
  init[modifier] = true;
  const event = new KeyboardEvent("keydown", init);
  target.dispatchEvent(event);
  return event;
}

function pressContentSearch(
  modifier: "ctrlKey" | "metaKey",
  target: EventTarget = window,
): KeyboardEvent {
  const init: KeyboardEventInit = {
    key: "f",
    shiftKey: true,
    bubbles: true,
    cancelable: true,
  };
  init[modifier] = true;
  const event = new KeyboardEvent("keydown", init);
  target.dispatchEvent(event);
  return event;
}

// Cover both `ctrlOrCmd` branches: Ctrl on Windows/Linux, Cmd (metaKey) on macOS.
const pressCtrlB = (target?: EventTarget) => pressB("ctrlKey", target);
const pressMetaB = (target?: EventTarget) => pressB("metaKey", target);

describe("useAppShortcuts", () => {
  beforeEach(() => {
    mockToggleAppSidebar.mockClear();
    mockOpenMode.mockClear();
    mockShortcuts = {};
    mockPathname = "/t/task-1";
    mockActiveTaskId = "task-1";
    mockActiveSessionId = "session-1";
    mockSessionTaskId = "task-1";
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

  it("toggles the sidebar with Cmd (metaKey) too — the macOS path", () => {
    mockShortcuts = CTRL_B_OVERRIDE;
    renderHook(() => useAppShortcuts());

    const event = pressMetaB();
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

  it("opens task content search before an editable target handles Cmd/Ctrl+Shift+F", () => {
    renderHook(() => useAppShortcuts());
    const input = document.createElement("input");
    document.body.appendChild(input);

    const event = pressContentSearch("ctrlKey", input);

    expect(mockOpenMode).toHaveBeenCalledWith("search-content");
    expect(event.defaultPrevented).toBe(true);
    input.remove();
  });

  it("opens task content search with Cmd on macOS", () => {
    renderHook(() => useAppShortcuts());

    const event = pressContentSearch("metaKey");

    expect(mockOpenMode).toHaveBeenCalledWith("search-content");
    expect(event.defaultPrevented).toBe(true);
  });

  it("does not intercept content search outside an active task workbench", () => {
    mockPathname = "/";
    renderHook(() => useAppShortcuts());

    const event = pressContentSearch("ctrlKey");

    expect(mockOpenMode).not.toHaveBeenCalled();
    expect(event.defaultPrevented).toBe(false);
  });

  it("uses a stored content-search override instead of its default", () => {
    mockShortcuts = {
      CONTENT_SEARCH: { key: "g", modifiers: { ctrlOrCmd: true, shift: true } },
    };
    renderHook(() => useAppShortcuts());

    pressContentSearch("ctrlKey");
    expect(mockOpenMode).not.toHaveBeenCalled();

    const event = new KeyboardEvent("keydown", {
      key: "g",
      ctrlKey: true,
      shiftKey: true,
      bubbles: true,
      cancelable: true,
    });
    window.dispatchEvent(event);

    expect(mockOpenMode).toHaveBeenCalledWith("search-content");
    expect(event.defaultPrevented).toBe(true);
  });

  it("leaves panel-local Cmd/Ctrl+F untouched", () => {
    renderHook(() => useAppShortcuts());
    const event = new KeyboardEvent("keydown", {
      key: "f",
      ctrlKey: true,
      bubbles: true,
      cancelable: true,
    });

    window.dispatchEvent(event);

    expect(mockOpenMode).not.toHaveBeenCalled();
    expect(event.defaultPrevented).toBe(false);
  });
});
