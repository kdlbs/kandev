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
const unsupportedTestId = "copilot-plugins-unsupported";
const startBlockedTestId = "copilot-plugins-start-blocked";
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

function unsupportedRuntime(
  overrides: Partial<CopilotPluginDiscoveryResponse> = {},
): CopilotPluginDiscoveryResponse {
  return {
    ...discovery,
    status: "unsupported",
    reason: "plugin_loading_unsupported",
    runtime_version: "9.9.9",
    mount_permitted: false,
    plugins: [],
    ...overrides,
  };
}

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
    expect(screen.getByText("Native status: unknown")).toBeTruthy();
    const checkbox = screen.getByRole("checkbox", { name: sparkCheckboxName });
    expect(checkbox.hasAttribute("disabled")).toBe(false);
    fireEvent.click(checkbox);
    expect(onSelectedPluginsChange).toHaveBeenCalledWith([]);
  });

  it("reports a malformed inventory while preserving saved selections", () => {
    mocks.discovery = {
      status: "unavailable",
      response: { ...discovery, status: "unavailable", reason: "invalid_inventory", plugins: [] },
      stale: false,
    };
    renderSelection([sparkPluginId]);
    expect(
      screen.getByText(
        "The selected Copilot CLI returned an inventory format Kandev cannot read. Saved choices are unchanged. Refresh or repair the CLI.",
      ),
    ).toBeTruthy();
    expect(screen.getByText("Selected runtime: Copilot 1.0.88")).toBeTruthy();
    expect(screen.getByTestId(startBlockedTestId)).toBeTruthy();
    const spark = screen.getByRole("checkbox", { name: sparkCheckboxName });
    expect(spark.getAttribute("aria-checked")).toBe("true");
    expect(spark.hasAttribute("disabled")).toBe(false);
  });

  it("marks retained successful rows unchecked after a failed refresh", () => {
    mocks.discovery = { status: "unavailable", response: discovery, stale: false };
    const { onSelectedPluginsChange } = renderSelection([sparkPluginId]);
    expect(screen.getByText(/Saved selection, availability not checked/)).toBeTruthy();
    expect(screen.getAllByText("Native status: unknown").length).toBeGreaterThan(0);
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
    expect(screen.getByText("Native status: unknown")).toBeTruthy();
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
  const warning = screen.getByTestId(startBlockedTestId).textContent ?? "";
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
  expect(screen.queryByTestId(startBlockedTestId)).toBeNull();
});

// @covers AC-AGENTS-COPILOT-PLUGINS-005.8, AC-AGENTS-COPILOT-PLUGINS-006.2, AC-AGENTS-COPILOT-PLUGINS-006.3
it("keeps a saved plugin removable when the runtime is unsupported", () => {
  mocks.discovery = {
    status: "unsupported",
    response: unsupportedRuntime(),
    stale: false,
  };
  const { onSelectedPluginsChange } = renderSelection([sparkPluginId]);
  expect(screen.getByTestId(unsupportedTestId)).toBeTruthy();
  expect(screen.getByText("Selected runtime: Copilot 9.9.9")).toBeTruthy();
  expect(
    screen.getByText(
      "This selected Copilot runtime does not advertise profile plugin loading. Saved selections are preserved; remove them before starting a new process.",
    ),
  ).toBeTruthy();
  expect(screen.queryByText(/qualified versions/i)).toBeNull();
  expect(screen.getByTestId(startBlockedTestId)).toBeTruthy();
  const spark = screen.getByRole("checkbox", { name: sparkCheckboxName });
  expect(spark.hasAttribute("disabled")).toBe(false);
  fireEvent.click(spark);
  expect(onSelectedPluginsChange).toHaveBeenCalledWith([]);
});

// @covers AC-AGENTS-COPILOT-PLUGINS-006.9, AC-AGENTS-COPILOT-PLUGINS-006.10
it("prioritizes stale launch-context guidance over a retained runtime rejection", () => {
  mocks.discovery = { status: "unsupported", response: unsupportedRuntime(), stale: true };
  const { onSelectedPluginsChange } = renderSelection([sparkPluginId]);
  expect(screen.getByTestId("copilot-plugins-stale")).toBeTruthy();
  expect(screen.queryByTestId(unsupportedTestId)).toBeNull();
  expect(screen.queryByText(/does not advertise profile plugin loading/)).toBeNull();
  expect(screen.queryByTestId(startBlockedTestId)).toBeNull();
  const spark = screen.getByRole("checkbox", { name: sparkCheckboxName });
  expect(spark.hasAttribute("disabled")).toBe(false);
  fireEvent.click(spark);
  expect(onSelectedPluginsChange).toHaveBeenCalledWith([]);
});

// @covers AC-AGENTS-COPILOT-PLUGINS-006.9, AC-AGENTS-COPILOT-PLUGINS-006.10
it("does not present a retained rejection while a new context is loading", () => {
  mocks.discovery = { status: "loading", response: unsupportedRuntime(), stale: false };
  renderSelection([sparkPluginId]);
  expect(screen.getByTestId("copilot-plugins-loading")).toBeTruthy();
  expect(screen.queryByTestId(unsupportedTestId)).toBeNull();
  expect(screen.queryByText(/does not advertise profile plugin loading/)).toBeNull();
  expect(screen.queryByTestId(startBlockedTestId)).toBeNull();
});

// @covers AC-AGENTS-COPILOT-PLUGINS-006.9, AC-AGENTS-COPILOT-PLUGINS-006.10
it("does not present a definite start block after a failed observation", () => {
  mocks.discovery = {
    status: "unavailable",
    response: unsupportedRuntime(),
    stale: false,
  };
  renderSelection([sparkPluginId]);
  expect(screen.getByTestId("copilot-plugins-failed")).toBeTruthy();
  expect(screen.queryByText("Selected runtime: Copilot 9.9.9")).toBeNull();
  expect(screen.queryByTestId(startBlockedTestId)).toBeNull();
  expect(screen.queryByTestId(unsupportedTestId)).toBeNull();
});

// @covers AC-AGENTS-COPILOT-PLUGINS-006.9, AC-AGENTS-COPILOT-PLUGINS-006.10
it("does not present a late old-context rejection as the current compatibility", () => {
  mocks.discovery = {
    status: "unsupported",
    response: unsupportedRuntime({
      reason: "unsupported_launch_context",
      runtime_version: "1.0.104",
      context_revision: "old-context",
    }),
    stale: true,
  };
  renderSelection([sparkPluginId]);
  expect(screen.getByTestId("copilot-plugins-stale")).toBeTruthy();
  expect(screen.queryByTestId(unsupportedTestId)).toBeNull();
  expect(screen.queryByText(/launch context cannot load profile plugins/)).toBeNull();
  expect(screen.queryByTestId(startBlockedTestId)).toBeNull();
});

// @covers AC-AGENTS-COPILOT-PLUGINS-006.9
it("uses safe fallback guidance when runtime metadata is missing", () => {
  mocks.discovery = {
    status: "unsupported",
    response: unsupportedRuntime({
      runtime_version: undefined,
    }),
    stale: false,
  };
  renderSelection([sparkPluginId]);
  expect(
    screen.getByText(
      "This selected Copilot runtime does not advertise profile plugin loading. Saved selections are preserved; remove them before starting a new process.",
    ),
  ).toBeTruthy();
  expect(screen.queryByText(/Copilot .*plugin adapter unavailable/)).toBeNull();
});

// @covers AC-AGENTS-COPILOT-PLUGINS-006.9
it("does not label a legacy policy response as a runtime version rejection", () => {
  mocks.discovery = {
    status: "unsupported",
    response: unsupportedRuntime({ reason: "policy_unverified", runtime_version: "1.0.88" }),
    stale: false,
  };
  renderSelection([sparkPluginId]);
  expect(screen.getByTestId(unsupportedTestId).textContent).toContain(
    "This Kandev version cannot verify plugin policy for this runtime.",
  );
  expect(screen.queryByText(/plugin adapter unavailable/)).toBeNull();
});

it("keeps legacy runtime rejection guidance without showing a version allowlist", () => {
  mocks.discovery = {
    status: "unsupported",
    response: unsupportedRuntime({ reason: "runtime_unsupported", runtime_version: "9.9.9" }),
    stale: false,
  };
  renderSelection([sparkPluginId]);
  expect(screen.getByText("Selected runtime: Copilot 9.9.9")).toBeTruthy();
  expect(screen.getByTestId(unsupportedTestId).textContent).toContain(
    "This Copilot runtime was marked unsupported by an older compatibility check.",
  );
  expect(screen.queryByText(/qualified versions/i)).toBeNull();
});

// @covers AC-AGENTS-COPILOT-PLUGINS-006.9
it("explains a wrapped launch context without claiming a runtime rejection", () => {
  mocks.discovery = {
    status: "unsupported",
    response: unsupportedRuntime({
      reason: "unsupported_launch_context",
      runtime_version: undefined,
    }),
    stale: false,
  };
  renderSelection([sparkPluginId]);
  expect(screen.getByTestId(unsupportedTestId).textContent).toContain(
    "This launch context cannot load profile plugins.",
  );
  expect(screen.queryByText(/plugin adapter unavailable/)).toBeNull();
});

// @covers AC-AGENTS-COPILOT-PLUGINS-006.9
it("uses a bounded fallback for an unknown failure reason", () => {
  mocks.discovery = {
    status: "unsupported",
    response: unsupportedRuntime({ reason: "future_reason", runtime_version: undefined }),
    stale: false,
  };
  renderSelection([sparkPluginId]);
  expect(screen.getByTestId(unsupportedTestId).textContent).toContain(
    "Copilot reported an unknown plugin compatibility reason.",
  );
  expect(screen.queryByText(/plugin adapter unavailable/)).toBeNull();
});
