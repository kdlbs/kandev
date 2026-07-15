import { type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider, useAppStore } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";

const updateUserSettings = vi.fn();
const CONFIRMATION_LABEL = "Confirm before archiving tasks";
const DATA_STATE_ATTRIBUTE = "data-state";
const CHECKED_STATE = "checked";
const UNCHECKED_STATE = "unchecked";

vi.mock("@/lib/api", () => ({
  updateUserSettings: (...args: unknown[]) => updateUserSettings(...args),
}));

import { ArchiveConfirmationSettings } from "./archive-confirmation-settings";

function renderSettings(confirmTaskArchive = true, children?: ReactNode) {
  return render(
    <StateProvider
      initialState={{
        userSettings: { ...defaultState.userSettings, confirmTaskArchive },
      }}
    >
      <ArchiveConfirmationSettings />
      {children}
    </StateProvider>,
  );
}

function RemoteSettingsUpdate() {
  const settings = useAppStore((state) => state.userSettings);
  const setUserSettings = useAppStore((state) => state.setUserSettings);

  return (
    <button
      type="button"
      onClick={() =>
        setUserSettings({ ...settings, workspaceId: "workspace-2", confirmTaskArchive: false })
      }
    >
      Apply remote settings
    </button>
  );
}

beforeEach(() => {
  updateUserSettings.mockReset().mockResolvedValue({ settings: {} });
});

afterEach(cleanup);

describe("ArchiveConfirmationSettings", () => {
  it("is enabled by default and persists an explicit false value", async () => {
    renderSettings();
    const toggle = screen.getByRole("switch", { name: CONFIRMATION_LABEL });

    expect(toggle.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(CHECKED_STATE);
    fireEvent.click(toggle);

    await waitFor(() =>
      expect(updateUserSettings).toHaveBeenCalledWith({ confirm_task_archive: false }),
    );
    expect(toggle.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(UNCHECKED_STATE);
  });

  it("rolls back when saving fails", async () => {
    updateUserSettings.mockRejectedValueOnce(new Error("save failed"));
    renderSettings();
    const toggle = screen.getByRole("switch", { name: CONFIRMATION_LABEL });

    fireEvent.click(toggle);

    await waitFor(() => expect(toggle.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(UNCHECKED_STATE));
    await waitFor(() => expect(toggle.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(CHECKED_STATE));
  });

  it("does not overwrite a newer settings update when saving fails", async () => {
    let rejectSave: (reason?: unknown) => void = () => {};
    updateUserSettings.mockImplementationOnce(
      () =>
        new Promise((_, reject: (reason?: unknown) => void) => {
          rejectSave = reject;
        }),
    );
    renderSettings(true, <RemoteSettingsUpdate />);
    const toggle = screen.getByRole("switch", { name: CONFIRMATION_LABEL });

    fireEvent.click(toggle);
    await waitFor(() => expect(toggle.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(UNCHECKED_STATE));
    fireEvent.click(screen.getByRole("button", { name: "Apply remote settings" }));
    rejectSave(new Error("save failed"));

    await waitFor(() => expect(toggle.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(UNCHECKED_STATE));
  });
});
