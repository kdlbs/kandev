import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import { SettingsSaveProvider } from "./settings-save-provider";

const updateUserSettings = vi.fn();

vi.mock("@/lib/api", () => ({
  updateUserSettings: (...args: unknown[]) => updateUserSettings(...args),
}));

import { AgentGeneratedTaskTitleSettings } from "./agent-generated-task-title-settings";

function renderSettings(agentGeneratedTaskTitles = false) {
  return render(
    <StateProvider
      initialState={{
        userSettings: { ...defaultState.userSettings, agentGeneratedTaskTitles },
      }}
    >
      <SettingsSaveProvider>
        <AgentGeneratedTaskTitleSettings />
      </SettingsSaveProvider>
    </StateProvider>,
  );
}

beforeEach(() => {
  updateUserSettings.mockReset().mockResolvedValue({ settings: {} });
});

afterEach(cleanup);

describe("AgentGeneratedTaskTitleSettings", () => {
  it("defaults to disabled and explains the prompt-first behavior", () => {
    renderSettings();

    const toggle = screen.getByRole("switch", { name: "Use the agent for new task titles" });
    expect(toggle.getAttribute("data-state")).toBe("unchecked");
    expect(screen.getByText(/first six words/i)).toBeTruthy();
    expect(screen.getByText(/targeting about three words/i)).toBeTruthy();
    expect(
      screen.getByText(/existing and edited tasks keep their normal title field/i),
    ).toBeTruthy();
  });

  it("keeps the draft local until Save changes is pressed", async () => {
    renderSettings();
    const toggle = screen.getByRole("switch", { name: "Use the agent for new task titles" });

    fireEvent.click(toggle);

    expect(updateUserSettings).not.toHaveBeenCalled();
    expect(toggle.getAttribute("data-state")).toBe("checked");
    expect(
      screen.getByTestId("agent-generated-task-title-card").getAttribute("data-settings-dirty"),
    ).toBe("true");

    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));

    await waitFor(() =>
      expect(updateUserSettings).toHaveBeenCalledWith({ agent_generated_task_titles: true }),
    );
    await waitFor(() =>
      expect(
        screen.getByTestId("agent-generated-task-title-card").getAttribute("data-settings-dirty"),
      ).toBe("false"),
    );
  });
});
