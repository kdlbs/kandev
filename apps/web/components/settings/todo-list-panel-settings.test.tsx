import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import type { AppState } from "@/lib/state/store";
import type { UserSettingsState } from "@/lib/state/slices/settings/types";
import { SettingsSaveProvider } from "./settings-save-provider";

const updateUserSettings = vi.fn();
const TODO_LIST_PANEL_LABEL = "Show agent todo list panel";
const ONLY_PIN_WHEN_NOT_EMPTY_LABEL = "Only pin when todo list is not empty";
const DATA_STATE_ATTR = "data-state";
const DATA_SETTINGS_DIRTY_ATTR = "data-settings-dirty";
const SAVE_CHANGES_BUTTON = "Save changes";

vi.mock("@/lib/api", () => ({
  updateUserSettings: (...args: unknown[]) => updateUserSettings(...args),
}));

import { TodoListPanelSettings } from "./todo-list-panel-settings";

beforeEach(() => {
  updateUserSettings.mockReset().mockResolvedValue({ settings: {} });
});

afterEach(cleanup);

/** Asserts the last save submitted both fields with the given values. */
async function expectSavedAs(show: boolean, onlyWhenNotEmpty: boolean): Promise<void> {
  await waitFor(() =>
    expect(updateUserSettings).toHaveBeenCalledWith({
      show_todo_list_panel: show,
      show_todo_list_panel_only_when_not_empty: onlyWhenNotEmpty,
    }),
  );
}

/** Makes the next save hang until the returned resolver is called. */
function deferNextSave(): () => void {
  let resolveSave: (value: { settings: object }) => void = () => undefined;
  updateUserSettings.mockImplementationOnce(
    () => new Promise<{ settings: object }>((resolve) => (resolveSave = resolve)),
  );
  return () => resolveSave({ settings: {} });
}

function renderTodoListPanelSettings(
  userSettingsOverrides: Partial<UserSettingsState> = {},
): StoreApi<AppState> {
  let storeApi: StoreApi<AppState> | null = null;
  function StoreProbe() {
    storeApi = useAppStoreApi();
    return null;
  }
  render(
    <StateProvider
      initialState={{
        userSettings: { ...defaultState.userSettings, ...userSettingsOverrides },
      }}
    >
      <StoreProbe />
      <SettingsSaveProvider>
        <TodoListPanelSettings />
      </SettingsSaveProvider>
    </StateProvider>,
  );
  return storeApi!;
}

describe("TodoListPanelSettings", () => {
  it("keeps an enabled preference local until Save changes persists it", async () => {
    renderTodoListPanelSettings({ showTodoListPanel: false });
    const toggle = screen.getByRole("switch", { name: TODO_LIST_PANEL_LABEL });

    expect(toggle.getAttribute(DATA_STATE_ATTR)).toBe("unchecked");
    fireEvent.click(toggle);

    expect(updateUserSettings).not.toHaveBeenCalled();
    expect(toggle.getAttribute(DATA_STATE_ATTR)).toBe("checked");
    expect(
      screen.getByTestId("todo-list-panel-settings-card").getAttribute(DATA_SETTINGS_DIRTY_ATTR),
    ).toBe("true");

    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_BUTTON }));

    await expectSavedAs(true, false);
    await waitFor(() => expect(toggle.getAttribute(DATA_SETTINGS_DIRTY_ATTR)).toBe("false"));
  });

  it("inhibits the sub-option while the main toggle is off and saves both fields", async () => {
    renderTodoListPanelSettings({
      showTodoListPanel: false,
      showTodoListPanelOnlyWhenNotEmpty: true,
    });

    // Inhibited (hidden entirely), not disabled, while the main preference is off.
    expect(screen.queryByRole("switch", { name: ONLY_PIN_WHEN_NOT_EMPTY_LABEL })).toBeNull();

    const toggle = screen.getByRole("switch", { name: TODO_LIST_PANEL_LABEL });
    fireEvent.click(toggle);

    // Appears once the main preference is on, in its preserved (on) state,
    // with visible help explaining that only automatic pinning is affected.
    const subToggle = screen.getByRole("switch", { name: ONLY_PIN_WHEN_NOT_EMPTY_LABEL });
    expect(subToggle.getAttribute(DATA_STATE_ATTR)).toBe("checked");
    expect(screen.getByText(/Manually added Todos tabs are never affected/)).toBeTruthy();

    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_BUTTON }));

    await expectSavedAs(true, true);
    await waitFor(() => expect(toggle.getAttribute(DATA_SETTINGS_DIRTY_ATTR)).toBe("false"));
  });

  it("preserves the sub-option state across a main-toggle off/on cycle without saving", () => {
    renderTodoListPanelSettings({
      showTodoListPanel: true,
      showTodoListPanelOnlyWhenNotEmpty: true,
    });

    const toggle = screen.getByRole("switch", { name: TODO_LIST_PANEL_LABEL });
    expect(screen.getByRole("switch", { name: ONLY_PIN_WHEN_NOT_EMPTY_LABEL })).toBeTruthy();

    fireEvent.click(toggle);
    expect(screen.queryByRole("switch", { name: ONLY_PIN_WHEN_NOT_EMPTY_LABEL })).toBeNull();

    fireEvent.click(toggle);
    const subToggle = screen.getByRole("switch", { name: ONLY_PIN_WHEN_NOT_EMPTY_LABEL });
    expect(subToggle.getAttribute(DATA_STATE_ATTR)).toBe("checked");
    expect(updateUserSettings).not.toHaveBeenCalled();
  });

  it("marks each toggle dirty only for its own unsaved change", () => {
    renderTodoListPanelSettings({
      showTodoListPanel: true,
      showTodoListPanelOnlyWhenNotEmpty: false,
    });

    const toggle = screen.getByRole("switch", { name: TODO_LIST_PANEL_LABEL });
    const subToggle = screen.getByRole("switch", { name: ONLY_PIN_WHEN_NOT_EMPTY_LABEL });
    expect(toggle.getAttribute(DATA_SETTINGS_DIRTY_ATTR)).toBe("false");
    expect(subToggle.getAttribute(DATA_SETTINGS_DIRTY_ATTR)).toBe("false");

    fireEvent.click(subToggle); // only the sub-option becomes dirty
    expect(subToggle.getAttribute(DATA_SETTINGS_DIRTY_ATTR)).toBe("true");
    expect(toggle.getAttribute(DATA_SETTINGS_DIRTY_ATTR)).toBe("false");

    fireEvent.click(subToggle); // reverting clears it
    expect(subToggle.getAttribute(DATA_SETTINGS_DIRTY_ATTR)).toBe("false");
    expect(toggle.getAttribute(DATA_SETTINGS_DIRTY_ATTR)).toBe("false");

    // Changing only the main toggle marks only the main toggle dirty (the
    // sub-option unmounts with the main toggle off).
    fireEvent.click(toggle);
    expect(toggle.getAttribute(DATA_SETTINGS_DIRTY_ATTR)).toBe("true");
    expect(updateUserSettings).not.toHaveBeenCalled();
  });
});

describe("TodoListPanelSettings save lifecycle", () => {
  it("does not clobber a newer store value with a stale delayed save", async () => {
    const resolveSave = deferNextSave();
    const storeApi = renderTodoListPanelSettings({ showTodoListPanel: false });

    const toggle = screen.getByRole("switch", { name: TODO_LIST_PANEL_LABEL });
    fireEvent.click(toggle); // draft: main on
    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_BUTTON }));

    // While the PATCH is in flight, a newer value arrives (e.g. via the
    // user.settings.updated WebSocket push from another tab).
    const { setUserSettings } = storeApi.getState();
    setUserSettings({
      ...storeApi.getState().userSettings,
      showTodoListPanel: false,
      showTodoListPanelOnlyWhenNotEmpty: true,
    });

    resolveSave();
    await waitFor(() => expect(updateUserSettings).toHaveBeenCalledTimes(1));

    expect(storeApi.getState().userSettings.showTodoListPanel).toBe(false);
    expect(storeApi.getState().userSettings.showTodoListPanelOnlyWhenNotEmpty).toBe(true);
    // The draft still differs from the newer WS baseline: not falsely clean.
    expect(toggle.getAttribute(DATA_SETTINGS_DIRTY_ATTR)).toBe("true");
  });

  it("keeps the draft dirty and the store unchanged when saving fails", async () => {
    updateUserSettings.mockRejectedValueOnce(new Error("save failed"));
    const storeApi = renderTodoListPanelSettings({ showTodoListPanel: false });
    const toggle = screen.getByRole("switch", { name: TODO_LIST_PANEL_LABEL });
    fireEvent.click(toggle); // draft: main on
    expect(toggle.getAttribute(DATA_STATE_ATTR)).toBe("checked");

    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_BUTTON }));
    await waitFor(() => expect(updateUserSettings).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(screen.getByText("Couldn't save")).toBeTruthy());

    // The failed PATCH must not touch the store, the draft, or the dirty flag.
    expect(storeApi.getState().userSettings.showTodoListPanel).toBe(false);
    expect(storeApi.getState().userSettings.showTodoListPanelOnlyWhenNotEmpty).toBe(false);
    expect(toggle.getAttribute(DATA_STATE_ATTR)).toBe("checked");
    expect(toggle.getAttribute(DATA_SETTINGS_DIRTY_ATTR)).toBe("true");
  });

  it("keeps a draft edited during an in-flight save dirty against the completed save", async () => {
    const resolveSave = deferNextSave();
    const storeApi = renderTodoListPanelSettings({ showTodoListPanel: false });
    const toggle = screen.getByRole("switch", { name: TODO_LIST_PANEL_LABEL });
    fireEvent.click(toggle); // draft: main on
    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_BUTTON }));

    fireEvent.click(toggle); // draft: main back off while the PATCH is pending

    resolveSave();
    await waitFor(() => expect(updateUserSettings).toHaveBeenCalledTimes(1));

    // The completed save owns the store and the baseline; the newer draft
    // stays unsaved and dirty.
    expect(storeApi.getState().userSettings.showTodoListPanel).toBe(true);
    expect(toggle.getAttribute(DATA_STATE_ATTR)).toBe("unchecked");
    expect(toggle.getAttribute(DATA_SETTINGS_DIRTY_ATTR)).toBe("true");
  });
});
