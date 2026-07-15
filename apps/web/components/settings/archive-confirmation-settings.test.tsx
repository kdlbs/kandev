import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";

const updateUserSettings = vi.fn();

vi.mock("@/lib/api", () => ({
  updateUserSettings: (...args: unknown[]) => updateUserSettings(...args),
}));

import { ArchiveConfirmationSettings } from "./archive-confirmation-settings";

function renderSettings(confirmTaskArchive = true) {
  return render(
    <StateProvider
      initialState={{
        userSettings: { ...defaultState.userSettings, confirmTaskArchive },
      }}
    >
      <ArchiveConfirmationSettings />
    </StateProvider>,
  );
}

beforeEach(() => {
  updateUserSettings.mockReset().mockResolvedValue({ settings: {} });
});

afterEach(cleanup);

describe("ArchiveConfirmationSettings", () => {
  it("is enabled by default and persists an explicit false value", async () => {
    renderSettings();
    const toggle = screen.getByRole("switch", { name: "Confirm before archiving tasks" });

    expect(toggle.getAttribute("data-state")).toBe("checked");
    fireEvent.click(toggle);

    await waitFor(() =>
      expect(updateUserSettings).toHaveBeenCalledWith({ confirm_task_archive: false }),
    );
    expect(toggle.getAttribute("data-state")).toBe("unchecked");
  });

  it("rolls back when saving fails", async () => {
    updateUserSettings.mockRejectedValueOnce(new Error("save failed"));
    renderSettings();
    const toggle = screen.getByRole("switch", { name: "Confirm before archiving tasks" });

    fireEvent.click(toggle);

    await waitFor(() => expect(toggle.getAttribute("data-state")).toBe("checked"));
  });
});
