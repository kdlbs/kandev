import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import { SettingsSaveProvider } from "./settings-save-provider";

const updateUserSettings = vi.fn();
const TOGGLE_LABEL = "Show anchored prompt bar";
const LAST_PROMPT_LABEL = "Show scroll to last prompt";
const SCROLL_TO_START_LABEL = "Show scroll to start";
const AUTO_SCROLL_CONTROL_LABEL = "Show transcript auto-scroll control";
const SAVE_CHANGES_LABEL = "Save changes";
const DATA_STATE = "data-state";

vi.mock("@/lib/api", () => ({
  updateUserSettings: (...args: unknown[]) => updateUserSettings(...args),
}));

import { AnchoredPromptBarSettings } from "./anchored-prompt-bar-settings";
function renderSettings(showAnchoredPromptBar = false) {
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
  it("renders the documented transcript-navigation defaults", () => {
    renderSettings();

    expect(screen.getByRole("switch", { name: TOGGLE_LABEL }).getAttribute(DATA_STATE)).toBe(
      "unchecked",
    );
    expect(screen.getByRole("switch", { name: LAST_PROMPT_LABEL }).getAttribute(DATA_STATE)).toBe(
      "checked",
    );
    expect(
      screen.getByRole("switch", { name: SCROLL_TO_START_LABEL }).getAttribute(DATA_STATE),
    ).toBe("unchecked");
    expect(
      screen.getByRole("switch", { name: AUTO_SCROLL_CONTROL_LABEL }).getAttribute(DATA_STATE),
    ).toBe("checked");
    screen.getByText(/desktop only/i);
    screen.getByText(/show scroll to last prompt/i);
  });

  it("keeps the choice local until Save changes is pressed", async () => {
    renderSettings(true);
    const toggle = screen.getByRole("switch", { name: TOGGLE_LABEL });

    fireEvent.click(toggle);
    expect(toggle.getAttribute(DATA_STATE)).toBe("unchecked");
    expect(updateUserSettings).not.toHaveBeenCalled();

    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_LABEL }));

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

    const lastPromptToggle = screen.getByRole("switch", { name: LAST_PROMPT_LABEL });
    const startToggle = screen.getByRole("switch", { name: SCROLL_TO_START_LABEL });
    expect(lastPromptToggle.getAttribute(DATA_STATE)).toBe("checked");
    expect(startToggle.getAttribute(DATA_STATE)).toBe("unchecked");

    fireEvent.click(lastPromptToggle);
    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_LABEL }));

    await waitFor(() =>
      expect(updateUserSettings).toHaveBeenCalledWith({ show_scroll_to_last_prompt: false }),
    );

    fireEvent.click(startToggle);
    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_LABEL }));

    await waitFor(() =>
      expect(updateUserSettings).toHaveBeenLastCalledWith({ show_scroll_to_start: true }),
    );
  });

  it("saves the auto-scroll-control visibility independently", async () => {
    renderSettings();

    const autoScrollControl = screen.getByRole("switch", { name: AUTO_SCROLL_CONTROL_LABEL });
    fireEvent.click(autoScrollControl);
    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_LABEL }));

    await waitFor(() =>
      expect(updateUserSettings).toHaveBeenCalledWith({
        show_transcript_auto_scroll_control: false,
      }),
    );
  });
});
