import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import { SettingsSaveProvider } from "./settings-save-provider";

const updateUserSettings = vi.fn();
const DIVIDER_LABEL = "Show New divider in transcripts";

vi.mock("@/lib/api", () => ({
  updateUserSettings: (...args: unknown[]) => updateUserSettings(...args),
}));

import { UnreadDividerSettings } from "./unread-divider-settings";

beforeEach(() => {
  updateUserSettings.mockReset().mockResolvedValue({ settings: {} });
});

afterEach(cleanup);

describe("UnreadDividerSettings", () => {
  it("keeps a disabled divider local until Save changes persists the preference", async () => {
    render(
      <StateProvider
        initialState={{ userSettings: { ...defaultState.userSettings, unreadDivider: true } }}
      >
        <SettingsSaveProvider>
          <UnreadDividerSettings />
        </SettingsSaveProvider>
      </StateProvider>,
    );
    const toggle = screen.getByRole("switch", { name: DIVIDER_LABEL });

    expect(toggle.getAttribute("data-state")).toBe("checked");
    fireEvent.click(toggle);

    expect(updateUserSettings).not.toHaveBeenCalled();
    expect(toggle.getAttribute("data-state")).toBe("unchecked");
    expect(
      screen.getByTestId("unread-divider-settings-card").getAttribute("data-settings-dirty"),
    ).toBe("true");

    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(updateUserSettings).toHaveBeenCalledWith({ unread_divider: false }));
    await waitFor(() => expect(toggle.getAttribute("data-settings-dirty")).toBe("false"));
  });
});
