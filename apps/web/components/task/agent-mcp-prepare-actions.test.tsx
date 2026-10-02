import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AgentMcpPrepareActions, agentMcpFailureLabelKey } from "./agent-mcp-prepare-actions";
import type { PrepareStepInfo } from "@/lib/state/slices/session-runtime/types";

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({
    getState: () => ({
      addUserShell: vi.fn(),
      setRightPanelActiveTab: vi.fn(),
      setMobileSessionPanel: vi.fn(),
    }),
  }),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    isFinePointer: true,
    usesDesktopWorkbench: true,
  }),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: () => vi.fn(),
}));

describe("agent MCP recovery feedback", () => {
  afterEach(cleanup);

  it("explains that recovery must wait for the current agent turn", () => {
    expect(agentMcpFailureLabelKey("session_busy")).toBe("task:agentMcpSessionBusy");
  });

  it("explains when the agent cannot reload MCP servers in the current session", () => {
    expect(agentMcpFailureLabelKey("session_reload_unsupported")).toBe(
      "task:agentMcpSessionReloadUnsupported",
    );
  });

  it("renders authentication required feedback with warning styling", () => {
    const step: PrepareStepInfo = {
      name: "",
      kind: "agent_mcp_verification",
      mcpServerId: "plugin-atlassian-atlassian",
      failureCode: "authentication_required",
      status: "failed",
    };

    render(<AgentMcpPrepareActions step={step} sessionId="session-1" taskId="task-1" />);

    const message = screen.getByText("Authentication is required.");
    expect(message.className).toContain("text-amber-700");
    expect(screen.getByTestId("agent-mcp-authenticate")).toBeTruthy();
    expect(screen.getByTestId("agent-mcp-retry")).toBeTruthy();
  });

  it("renders non-auth failure feedback with destructive styling", () => {
    const step: PrepareStepInfo = {
      name: "",
      kind: "agent_mcp_verification",
      mcpServerId: "plugin-atlassian-atlassian",
      failureCode: "connection_failed",
      status: "failed",
    };

    render(<AgentMcpPrepareActions step={step} sessionId="session-1" taskId="task-1" />);

    const message = screen.getByText("Could not connect to this MCP server.");
    expect(message.className).toContain("text-destructive");
    expect(screen.queryByTestId("agent-mcp-authenticate")).toBeNull();
    expect(screen.getByTestId("agent-mcp-retry")).toBeTruthy();
  });
});
