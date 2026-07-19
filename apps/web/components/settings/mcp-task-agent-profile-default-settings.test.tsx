import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import type { MCPTaskAgentProfileDefault } from "@/lib/types/http";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { SettingsSaveProvider } from "./settings-save-provider";

const updateUserSettings = vi.fn();
const ARIA_CHECKED = "aria-checked";
const CURRENT_TASK_LABEL = "Current task profile";
const WORKSPACE_DEFAULT_LABEL = "Workspace default profile";

vi.mock("@/lib/api", () => ({
  updateUserSettings: (...args: unknown[]) => updateUserSettings(...args),
}));

import { MCPTaskAgentProfileDefaultSettings } from "./mcp-task-agent-profile-default-settings";

function renderSettings(preference: MCPTaskAgentProfileDefault = "current_task") {
  return render(
    <StateProvider
      initialState={{
        userSettings: {
          ...defaultState.userSettings,
          workspaceId: "workspace-1",
          mcpTaskAgentProfileDefault: preference,
        },
      }}
    >
      <TooltipProvider delayDuration={0}>
        <SettingsSaveProvider>
          <MCPTaskAgentProfileDefaultSettings />
        </SettingsSaveProvider>
      </TooltipProvider>
    </StateProvider>,
  );
}

beforeEach(() => {
  updateUserSettings.mockReset().mockResolvedValue({ settings: {} });
});

afterEach(cleanup);

describe("MCPTaskAgentProfileDefaultSettings", () => {
  it("renders accessible descriptive choices", () => {
    renderSettings();

    screen.getByRole("heading", { name: "Profile for Tasks Created by Agents" });
    screen.getByText(/when an agent calls a Kandev MCP tool that creates a task/i);
    screen.getByText(/Kandev must assign an agent profile/i);
    screen.getByText("create_task_kandev");
    screen.getByText(/creates new tasks and subtasks/i);
    screen.getByText(/applies only when the call omits/i);
    screen.getByText("agent_profile_id");
    screen.getByText("spawn_session_kandev");
    screen.getByText(/tasks you create yourself are not affected/i);
    screen.getByRole("button", { name: "About affected Kandev MCP tools" });
    expect(screen.getByRole("radio", { name: CURRENT_TASK_LABEL }).getAttribute(ARIA_CHECKED)).toBe(
      "true",
    );
    expect(
      screen.getByRole("radio", { name: WORKSPACE_DEFAULT_LABEL }).getAttribute(ARIA_CHECKED),
    ).toBe("false");
    screen.getByText(/follow-up work needs the same model and agent setup/i);
    screen.getByText(/may reuse a more expensive profile/i);
    screen.getByText(/workflow profile when one is set/i);
    screen.getByText(/keep agent-created tasks on your standard workspace model/i);
  });

  it("explains why session creation does not use this preference", async () => {
    renderSettings();

    fireEvent.focus(screen.getByRole("button", { name: "About affected Kandev MCP tools" }));

    const tooltip = await screen.findByRole("tooltip");
    expect(tooltip.textContent).toMatch(/create_task_kandev creates a separate task/i);
    expect(tooltip.textContent).toMatch(/spawn_session_kandev adds a session to the current task/i);
  });

  it("keeps the choice local until Save changes is pressed", async () => {
    renderSettings();
    const workspaceDefault = screen.getByRole("radio", {
      name: WORKSPACE_DEFAULT_LABEL,
    });

    fireEvent.click(workspaceDefault);

    expect(workspaceDefault.getAttribute(ARIA_CHECKED)).toBe("true");
    expect(updateUserSettings).not.toHaveBeenCalled();
    expect(screen.getByRole("radiogroup").getAttribute("data-settings-dirty")).toBe("true");
    expect(
      screen.getByTestId("mcp-task-profile-default-card").getAttribute("data-settings-dirty"),
    ).toBe("true");

    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));

    await waitFor(() =>
      expect(updateUserSettings).toHaveBeenCalledWith({
        mcp_task_agent_profile_default: "workspace_default",
      }),
    );
    await waitFor(() =>
      expect(screen.getByRole("radiogroup").getAttribute("data-settings-dirty")).toBe("false"),
    );
  });

  it("keeps the draft selected when saving fails", async () => {
    updateUserSettings.mockRejectedValueOnce(new Error("save failed"));
    renderSettings();
    const workspaceDefault = screen.getByRole("radio", {
      name: WORKSPACE_DEFAULT_LABEL,
    });

    fireEvent.click(workspaceDefault);
    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(workspaceDefault.getAttribute(ARIA_CHECKED)).toBe("true"));
    await waitFor(() => expect(screen.getByText("Couldn't save")).toBeTruthy());
  });
});
