import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider, useAppStore } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import { SettingsSaveProvider } from "./settings-save-provider";

const updateUserSettings = vi.fn();
const TOGGLE_LABEL = "Auto-focus new tasks";
const SAVE_LABEL = "Save changes";

vi.mock("@/lib/api", () => ({
  updateUserSettings: (...args: unknown[]) => updateUserSettings(...args),
}));

import { CreationAutoFocusSettings } from "./creation-auto-focus-settings";

function EffectivePreference() {
  const enabled = useAppStore((state) => state.userSettings.autoFocusNewTasks);
  return <output data-testid="effective-auto-focus">{String(enabled)}</output>;
}

function renderSettings(autoFocusNewTasks = true) {
  return render(
    <StateProvider
      initialState={{
        userSettings: { ...defaultState.userSettings, autoFocusNewTasks },
      }}
    >
      <SettingsSaveProvider>
        <CreationAutoFocusSettings />
        <EffectivePreference />
      </SettingsSaveProvider>
    </StateProvider>,
  );
}

const DATA_STATE_ATTRIBUTE = "data-state";
const CHECKED_STATE = "checked";
const UNCHECKED_STATE = "unchecked";

beforeEach(() => {
  updateUserSettings.mockReset().mockResolvedValue({ settings: {} });
});

afterEach(cleanup);

describe("CreationAutoFocusSettings", () => {
  it("keeps an explicit true value local until Save changes is pressed", async () => {
    renderSettings(true);

    const toggle = screen.getByRole("switch", { name: TOGGLE_LABEL });
    expect(toggle.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(CHECKED_STATE);
    expect(updateUserSettings).not.toHaveBeenCalled();

    fireEvent.click(toggle);
    expect(toggle.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(UNCHECKED_STATE);

    fireEvent.click(screen.getByRole("button", { name: SAVE_LABEL }));
    await waitFor(() => {
      expect(updateUserSettings).toHaveBeenCalledWith({
        auto_focus_new_tasks: false,
      });
    });
    expect(toggle.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(UNCHECKED_STATE);
  });

  it("persists enabling the preference", async () => {
    renderSettings(false);

    const toggle = screen.getByRole("switch", { name: TOGGLE_LABEL });
    fireEvent.click(toggle);
    expect(toggle.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(CHECKED_STATE);

    fireEvent.click(screen.getByRole("button", { name: SAVE_LABEL }));
    await waitFor(() => {
      expect(updateUserSettings).toHaveBeenCalledWith({
        auto_focus_new_tasks: true,
      });
    });
  });

  it("registers its own save contributor so the archive card keeps working", async () => {
    renderSettings(false);
    fireEvent.click(screen.getByRole("switch", { name: TOGGLE_LABEL }));
    fireEvent.click(screen.getByRole("button", { name: SAVE_LABEL }));
    await waitFor(() => {
      expect(updateUserSettings).toHaveBeenCalledWith({
        auto_focus_new_tasks: true,
      });
    });
  });
});

it("discards an unsaved change without altering effective behavior", async () => {
  renderSettings();
  fireEvent.click(screen.getByRole("switch", { name: TOGGLE_LABEL }));
  expect(screen.getByTestId("effective-auto-focus").textContent).toBe("true");
  fireEvent.click(screen.getByRole("button", { name: "Reset" }));
  await waitFor(() =>
    expect(screen.getByRole("switch").getAttribute("data-state")).toBe("checked"),
  );
  expect(updateUserSettings).not.toHaveBeenCalled();
});

it("keeps the saved value effective after a failed save and allows retry", async () => {
  updateUserSettings.mockRejectedValueOnce(new Error("save failed"));
  renderSettings();
  fireEvent.click(screen.getByRole("switch", { name: TOGGLE_LABEL }));
  fireEvent.click(screen.getByRole("button", { name: SAVE_LABEL }));
  await screen.findByRole("button", { name: "Retry save" });
  expect(screen.getByTestId("effective-auto-focus").textContent).toBe("true");
  expect(screen.getByRole("switch").getAttribute("data-settings-dirty")).toBe("true");
  fireEvent.click(screen.getByRole("button", { name: "Retry save" }));
  await waitFor(() => expect(screen.getByTestId("effective-auto-focus").textContent).toBe("false"));
});
