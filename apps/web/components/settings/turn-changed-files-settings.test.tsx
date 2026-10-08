import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStore } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import { SettingsSaveProvider } from "./settings-save-provider";

const updateUserSettings = vi.fn();
const TOGGLE_LABEL = "Show changed files after each turn";
const SAVE_LABEL = "Save changes";
const DISCARD_LABEL = "Reset";
const ARIA_CHECKED_ATTRIBUTE = "aria-checked";
const TRUE_TEXT = "true";
const FALSE_TEXT = "false";

vi.mock("@/lib/api", () => ({
  updateUserSettings: (...args: unknown[]) => updateUserSettings(...args),
}));

import { TurnChangedFilesSettings } from "./turn-changed-files-settings";

function EffectivePreference() {
  const enabled = useAppStore((state) => state.userSettings.showTurnChangedFiles);
  return <output data-testid="effective-turn-changed-files">{String(enabled)}</output>;
}

function renderSettings() {
  return render(
    <StateProvider
      initialState={{
        userSettings: { ...defaultState.userSettings, showTurnChangedFiles: true },
      }}
    >
      <SettingsSaveProvider>
        <TurnChangedFilesSettings />
        <EffectivePreference />
      </SettingsSaveProvider>
    </StateProvider>,
  );
}

beforeEach(() => {
  updateUserSettings.mockReset().mockResolvedValue({
    settings: { show_turn_changed_files: false, revision: 2 },
  });
});

afterEach(cleanup);

describe("TurnChangedFilesSettings", () => {
  it("keeps a draft local, supports discard, and saves the explicit false value", async () => {
    renderSettings();
    const toggle = screen.getByRole("switch", { name: TOGGLE_LABEL });
    expect(toggle.getAttribute(ARIA_CHECKED_ATTRIBUTE)).toBe(TRUE_TEXT);
    fireEvent.click(toggle);
    expect(toggle.getAttribute(ARIA_CHECKED_ATTRIBUTE)).toBe(FALSE_TEXT);
    expect(screen.getByTestId("effective-turn-changed-files").textContent).toBe(TRUE_TEXT);
    expect(updateUserSettings).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: DISCARD_LABEL }));
    await waitFor(() => expect(toggle.getAttribute(ARIA_CHECKED_ATTRIBUTE)).toBe(TRUE_TEXT));
    expect(updateUserSettings).not.toHaveBeenCalled();

    fireEvent.click(toggle);
    fireEvent.click(screen.getByRole("button", { name: SAVE_LABEL }));
    await waitFor(() =>
      expect(updateUserSettings).toHaveBeenCalledWith({ show_turn_changed_files: false }),
    );
    await waitFor(() =>
      expect(screen.getByTestId("effective-turn-changed-files").textContent).toBe(FALSE_TEXT),
    );
    expect(toggle.getAttribute(ARIA_CHECKED_ATTRIBUTE)).toBe(FALSE_TEXT);
  });

  it("keeps a newer draft when the saved response arrives", async () => {
    let resolveSave: (response: {
      settings: { show_turn_changed_files: boolean; revision: number };
    }) => void = () => undefined;
    updateUserSettings.mockImplementationOnce(
      () => new Promise((resolve) => (resolveSave = resolve)),
    );
    renderSettings();
    const toggle = screen.getByRole("switch", { name: TOGGLE_LABEL });
    fireEvent.click(toggle);
    fireEvent.click(screen.getByRole("button", { name: SAVE_LABEL }));
    await waitFor(() => expect(updateUserSettings).toHaveBeenCalled());
    fireEvent.click(toggle);

    await act(async () => {
      resolveSave({ settings: { show_turn_changed_files: false, revision: 2 } });
    });
    await waitFor(() =>
      expect(screen.getByTestId("effective-turn-changed-files").textContent).toBe(FALSE_TEXT),
    );
    expect(toggle.getAttribute(ARIA_CHECKED_ATTRIBUTE)).toBe(TRUE_TEXT);
    expect(toggle.getAttribute("data-settings-dirty")).toBe("true");
  });
});
