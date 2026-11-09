/* eslint-disable max-lines-per-function -- Table-driven review-state coverage shares one fixture. */
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import type { ComponentProps } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type {
  AutomationPluginToolGroup,
  AutomationPluginToolSelection,
} from "@/lib/types/automation";
import { AutomationPluginTools } from "./automation-plugin-tools";

afterEach(cleanup);

const PLUGIN_ID = "kandev-security";
const INSTALLATION_ID = "installation-1";
const POLICY_VERSION = "policy-2";
const CHECKED_ATTR = "aria-checked";

const groups: AutomationPluginToolGroup[] = [
  {
    plugin_id: PLUGIN_ID,
    display_name: "Kandev Security",
    version: "1.2.0",
    installation_id: INSTALLATION_ID,
    policy_version: POLICY_VERSION,
    tools: [
      {
        local_name: "list_findings",
        description: "Read findings in this workspace.",
        input_schema: { type: "object" },
        read_only_hint: true,
        destructive_hint: false,
        idempotent_hint: true,
        open_world_hint: false,
      },
      {
        local_name: "complete_report",
        description: "Publish a report from this automation.",
        input_schema: { type: "object" },
        read_only_hint: false,
        destructive_hint: false,
        idempotent_hint: false,
        open_world_hint: false,
      },
    ],
  },
];

const saved: AutomationPluginToolSelection[] = [
  {
    plugin_id: PLUGIN_ID,
    installation_id: INSTALLATION_ID,
    policy_version: POLICY_VERSION,
    tool_names: ["list_findings"],
  },
];

function renderTools(overrides: Partial<ComponentProps<typeof AutomationPluginTools>> = {}) {
  const onChange = vi.fn();
  render(
    <AutomationPluginTools
      groups={groups}
      loading={false}
      error={false}
      savedSelection={saved}
      selection={saved}
      onChange={onChange}
      onRetry={vi.fn()}
      isDirty={false}
      {...overrides}
    />,
  );
  return { onChange };
}

describe("AutomationPluginTools", () => {
  it("checks the exact saved selection", () => {
    renderTools();
    expect(screen.getByTestId(`plugin-tool-${PLUGIN_ID}-list_findings`)).toBeTruthy();
    const checkboxes = screen.getAllByRole("checkbox") as HTMLButtonElement[];
    expect(checkboxes[0].getAttribute(CHECKED_ATTR)).toBe("true");
    expect(checkboxes[1].getAttribute(CHECKED_ATTR)).toBe("false");
    expect(screen.getByText("Read")).toBeTruthy();
    expect(screen.getByText("Write")).toBeTruthy();
  });

  it("adds a tool using the displayed installation and policy version", () => {
    const { onChange } = renderTools();
    fireEvent.click(screen.getByTestId(`plugin-tool-${PLUGIN_ID}-complete_report`));
    expect(onChange).toHaveBeenCalledWith([
      {
        plugin_id: PLUGIN_ID,
        installation_id: INSTALLATION_ID,
        policy_version: POLICY_VERSION,
        tool_names: ["list_findings", "complete_report"],
      },
    ]);
  });

  it("drops stale entries from other groups when the owner selects a current tool", () => {
    const { onChange } = renderTools({
      savedSelection: [
        {
          plugin_id: "removed-plugin",
          installation_id: "installation-removed",
          policy_version: "policy-removed",
          tool_names: ["old_report"],
        },
        {
          plugin_id: PLUGIN_ID,
          installation_id: INSTALLATION_ID,
          policy_version: "policy-1",
          tool_names: ["list_findings"],
        },
      ],
      selection: [
        {
          plugin_id: "removed-plugin",
          installation_id: "installation-removed",
          policy_version: "policy-removed",
          tool_names: ["old_report"],
        },
        {
          plugin_id: PLUGIN_ID,
          installation_id: INSTALLATION_ID,
          policy_version: "policy-1",
          tool_names: ["list_findings"],
        },
      ],
    });
    fireEvent.click(screen.getByTestId(`plugin-tool-${PLUGIN_ID}-list_findings`));
    expect(onChange).toHaveBeenCalledWith([
      {
        plugin_id: PLUGIN_ID,
        installation_id: INSTALLATION_ID,
        policy_version: POLICY_VERSION,
        tool_names: ["list_findings"],
      },
    ]);
  });

  it("clears one plugin group and preserves a sibling", () => {
    const sibling: AutomationPluginToolGroup = {
      ...groups[0],
      plugin_id: "kandev-sibling",
      display_name: "Kandev Sibling",
      installation_id: "installation-2",
      policy_version: "policy-sibling",
      tools: [
        ...groups[0].tools,
        {
          local_name: "sibling_tool",
          description: "Sibling tool",
          input_schema: { type: "object" },
          read_only_hint: true,
          destructive_hint: false,
          idempotent_hint: false,
          open_world_hint: false,
        },
      ],
    };
    const siblingSelection: AutomationPluginToolSelection = {
      plugin_id: sibling.plugin_id,
      installation_id: sibling.installation_id,
      policy_version: sibling.policy_version,
      tool_names: ["sibling_tool"],
    };
    const { onChange } = renderTools({
      groups: [groups[0], sibling],
      selection: [...saved, siblingSelection],
      savedSelection: [...saved, siblingSelection],
    });
    fireEvent.click(screen.getByTestId(`plugin-tools-clear-${PLUGIN_ID}`));
    expect(onChange).toHaveBeenCalledWith([siblingSelection]);
  });

  it("shows review required for a stale saved selection without checking current tools", () => {
    renderTools({
      savedSelection: [
        {
          plugin_id: PLUGIN_ID,
          installation_id: INSTALLATION_ID,
          policy_version: "policy-1",
          tool_names: ["list_findings"],
        },
      ],
      selection: [],
    });
    expect(screen.getByTestId("automation-plugin-tools-review")).toBeTruthy();
    expect(screen.getByText(/Previously selected: list_findings/)).toBeTruthy();
    const checkboxes = screen.getAllByRole("checkbox") as HTMLButtonElement[];
    expect(checkboxes.every((checkbox) => checkbox.getAttribute(CHECKED_ATTR) === "false")).toBe(
      true,
    );
  });

  it("requires reselection when the plugin policy changes while the editor is open", () => {
    const currentGroups = groups.map((group) => ({ ...group, policy_version: "policy-3" }));
    const { rerender } = render(
      <AutomationPluginTools
        groups={groups}
        loading={false}
        error={false}
        savedSelection={saved}
        selection={saved}
        onChange={vi.fn()}
        onRetry={vi.fn()}
        isDirty={false}
      />,
    );
    expect(screen.queryByTestId("automation-plugin-tools-review")).toBeNull();
    rerender(
      <AutomationPluginTools
        groups={currentGroups}
        loading={false}
        error={false}
        savedSelection={saved}
        selection={saved}
        onChange={vi.fn()}
        onRetry={vi.fn()}
        isDirty={false}
      />,
    );
    expect(screen.getByTestId("automation-plugin-tools-review")).toBeTruthy();
    const checkboxes = screen.getAllByRole("checkbox") as HTMLButtonElement[];
    expect(checkboxes.every((checkbox) => checkbox.getAttribute(CHECKED_ATTR) === "false")).toBe(
      true,
    );
  });

  it("keeps an unavailable saved plugin visible with a clear action", () => {
    const savedSelection: AutomationPluginToolSelection[] = [
      {
        plugin_id: "removed-plugin",
        installation_id: "installation-removed",
        policy_version: "policy-removed",
        tool_names: ["old_report"],
      },
    ];
    const { onChange } = renderTools({ groups: [], savedSelection, selection: savedSelection });
    const unavailable = screen.getByTestId("plugin-tools-unavailable-removed-plugin");
    expect(unavailable.textContent).toContain("old_report");
    fireEvent.click(within(unavailable).getByRole("button", { name: "Clear" }));
    expect(onChange).toHaveBeenCalledWith([]);
  });

  it("keeps loading, empty, and error states visible", () => {
    renderTools({ groups: [], loading: true });
    expect(screen.getByTestId("automation-plugin-tools-loading")).toBeTruthy();
    cleanup();
    renderTools({ groups: [] });
    expect(screen.getByTestId("automation-plugin-tools-empty")).toBeTruthy();
    cleanup();
    const onRetry = vi.fn();
    renderTools({ error: true, onRetry });
    expect(screen.getByTestId("automation-plugin-tools-error")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(onRetry).toHaveBeenCalled();
  });
});
