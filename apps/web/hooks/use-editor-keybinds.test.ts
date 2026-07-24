import { describe, it, expect, vi, afterEach } from "vitest";
import { renderHook, cleanup } from "@testing-library/react";
import type { DockviewApi } from "dockview-react";

const mockToggleAppSidebar = vi.fn();
const mockToggleBottomTerminal = vi.fn();

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({
    getState: () => ({
      userSettings: { keyboardShortcuts: {} },
      bottomTerminal: { isOpen: false },
      toggleAppSidebar: mockToggleAppSidebar,
      toggleBottomTerminal: mockToggleBottomTerminal,
      tasks: { activeTaskId: null, activeSessionId: null },
      environmentIdBySessionId: {},
    }),
  }),
}));

import { useEditorKeybinds } from "./use-editor-keybinds";
import { useDockviewStore } from "@/lib/state/dockview-store";

function pressKey(
  key: string,
  init: KeyboardEventInit = {},
  target: EventTarget = window,
): KeyboardEvent {
  const event = new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true, ...init });
  target.dispatchEvent(event);
  return event;
}

/**
 * Minimal fake `DockviewApi` exercising only the tab-navigation branch of
 * `useEditorKeybinds`: two panels in the active panel's group, so
 * `handleTabNavigation` can find the "next" panel and call its
 * `api.setActive()`.
 */
function makeFakeApi(activateSpy: (id: string) => void): DockviewApi {
  type FakePanel = { id: string; api: { setActive: () => void }; group?: { panels: FakePanel[] } };
  const panelA: FakePanel = { id: "panel-a", api: { setActive: () => activateSpy("panel-a") } };
  const panelB: FakePanel = { id: "panel-b", api: { setActive: () => activateSpy("panel-b") } };
  const group = { panels: [panelA, panelB] };
  panelA.group = group;
  panelB.group = group;

  return {
    activePanel: panelA,
  } as unknown as DockviewApi;
}

describe("useEditorKeybinds", () => {
  afterEach(() => {
    cleanup();
    useDockviewStore.setState({ api: null });
    mockToggleAppSidebar.mockClear();
    mockToggleBottomTerminal.mockClear();
  });

  it("does not run the editor action (tab navigation) when the event is already defaultPrevented", () => {
    // Regression: `useEditorKeybinds`'s capture-phase handler guards on
    // `event.defaultPrevented` so a combo already handled by another
    // capture-phase core dispatcher or a plugin keybinding doesn't also run
    // the editor's tab-navigation/terminal-toggle logic (double action for a
    // single keypress). This exercises that guard directly, at the hook
    // level, rather than only through `useKeyboardShortcut`.
    const activateSpy = vi.fn();
    useDockviewStore.setState({ api: makeFakeApi(activateSpy) });

    // Register this capture-phase listener BEFORE mounting the hook so it
    // runs first (capture-phase listeners on the same target fire in
    // registration order) — mirroring a real prior capture dispatcher (a
    // core shortcut or plugin keybinding) that already handled the event.
    const preventDefaultListener = (event: KeyboardEvent) => {
      event.preventDefault();
    };
    window.addEventListener("keydown", preventDefaultListener, { capture: true });

    renderHook(() => useEditorKeybinds());

    try {
      pressKey("]", { ctrlKey: true, shiftKey: true, code: "BracketRight" });

      expect(activateSpy).not.toHaveBeenCalled();
    } finally {
      window.removeEventListener("keydown", preventDefaultListener, { capture: true });
    }
  });

  it("runs the editor action (tab navigation) for a non-prevented matching event", () => {
    const activateSpy = vi.fn();
    useDockviewStore.setState({ api: makeFakeApi(activateSpy) });

    renderHook(() => useEditorKeybinds());

    pressKey("]", { ctrlKey: true, shiftKey: true, code: "BracketRight" });

    expect(activateSpy).toHaveBeenCalledWith("panel-b");
  });
});
