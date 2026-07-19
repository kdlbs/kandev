import { type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import type { MCPTaskAgentProfileDefault } from "@/lib/types/http";

const updateUserSettings = vi.fn();
const ARIA_CHECKED = "aria-checked";
const CURRENT_TASK_LABEL = "Current task profile";
const WORKSPACE_DEFAULT_LABEL = "Workspace default profile";
const SAVE_FAILED = "save failed";

vi.mock("@/lib/api", () => ({
  updateUserSettings: (...args: unknown[]) => updateUserSettings(...args),
}));

import { MCPTaskAgentProfileDefaultSettings } from "./mcp-task-agent-profile-default-settings";

function renderSettings(
  preference: MCPTaskAgentProfileDefault = "current_task",
  children?: ReactNode,
) {
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
      <MCPTaskAgentProfileDefaultSettings />
      {children}
    </StateProvider>,
  );
}

function RemoteSettingsUpdate({
  workspaceId = "workspace-2",
  preference = "current_task",
}: {
  workspaceId?: string;
  preference?: MCPTaskAgentProfileDefault;
}) {
  const storeApi = useAppStoreApi();

  return (
    <button
      type="button"
      onClick={() => {
        storeApi.setState((state) => ({
          ...state,
          userSettings: {
            ...defaultState.userSettings,
            workspaceId,
            mcpTaskAgentProfileDefault: preference,
            loaded: true,
          },
          userSettingsServerRevision: state.userSettingsServerRevision + 1,
        }));
      }}
    >
      Apply remote settings
    </button>
  );
}

function LocalSettingsUpdate() {
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const confirmTaskArchive = useAppStore((state) => state.userSettings.confirmTaskArchive);

  return (
    <>
      <button
        type="button"
        onClick={() => {
          const current = storeApi.getState().userSettings;
          setUserSettings({ ...current, confirmTaskArchive: false });
        }}
      >
        Apply local settings
      </button>
      <output>{confirmTaskArchive ? "archive-enabled" : "archive-disabled"}</output>
    </>
  );
}

function mockPendingSave() {
  let rejectSave: (reason?: unknown) => void = () => {};
  updateUserSettings.mockImplementationOnce(
    () =>
      new Promise((_, reject: (reason?: unknown) => void) => {
        rejectSave = reject;
      }),
  );
  return () => rejectSave(new Error(SAVE_FAILED));
}

beforeEach(() => {
  updateUserSettings.mockReset().mockResolvedValue({ settings: {} });
});

afterEach(cleanup);

describe("MCPTaskAgentProfileDefaultSettings", () => {
  it("renders accessible descriptive choices", () => {
    renderSettings();

    expect(screen.getByRole("radio", { name: CURRENT_TASK_LABEL }).getAttribute(ARIA_CHECKED)).toBe(
      "true",
    );
    expect(
      screen.getByRole("radio", { name: WORKSPACE_DEFAULT_LABEL }).getAttribute(ARIA_CHECKED),
    ).toBe("false");
    screen.getByText(/workflow profile first/i);
  });

  it("selects optimistically and sends only the changed preference", async () => {
    renderSettings();
    const workspaceDefault = screen.getByRole("radio", {
      name: WORKSPACE_DEFAULT_LABEL,
    });

    fireEvent.click(workspaceDefault);

    expect(workspaceDefault.getAttribute(ARIA_CHECKED)).toBe("true");
    await waitFor(() =>
      expect(updateUserSettings).toHaveBeenCalledWith({
        mcp_task_agent_profile_default: "workspace_default",
      }),
    );
  });

  it("disables duplicate changes while a save is pending", async () => {
    updateUserSettings.mockImplementationOnce(() => new Promise(() => {}));
    renderSettings();
    const currentTask = screen.getByRole("radio", { name: CURRENT_TASK_LABEL });
    const workspaceDefault = screen.getByRole("radio", {
      name: WORKSPACE_DEFAULT_LABEL,
    });

    fireEvent.click(workspaceDefault);

    await waitFor(() => expect(currentTask.hasAttribute("disabled")).toBe(true));
    expect(workspaceDefault.hasAttribute("disabled")).toBe(true);
    fireEvent.click(currentTask);
    expect(updateUserSettings).toHaveBeenCalledTimes(1);
  });

  it("rolls back the optimistic selection when saving fails", async () => {
    const rejectSave = mockPendingSave();
    renderSettings();
    const currentTask = screen.getByRole("radio", { name: CURRENT_TASK_LABEL });
    const workspaceDefault = screen.getByRole("radio", {
      name: WORKSPACE_DEFAULT_LABEL,
    });

    fireEvent.click(workspaceDefault);

    await waitFor(() => expect(workspaceDefault.getAttribute(ARIA_CHECKED)).toBe("true"));
    rejectSave();
    await waitFor(() => expect(currentTask.getAttribute(ARIA_CHECKED)).toBe("true"));
  });

  it("does not overwrite a newer workspace update when saving fails", async () => {
    const rejectSave = mockPendingSave();
    renderSettings("current_task", <RemoteSettingsUpdate />);

    fireEvent.click(screen.getByRole("radio", { name: WORKSPACE_DEFAULT_LABEL }));
    fireEvent.click(screen.getByRole("button", { name: "Apply remote settings" }));
    rejectSave();

    await waitFor(() =>
      expect(
        screen.getByRole("radio", { name: CURRENT_TASK_LABEL }).getAttribute(ARIA_CHECKED),
      ).toBe("true"),
    );
  });

  it("does not overwrite a same-value remote update when saving fails", async () => {
    const rejectSave = mockPendingSave();
    renderSettings(
      "current_task",
      <RemoteSettingsUpdate workspaceId="workspace-1" preference="workspace_default" />,
    );

    fireEvent.click(screen.getByRole("radio", { name: WORKSPACE_DEFAULT_LABEL }));
    fireEvent.click(screen.getByRole("button", { name: "Apply remote settings" }));
    rejectSave();

    await waitFor(() =>
      expect(
        screen.getByRole("radio", { name: WORKSPACE_DEFAULT_LABEL }).getAttribute(ARIA_CHECKED),
      ).toBe("true"),
    );
  });

  it("rolls back after an unrelated local settings replacement", async () => {
    const rejectSave = mockPendingSave();
    renderSettings("current_task", <LocalSettingsUpdate />);

    fireEvent.click(screen.getByRole("radio", { name: WORKSPACE_DEFAULT_LABEL }));
    fireEvent.click(screen.getByRole("button", { name: "Apply local settings" }));
    rejectSave();

    await waitFor(() =>
      expect(
        screen.getByRole("radio", { name: CURRENT_TASK_LABEL }).getAttribute(ARIA_CHECKED),
      ).toBe("true"),
    );
    expect(screen.getByText("archive-disabled")).toBeTruthy();
  });
});
