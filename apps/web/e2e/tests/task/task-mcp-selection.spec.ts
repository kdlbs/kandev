import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

const TEST_RUNTIME_PREFIX = "e2e-mcp-task-";

async function cleanupMCPServers(apiClient: ApiClient, workspaceId: string) {
  const servers = await apiClient.listMCPServers(workspaceId);
  for (const server of servers) {
    if (server.runtime_name.startsWith(TEST_RUNTIME_PREFIX)) {
      await apiClient.deleteMCPServer(workspaceId, server.id, server.revision);
    }
  }
}

test.describe("Task MCP selection", () => {
  test.afterEach(async ({ apiClient, seedData }) => {
    await cleanupMCPServers(apiClient, seedData.workspaceId);
  });

  test("composes profile, repository, task, and idle-session additions with origins", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const definitions = await Promise.all(
      ["Profile MCP", "Repository MCP", "Task MCP", "Shared MCP", "Session MCP"].map(
        (displayName, index) =>
          apiClient.createMCPServer(seedData.workspaceId, {
            runtime_name: `${TEST_RUNTIME_PREFIX}${index}`,
            display_name: displayName,
            description: `${displayName} for additive scope coverage.`,
            execution_mode: "remote",
            transport: "streamable_http",
            configuration: { url: `https://mcp.example.test/task-${index}` },
            source: "custom",
          }),
      ),
    );
    const [profileMCP, repositoryMCP, taskMCP, sharedMCP, sessionMCP] = definitions;

    await apiClient.replaceMCPSelections("profile", seedData.agentProfileId, seedData.workspaceId, [
      profileMCP.id,
      sharedMCP.id,
    ]);
    await apiClient.replaceMCPSelections(
      "repository",
      seedData.repositoryId,
      seedData.workspaceId,
      [repositoryMCP.id, sharedMCP.id],
    );
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Task MCP union task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        mcp_server_ids: [taskMCP.id, sharedMCP.id],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

    await expect
      .poll(async () => {
        const selection = await apiClient.getMCPSelections("task", task.id, seedData.workspaceId);
        return selection.definition_ids;
      })
      .toEqual([sharedMCP.id, taskMCP.id].sort());

    await waitForSessionDone(apiClient, task.id, task.session_id, "Waiting for MCP union session");
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    const settings = testPage.getByTestId("task-session-mcp-settings");
    await expect(settings).toBeVisible();
    const trigger = settings.getByRole("button", { name: "Session MCP additions" });
    await expect(trigger).toHaveAttribute("aria-expanded", "false");
    await trigger.click();
    await expect(trigger).toHaveAttribute("aria-expanded", "true");

    const picker = settings.getByTestId("task-session-mcp-settings-picker");
    await picker.getByRole("button", { name: /^MCP servers/ }).click();
    await expect(picker).toContainText("Inherited MCP servers");
    await expect(picker).toContainText("Profile MCP");
    await expect(picker).toContainText(`Profile: ${seedData.agentProfileId}`);
    await expect(picker).toContainText("Repository MCP");
    await expect(picker).toContainText(`Repository: ${seedData.repositoryId}`);
    await expect(picker).toContainText("Task MCP");
    await expect(picker).toContainText(`Task: ${task.id}`);
    await expect(picker).toContainText("Shared MCP");
    const sharedOrigin = picker
      .getByText("Shared MCP", { exact: true })
      .locator("xpath=ancestor::div[contains(@class, 'border-dashed')][1]");
    await expect(sharedOrigin).toContainText(`Profile: ${seedData.agentProfileId}`);
    await expect(sharedOrigin).toContainText(`Repository: ${seedData.repositoryId}`);
    await expect(sharedOrigin).toContainText(`Task: ${task.id}`);

    await picker.getByText("Session MCP", { exact: true }).click();
    await expect
      .poll(async () => {
        const selection = await apiClient.getMCPSelections(
          "task_session",
          task.session_id!,
          seedData.workspaceId,
        );
        return selection;
      })
      .toMatchObject({
        definition_ids: [sessionMCP.id],
        mcp_state: expect.objectContaining({ apply_state: "applied" }),
      });
    await expect(settings).toContainText("Applied");
  });
});
