import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";

export const E2E_MCP_SERVER_ID = "plugin-atlassian-jira";
export const E2E_MCP_SECOND_SERVER_ID = "plugin-atlassian-confluence";
export const E2E_MCP_LEGACY_ERROR = "Raw error must stay hidden";
export const E2E_MCP_LEGACY_OUTPUT = "Raw output must stay hidden";
export const E2E_MCP_LONG_DIAGNOSTIC = `Native MCP command failed: ${"x".repeat(930)}`;

export async function createMcpRecoveryFixture(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  const sessionId = task.session_id;
  if (!sessionId) throw new Error("MCP recovery task did not return a session ID");

  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return sessions.find((session) => session.id === sessionId)?.state;
      },
      { timeout: 45_000, message: "MCP recovery session did not finish its initial turn" },
    )
    .toMatch(/^(COMPLETED|WAITING_FOR_INPUT)$/);

  const { sessions } = await apiClient.listTaskSessions(task.id);
  const session = sessions.find((item) => item.id === sessionId);
  const taskEnvironmentId = session?.task_environment_id;
  if (!taskEnvironmentId) throw new Error("MCP recovery session has no task environment");

  await apiClient.seedTaskSession(task.id, {
    state: "WAITING_FOR_INPUT",
    sessionId,
    agentProfileId: seedData.agentProfileId,
    metadata: {
      prepare_result: {
        status: "completed",
        preparation_id: "e2e-mcp-recovery-attempt",
        preparation_started_at: "2099-01-01T00:00:00.000000001Z",
        steps: [
          { name: "Prepare workspace", kind: "executor_workspace", status: "completed" },
          {
            name: "",
            kind: "agent_mcp_discovery",
            mcp_provider: "cursor",
            mcp_server_id: E2E_MCP_SERVER_ID,
            status: "completed",
          },
          {
            name: "",
            kind: "agent_mcp_selection",
            mcp_provider: "cursor",
            mcp_server_id: E2E_MCP_SERVER_ID,
            status: "completed",
          },
          {
            name: "",
            kind: "agent_mcp_credentials",
            mcp_provider: "cursor",
            mcp_server_id: E2E_MCP_SERVER_ID,
            status: "completed",
          },
          {
            name: "",
            kind: "agent_mcp_approval",
            mcp_provider: "cursor",
            mcp_server_id: E2E_MCP_SERVER_ID,
            status: "completed",
          },
          {
            name: "",
            kind: "agent_mcp_verification",
            mcp_provider: "cursor",
            mcp_server_id: E2E_MCP_SERVER_ID,
            failure_code: "authentication_required",
            status: "failed",
          },
        ],
      },
    },
  });

  const existingTerminal = await apiClient.wsRequest<{ terminal_id: string }>("user_shell.create", {
    task_id: task.id,
    task_environment_id: taskEnvironmentId,
  });
  const authenticationTerminal = await apiClient.wsRequest<{ terminal_id: string }>(
    "user_shell.create",
    { task_id: task.id, task_environment_id: taskEnvironmentId },
  );
  return {
    taskId: task.id,
    sessionId,
    taskEnvironmentId,
    existingTerminalId: existingTerminal.terminal_id,
    authenticationTerminalId: authenticationTerminal.terminal_id,
  };
}

export async function seedMcpDiagnosticPreparation(
  apiClient: ApiClient,
  seedData: SeedData,
  fixture: { taskId: string; sessionId: string },
  options: { longMessage?: boolean } = {},
) {
  const primaryMessage = options.longMessage
    ? E2E_MCP_LONG_DIAGNOSTIC
    : "exec: WaitDelay expired before I/O complete";
  await apiClient.seedTaskSession(fixture.taskId, {
    state: "WAITING_FOR_INPUT",
    sessionId: fixture.sessionId,
    agentProfileId: seedData.agentProfileId,
    metadata: {
      prepare_result: {
        status: "failed",
        preparation_id: "e2e-mcp-diagnostic-attempt",
        preparation_started_at: "2099-01-01T00:00:00.000000001Z",
        steps: [
          {
            name: "",
            kind: "agent_mcp_approval",
            mcp_provider: "cursor",
            mcp_server_id: E2E_MCP_SERVER_ID,
            failure_code: "connection_failed",
            status: "failed",
            error: E2E_MCP_LEGACY_ERROR,
            output: E2E_MCP_LEGACY_OUTPUT,
            mcp_diagnostic: {
              operation: "enable",
              stage: "wait",
              kind: "output_wait_timeout",
              message: primaryMessage,
              exit_code: 0,
              cleanup_message: "process cleanup failed",
            },
          },
          {
            name: "",
            kind: "agent_mcp_verification",
            mcp_provider: "cursor",
            mcp_server_id: E2E_MCP_SECOND_SERVER_ID,
            failure_code: "connection_failed",
            status: "failed",
            error: E2E_MCP_LEGACY_ERROR,
            output: E2E_MCP_LEGACY_OUTPUT,
            mcp_diagnostic: {
              operation: "list_tools",
              stage: "wait",
              kind: "wait_failed",
              message: "Native MCP connection verification command failed",
            },
          },
        ],
      },
    },
  });
}

export async function destroyMcpRecoveryTerminals(
  apiClient: ApiClient,
  fixture: {
    taskId: string;
    taskEnvironmentId: string;
    existingTerminalId: string;
    authenticationTerminalId: string;
  },
) {
  await Promise.allSettled(
    [fixture.existingTerminalId, fixture.authenticationTerminalId].map((terminalId) =>
      apiClient.wsRequest("user_shell.destroy", {
        task_id: fixture.taskId,
        task_environment_id: fixture.taskEnvironmentId,
        terminal_id: terminalId,
      }),
    ),
  );
}

export type McpRecoveryRequests = {
  authenticate: Array<{ server_id?: string; [key: string]: unknown }>;
  retry: Array<{ server_id?: string; [key: string]: unknown }>;
};

export async function installAgentMcpRecoveryRoutes(
  page: Page,
  args: {
    sessionId: string;
    taskEnvironmentId: string;
    terminalId: string;
    retryResponses?: Array<{ status: number; body: Record<string, unknown> }>;
  },
): Promise<McpRecoveryRequests> {
  const requests: McpRecoveryRequests = { authenticate: [], retry: [] };
  let retryCount = 0;
  await page.route(`**/api/v1/task-sessions/${args.sessionId}/mcp/authenticate`, async (route) => {
    requests.authenticate.push(route.request().postDataJSON() as { server_id?: string });
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        terminal_id: args.terminalId,
        task_environment_id: args.taskEnvironmentId,
        label: "Sign in to Atlassian",
        reused: false,
      }),
    });
  });
  await page.route(`**/api/v1/task-sessions/${args.sessionId}/mcp/retry`, async (route) => {
    requests.retry.push(route.request().postDataJSON() as { server_id?: string });
    const retryResponse =
      args.retryResponses?.[Math.min(retryCount++, (args.retryResponses?.length ?? 1) - 1)];
    await route.fulfill({
      contentType: "application/json",
      status: retryResponse?.status ?? 200,
      body: JSON.stringify(
        retryResponse?.body ?? {
          provider_id: "cursor",
          server_id: E2E_MCP_SERVER_ID,
          status: "ready",
          tool_count: 3,
        },
      ),
    });
  });
  return requests;
}
