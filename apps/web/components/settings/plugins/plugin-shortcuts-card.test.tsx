import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { PluginRecord } from "@/lib/types/plugins";
import { defaultSettingsState } from "@/lib/state/slices/settings/settings-slice";
import type { UserSettingsState } from "@/lib/state/slices/settings/types";
import { SettingsSaveProvider } from "../settings-save-provider";
import { PluginShortcutsCard } from "./plugin-shortcuts-card";

const apiMocks = vi.hoisted(() => ({ updateUserSettings: vi.fn() }));
const storeMocks = vi.hoisted(() => ({
  state: {} as Record<string, unknown>,
  setUserSettings: vi.fn(),
}));

vi.mock("@/lib/api", () => ({
  updateUserSettings: (...args: unknown[]) => apiMocks.updateUserSettings(...args),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector(storeMocks.state),
  useAppStoreApi: () => ({ getState: () => storeMocks.state }),
}));

vi.mock("@kandev/ui/kbd", () => ({
  Kbd: ({ children }: { children: ReactNode }) => <kbd>{children}</kbd>,
}));

const PLUGIN_ID = "session-cost";
const SHORTCUT_ID = `plugin:${PLUGIN_ID}:open-panel`;
const SAVE_CHANGES = "Save changes";

function plugin(overrides: Partial<PluginRecord> = {}): PluginRecord {
  return {
    id: PLUGIN_ID,
    api_version: 1,
    version: "1.0.0",
    display_name: "Session Cost",
    description: "",
    author: "",
    categories: [],
    capabilities: {},
    status: "disabled",
    install_path: "",
    signed: true,
    installed_at: "",
    restart_count: 0,
    ui: {
      bundle: "ui/bundle.js",
      keybindings: [{ id: "open-panel", default: "mod+u", description: "Open panel" }],
    },
    ...overrides,
  };
}

function renderCard(selected: PluginRecord, plugins: PluginRecord[] = [selected]) {
  return render(
    <SettingsSaveProvider>
      <PluginShortcutsCard plugin={selected} plugins={plugins} />
    </SettingsSaveProvider>,
  );
}

function rerenderCard(
  rerender: ReturnType<typeof render>["rerender"],
  selected: PluginRecord,
  plugins: PluginRecord[] = [selected],
) {
  rerender(
    <SettingsSaveProvider>
      <PluginShortcutsCard plugin={selected} plugins={plugins} />
    </SettingsSaveProvider>,
  );
}

function updateStoreUserSettings(patch: Partial<UserSettingsState>) {
  const current = storeMocks.state.userSettings as UserSettingsState;
  storeMocks.state = {
    ...storeMocks.state,
    userSettings: { ...current, ...patch },
  };
}

function deferredResponse() {
  let resolve!: (value: {
    settings: {
      keyboard_shortcuts: Record<string, unknown>;
      revision: number;
    };
  }) => void;
  const promise = new Promise<{
    settings: {
      keyboard_shortcuts: Record<string, unknown>;
      revision: number;
    };
  }>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

beforeEach(() => {
  apiMocks.updateUserSettings.mockReset();
  apiMocks.updateUserSettings.mockImplementation(
    async ({ keyboard_shortcuts }: { keyboard_shortcuts: Record<string, unknown> }) => ({
      settings: { keyboard_shortcuts, revision: 2 },
    }),
  );
  storeMocks.setUserSettings.mockReset();
  storeMocks.state = {
    userSettings: {
      ...defaultSettingsState.userSettings,
      keyboardShortcuts: {
        [SHORTCUT_ID]: { key: "u", modifiers: { ctrlOrCmd: true } },
        "plugin:other:keep": { key: "x" },
      },
      loaded: true,
      revision: 1,
    },
    setUserSettings: storeMocks.setUserSettings,
  };
});

afterEach(cleanup);

describe("PluginShortcutsCard", () => {
  it("renders only the selected plugin and keeps other plugin names in conflict labels", () => {
    const other = plugin({
      id: "other-plugin",
      display_name: "Other Plugin",
      ui: {
        bundle: "ui/bundle.js",
        keybindings: [{ id: "open-panel", default: "mod+u", description: "Open panel" }],
      },
    });

    renderCard(plugin(), [plugin(), other]);

    expect(screen.getByTestId("plugin-shortcuts-card")).toBeTruthy();
    expect(screen.getByTestId(`shortcut-recorder-${SHORTCUT_ID}`)).toBeTruthy();
    expect(screen.queryByTestId("shortcut-recorder-plugin:other-plugin:open-panel")).toBeNull();
    expect(screen.getByText("Open panel")).toBeTruthy();
    expect(screen.getByTitle("Same shortcut as: Other Plugin: Open panel")).toBeTruthy();
  });

  it("keeps no empty card for a plugin without parseable declarations", () => {
    renderCard(
      plugin({
        ui: {
          bundle: "ui/bundle.js",
          keybindings: [{ id: "bad", default: "banana", description: "Bad" }],
        },
      }),
    );

    expect(screen.queryByTestId("plugin-shortcuts-card")).toBeNull();
  });

  it("saves the complete override map through user settings only", async () => {
    renderCard(plugin());

    const recorder = screen.getByTestId(`shortcut-recorder-${SHORTCUT_ID}`);
    fireEvent.click(recorder);
    fireEvent.keyDown(window, { key: "t", ctrlKey: true });

    expect(apiMocks.updateUserSettings).not.toHaveBeenCalled();
    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES }));

    await waitFor(() => expect(apiMocks.updateUserSettings).toHaveBeenCalledOnce());
    expect(apiMocks.updateUserSettings).toHaveBeenCalledWith({
      keyboard_shortcuts: {
        [SHORTCUT_ID]: { key: "t", modifiers: { ctrlOrCmd: true } },
        "plugin:other:keep": { key: "x" },
      },
    });
    expect(storeMocks.setUserSettings).toHaveBeenCalledWith(
      expect.objectContaining({
        keyboardShortcuts: {
          [SHORTCUT_ID]: { key: "t", modifiers: { ctrlOrCmd: true } },
          "plugin:other:keep": { key: "x" },
        },
      }),
    );
  });
});

describe("PluginShortcutsCard hydration", () => {
  it("waits for hydration and rebases a pre-hydration edit onto the loaded map", async () => {
    updateStoreUserSettings({ keyboardShortcuts: {}, loaded: false, revision: null });
    const selected = plugin();
    const { rerender } = renderCard(selected);

    fireEvent.click(screen.getByTestId(`shortcut-recorder-${SHORTCUT_ID}`));
    fireEvent.keyDown(window, { key: "t", ctrlKey: true });

    expect(screen.queryByRole("button", { name: SAVE_CHANGES })).toBeNull();

    updateStoreUserSettings({
      keyboardShortcuts: {
        SEARCH: { key: "k", modifiers: { ctrlOrCmd: true } },
        "plugin:other:keep": { key: "y" },
      },
      loaded: true,
      revision: 4,
    });
    rerenderCard(rerender, selected);

    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES }));
    await waitFor(() => expect(apiMocks.updateUserSettings).toHaveBeenCalledOnce());
    expect(apiMocks.updateUserSettings).toHaveBeenCalledWith({
      keyboard_shortcuts: {
        SEARCH: { key: "k", modifiers: { ctrlOrCmd: true } },
        [SHORTCUT_ID]: { key: "t", modifiers: { ctrlOrCmd: true } },
        "plugin:other:keep": { key: "y" },
      },
    });
  });

  it("adopts a newer complete map before saving a later local edit", async () => {
    const selected = plugin();
    const { rerender } = renderCard(selected);

    updateStoreUserSettings({
      keyboardShortcuts: {
        [SHORTCUT_ID]: { key: "r", modifiers: { ctrlOrCmd: true } },
        "plugin:other:keep": { key: "y" },
        SEARCH: { key: "k", modifiers: { ctrlOrCmd: true } },
      },
      revision: 2,
    });
    rerenderCard(rerender, selected);

    fireEvent.click(screen.getByTestId(`shortcut-recorder-${SHORTCUT_ID}`));
    fireEvent.keyDown(window, { key: "t", ctrlKey: true });
    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES }));

    await waitFor(() => expect(apiMocks.updateUserSettings).toHaveBeenCalledOnce());
    expect(apiMocks.updateUserSettings).toHaveBeenCalledWith({
      keyboard_shortcuts: {
        [SHORTCUT_ID]: { key: "t", modifiers: { ctrlOrCmd: true } },
        "plugin:other:keep": { key: "y" },
        SEARCH: { key: "k", modifiers: { ctrlOrCmd: true } },
      },
    });
  });

  it("preserves a local reset while adopting a newer complete map", async () => {
    const selected = plugin();
    const { rerender } = renderCard(selected);

    fireEvent.click(screen.getByTestId(`shortcut-recorder-${SHORTCUT_ID}`));
    fireEvent.keyDown(window, { key: "t", ctrlKey: true });
    fireEvent.click(screen.getByRole("button", { name: "Reset to default" }));
    updateStoreUserSettings({
      keyboardShortcuts: {
        [SHORTCUT_ID]: { key: "r", modifiers: { ctrlOrCmd: true } },
        "plugin:other:keep": { key: "y" },
        SEARCH: { key: "k", modifiers: { ctrlOrCmd: true } },
      },
      revision: 2,
    });
    rerenderCard(rerender, selected);
    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES }));

    await waitFor(() => expect(apiMocks.updateUserSettings).toHaveBeenCalledOnce());
    expect(apiMocks.updateUserSettings).toHaveBeenCalledWith({
      keyboard_shortcuts: {
        "plugin:other:keep": { key: "y" },
        SEARCH: { key: "k", modifiers: { ctrlOrCmd: true } },
      },
    });
  });
});

describe("PluginShortcutsCard synchronization", () => {
  it("preserves a dirty shortcut while adopting unrelated newer entries", async () => {
    const selected = plugin();
    const { rerender } = renderCard(selected);

    fireEvent.click(screen.getByTestId(`shortcut-recorder-${SHORTCUT_ID}`));
    fireEvent.keyDown(window, { key: "t", ctrlKey: true });

    updateStoreUserSettings({
      keyboardShortcuts: {
        [SHORTCUT_ID]: { key: "r", modifiers: { ctrlOrCmd: true } },
        "plugin:other:keep": { key: "y" },
        SEARCH: { key: "k", modifiers: { ctrlOrCmd: true } },
      },
      revision: 2,
    });
    rerenderCard(rerender, selected);
    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES }));

    await waitFor(() => expect(apiMocks.updateUserSettings).toHaveBeenCalledOnce());
    expect(apiMocks.updateUserSettings).toHaveBeenCalledWith({
      keyboard_shortcuts: {
        [SHORTCUT_ID]: { key: "t", modifiers: { ctrlOrCmd: true } },
        "plugin:other:keep": { key: "y" },
        SEARCH: { key: "k", modifiers: { ctrlOrCmd: true } },
      },
    });
  });

  it("keeps edits made while a save is in flight dirty after the response", async () => {
    const deferred = deferredResponse();
    apiMocks.updateUserSettings.mockReturnValueOnce(deferred.promise);
    renderCard(plugin());

    const recorder = screen.getByTestId(`shortcut-recorder-${SHORTCUT_ID}`);
    fireEvent.click(recorder);
    fireEvent.keyDown(window, { key: "t", ctrlKey: true });
    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES }));
    await waitFor(() => expect(apiMocks.updateUserSettings).toHaveBeenCalledOnce());

    fireEvent.click(recorder);
    fireEvent.keyDown(window, { key: "v", ctrlKey: true });
    deferred.resolve({
      settings: {
        keyboard_shortcuts: {
          [SHORTCUT_ID]: { key: "t", modifiers: { ctrlOrCmd: true } },
          "plugin:other:keep": { key: "x" },
        },
        revision: 2,
      },
    });

    await waitFor(() => expect(recorder.textContent).toBe("Ctrl+V"));
    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES }));
    await waitFor(() => expect(apiMocks.updateUserSettings).toHaveBeenCalledTimes(2));
    expect(apiMocks.updateUserSettings).toHaveBeenLastCalledWith({
      keyboard_shortcuts: {
        [SHORTCUT_ID]: { key: "v", modifiers: { ctrlOrCmd: true } },
        "plugin:other:keep": { key: "x" },
      },
    });
  });

  it("does not let an older save response replace a newer store revision", async () => {
    const deferred = deferredResponse();
    apiMocks.updateUserSettings.mockReturnValueOnce(deferred.promise);
    const selected = plugin();
    const { rerender } = renderCard(selected);

    const recorder = screen.getByTestId(`shortcut-recorder-${SHORTCUT_ID}`);
    fireEvent.click(recorder);
    fireEvent.keyDown(window, { key: "t", ctrlKey: true });
    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES }));
    await waitFor(() => expect(apiMocks.updateUserSettings).toHaveBeenCalledOnce());

    updateStoreUserSettings({
      keyboardShortcuts: {
        [SHORTCUT_ID]: { key: "r", modifiers: { ctrlOrCmd: true } },
        "plugin:other:keep": { key: "y" },
      },
      revision: 3,
    });
    rerenderCard(rerender, selected);
    deferred.resolve({
      settings: {
        keyboard_shortcuts: {
          [SHORTCUT_ID]: { key: "t", modifiers: { ctrlOrCmd: true } },
          "plugin:other:keep": { key: "x" },
        },
        revision: 2,
      },
    });

    await waitFor(() => expect(recorder.textContent).toBe("Ctrl+R"));
    expect(storeMocks.setUserSettings).not.toHaveBeenCalled();
  });
});

describe("PluginShortcutsCard reset", () => {
  it("resets a customized selected-plugin shortcut in the local draft", async () => {
    renderCard(plugin());

    fireEvent.click(screen.getByTestId(`shortcut-recorder-${SHORTCUT_ID}`));
    fireEvent.keyDown(window, { key: "t", ctrlKey: true });
    fireEvent.click(screen.getByRole("button", { name: "Reset to default" }));

    expect(apiMocks.updateUserSettings).not.toHaveBeenCalled();
    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES }));
    await waitFor(() => expect(apiMocks.updateUserSettings).toHaveBeenCalledOnce());
    expect(apiMocks.updateUserSettings).toHaveBeenCalledWith({
      keyboard_shortcuts: { "plugin:other:keep": { key: "x" } },
    });
  });
});
