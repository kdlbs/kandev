import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FormState } from "./automation-payload";
import { AgentToolsDisclosure } from "./automation-agent-tools-section";

vi.mock("@/hooks/domains/settings/use-automation-mcp-options", () => ({
  useAutomationMCPOptions: () => ({
    options: {
      packages: [
        { id: "core", label_key: "settings:builtinMcp.packages.core.label" },
        { id: "rich_output", label_key: "settings:builtinMcp.packages.rich_output.label" },
      ],
      definitions: [{ id: "def-github", name: "github", enabled: true }],
      inherited: [],
      task_defaults: [],
      task_blocked: [],
    },
    error: false,
    loading: false,
    reload: vi.fn(),
  }),
}));
vi.mock("./automation-mcp-access", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./automation-mcp-access")>()),
  AutomationMCPAccess: () => <div data-testid="mcp-access" />,
}));
vi.mock("./automation-plugin-tools", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./automation-plugin-tools")>()),
  AutomationPluginTools: () => <div data-testid="plugin-tools" />,
}));

afterEach(cleanup);

function form(overrides: Partial<FormState> = {}): FormState {
  return {
    name: "Nightly",
    description: "",
    workflowId: "wf-1",
    workflowStepId: "",
    agentProfileId: "agent-1",
    executorProfileId: "exec-1",
    taskMode: "automation_run",
    repositoryMode: "none",
    repositorySelections: [],
    prompt: "Run",
    taskTitleTemplate: "",
    enabled: true,
    maxConcurrentRuns: 1,
    continuationPolicy: "new_task",
    followUpPrompt: "",
    prContinuationStepId: "step-review",
    prMergedAction: "keep",
    prClosedAction: "keep",
    retryPolicy: {
      mode: "disabled",
      max_retries: "0",
      delay_seconds: "0",
      backoff: "fixed",
      history_mode: "attempts",
    },
    pluginTools: [],
    ...overrides,
  };
}

const GROUP = {
  plugin_id: "reports",
  display_name: "Reports",
  version: "1.0.0",
  installation_id: "inst-1",
  policy_version: "policy-2",
  tools: [],
};

function renderDisclosure(current: FormState, saveFailed = false, groups = [GROUP]) {
  render(
    <AgentToolsDisclosure
      form={current}
      savedForm={current}
      workspaceId="ws-1"
      supportsMultiRepo
      updateField={vi.fn()}
      mcp={{ saveFailed, reload: vi.fn() }}
      pluginTools={{ groups, loading: false, error: false, reload: vi.fn() }}
    />,
  );
}

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-002.5
describe("AgentToolsDisclosure", () => {
  it("starts collapsed and names the enabled tools", () => {
    renderDisclosure(
      form({ mcpSelection: { builtin_packages: ["core"], definition_ids: ["def-github"] } }),
    );
    const toggle = screen.getByRole("button", { name: /Tools/ });
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    expect(toggle.textContent).toContain("github");
    expect(screen.queryByTestId("mcp-access")).toBeNull();
  });

  it("opens the MCP and plugin tool choices on request", () => {
    renderDisclosure(form());
    fireEvent.click(screen.getByRole("button", { name: /Tools/ }));
    expect(screen.getByTestId("mcp-access")).toBeTruthy();
    expect(screen.getByTestId("plugin-tools")).toBeTruthy();
  });

  it("offers plugin tools only for automation history", () => {
    renderDisclosure(form({ taskMode: "normal_task" }));
    fireEvent.click(screen.getByRole("button", { name: /Tools/ }));
    expect(screen.queryByTestId("plugin-tools")).toBeNull();
  });

  it("opens by itself when approved plugin tools need review", () => {
    renderDisclosure(
      form({
        pluginTools: [
          {
            plugin_id: "reports",
            installation_id: "inst-1",
            policy_version: "policy-1",
            tool_names: ["list_findings"],
          },
        ],
      }),
    );
    expect(screen.getByTestId("plugin-tools")).toBeTruthy();
  });

  it("opens by itself when saved MCP choices need a reload", () => {
    renderDisclosure(form(), true);
    expect(screen.getByTestId("mcp-access")).toBeTruthy();
  });
});
