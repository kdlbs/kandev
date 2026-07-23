import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, cleanup } from "@testing-library/react";
import type { StoredShortcutOverrides } from "@/lib/keyboard/shortcut-overrides";
import type { PluginRecord } from "@/lib/types/plugins";
import { pluginRegistry } from "@/lib/plugins/registry";

let mockOverrides: StoredShortcutOverrides = {};
let mockItems: PluginRecord[] = [];
const mockToggleAppSidebar = vi.fn();

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({
    getState: () => ({
      userSettings: { keyboardShortcuts: mockOverrides },
      toggleAppSidebar: mockToggleAppSidebar,
    }),
  }),
}));

vi.mock("@/hooks/domains/plugins/use-plugins", () => ({
  usePlugins: () => ({ items: mockItems, loaded: true, loading: false, error: null }),
}));

import { usePluginShortcuts } from "./use-plugin-shortcuts";
import { useAppShortcuts } from "./use-app-shortcuts";
import { useKeyboardShortcut } from "./use-keyboard-shortcut";
import type { KeyboardShortcut } from "@/lib/keyboard/constants";

const PLUGIN_ID = "session-cost";

function makePlugin(overrides: Partial<PluginRecord> = {}): PluginRecord {
  return {
    id: PLUGIN_ID,
    api_version: 1,
    version: "1.0.0",
    display_name: "Session Cost",
    description: "",
    author: "",
    categories: [],
    capabilities: {},
    status: "active",
    install_path: "",
    signed: true,
    installed_at: "",
    restart_count: 0,
    ...overrides,
  };
}

function pressKey(
  key: string,
  init: KeyboardEventInit = {},
  target: EventTarget = window,
): KeyboardEvent {
  const event = new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true, ...init });
  target.dispatchEvent(event);
  return event;
}

const TOGGLE_KEYBINDING = { id: "toggle", default: "mod+shift+k", description: "Toggle" };

function withToggleKeybinding(overrides: Partial<PluginRecord> = {}): PluginRecord {
  return makePlugin({ ui: { keybindings: [TOGGLE_KEYBINDING] }, ...overrides });
}

describe("usePluginShortcuts", () => {
  beforeEach(() => {
    mockOverrides = {};
    mockItems = [];
    mockToggleAppSidebar.mockClear();
  });

  afterEach(() => {
    cleanup();
    pluginRegistry.unregisterPlugin(PLUGIN_ID);
    pluginRegistry.unregisterPlugin("plugin-a");
    pluginRegistry.unregisterPlugin("plugin-b");
  });

  it("invokes the bound handler when the manifest default combo is pressed", () => {
    mockItems = [withToggleKeybinding()];
    const handler = vi.fn();
    pluginRegistry.forPlugin(PLUGIN_ID).registerKeybinding("toggle", handler);

    renderHook(() => usePluginShortcuts());
    const event = pressKey("k", { ctrlKey: true, shiftKey: true });

    expect(handler).toHaveBeenCalledTimes(1);
    expect(event.defaultPrevented).toBe(true);
  });

  it("does not invoke the handler when no key matches", () => {
    mockItems = [withToggleKeybinding()];
    const handler = vi.fn();
    pluginRegistry.forPlugin(PLUGIN_ID).registerKeybinding("toggle", handler);

    renderHook(() => usePluginShortcuts());
    pressKey("j", { ctrlKey: true });

    expect(handler).not.toHaveBeenCalled();
  });

  it("skips dispatch while typing in an editable element", () => {
    mockItems = [withToggleKeybinding()];
    const handler = vi.fn();
    pluginRegistry.forPlugin(PLUGIN_ID).registerKeybinding("toggle", handler);

    renderHook(() => usePluginShortcuts());
    const input = document.createElement("input");
    document.body.appendChild(input);
    pressKey("k", { ctrlKey: true, shiftKey: true }, input);
    input.remove();

    expect(handler).not.toHaveBeenCalled();
  });

  it("does nothing when the plugin bundle has not registered a handler yet", () => {
    mockItems = [withToggleKeybinding()];

    renderHook(() => usePluginShortcuts());
    expect(() => pressKey("k", { ctrlKey: true, shiftKey: true })).not.toThrow();
  });

  it("respects a user override combo instead of the manifest default", () => {
    mockItems = [withToggleKeybinding()];
    mockOverrides = {
      [`plugin:${PLUGIN_ID}:toggle`]: { key: "j", modifiers: { ctrlOrCmd: true } },
    };
    const handler = vi.fn();
    pluginRegistry.forPlugin(PLUGIN_ID).registerKeybinding("toggle", handler);

    renderHook(() => usePluginShortcuts());
    pressKey("k", { ctrlKey: true, shiftKey: true }); // manifest default — no longer bound
    expect(handler).not.toHaveBeenCalled();

    pressKey("j", { ctrlKey: true }); // user override — now bound
    expect(handler).toHaveBeenCalledTimes(1);
  });

  it("dispatches to every plugin bound to the same combo, in registration order", () => {
    mockItems = [
      makePlugin({
        id: "plugin-a",
        display_name: "Plugin A",
        ui: { keybindings: [{ id: "open", default: "mod+j", description: "Open" }] },
      }),
      makePlugin({
        id: "plugin-b",
        display_name: "Plugin B",
        ui: { keybindings: [{ id: "open", default: "mod+j", description: "Open" }] },
      }),
    ];
    const callOrder: string[] = [];
    pluginRegistry.forPlugin("plugin-a").registerKeybinding("open", () => callOrder.push("a"));
    pluginRegistry.forPlugin("plugin-b").registerKeybinding("open", () => callOrder.push("b"));

    renderHook(() => usePluginShortcuts());
    pressKey("j", { ctrlKey: true });

    expect(callOrder).toEqual(["a", "b"]);
  });

  it("removes its listener on unmount", () => {
    mockItems = [withToggleKeybinding()];
    const handler = vi.fn();
    pluginRegistry.forPlugin(PLUGIN_ID).registerKeybinding("toggle", handler);

    const { unmount } = renderHook(() => usePluginShortcuts());
    unmount();
    pressKey("k", { ctrlKey: true, shiftKey: true });

    expect(handler).not.toHaveBeenCalled();
  });
});

describe("usePluginShortcuts handler exceptions", () => {
  beforeEach(() => {
    mockOverrides = {};
    mockItems = [];
    mockToggleAppSidebar.mockClear();
  });

  afterEach(() => {
    cleanup();
    pluginRegistry.unregisterPlugin("plugin-a");
    pluginRegistry.unregisterPlugin("plugin-b");
  });

  it("logs and continues past a throwing handler so later handlers still run", () => {
    mockItems = [
      makePlugin({
        id: "plugin-a",
        display_name: "Plugin A",
        ui: { keybindings: [{ id: "open", default: "mod+j", description: "Open" }] },
      }),
      makePlugin({
        id: "plugin-b",
        display_name: "Plugin B",
        ui: { keybindings: [{ id: "open", default: "mod+j", description: "Open" }] },
      }),
    ];
    const secondHandler = vi.fn();
    pluginRegistry.forPlugin("plugin-a").registerKeybinding("open", () => {
      throw new Error("boom");
    });
    pluginRegistry.forPlugin("plugin-b").registerKeybinding("open", secondHandler);
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});

    renderHook(() => usePluginShortcuts());
    expect(() => pressKey("j", { ctrlKey: true })).not.toThrow();

    expect(secondHandler).toHaveBeenCalledTimes(1);
    expect(consoleErrorSpy).toHaveBeenCalledWith(
      expect.stringContaining("plugin-a"),
      expect.any(Error),
    );

    consoleErrorSpy.mockRestore();
  });
});

describe("core vs plugin shortcut precedence", () => {
  beforeEach(() => {
    mockOverrides = {};
    mockItems = [];
    mockToggleAppSidebar.mockClear();
  });

  afterEach(() => {
    cleanup();
    pluginRegistry.unregisterPlugin(PLUGIN_ID);
  });

  it("fires only the core action when a combo matches both a core shortcut and a plugin keybinding", () => {
    mockOverrides = { TOGGLE_SIDEBAR: { key: "b", modifiers: { ctrlOrCmd: true } } };
    mockItems = [
      makePlugin({
        ui: { keybindings: [{ id: "open", default: "mod+b", description: "Open" }] },
      }),
    ];
    const pluginHandler = vi.fn();
    pluginRegistry.forPlugin(PLUGIN_ID).registerKeybinding("open", pluginHandler);

    renderHook(() => {
      useAppShortcuts();
      usePluginShortcuts();
    });
    const event = pressKey("b", { ctrlKey: true });

    expect(mockToggleAppSidebar).toHaveBeenCalledTimes(1);
    expect(pluginHandler).not.toHaveBeenCalled();
    expect(event.defaultPrevented).toBe(true);
  });

  it("still fires the plugin handler for a plugin-only combo (no core shortcut match)", () => {
    mockItems = [withToggleKeybinding()];
    const handler = vi.fn();
    pluginRegistry.forPlugin(PLUGIN_ID).registerKeybinding("toggle", handler);

    renderHook(() => {
      useAppShortcuts();
      usePluginShortcuts();
    });
    pressKey("k", { ctrlKey: true, shiftKey: true });

    expect(handler).toHaveBeenCalledTimes(1);
    expect(mockToggleAppSidebar).not.toHaveBeenCalled();
  });

  it("fires only the plugin action when a combo matches both a plugin keybinding and a per-component core shortcut bound via useKeyboardShortcut", () => {
    // Regression for Greptile P1 (PR #1895, comment 3638620657): a
    // per-component core shortcut registered through `useKeyboardShortcut`
    // runs in the bubble phase, after this capture-phase plugin dispatcher.
    // Without `useKeyboardShortcut` respecting `event.defaultPrevented`, the
    // plugin handler AND the core action would both fire for the same combo.
    mockItems = [withToggleKeybinding()];
    const pluginHandler = vi.fn();
    pluginRegistry.forPlugin(PLUGIN_ID).registerKeybinding("toggle", pluginHandler);

    const coreShortcut: KeyboardShortcut = {
      key: "k",
      modifiers: { ctrlOrCmd: true, shift: true },
    };
    const coreCallback = vi.fn();

    renderHook(() => {
      // usePluginShortcuts registers its capture-phase listener on mount,
      // which — like the real app (`components/global-commands.tsx`) —
      // fires before the bubble-phase useKeyboardShortcut listener below.
      usePluginShortcuts();
      useKeyboardShortcut(coreShortcut, coreCallback);
    });

    const event = pressKey("k", { ctrlKey: true, shiftKey: true });

    expect(pluginHandler).toHaveBeenCalledTimes(1);
    expect(coreCallback).not.toHaveBeenCalled();
    expect(event.defaultPrevented).toBe(true);
  });
});
