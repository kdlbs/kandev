import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { CopilotPluginDiscoveryResponse } from "@/lib/types/copilot-plugin-discovery";

const mocks = vi.hoisted(() => ({
  isFinePointer: true,
  usesDesktopWorkbench: true,
  discovery: null as {
    status: "loading" | "ready" | "unavailable" | "unsupported";
    response?: CopilotPluginDiscoveryResponse;
    stale: boolean;
  } | null,
  refresh: vi.fn(),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    isFinePointer: mocks.isFinePointer,
    usesDesktopWorkbench: mocks.usesDesktopWorkbench,
  }),
}));
vi.mock("@/hooks/domains/settings/use-copilot-plugin-discovery", () => ({
  useCopilotPluginDiscovery: () => ({
    status: mocks.discovery?.status ?? "ready",
    response: mocks.discovery?.response,
    stale: mocks.discovery?.stale ?? false,
    refresh: mocks.refresh,
  }),
}));

import { CopilotPluginProfileSelection } from "./copilot-plugin-profile-selection";

const sparkPluginId = "installed:copilot-plugins/spark";
const sparkCheckboxName = "Select spark";

const discovery: CopilotPluginDiscoveryResponse = {
  agent_id: "agent-1",
  profile_id: "profile-1",
  provider_id: "copilot-acp",
  status: "ready",
  plugins: [
    {
      id: sparkPluginId,
      name: "spark",
      marketplace: "copilot-plugins",
      version: "1.0.0",
      native_enabled: false,
      availability: "available",
    },
    {
      id: "builtin:/browser-use",
      name: "browser-use",
      version: "0.0.4",
      native_enabled: false,
      availability: "unavailable",
      reason_code: "builtin_not_mountable",
    },
  ],
};

function renderSelection(selectedPluginIds: string[] = []) {
  const onSelectedPluginsChange = vi.fn();
  render(
    <CopilotPluginProfileSelection
      agentId="agent-1"
      profileId="profile-1"
      selectedPluginIds={selectedPluginIds}
      onSelectedPluginsChange={onSelectedPluginsChange}
    />,
  );
  return { onSelectedPluginsChange };
}

afterEach(() => {
  cleanup();
  mocks.isFinePointer = true;
  mocks.usesDesktopWorkbench = true;
  mocks.discovery = { status: "ready", response: discovery, stale: false };
  mocks.refresh.mockReset();
});

describe("CopilotPluginProfileSelection", () => {
  it("selects a native-disabled installed plugin and disables unavailable rows", () => {
    mocks.discovery = { status: "ready", response: discovery, stale: false };
    const { onSelectedPluginsChange } = renderSelection();

    fireEvent.click(screen.getByRole("checkbox", { name: sparkCheckboxName }));
    expect(onSelectedPluginsChange).toHaveBeenCalledWith([sparkPluginId]);
    const builtin = screen.getByRole("checkbox", { name: "Select browser-use" });
    expect(builtin.hasAttribute("disabled")).toBe(true);
    expect(screen.getByText("Built-in plugin. Cannot load by directory.")).toBeTruthy();
  });

  it("keeps saved selections removable after a failed refresh", () => {
    mocks.discovery = { status: "unavailable", stale: false };
    const { onSelectedPluginsChange } = renderSelection([sparkPluginId]);
    expect(screen.getByText(/Saved selection, availability not checked/)).toBeTruthy();
    const checkbox = screen.getByRole("checkbox", { name: sparkCheckboxName });
    expect(checkbox.hasAttribute("disabled")).toBe(false);
    fireEvent.click(checkbox);
    expect(onSelectedPluginsChange).toHaveBeenCalledWith([]);
  });

  it("marks retained successful rows unchecked after a failed refresh", () => {
    mocks.discovery = { status: "unavailable", response: discovery, stale: false };
    const { onSelectedPluginsChange } = renderSelection([sparkPluginId]);
    expect(screen.getByText(/Saved selection, availability not checked/)).toBeTruthy();
    const spark = screen.getByRole("checkbox", { name: sparkCheckboxName });
    expect(spark.hasAttribute("disabled")).toBe(false);
    fireEvent.click(spark);
    expect(onSelectedPluginsChange).toHaveBeenCalledWith([]);
  });

  it("reports a missing saved plugin and blocks adding new rows", () => {
    mocks.discovery = {
      status: "ready",
      response: { ...discovery, plugins: [] },
      stale: false,
    };
    renderSelection(["installed:missing/plugin"]);
    expect(screen.getByText(/Plugin is missing/)).toBeTruthy();
    expect(screen.getByText(/A new process cannot start with this selection/)).toBeTruthy();
    expect(screen.queryByRole("checkbox", { name: "Select browser-use" })).toBeNull();
  });

  it("explains stale launch settings without allowing new selections", () => {
    mocks.discovery = { status: "ready", response: discovery, stale: true };
    renderSelection();
    expect(screen.getByText(/Launch settings changed/)).toBeTruthy();
    expect(screen.getByRole("checkbox", { name: sparkCheckboxName }).hasAttribute("disabled")).toBe(
      true,
    );
  });

  it("uses phone-sized controls below the desktop workbench", () => {
    mocks.isFinePointer = false;
    mocks.usesDesktopWorkbench = false;
    mocks.discovery = { status: "ready", response: discovery, stale: false };
    renderSelection();
    expect(screen.getByTestId("copilot-plugins-refresh").className).toContain("min-h-11");
    expect(screen.getAllByTestId("copilot-plugin-row")[0].className).toContain("min-h-11");
  });

  it("describes whole-plugin execution, scope, and next-process timing", () => {
    mocks.discovery = { status: "ready", response: discovery, stale: false };
    renderSelection();
    expect(screen.getByText(/Load whole plugins for this profile/)).toBeTruthy();
    expect(screen.getByText(/Local and worktree executors only/)).toBeTruthy();
  });
});

it("warns that a selected plugin blocks new processes while mounting is unverified", () => {
  mocks.discovery = {
    status: "ready",
    response: { ...discovery, mount_permitted: false },
    stale: false,
  };
  renderSelection([sparkPluginId]);
  expect(screen.getByTestId("copilot-plugins-start-blocked").textContent).toContain(
    "A new process cannot start with this selection.",
  );
});

it("omits the start-blocked warning once mounting is permitted", () => {
  mocks.discovery = {
    status: "ready",
    response: { ...discovery, mount_permitted: true },
    stale: false,
  };
  renderSelection([sparkPluginId]);
  expect(screen.queryByTestId("copilot-plugins-start-blocked")).toBeNull();
});
