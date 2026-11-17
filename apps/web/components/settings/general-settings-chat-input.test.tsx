import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { defaultSettingsState } from "@/lib/state/slices/settings/settings-slice";
import { SettingsSaveProvider } from "./settings-save-provider";
import { KeyboardShortcutsSettings } from "./general-settings";

const apiMocks = vi.hoisted(() => ({ updateUserSettings: vi.fn() }));
const storeMocks = vi.hoisted(() => ({
  state: {} as Record<string, unknown>,
  setUserSettings: vi.fn(),
}));

const ESCAPE_SETTING_LABEL = "Escape pauses the agent";
const SUBMIT_KEY_LABEL = "Message Submit Key";
const CMD_ENTER_OPTION = "Cmd/Ctrl+Enter to send";
const SAVE_CHANGES_LABEL = "Save changes";
const ESCAPE_SETTING_FIELD = "escape_cancels_agent";
const SUBMIT_KEY_FIELD = "chat_submit_key";
const SUBMIT_ENTER = "enter";
const SUBMIT_CMD_ENTER = "cmd_enter";
const CHECKED_STATE = "checked";
const DATA_STATE_ATTRIBUTE = "data-state";
const DIRTY_ATTRIBUTE = "data-settings-dirty";

vi.mock("@/lib/api", () => ({
  updateUserSettings: (...args: unknown[]) => apiMocks.updateUserSettings(...args),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector(storeMocks.state),
  useAppStoreApi: () => ({ getState: () => storeMocks.state }),
}));

vi.mock("@/hooks/domains/plugins/use-plugins", () => ({ usePlugins: () => ({ items: [] }) }));
vi.mock("@/hooks/use-integration-shortcut-entries", () => ({
  useIntegrationShortcutEntries: () => [],
}));

beforeEach(() => {
  apiMocks.updateUserSettings.mockReset();
  storeMocks.setUserSettings.mockReset();
  storeMocks.state = {
    userSettings: {
      ...defaultSettingsState.userSettings,
      loaded: true,
      revision: 1,
      keyboardShortcuts: {},
    },
    setUserSettings: storeMocks.setUserSettings,
  };
});

afterEach(cleanup);

function renderKeyboardSettings() {
  return render(
    <SettingsSaveProvider>
      <KeyboardShortcutsSettings />
    </SettingsSaveProvider>,
  );
}

function updateKeyboardSettings(patch: Record<string, unknown> = {}) {
  storeMocks.state = {
    ...storeMocks.state,
    userSettings: {
      ...defaultSettingsState.userSettings,
      loaded: true,
      revision: 1,
      keyboardShortcuts: {},
      ...patch,
    },
  };
}

it("does not save an untouched Escape setting over a newer account update", async () => {
  updateKeyboardSettings({ chatSubmitKey: SUBMIT_ENTER });
  apiMocks.updateUserSettings.mockResolvedValue({
    settings: {
      [ESCAPE_SETTING_FIELD]: true,
      [SUBMIT_KEY_FIELD]: SUBMIT_CMD_ENTER,
      revision: 3,
    },
  });
  const view = renderKeyboardSettings();

  fireEvent.click(screen.getByRole("combobox", { name: SUBMIT_KEY_LABEL }));
  fireEvent.click(screen.getByRole("option", { name: CMD_ENTER_OPTION }));
  updateKeyboardSettings({
    chatSubmitKey: SUBMIT_ENTER,
    escapeCancelsAgent: true,
    revision: 2,
  });
  view.rerender(
    <SettingsSaveProvider>
      <KeyboardShortcutsSettings />
    </SettingsSaveProvider>,
  );
  fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_LABEL }));

  await waitFor(() => expect(apiMocks.updateUserSettings).toHaveBeenCalledOnce());
  const payload = apiMocks.updateUserSettings.mock.calls[0]?.[0];
  expect(payload).toMatchObject({ [SUBMIT_KEY_FIELD]: SUBMIT_CMD_ENTER });
  expect(payload).not.toHaveProperty(ESCAPE_SETTING_FIELD);
});

it("does not save an untouched submit key over a newer account update", async () => {
  updateKeyboardSettings({ chatSubmitKey: SUBMIT_ENTER });
  apiMocks.updateUserSettings.mockResolvedValue({
    settings: {
      [ESCAPE_SETTING_FIELD]: true,
      [SUBMIT_KEY_FIELD]: SUBMIT_CMD_ENTER,
      revision: 3,
    },
  });
  const view = renderKeyboardSettings();

  fireEvent.click(screen.getByRole("switch", { name: ESCAPE_SETTING_LABEL }));
  updateKeyboardSettings({ chatSubmitKey: SUBMIT_CMD_ENTER, revision: 2 });
  view.rerender(
    <SettingsSaveProvider>
      <KeyboardShortcutsSettings />
    </SettingsSaveProvider>,
  );
  fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_LABEL }));

  await waitFor(() => expect(apiMocks.updateUserSettings).toHaveBeenCalledOnce());
  const payload = apiMocks.updateUserSettings.mock.calls[0]?.[0];
  expect(payload).toMatchObject({ [ESCAPE_SETTING_FIELD]: true });
  expect(payload).not.toHaveProperty(SUBMIT_KEY_FIELD);
});

it("keeps a newer live setting when an older settings save response arrives", async () => {
  updateKeyboardSettings({ escapeCancelsAgent: true });
  let resolveSave: (value: unknown) => void = () => {};
  apiMocks.updateUserSettings.mockReturnValueOnce(
    new Promise((resolve) => {
      resolveSave = resolve;
    }),
  );
  const view = renderKeyboardSettings();

  fireEvent.click(screen.getByRole("switch", { name: ESCAPE_SETTING_LABEL }));
  fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_LABEL }));
  await waitFor(() => expect(apiMocks.updateUserSettings).toHaveBeenCalledOnce());

  updateKeyboardSettings({ escapeCancelsAgent: true, revision: 3 });
  view.rerender(
    <SettingsSaveProvider>
      <KeyboardShortcutsSettings />
    </SettingsSaveProvider>,
  );
  resolveSave({
    settings: {
      [ESCAPE_SETTING_FIELD]: false,
      [SUBMIT_KEY_FIELD]: SUBMIT_ENTER,
      revision: 2,
    },
  });

  const toggle = screen.getByRole("switch", { name: ESCAPE_SETTING_LABEL });
  await waitFor(() => expect(toggle.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(CHECKED_STATE));
  expect(storeMocks.setUserSettings).not.toHaveBeenCalled();
});

it("keeps edits made while a keyboard settings save is pending", async () => {
  updateKeyboardSettings();
  let resolveSave: (value: unknown) => void = () => {};
  apiMocks.updateUserSettings.mockReturnValueOnce(
    new Promise((resolve) => {
      resolveSave = resolve;
    }),
  );
  renderKeyboardSettings();
  const toggle = screen.getByRole("switch", { name: ESCAPE_SETTING_LABEL });

  fireEvent.click(toggle);
  fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_LABEL }));
  await waitFor(() => expect(apiMocks.updateUserSettings).toHaveBeenCalledOnce());
  fireEvent.click(toggle);
  resolveSave({
    settings: {
      [ESCAPE_SETTING_FIELD]: true,
      [SUBMIT_KEY_FIELD]: SUBMIT_ENTER,
      revision: 2,
    },
  });

  await waitFor(() => expect(toggle.getAttribute(DATA_STATE_ATTRIBUTE)).not.toBe(CHECKED_STATE));
  expect(toggle.closest("[data-settings-row]")?.getAttribute(DIRTY_ATTRIBUTE)).toBe("true");
});

it("preserves an edit made during save when it matches the submitted value", async () => {
  updateKeyboardSettings({ chatSubmitKey: SUBMIT_ENTER });
  let resolveSave: (value: unknown) => void = () => {};
  apiMocks.updateUserSettings.mockReturnValueOnce(
    new Promise((resolve) => {
      resolveSave = resolve;
    }),
  );
  const view = renderKeyboardSettings();

  fireEvent.click(screen.getByRole("combobox", { name: SUBMIT_KEY_LABEL }));
  fireEvent.click(screen.getByRole("option", { name: CMD_ENTER_OPTION }));
  fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_LABEL }));
  await waitFor(() => expect(apiMocks.updateUserSettings).toHaveBeenCalledOnce());

  updateKeyboardSettings({
    chatSubmitKey: SUBMIT_CMD_ENTER,
    escapeCancelsAgent: true,
    revision: 3,
  });
  view.rerender(
    <SettingsSaveProvider>
      <KeyboardShortcutsSettings />
    </SettingsSaveProvider>,
  );
  const toggle = screen.getByRole("switch", { name: ESCAPE_SETTING_LABEL });
  fireEvent.click(toggle);
  await act(async () => {
    resolveSave({
      settings: {
        [SUBMIT_KEY_FIELD]: SUBMIT_CMD_ENTER,
        [ESCAPE_SETTING_FIELD]: false,
        revision: 2,
      },
    });
  });

  await waitFor(() => expect(toggle.getAttribute(DATA_STATE_ATTRIBUTE)).not.toBe(CHECKED_STATE));
  expect(toggle.closest("[data-settings-row]")?.getAttribute(DIRTY_ATTRIBUTE)).toBe("true");
});

it("keeps the Escape draft dirty after a failed settings save", async () => {
  updateKeyboardSettings();
  apiMocks.updateUserSettings.mockRejectedValueOnce(new Error("offline"));
  renderKeyboardSettings();
  const toggle = screen.getByRole("switch", { name: ESCAPE_SETTING_LABEL });

  fireEvent.click(toggle);
  fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_LABEL }));

  expect(await screen.findByText("Couldn't save")).toBeTruthy();
  expect(toggle.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(CHECKED_STATE);
  expect(toggle.closest("[data-settings-row]")?.getAttribute(DIRTY_ATTRIBUTE)).toBe("true");
  expect(storeMocks.setUserSettings).not.toHaveBeenCalled();
});
