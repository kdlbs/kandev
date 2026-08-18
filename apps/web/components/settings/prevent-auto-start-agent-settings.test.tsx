import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import { SettingsSaveProvider } from "./settings-save-provider";

const updateUserSettings = vi.fn();

vi.mock("@/lib/api", () => ({
  updateUserSettings: (...args: unknown[]) => updateUserSettings(...args),
}));

import { PreventAutoStartAgentSettings } from "./prevent-auto-start-agent-settings";

function renderSettings(preventAutoStartAgentOnOpen = false) {
  return render(
    <StateProvider
      initialState={{
        userSettings: { ...defaultState.userSettings, preventAutoStartAgentOnOpen },
      }}
    >
      <SettingsSaveProvider>
        <PreventAutoStartAgentSettings />
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

describe("PreventAutoStartAgentSettings", () => {
  it("keeps an explicit true value local until Save changes is pressed", async () => {
    renderSettings(true);

    const toggle = screen.getByRole("switch", { name: "Prevent auto-start on open" });
    expect(toggle.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(CHECKED_STATE);
    expect(updateUserSettings).not.toHaveBeenCalled();

    fireEvent.click(toggle);
    expect(toggle.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(UNCHECKED_STATE);

    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await waitFor(() => {
      expect(updateUserSettings).toHaveBeenCalledWith({
        prevent_auto_start_agent_on_open: false,
      });
    });
    expect(toggle.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(UNCHECKED_STATE);
  });

  it("persists enabling the preference", async () => {
    renderSettings(false);

    const toggle = screen.getByRole("switch", { name: "Prevent auto-start on open" });
    fireEvent.click(toggle);
    expect(toggle.getAttribute(DATA_STATE_ATTRIBUTE)).toBe(CHECKED_STATE);

    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await waitFor(() => {
      expect(updateUserSettings).toHaveBeenCalledWith({
        prevent_auto_start_agent_on_open: true,
      });
    });
  });

  it("registers its own save contributor so the archive card keeps working", async () => {
    renderSettings(false);
    fireEvent.click(screen.getByRole("switch", { name: "Prevent auto-start on open" }));
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await waitFor(() => {
      expect(updateUserSettings).toHaveBeenCalledWith({
        prevent_auto_start_agent_on_open: true,
      });
    });
  });
});
