import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import type { AgentProfileOption } from "@/lib/state/slices";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { SettingsSaveProvider } from "./settings-save-provider";

const updateUserSettings = vi.fn();
const SELECT_TESTID = "utility-agent-profile-select";
const CLAUDE_FAST_LABEL = "Claude • Fast";

vi.mock("@/lib/api", () => ({
  updateUserSettings: (...args: unknown[]) => updateUserSettings(...args),
}));

import { UtilityAgentProfileSettings } from "./utility-agent-profile-settings";

const PROFILES: AgentProfileOption[] = [
  {
    id: "profile-1",
    label: CLAUDE_FAST_LABEL,
    agent_id: "agent-1",
    agent_name: "Claude",
    cli_passthrough: false,
  },
  {
    id: "profile-2",
    label: "Codex • Thorough",
    agent_id: "agent-2",
    agent_name: "Codex",
    cli_passthrough: false,
  },
];

function renderSettings(utilityAgentProfileId: string | null = null) {
  return render(
    <StateProvider
      initialState={{
        userSettings: {
          ...defaultState.userSettings,
          workspaceId: "workspace-1",
          utilityAgentProfileId,
        },
        agentProfiles: { items: PROFILES, version: 0 },
      }}
    >
      <TooltipProvider delayDuration={0}>
        <SettingsSaveProvider>
          <UtilityAgentProfileSettings />
        </SettingsSaveProvider>
      </TooltipProvider>
    </StateProvider>,
  );
}

beforeEach(() => {
  updateUserSettings.mockReset().mockResolvedValue({ settings: {} });
});

afterEach(cleanup);

describe("UtilityAgentProfileSettings", () => {
  it("renders the heading, description, and available profiles", () => {
    renderSettings();

    screen.getByRole("heading", { name: "Utility agent" });
    screen.getByText(/lightweight one-shot LLM calls that plugins delegate to/i);
    screen.getByText("agent_invoke");
    const trigger = screen.getByTestId(SELECT_TESTID);
    expect(trigger.textContent).toContain("None");
  });

  it("defaults to None when no profile is configured", () => {
    renderSettings(null);
    expect(screen.getByTestId(SELECT_TESTID).textContent).toContain("None");
  });

  it("shows the currently configured profile label", () => {
    renderSettings("profile-2");
    expect(screen.getByTestId(SELECT_TESTID).textContent).toContain("Codex • Thorough");
  });

  it("persists the selected profile only after Save changes is pressed", async () => {
    renderSettings();
    const trigger = screen.getByTestId(SELECT_TESTID);

    fireEvent.click(trigger);
    fireEvent.click(await screen.findByText(CLAUDE_FAST_LABEL));

    expect(updateUserSettings).not.toHaveBeenCalled();
    expect(trigger.getAttribute("data-settings-dirty")).toBe("true");

    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));

    await waitFor(() =>
      expect(updateUserSettings).toHaveBeenCalledWith({
        utility_agent_profile_id: "profile-1",
      }),
    );
    await waitFor(() => expect(trigger.getAttribute("data-settings-dirty")).toBe("false"));
  });

  it("clears the setting when None is chosen after a profile was configured", async () => {
    renderSettings("profile-1");
    const trigger = screen.getByTestId(SELECT_TESTID);

    fireEvent.click(trigger);
    fireEvent.click(await screen.findByText("None"));
    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));

    await waitFor(() =>
      expect(updateUserSettings).toHaveBeenCalledWith({ utility_agent_profile_id: "" }),
    );
  });

  it("keeps the draft selected when saving fails", async () => {
    updateUserSettings.mockRejectedValueOnce(new Error("save failed"));
    renderSettings();
    const trigger = screen.getByTestId(SELECT_TESTID);

    fireEvent.click(trigger);
    fireEvent.click(await screen.findByText(CLAUDE_FAST_LABEL));
    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(trigger.textContent).toContain(CLAUDE_FAST_LABEL));
    await waitFor(() => expect(screen.getByText("Couldn't save")).toBeTruthy());
  });
});
