import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AgentMcpDiscoveryResponse } from "@/lib/types/agent-mcp-discovery";

const mocks = vi.hoisted(() => ({
  isFinePointer: true,
  discovery: null as AgentMcpDiscoveryResponse | null,
  refresh: vi.fn(),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: mocks.isFinePointer }),
}));
vi.mock("@/hooks/domains/settings/use-agent-mcp-discovery", () => ({
  useAgentMcpDiscovery: () => ({
    status: mocks.discovery?.status ?? "ready",
    response: mocks.discovery,
    refresh: mocks.refresh,
  }),
}));

import { CursorMcpProfileSelection } from "./cursor-mcp-profile-selection";

const SERVER_NAME = "Atlassian MCP server with a long translated name that should wrap on a phone";
const discovery: AgentMcpDiscoveryResponse = {
  agent_id: "agent-1",
  provider_id: "cursor",
  status: "ready",
  servers: [
    {
      id: "plugin-atlassian-jira",
      name: SERVER_NAME,
      plugin_name: "Atlassian tools",
      source_kind: "plugin",
      credentials_available: true,
    },
  ],
};

function renderSelection({
  mode = "inherit",
  selectedServerIds = [],
}: {
  mode?: "inherit" | "selected";
  selectedServerIds?: string[];
} = {}) {
  const onModeChange = vi.fn();
  const onSelectedServersChange = vi.fn();
  render(
    <CursorMcpProfileSelection
      agentId="agent-1"
      profileId="profile-1"
      mode={mode}
      selectedServerIds={selectedServerIds}
      savedMode="inherit"
      savedSelectedServerIds={[]}
      onModeChange={onModeChange}
      onSelectedServersChange={onSelectedServersChange}
    />,
  );
  return { onModeChange, onSelectedServersChange };
}

afterEach(() => {
  cleanup();
  mocks.isFinePointer = true;
  mocks.discovery = discovery;
  mocks.refresh.mockReset();
});

describe("CursorMcpProfileSelection", () => {
  it("previews inherited servers as candidates on desktop without making them editable", () => {
    mocks.isFinePointer = true;
    mocks.discovery = discovery;
    renderSelection();

    const checkbox = screen.getByRole("checkbox", { name: `Select ${SERVER_NAME}` });
    expect(checkbox.getAttribute("aria-checked")).toBe("true");
    expect(checkbox.hasAttribute("disabled")).toBe(true);
    expect(screen.getByText("Credentials available")).toBeTruthy();
    const preview = screen.getByTestId("cursor-mcp-inherit-preview");
    expect(preview.textContent).toContain("candidates");
    expect(preview.textContent).toContain("executor policy");
    expect(screen.getByTestId("cursor-mcp-server-row").className).toContain("flex-row");
  });

  it("stacks phone metadata and preserves saved selections while discovery is unavailable", () => {
    mocks.isFinePointer = false;
    mocks.discovery = { ...discovery, status: "unavailable", servers: [] };
    renderSelection({ mode: "selected", selectedServerIds: ["missing-server"] });

    const checkbox = screen.getByRole("checkbox", { name: "Select missing-server" });
    expect(checkbox.getAttribute("aria-checked")).toBe("true");
    expect(checkbox.getAttribute("aria-disabled")).not.toBe("true");
    expect(screen.getByTestId("cursor-mcp-discovery-unavailable")).toBeTruthy();
    expect(screen.getByTestId("cursor-mcp-server-row").className).toContain("flex-col");
    expect(screen.getByTestId("cursor-mcp-server-row").className).toContain("min-h-11");
  });

  it("updates the selected-only ID list from checkbox changes", () => {
    mocks.isFinePointer = true;
    mocks.discovery = discovery;
    const { onSelectedServersChange } = renderSelection({ mode: "selected" });
    fireEvent.click(screen.getByRole("checkbox", { name: `Select ${SERVER_NAME}` }));
    expect(onSelectedServersChange).toHaveBeenCalledWith(["plugin-atlassian-jira"]);
  });
});
