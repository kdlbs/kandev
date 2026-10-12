import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, renderHook } from "@testing-library/react";
import type { StoredShortcutOverrides } from "@/lib/keyboard/shortcut-overrides";
import { pluginRegistry } from "@/lib/plugins/registry";
import * as navigation from "@/lib/navigation/resolve-destinations";
import { useIntegrationShortcuts } from "./use-integration-shortcuts";
import { useKeyboardShortcut } from "./use-keyboard-shortcut";
import { SHORTCUTS } from "@/lib/keyboard/constants";

const mocks = vi.hoisted(() => ({
  push: vi.fn(),
  context: { workspaceId: "workspace-1", inOffice: false },
  overrides: {} as StoredShortcutOverrides,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({
    getState: () => ({ userSettings: { keyboardShortcuts: mocks.overrides } }),
  }),
}));
vi.mock("@/hooks/use-app-destinations", () => ({ useNavContext: () => mocks.context }));
vi.mock("@/hooks/domains/plugins/use-plugins", () => ({ usePlugins: () => ({ items: [] }) }));
vi.mock("@/lib/routing/client-router", () => ({ useRouter: () => ({ push: mocks.push }) }));

function press(init: KeyboardEventInit = {}, target: EventTarget = window) {
  const event = new KeyboardEvent("keydown", {
    key: "g",
    ctrlKey: true,
    altKey: true,
    bubbles: true,
    cancelable: true,
    ...init,
  });
  target.dispatchEvent(event);
  return event;
}

beforeEach(() => {
  mocks.push.mockClear();
  mocks.context = { workspaceId: "workspace-1", inOffice: false };
  mocks.overrides = {
    "integration:github": { key: "g", modifiers: { ctrlOrCmd: true, alt: true } },
  };
});
afterEach(() => {
  vi.restoreAllMocks();
  cleanup();
  document.body.innerHTML = "";
  pluginRegistry.unregisterPlugin("shortcuts-test");
});

describe("integration shortcut dispatch", () => {
  // @covers AC-UI-INTEGRATION-PAGE-SHORTCUTS-001.4
  it("opens a saved destination and reads changed overrides without remounting", () => {
    renderHook(() => useIntegrationShortcuts());
    expect(press().defaultPrevented).toBe(true);
    expect(mocks.push).toHaveBeenCalledWith("/github");
    mocks.overrides = {
      "integration:gitlab": { key: "g", modifiers: { ctrlOrCmd: true, alt: true } },
    };
    press();
    expect(mocks.push).toHaveBeenLastCalledWith("/gitlab");
  });

  it("accepts the Cmd path and navigates only once for duplicate integration chords", () => {
    mocks.overrides["integration:jira"] = mocks.overrides["integration:github"];
    renderHook(() => useIntegrationShortcuts());
    press({ ctrlKey: false, metaKey: true });
    expect(mocks.push).toHaveBeenCalledExactlyOnceWith("/github");
  });

  it("resolves navigation using the current workspace after switching", () => {
    const resolveHref = vi.spyOn(navigation, "resolveHref");
    const { rerender } = renderHook(() => useIntegrationShortcuts());
    press();
    expect(resolveHref).toHaveBeenLastCalledWith("/github", mocks.context);
    mocks.context = { workspaceId: "workspace-2", inOffice: false };
    rerender();
    press();
    expect(resolveHref).toHaveBeenLastCalledWith("/github", mocks.context);
    expect(mocks.push).toHaveBeenLastCalledWith("/github");
    expect(mocks.context.workspaceId).toBe("workspace-2");
  });
});

describe("integration shortcut event guards", () => {
  // @covers AC-UI-INTEGRATION-PAGE-SHORTCUTS-001.5
  it.each(["input", "textarea"])("ignores typing in %s", (tag) => {
    const input = document.createElement(tag);
    document.body.append(input);
    renderHook(() => useIntegrationShortcuts());
    expect(press({}, input).defaultPrevented).toBe(false);
    expect(mocks.push).not.toHaveBeenCalled();
    press();
    expect(mocks.push).toHaveBeenCalledOnce();
  });

  it("ignores contenteditable text entry", () => {
    const editable = document.createElement("div");
    Object.defineProperty(editable, "isContentEditable", { value: true });
    document.body.append(editable);
    renderHook(() => useIntegrationShortcuts());
    expect(press({}, editable).defaultPrevented).toBe(false);
    expect(mocks.push).not.toHaveBeenCalled();
  });

  it("yields while recording and resumes after recording stops", () => {
    const recorder = document.createElement("button");
    recorder.dataset.shortcutRecording = "true";
    document.body.append(recorder);
    renderHook(() => useIntegrationShortcuts());
    expect(press().defaultPrevented).toBe(false);
    expect(mocks.push).not.toHaveBeenCalled();
    recorder.remove();
    press();
    expect(mocks.push).toHaveBeenCalledOnce();
  });

  it("leaves repeated and already handled events alone", () => {
    renderHook(() => useIntegrationShortcuts());
    expect(press({ repeat: true }).defaultPrevented).toBe(false);
    const handled = new KeyboardEvent("keydown", {
      key: "g",
      ctrlKey: true,
      altKey: true,
      cancelable: true,
    });
    handled.preventDefault();
    window.dispatchEvent(handled);
    expect(mocks.push).not.toHaveBeenCalled();
  });

  it("ignores a saved unbound override", () => {
    mocks.overrides = { "integration:github": { key: "" } };
    renderHook(() => useIntegrationShortcuts());
    expect(press().defaultPrevented).toBe(false);
    expect(mocks.push).not.toHaveBeenCalled();
  });

  it.each([{}, { key: 7 }, { key: "g", modifiers: { alt: "yes" } }])(
    "ignores malformed overrides: %j",
    (override) => {
      mocks.overrides = { "integration:github": override } as unknown as StoredShortcutOverrides;
      renderHook(() => useIntegrationShortcuts());
      expect(press().defaultPrevented).toBe(false);
      expect(mocks.push).not.toHaveBeenCalled();
    },
  );
});

describe("integration shortcuts preserve global keyboard controls", () => {
  it.each([
    { ctrlKey: true, metaKey: false },
    { ctrlKey: false, metaKey: true },
  ])("leaves the shifted command-panel shortcut to its existing handler: %j", (modifier) => {
    mocks.overrides["integration:github"] = SHORTCUTS.COMMAND_PANEL_SHIFT;
    const openCommands = vi.fn();
    renderHook(() => {
      useIntegrationShortcuts();
      useKeyboardShortcut(SHORTCUTS.COMMAND_PANEL_SHIFT, openCommands);
    });
    press({ key: "p", altKey: false, shiftKey: true, ...modifier });
    expect(mocks.push).not.toHaveBeenCalled();
    expect(openCommands).toHaveBeenCalledOnce();
  });

  it.each([false, true])("does not consume focus traversal with Shift=%s", (shiftKey) => {
    mocks.overrides = {
      "integration:github": { key: "Tab", modifiers: { shift: shiftKey } },
      TOGGLE_PLAN_MODE: { key: "" },
    };
    renderHook(() => useIntegrationShortcuts());
    expect(press({ key: "Tab", ctrlKey: false, altKey: false, shiftKey }).defaultPrevented).toBe(
      false,
    );
    expect(mocks.push).not.toHaveBeenCalled();
  });
});

describe("integration shortcut precedence and lifecycle", () => {
  // @covers AC-UI-INTEGRATION-PAGE-SHORTCUTS-001.6
  it.each(["k", "s", "f"])("yields to existing core/reserved Ctrl+%s", (key) => {
    mocks.overrides["integration:github"] = { key, modifiers: { ctrlOrCmd: true } };
    renderHook(() => useIntegrationShortcuts());
    expect(press({ key, altKey: false }).defaultPrevented).toBe(false);
    expect(mocks.push).not.toHaveBeenCalled();
  });

  it("takes precedence over a later plugin action listener", () => {
    renderHook(() => useIntegrationShortcuts());
    const action = vi.fn();
    const listener = (event: KeyboardEvent) => {
      if (!event.defaultPrevented) action();
    };
    window.addEventListener("keydown", listener, true);
    try {
      press();
      expect(mocks.push).toHaveBeenCalledOnce();
      expect(action).not.toHaveBeenCalled();
    } finally {
      window.removeEventListener("keydown", listener, true);
    }
  });

  it("makes removed plugin registrations inert while retaining their saved binding", () => {
    pluginRegistry.registerNavItem("shortcuts-test", {
      id: "reviews",
      label: "Reviews",
      path: "/plugins/reviews",
      section: "integrations",
    });
    mocks.overrides = {
      "integration:plugin:shortcuts-test:reviews": {
        key: "g",
        modifiers: { ctrlOrCmd: true, alt: true },
      },
    };
    const { rerender } = renderHook(() => useIntegrationShortcuts());
    press();
    expect(mocks.push).toHaveBeenCalledWith("/plugins/reviews");
    pluginRegistry.unregisterPlugin("shortcuts-test");
    rerender();
    mocks.push.mockClear();
    expect(press().defaultPrevented).toBe(false);
    expect(mocks.push).not.toHaveBeenCalled();
    expect(mocks.overrides).toHaveProperty("integration:plugin:shortcuts-test:reviews");
  });

  it("removes the keyboard listener on unmount", () => {
    const { unmount } = renderHook(() => useIntegrationShortcuts());
    unmount();
    expect(press().defaultPrevented).toBe(false);
    expect(mocks.push).not.toHaveBeenCalled();
  });
});
