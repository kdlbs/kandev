import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { PluginRecord } from "@/lib/types/plugins";
import { defaultSettingsState } from "@/lib/state/slices/settings/settings-slice";
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

beforeEach(() => {
  apiMocks.updateUserSettings.mockReset();
  apiMocks.updateUserSettings.mockImplementation(
    async ({ keyboard_shortcuts }: { keyboard_shortcuts: Record<string, unknown> }) => ({
      settings: { keyboard_shortcuts },
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
    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));

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

  it("resets a customized selected-plugin shortcut in the local draft", async () => {
    renderCard(plugin());

    fireEvent.click(screen.getByTestId(`shortcut-recorder-${SHORTCUT_ID}`));
    fireEvent.keyDown(window, { key: "t", ctrlKey: true });
    fireEvent.click(screen.getByRole("button", { name: "Reset to default" }));

    expect(apiMocks.updateUserSettings).not.toHaveBeenCalled();
    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));
    await waitFor(() => expect(apiMocks.updateUserSettings).toHaveBeenCalledOnce());
    expect(apiMocks.updateUserSettings).toHaveBeenCalledWith({
      keyboard_shortcuts: { "plugin:other:keep": { key: "x" } },
    });
  });
});
