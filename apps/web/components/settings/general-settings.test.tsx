import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defaultSettingsState } from "@/lib/state/slices/settings/settings-slice";
import { SettingsSaveProvider } from "./settings-save-provider";
import { AppearanceSettings } from "./general-settings";

const apiMocks = vi.hoisted(() => ({ updateUserSettings: vi.fn() }));
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
    apiMocks.updateUserSettings.mockResolvedValue({});
    renderAppearance();

    const toggle = screen.getByRole("switch", { name: "Show status bar" });
    expect(toggle.getAttribute("data-state")).toBe("checked");

    fireEvent.click(toggle);

    expect(apiMocks.updateUserSettings).not.toHaveBeenCalled();
    expect(storeMocks.setUserSettings).not.toHaveBeenCalled();
    expect(toggle.getAttribute("data-settings-dirty")).toBe("true");

    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(apiMocks.updateUserSettings).toHaveBeenCalledOnce());
    expect(apiMocks.updateUserSettings).toHaveBeenCalledWith(
      expect.objectContaining({ app_status_bar_enabled: false }),
    );
    expect(storeMocks.setUserSettings).toHaveBeenCalledWith(
      expect.objectContaining({ appStatusBarEnabled: false }),
    );
  });

  it("restores the saved preference through Reset", async () => {
    renderAppearance();

    const toggle = screen.getByRole("switch", { name: "Show status bar" });
    fireEvent.click(toggle);
    fireEvent.click(await screen.findByRole("button", { name: "Reset" }));

    await waitFor(() => expect(toggle.getAttribute("data-state")).toBe("checked"));
    expect(apiMocks.updateUserSettings).not.toHaveBeenCalled();
    expect(storeMocks.setUserSettings).not.toHaveBeenCalled();
  });

  it("keeps the draft dirty and confirmed state unchanged after a failed save", async () => {
    apiMocks.updateUserSettings.mockRejectedValueOnce(new Error("offline"));
    renderAppearance();

    const toggle = screen.getByRole("switch", { name: "Show status bar" });
    fireEvent.click(toggle);
    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));

    expect(await screen.findByText("Couldn't save")).toBeTruthy();
    expect(toggle.getAttribute("data-state")).toBe("unchecked");
    expect(toggle.getAttribute("data-settings-dirty")).toBe("true");
    expect(storeMocks.setUserSettings).not.toHaveBeenCalled();
    expect(
      (storeMocks.state.userSettings as { appStatusBarEnabled: boolean }).appStatusBarEnabled,
    ).toBe(true);
  });
});
