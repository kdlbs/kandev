import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AgentMcpPrepareActions, agentMcpFailureLabelKey } from "./agent-mcp-prepare-actions";
import type { PrepareStepInfo } from "@/lib/state/slices/session-runtime/types";
import { ApiError } from "@/lib/api/client";

const { retryAgentMcpConnection } = vi.hoisted(() => ({ retryAgentMcpConnection: vi.fn() }));
const WAIT_DELAY_MESSAGE = "WaitDelay expired before I/O complete";
const RETRY_BUTTON_TEST_ID = "agent-mcp-retry";
const DIAGNOSTIC_MESSAGE_TEST_ID = "agent-mcp-diagnostic-message";
const TEST_SERVER_ID = "server-a";

vi.mock("@/lib/api/domains/session-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/session-api")>();
  return { ...actual, retryAgentMcpConnection };
});

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
  beforeEach(() => retryAgentMcpConnection.mockReset());

  it("explains that recovery must wait for the current agent turn", () => {
    expect(agentMcpFailureLabelKey("session_busy")).toBe("task:agentMcpSessionBusy");
  });

  it("explains when the agent cannot reload MCP servers in the current session", () => {
    expect(agentMcpFailureLabelKey("session_reload_unsupported")).toBe(
      "task:agentMcpSessionReloadUnsupported",
    );
  });

  it("uses command-specific explanations when a retained diagnostic identifies a failed command", () => {
    expect(
      agentMcpFailureLabelKey("connection_failed", {
        operation: "enable",
        stage: "wait",
        kind: "output_wait_timeout",
        message: "command timed out",
      }),
    ).toBe("task:agentMcpApprovalCommandFailed");
    expect(
      agentMcpFailureLabelKey("connection_failed", {
        operation: "list_tools",
        stage: "wait",
        kind: "wait_failed",
        message: "command failed",
      }),
    ).toBe("task:agentMcpVerificationCommandFailed");
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
    expect(screen.getByTestId(RETRY_BUTTON_TEST_ID)).toBeTruthy();
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
    expect(screen.getByTestId(RETRY_BUTTON_TEST_ID)).toBeTruthy();
  });
});

describe("agent MCP diagnostic details", () => {
  afterEach(cleanup);
  beforeEach(() => retryAgentMcpConnection.mockReset());

  it("renders safe command details as selectable plain text", () => {
    const step: PrepareStepInfo = {
      name: "",
      kind: "agent_mcp_approval",
      mcpServerId: "plugin-atlassian-atlassian",
      failureCode: "connection_failed",
      status: "failed",
      mcpDiagnostic: {
        operation: "enable",
        stage: "wait",
        kind: "output_wait_timeout",
        message: "<script>WaitDelay expired</script>",
        exitCode: 0,
      },
    };

    render(<AgentMcpPrepareActions step={step} sessionId="session-1" taskId="task-1" />);

    expect(screen.getByText("Native command failed during server approval.")).toBeTruthy();
    const details = screen.getByTestId("agent-mcp-diagnostic");
    expect(details.textContent).toContain("Operation");
    expect(details.textContent).toContain("Approval");
    expect(details.textContent).toContain("Waiting for command");
    expect(details.textContent).toContain("Exit status: 0");
    const message = screen.getByTestId(DIAGNOSTIC_MESSAGE_TEST_ID);
    expect(message.textContent).toBe("<script>WaitDelay expired</script>");
    expect(message.querySelector("script")).toBeNull();
    expect(message.className).toContain("select-text");
  });

  it("renders the translated cleanup-error label with its separator", () => {
    const step: PrepareStepInfo = {
      name: "",
      kind: "agent_mcp_approval",
      mcpServerId: TEST_SERVER_ID,
      failureCode: "connection_failed",
      status: "failed",
      mcpDiagnostic: {
        operation: "enable",
        stage: "wait",
        kind: "wait_failed",
        message: "approval command failed",
        cleanupMessage: "process cleanup failed",
      },
    };

    render(<AgentMcpPrepareActions step={step} sessionId="session-1" taskId="task-1" />);

    expect(screen.getByTestId("agent-mcp-diagnostic").textContent).toContain(
      "Cleanup error: process cleanup failed",
    );
  });

  it("keeps the diagnostic after a busy rejection and clears it when retry succeeds", async () => {
    const step: PrepareStepInfo = {
      name: "",
      kind: "agent_mcp_approval",
      mcpServerId: TEST_SERVER_ID,
      failureCode: "connection_failed",
      status: "failed",
      mcpDiagnostic: {
        operation: "enable",
        stage: "wait",
        kind: "output_wait_timeout",
        message: WAIT_DELAY_MESSAGE,
      },
    };
    retryAgentMcpConnection
      .mockRejectedValueOnce(
        new ApiError("The agent is using this session.", 409, {
          error: "The agent is using this session.",
          error_code: "mcp_recovery_session_busy",
        }),
      )
      .mockResolvedValueOnce({
        provider_id: "cursor",
        server_id: TEST_SERVER_ID,
        status: "ready",
        tool_count: 1,
      });

    render(<AgentMcpPrepareActions step={step} sessionId="session-1" taskId="task-1" />);

    fireEvent.click(screen.getByTestId(RETRY_BUTTON_TEST_ID));
    expect(
      await screen.findByText(
        "The agent is using this session. Wait for its current turn to finish, then retry.",
      ),
    ).toBeTruthy();
    expect(screen.getByTestId(DIAGNOSTIC_MESSAGE_TEST_ID).textContent).toBe(WAIT_DELAY_MESSAGE);

    fireEvent.click(screen.getByTestId(RETRY_BUTTON_TEST_ID));
    await waitFor(() => expect(screen.queryByTestId("agent-mcp-diagnostic")).toBeNull());
    expect(screen.getByText("Connection ready.")).toBeTruthy();
  });
});

describe("agent MCP retry command feedback", () => {
  afterEach(cleanup);
  beforeEach(() => retryAgentMcpConnection.mockReset());

  it.each([
    {
      operation: "enable",
      kind: "output_wait_timeout",
      message: "approval command timed out",
      expected: "Native command failed during server approval.",
    },
    {
      operation: "list_tools",
      kind: "wait_failed",
      message: "verification command failed",
      expected: "Native command failed during connection verification.",
    },
  ] as const)("uses $operation diagnostics for failed retry feedback", async (testCase) => {
    retryAgentMcpConnection.mockResolvedValueOnce({
      provider_id: "cursor",
      server_id: TEST_SERVER_ID,
      status: "connection_failed",
      reason_code: "connection_failed",
      mcp_diagnostic: {
        operation: testCase.operation,
        stage: "wait",
        kind: testCase.kind,
        message: testCase.message,
      },
    });

    render(
      <AgentMcpPrepareActions
        step={{
          name: "",
          kind: "agent_mcp_verification",
          mcpServerId: TEST_SERVER_ID,
          failureCode: "connection_failed",
          status: "failed",
        }}
        sessionId="session-1"
        taskId="task-1"
      />,
    );

    fireEvent.click(screen.getByTestId(RETRY_BUTTON_TEST_ID));

    await waitFor(() => expect(screen.getAllByText(testCase.expected)).toHaveLength(2));
    expect(screen.getByTestId(DIAGNOSTIC_MESSAGE_TEST_ID).textContent).toBe(testCase.message);
    expect(screen.queryByText("Could not connect to this MCP server.")).toBeNull();
  });

  it("keeps authentication-required feedback and controls when the retry diagnostic is command-scoped", async () => {
    retryAgentMcpConnection.mockResolvedValueOnce({
      provider_id: "cursor",
      server_id: TEST_SERVER_ID,
      status: "authentication_required",
      reason_code: "authentication_required",
      mcp_diagnostic: {
        operation: "enable",
        stage: "wait",
        kind: "wait_failed",
        message: "sign in before approving this server",
      },
    });

    render(
      <AgentMcpPrepareActions
        step={{
          name: "",
          kind: "agent_mcp_verification",
          mcpServerId: TEST_SERVER_ID,
          failureCode: "authentication_required",
          status: "failed",
        }}
        sessionId="session-1"
        taskId="task-1"
      />,
    );

    fireEvent.click(screen.getByTestId(RETRY_BUTTON_TEST_ID));

    await waitFor(() => expect(screen.getAllByText("Authentication is required.")).toHaveLength(2));
    expect(screen.getByTestId("agent-mcp-authenticate")).toBeTruthy();
    expect(screen.getByTestId(DIAGNOSTIC_MESSAGE_TEST_ID).textContent).toBe(
      "sign in before approving this server",
    );
  });
});
