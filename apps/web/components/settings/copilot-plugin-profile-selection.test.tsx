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
  runtime_version: "1.0.88",
  mount_permitted: true,
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

  it("reports and allows removal of a missing saved plugin", () => {
    mocks.discovery = {
      status: "ready",
      response: { ...discovery, plugins: [] },
      stale: false,
    };
    const { onSelectedPluginsChange } = renderSelection(["installed:missing/plugin"]);
    expect(screen.getByText(/Plugin is missing/)).toBeTruthy();
    expect(screen.getByText(/A new process cannot start with this selection/)).toBeTruthy();
    expect(screen.queryByRole("checkbox", { name: "Select browser-use" })).toBeNull();
    const missing = screen.getByRole("checkbox", { name: "Select plugin" });
    expect(missing.hasAttribute("disabled")).toBe(false);
    fireEvent.click(missing);
    expect(onSelectedPluginsChange).toHaveBeenCalledWith([]);
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

// @covers AC-AGENTS-COPILOT-PLUGINS-005.8, AC-AGENTS-COPILOT-PLUGINS-006.2, AC-AGENTS-COPILOT-PLUGINS-006.3
it("keeps a selected plugin removable when the launch context cannot mount it", () => {
  mocks.discovery = {
    status: "ready",
    response: { ...discovery, mount_permitted: false },
    stale: false,
  };
  const { onSelectedPluginsChange } = renderSelection([sparkPluginId]);
  const warning = screen.getByTestId("copilot-plugins-start-blocked").textContent ?? "";
  expect(warning).toContain("A new process cannot start with this selection.");
  expect(warning).toMatch(/uncheck.+save/i);
  const checkbox = screen.getByRole("checkbox", { name: sparkCheckboxName });
  expect(checkbox.hasAttribute("disabled")).toBe(false);
  fireEvent.click(checkbox);
  expect(onSelectedPluginsChange).toHaveBeenCalledWith([]);
});

// @covers AC-AGENTS-COPILOT-PLUGINS-005.8, AC-AGENTS-COPILOT-PLUGINS-006.2
it("omits the start-blocked warning for the supported default discovery", () => {
  mocks.discovery = { status: "ready", response: discovery, stale: false };
  renderSelection([sparkPluginId]);
  expect(screen.queryByTestId("copilot-plugins-start-blocked")).toBeNull();
});

// @covers AC-AGENTS-COPILOT-PLUGINS-005.8, AC-AGENTS-COPILOT-PLUGINS-006.2, AC-AGENTS-COPILOT-PLUGINS-006.3
it("keeps a saved plugin removable when the runtime is unsupported", () => {
  mocks.discovery = {
    status: "unsupported",
    response: {
      ...discovery,
      status: "unsupported",
      runtime_version: "unsupported-fixture",
      mount_permitted: false,
      plugins: [],
    },
    stale: false,
  };
  const { onSelectedPluginsChange } = renderSelection([sparkPluginId]);
  expect(screen.getByTestId("copilot-plugins-unsupported")).toBeTruthy();
  expect(screen.getByTestId("copilot-plugins-start-blocked")).toBeTruthy();
  const spark = screen.getByRole("checkbox", { name: sparkCheckboxName });
  expect(spark.hasAttribute("disabled")).toBe(false);
  fireEvent.click(spark);
  expect(onSelectedPluginsChange).toHaveBeenCalledWith([]);
});
