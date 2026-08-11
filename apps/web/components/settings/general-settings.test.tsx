import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defaultSettingsState } from "@/lib/state/slices/settings/settings-slice";
import { SettingsSaveProvider } from "./settings-save-provider";
import { AppearanceSettings } from "./general-settings";

const apiMocks = vi.hoisted(() => ({ updateUserSettings: vi.fn() }));
const SHOW_STATUS_BAR_LABEL = "Show status bar";
const SAVE_CHANGES_LABEL = "Save changes";
const CHECKED_STATE = "checked";
const DATA_STATE_ATTRIBUTE = "data-state";
const themeMocks = vi.hoisted(() => ({
  previewTheme: vi.fn(),
  commitTheme: vi.fn(),
  restoreTheme: vi.fn(),
}));
const storeMocks = vi.hoisted(() => ({
  state: {} as Record<string, unknown>,
  setUserSettings: vi.fn(),
  previewSettingsMenuMode: vi.fn(),
  commitSettingsMenuMode: vi.fn(),
  restoreSettingsMenuMode: vi.fn(),
}));

vi.mock("@/lib/api", () => ({
  updateUserSettings: (...args: unknown[]) => apiMocks.updateUserSettings(...args),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector(storeMocks.state),
  useAppStoreApi: () => ({ getState: () => storeMocks.state }),
}));

vi.mock("@/components/theme/app-theme", () => ({
  useTheme: () => ({
    savedTheme: "system",
    previewTheme: themeMocks.previewTheme,
    commitTheme: themeMocks.commitTheme,
    restoreTheme: themeMocks.restoreTheme,
  }),
}));

vi.mock("@/components/settings/language-settings", () => ({ LanguageSettings: () => null }));
vi.mock("@/components/settings/startup-page-settings-card", () => ({
  StartupPageSettingsCard: () => null,
}));
vi.mock("@/components/settings/system-metrics-settings-card", () => ({
  SystemMetricsSettingsCard: () => null,
}));

function renderAppearance() {
  return render(
    <SettingsSaveProvider>
      <AppearanceSettings />
    </SettingsSaveProvider>,
  );
}

function appearanceSettingsResponse(overrides: Record<string, unknown> = {}): {
  settings: Record<string, unknown>;
} {
  return {
    settings: {
      startup_page: "task_overview",
      changes_panel_layout: "tree",
      app_status_bar_enabled: true,
      system_metrics_display: { show_in_topbar: false, simplified: false },
      ...overrides,
    },
  };
}

async function verifyUntouchedExternalAppearanceValueIsNotOverwritten() {
  apiMocks.updateUserSettings.mockResolvedValue(
    appearanceSettingsResponse({
      app_status_bar_enabled: false,
      changes_panel_layout: "flat",
    }),
  );
  const view = renderAppearance();

  fireEvent.click(screen.getByRole("switch", { name: SHOW_STATUS_BAR_LABEL }));
  storeMocks.state = {
    ...storeMocks.state,
    userSettings: {
      ...(storeMocks.state.userSettings as typeof defaultSettingsState.userSettings),
      changesPanelLayout: "flat",
    },
  };
  view.rerender(
    <SettingsSaveProvider>
      <AppearanceSettings />
    </SettingsSaveProvider>,
  );

  fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_LABEL }));

  await waitFor(() => expect(apiMocks.updateUserSettings).toHaveBeenCalledOnce());
  const payload = apiMocks.updateUserSettings.mock.calls[0]?.[0];
  expect(payload).toMatchObject({ app_status_bar_enabled: false });
  expect(payload).not.toHaveProperty("changes_panel_layout");
}

async function verifyNewerLiveUpdateWinsOverOlderSaveResponse() {
  let resolveSave: (value: ReturnType<typeof appearanceSettingsResponse>) => void = () => {};
  apiMocks.updateUserSettings.mockReturnValueOnce(
    new Promise((resolve) => {
      resolveSave = resolve;
    }),
  );
  const view = renderAppearance();

  fireEvent.click(screen.getByRole("switch", { name: SHOW_STATUS_BAR_LABEL }));
  fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_LABEL }));
  await waitFor(() => expect(apiMocks.updateUserSettings).toHaveBeenCalledOnce());

  storeMocks.state = {
    ...storeMocks.state,
    userSettings: {
      ...(storeMocks.state.userSettings as typeof defaultSettingsState.userSettings),
      appStatusBarEnabled: true,
    },
  };
  view.rerender(
    <SettingsSaveProvider>
      <AppearanceSettings />
    </SettingsSaveProvider>,
  );
  resolveSave(appearanceSettingsResponse({ app_status_bar_enabled: false }));

  await waitFor(() => expect(storeMocks.setUserSettings).toHaveBeenCalledOnce());
  expect(storeMocks.setUserSettings).toHaveBeenCalledWith(
    expect.objectContaining({ appStatusBarEnabled: true }),
  );
}

async function verifyEditDuringSaveSurvivesItsLiveUpdate() {
  let resolveSave: (value: ReturnType<typeof appearanceSettingsResponse>) => void = () => {};
  apiMocks.updateUserSettings.mockReturnValueOnce(
    new Promise((resolve) => {
      resolveSave = resolve;
    }),
  );
  const view = renderAppearance();
  const toggle = screen.getByRole("switch", { name: SHOW_STATUS_BAR_LABEL });

  fireEvent.click(toggle);
  fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_LABEL }));
  await waitFor(() => expect(apiMocks.updateUserSettings).toHaveBeenCalledOnce());

  fireEvent.click(toggle);
  storeMocks.state = {
    ...storeMocks.state,
    userSettings: {
      ...(storeMocks.state.userSettings as typeof defaultSettingsState.userSettings),
      appStatusBarEnabled: false,
    },
  };
  view.rerender(
    <SettingsSaveProvider>
      <AppearanceSettings />
    </SettingsSaveProvider>,
  );

  await waitFor(() => expect(toggle.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(CHECKED_STATE));
  resolveSave(appearanceSettingsResponse({ app_status_bar_enabled: false }));
  await waitFor(() => expect(storeMocks.setUserSettings).toHaveBeenCalledOnce());
  expect(toggle.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(CHECKED_STATE);
  expect(toggle.getAttribute("data-settings-dirty")).toBe("true");
}

beforeEach(() => {
  apiMocks.updateUserSettings.mockReset();
  storeMocks.setUserSettings.mockReset();
  themeMocks.previewTheme.mockReset();
  themeMocks.commitTheme.mockReset();
  themeMocks.restoreTheme.mockReset();
  storeMocks.previewSettingsMenuMode.mockReset();
  storeMocks.commitSettingsMenuMode.mockReset();
  storeMocks.restoreSettingsMenuMode.mockReset();
  storeMocks.state = {
    userSettings: {
      ...defaultSettingsState.userSettings,
      appStatusBarEnabled: true,
    },
    settingsMenu: { savedMode: "flat" },
    setUserSettings: storeMocks.setUserSettings,
    previewSettingsMenuMode: storeMocks.previewSettingsMenuMode,
    commitSettingsMenuMode: storeMocks.commitSettingsMenuMode,
    restoreSettingsMenuMode: storeMocks.restoreSettingsMenuMode,
  };
});

afterEach(cleanup);

describe("AppearanceSettings status bar preference", () => {
  it("saves through the shared appearance action without optimistic store mutation", async () => {
    apiMocks.updateUserSettings.mockResolvedValue(
      appearanceSettingsResponse({ app_status_bar_enabled: false }),
    );
    renderAppearance();

    const toggle = screen.getByRole("switch", { name: SHOW_STATUS_BAR_LABEL });
    expect(toggle.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(CHECKED_STATE);

    fireEvent.click(toggle);

    expect(apiMocks.updateUserSettings).not.toHaveBeenCalled();
    expect(storeMocks.setUserSettings).not.toHaveBeenCalled();
    expect(toggle.getAttribute("data-settings-dirty")).toBe("true");

    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_LABEL }));

    await waitFor(() => expect(apiMocks.updateUserSettings).toHaveBeenCalledOnce());
    expect(apiMocks.updateUserSettings).toHaveBeenCalledWith(
      expect.objectContaining({ app_status_bar_enabled: false }),
    );
    expect(storeMocks.setUserSettings).toHaveBeenCalledWith(
      expect.objectContaining({ appStatusBarEnabled: false }),
    );
    await waitFor(() =>
      expect(screen.getByTestId("settings-floating-save").getAttribute("data-status")).toBe(
        "saved",
      ),
    );
  });

  it("restores the saved preference through Reset", async () => {
    renderAppearance();

    const toggle = screen.getByRole("switch", { name: SHOW_STATUS_BAR_LABEL });
    fireEvent.click(toggle);
    fireEvent.click(await screen.findByRole("button", { name: "Reset" }));

    await waitFor(() => expect(toggle.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(CHECKED_STATE));
    expect(apiMocks.updateUserSettings).not.toHaveBeenCalled();
    expect(storeMocks.setUserSettings).not.toHaveBeenCalled();
  });

  it("keeps the draft dirty and confirmed state unchanged after a failed save", async () => {
    apiMocks.updateUserSettings.mockRejectedValueOnce(new Error("offline"));
    renderAppearance();

    const toggle = screen.getByRole("switch", { name: SHOW_STATUS_BAR_LABEL });
    fireEvent.click(toggle);
    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_LABEL }));

    expect(await screen.findByText("Couldn't save")).toBeTruthy();
    expect(toggle.getAttribute(DATA_STATE_ATTRIBUTE)).toBe("unchecked");
    expect(toggle.getAttribute("data-settings-dirty")).toBe("true");
    expect(storeMocks.setUserSettings).not.toHaveBeenCalled();
    expect(
      (storeMocks.state.userSettings as { appStatusBarEnabled: boolean }).appStatusBarEnabled,
    ).toBe(true);
  });

  it(
    "does not overwrite a newer untouched Appearance value from another client",
    verifyUntouchedExternalAppearanceValueIsNotOverwritten,
  );

  it(
    "does not replace a newer live update when an older save response finishes",
    verifyNewerLiveUpdateWinsOverOlderSaveResponse,
  );

  it(
    "keeps an edit made during save when the submitted value arrives live",
    verifyEditDuringSaveSurvivesItsLiveUpdate,
  );
});
