import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import { SettingsSaveProvider } from "./settings-save-provider";

const updateUserSettings = vi.fn();
const TOGGLE_LABEL = "Show anchored prompt bar";
const DATA_STATE = "data-state";

vi.mock("@/lib/api", () => ({
  updateUserSettings: (...args: unknown[]) => updateUserSettings(...args),
}));

import { AnchoredPromptBarSettings } from "./anchored-prompt-bar-settings";
function renderSettings(showAnchoredPromptBar = true) {
  return render(
    <StateProvider
      initialState={{
        userSettings: {
          ...defaultState.userSettings,
          workspaceId: "workspace-1",
          showAnchoredPromptBar,
        },
      }}
    >
      <SettingsSaveProvider>
        <AnchoredPromptBarSettings />
      </SettingsSaveProvider>
    </StateProvider>,
  );
}

beforeEach(() => {
  updateUserSettings.mockReset().mockResolvedValue({ settings: {} });
});

afterEach(cleanup);

describe("AnchoredPromptBarSettings", () => {
  it("renders enabled by default with a desktop-only explanation", () => {
    renderSettings();

    const toggle = screen.getByRole("switch", { name: TOGGLE_LABEL });
    expect(toggle.getAttribute(DATA_STATE)).toBe("checked");
    screen.getByText(/desktop only/i);
    screen.getByText(/show scroll to last prompt/i);
  });

  it("keeps the choice local until Save changes is pressed", async () => {
    renderSettings();
    const toggle = screen.getByRole("switch", { name: TOGGLE_LABEL });

    fireEvent.click(toggle);
    expect(toggle.getAttribute(DATA_STATE)).toBe("unchecked");
    expect(updateUserSettings).not.toHaveBeenCalled();

    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));

    await waitFor(() =>
      expect(updateUserSettings).toHaveBeenCalledWith({ show_anchored_prompt_bar: false }),
    );
  });

  it("reflects an already-enabled preference", () => {
    renderSettings(true);

    const toggle = screen.getByRole("switch", { name: TOGGLE_LABEL });
    expect(toggle.getAttribute(DATA_STATE)).toBe("checked");
  });

  it("saves each transcript navigation control independently", async () => {
    renderSettings();

    const lastPromptToggle = screen.getByRole("switch", { name: "Show scroll to last prompt" });
    const startToggle = screen.getByRole("switch", { name: "Show scroll to start" });
    expect(lastPromptToggle.getAttribute(DATA_STATE)).toBe("checked");
    expect(startToggle.getAttribute(DATA_STATE)).toBe("checked");

    fireEvent.click(lastPromptToggle);
    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));

    await waitFor(() =>
      expect(updateUserSettings).toHaveBeenCalledWith({ show_scroll_to_last_prompt: false }),
    );

    fireEvent.click(startToggle);
    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));

    await waitFor(() =>
      expect(updateUserSettings).toHaveBeenLastCalledWith({ show_scroll_to_start: false }),
    );
  });
});
