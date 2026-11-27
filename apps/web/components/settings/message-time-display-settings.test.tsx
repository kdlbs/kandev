import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import type { AppState } from "@/lib/state/store";
import type { UserSettingsState } from "@/lib/state/slices/settings/types";
import {
  SettingsSaveProvider,
  useSettingsSaveCoordinator,
  type SettingsSaveCoordinator,
} from "./settings-save-provider";
import { MessageTimeDisplaySettings } from "./message-time-display-settings";

const mocks = vi.hoisted(() => ({ updateUserSettings: vi.fn() }));
vi.mock("@/lib/api", () => ({ updateUserSettings: mocks.updateUserSettings }));

const MESSAGE_TIME_LABEL = "Message time";

beforeEach(() => {
  mocks.updateUserSettings.mockReset().mockResolvedValue({
    settings: { message_time_display: "absolute_short", revision: 2 },
  });
});
afterEach(cleanup);

function renderSettings(overrides: Partial<UserSettingsState> = {}): {
  store: StoreApi<AppState>;
  coordinator: SettingsSaveCoordinator;
} {
  let store: StoreApi<AppState> | null = null;
  let coordinator: SettingsSaveCoordinator | null = null;
  function Probe() {
    store = useAppStoreApi();
    coordinator = useSettingsSaveCoordinator();
    return null;
  }

  render(
    <StateProvider initialState={{ userSettings: { ...defaultState.userSettings, ...overrides } }}>
      <SettingsSaveProvider>
        <Probe />
        <MessageTimeDisplaySettings />
      </SettingsSaveProvider>
    </StateProvider>,
  );
  if (!store || !coordinator) throw new Error("Settings providers were not captured");
  return { store, coordinator };
}

async function choose(label: string) {
  fireEvent.click(screen.getByRole("combobox", { name: MESSAGE_TIME_LABEL }));
  fireEvent.click(await screen.findByRole("option", { name: label }));
}

describe("MessageTimeDisplaySettings", () => {
  it("keeps selection as a draft until Save and persists only the setting", async () => {
    const { store } = renderSettings();
    expect(
      screen.getByRole("combobox", { name: MESSAGE_TIME_LABEL }).getAttribute("aria-describedby"),
    ).toBe("message-time-display-description");
    await choose("Absolute (short)");
    expect(mocks.updateUserSettings).not.toHaveBeenCalled();
    expect(
      screen.getByTestId("message-time-display-settings-card").getAttribute("data-settings-dirty"),
    ).toBe("true");

    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));
    await waitFor(() =>
      expect(mocks.updateUserSettings).toHaveBeenCalledWith({
        message_time_display: "absolute_short",
      }),
    );
    await waitFor(() =>
      expect(store.getState().userSettings.messageTimeDisplay).toBe("absolute_short"),
    );
  });

  it("Reset restores the confirmed preference without a request", async () => {
    renderSettings({ messageTimeDisplay: "absolute_long" });
    await choose("Relative");
    fireEvent.click(await screen.findByRole("button", { name: "Reset" }));

    expect(mocks.updateUserSettings).not.toHaveBeenCalled();
    expect(screen.getByRole("combobox", { name: MESSAGE_TIME_LABEL }).textContent).toContain(
      "Absolute (long)",
    );
  });

  it("does not replace a newer setting with a stale save response", async () => {
    const deferred = Promise.withResolvers<{
      settings: { message_time_display: string; revision: number };
    }>();
    mocks.updateUserSettings.mockReturnValueOnce(deferred.promise);
    const { store, coordinator } = renderSettings();
    await choose("Absolute (short)");
    let savePromise!: Promise<unknown>;
    act(() => {
      savePromise = coordinator.saveAll();
    });
    await waitFor(() => expect(mocks.updateUserSettings).toHaveBeenCalledOnce());

    act(() => {
      store.getState().setUserSettings({
        ...store.getState().userSettings,
        messageTimeDisplay: "absolute_long",
        revision: 3,
      });
    });
    await act(async () => {
      deferred.resolve({ settings: { message_time_display: "absolute_short", revision: 2 } });
      await savePromise;
    });
    await expect(savePromise).resolves.toMatchObject({ canLeave: false });

    expect(store.getState().userSettings.messageTimeDisplay).toBe("absolute_long");
    expect(screen.getByRole("combobox", { name: MESSAGE_TIME_LABEL }).textContent).toContain(
      "Absolute (short)",
    );
    expect(
      screen.getByTestId("message-time-display-settings-card").getAttribute("data-settings-dirty"),
    ).toBe("true");
  });

  it("retains a dirty draft and confirmed store value after a failed save", async () => {
    mocks.updateUserSettings.mockRejectedValueOnce(new Error("save failed"));
    const { store } = renderSettings();
    await choose("Absolute (long)");
    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(screen.getByText("Couldn't save")).toBeTruthy());
    expect(store.getState().userSettings.messageTimeDisplay).toBe("relative");
    expect(
      screen.getByTestId("message-time-display-settings-card").getAttribute("data-settings-dirty"),
    ).toBe("true");
  });
});
