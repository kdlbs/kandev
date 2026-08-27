import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, cleanup } from "@testing-library/react";
import type { StoredShortcutOverrides } from "@/lib/keyboard/shortcut-overrides";

const mockToggleAppSidebar = vi.fn();
const mockSetWorkspacePickerOpen = vi.fn();
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
    setWorkspacePickerOpen: mockSetWorkspacePickerOpen,
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

function pressWorkspacePicker(
  modifier: "ctrlKey" | "metaKey",
  target: EventTarget = window,
): KeyboardEvent {
  const init: KeyboardEventInit = {
    key: "e",
    bubbles: true,
    cancelable: true,
  };
  init[modifier] = true;
  const event = new KeyboardEvent("keydown", init);
  target.dispatchEvent(event);
  return event;
}

/**
 * jsdom has no `window.matchMedia`; the WORKSPACE_PICKER branch queries it to
 * suppress the shortcut where the sidebar is display:none (below md/768px).
 * Default every test to a desktop-sized answer.
 */
function stubMatchMedia(matches: boolean) {
  window.matchMedia = vi.fn(
    (query: string) => ({ media: query, matches }) as unknown as MediaQueryList,
  );
}

// Cover both `ctrlOrCmd` branches: Ctrl on Windows/Linux, Cmd (metaKey) on macOS.
const pressCtrlB = (target?: EventTarget) => pressB("ctrlKey", target);
const pressMetaB = (target?: EventTarget) => pressB("metaKey", target);

function resetShortcutMocks() {
  mockToggleAppSidebar.mockClear();
  mockSetWorkspacePickerOpen.mockClear();
  mockOpenMode.mockClear();
  mockShortcuts = {};
  mockPathname = "/t/task-1";
  mockActiveTaskId = "task-1";
  mockActiveSessionId = "session-1";
  mockSessionTaskId = "task-1";
  stubMatchMedia(true);
}

describe("useAppShortcuts", () => {
  beforeEach(resetShortcutMocks);

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

describe("useAppShortcuts — workspace picker", () => {
  beforeEach(resetShortcutMocks);

  afterEach(() => cleanup());

  it("opens the workspace picker on Cmd/Ctrl+E", () => {
    renderHook(() => useAppShortcuts());

    const event = pressWorkspacePicker("ctrlKey");

    expect(mockSetWorkspacePickerOpen).toHaveBeenCalledWith(true);
    expect(event.defaultPrevented).toBe(true);
  });

  it("opens the workspace picker with Cmd on macOS", () => {
    renderHook(() => useAppShortcuts());

    const event = pressWorkspacePicker("metaKey");

    expect(mockSetWorkspacePickerOpen).toHaveBeenCalledWith(true);
    expect(event.defaultPrevented).toBe(true);
  });

  it("ignores the workspace picker shortcut while typing in an editable element", () => {
    renderHook(() => useAppShortcuts());
    const input = document.createElement("input");
    document.body.appendChild(input);

    pressWorkspacePicker("ctrlKey", input);

    expect(mockSetWorkspacePickerOpen).not.toHaveBeenCalled();
    input.remove();
  });

  it("uses a stored workspace-picker override instead of its default", () => {
    mockShortcuts = {
      WORKSPACE_PICKER: { key: "w", modifiers: { ctrlOrCmd: true, alt: true } },
    };
    renderHook(() => useAppShortcuts());

    pressWorkspacePicker("ctrlKey");
    expect(mockSetWorkspacePickerOpen).not.toHaveBeenCalled();

    const event = new KeyboardEvent("keydown", {
      key: "w",
      ctrlKey: true,
      altKey: true,
      bubbles: true,
      cancelable: true,
    });
    window.dispatchEvent(event);

    expect(mockSetWorkspacePickerOpen).toHaveBeenCalledWith(true);
    expect(event.defaultPrevented).toBe(true);
  });

  it("does not toggle the sidebar when the picker shortcut fires", () => {
    mockShortcuts = CTRL_B_OVERRIDE;
    renderHook(() => useAppShortcuts());

    pressWorkspacePicker("ctrlKey");

    expect(mockToggleAppSidebar).not.toHaveBeenCalled();
  });

  it("is a no-op below the sidebar's md boundary and leaves the event alone", () => {
    // Below 768px the sidebar is display:none; opening the portaled menu there
    // would float it detached over the mobile nav.
    stubMatchMedia(false);
    renderHook(() => useAppShortcuts());

    // Built inline rather than via pressWorkspacePicker so the spy is attached
    // before dispatch: the contract is that the event is left entirely alone,
    // so neither half of the handled-event pair may fire. Asserting only
    // `defaultPrevented` would miss a stopPropagation moved above the guard,
    // which would silently starve every other listener on the combo.
    const event = new KeyboardEvent("keydown", {
      key: "e",
      ctrlKey: true,
      bubbles: true,
      cancelable: true,
    });
    const stopPropagation = vi.spyOn(event, "stopPropagation");
    window.dispatchEvent(event);

    expect(mockSetWorkspacePickerOpen).not.toHaveBeenCalled();
    expect(event.defaultPrevented).toBe(false);
    expect(stopPropagation).not.toHaveBeenCalled();
    expect(window.matchMedia).toHaveBeenCalledWith("(min-width: 768px)");
  });
});
