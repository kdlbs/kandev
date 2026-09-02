import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import {
  assertLocatorWithinViewportX,
  assertNoDocumentHorizontalOverflow,
} from "../../helpers/layout-assertions";
import { useRegularMode } from "../../helpers/regular-mode";
import { waitForSessionDone } from "../../helpers/session";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { SessionPage } from "../../pages/session-page";

useRegularMode();

const TEST_RUNTIME_PREFIX = "e2e-mcp-mobile-task-";

async function cleanupMCPServers(apiClient: ApiClient, workspaceId: string) {
  const servers = await apiClient.listMCPServers(workspaceId);
  for (const server of servers) {
    if (server.runtime_name.startsWith(TEST_RUNTIME_PREFIX)) {
      await apiClient.deleteMCPServer(workspaceId, server.id, server.revision);
    }
  }
}

test.describe("Task MCP selection on mobile", () => {
  test.afterEach(async ({ apiClient, seedData }) => {
    await cleanupMCPServers(apiClient, seedData.workspaceId);
  });

  test("keeps task additions inside closed Advanced settings and a touch drawer", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const [inheritedMCP, taskMCP] = await Promise.all(
      ["Mobile inherited MCP", "Mobile task MCP"].map((displayName, index) =>
        apiClient.createMCPServer(seedData.workspaceId, {
          runtime_name: `${TEST_RUNTIME_PREFIX}${index}`,
          display_name: displayName,
          description: `${displayName} for mobile selection coverage.`,
          execution_mode: "remote",
          transport: "streamable_http",
          configuration: { url: `https://mcp.example.test/mobile-task-${index}` },
          source: "custom",
        }),
      ),
    );
    await apiClient.replaceMCPSelections("profile", seedData.agentProfileId, seedData.workspaceId, [
      inheritedMCP.id,
    ]);

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.mobileFab.tap();

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    const advanced = dialog.getByTestId("task-create-advanced-settings");
    const advancedTrigger = advanced.getByTestId("task-create-advanced-settings-trigger");
    await expect(advancedTrigger).toHaveAttribute("aria-expanded", "false");
    await expect(dialog.getByTestId("task-create-mcp-selection")).toHaveCount(0);
    const triggerBox = await advancedTrigger.boundingBox();
    expect(triggerBox).not.toBeNull();
    expect(triggerBox!.height).toBeGreaterThanOrEqual(44);
    await advancedTrigger.tap();

    await expect(advancedTrigger).toHaveAttribute("aria-expanded", "true");
    const mcpSelection = dialog.getByTestId("task-create-mcp-selection");
    await expect(mcpSelection).toBeVisible();
    await expect(mcpSelection).toContainText("MCP servers");

    const pickerTrigger = mcpSelection.getByRole("button");
    const pickerTriggerBox = await pickerTrigger.boundingBox();
    expect(pickerTriggerBox).not.toBeNull();
    expect(pickerTriggerBox!.height).toBeGreaterThanOrEqual(44);
    await pickerTrigger.tap();

    const drawer = testPage.locator('[data-slot="drawer-content"][data-state="open"]').last();
    await expect(drawer).toBeVisible();
    await assertLocatorWithinViewportX(drawer, "mobile task MCP drawer");
    await assertNoDocumentHorizontalOverflow(testPage, "mobile task MCP drawer");
    await expect(drawer).toContainText("Inherited MCP servers");
    await expect(drawer).toContainText("Mobile inherited MCP");
    const taskRow = drawer.getByText("Mobile task MCP", { exact: true });
    await expect(taskRow).toBeVisible();
    await taskRow.tap();
    await expect(mcpSelection).toContainText("1 selected");

    await testPage.keyboard.press("Escape");
    const taskTitle = "Mobile MCP task";
    await dialog.getByTestId("task-title-input").fill(taskTitle);
    await dialog.getByTestId("task-description-input").fill("Persist the mobile MCP selection.");
    await expect(dialog.getByTestId("submit-start-agent")).toBeEnabled({ timeout: 30_000 });
    await dialog.getByTestId("mobile-submit-create-without-agent").tap();

    await expect(dialog).not.toBeVisible({ timeout: 15_000 });
    const findCreatedTask = async () => {
      const { tasks } = await apiClient.listTasks(seedData.workspaceId);
      return tasks.find((task) => task.title === taskTitle)?.id;
    };
    await expect.poll(findCreatedTask, { timeout: 30_000 }).toBeDefined();
    const taskId = await findCreatedTask();
    if (!taskId) throw new Error("Task was not created from the mobile dialog");
    await expect
      .poll(
        async () =>
          (await apiClient.getMCPSelections("task", taskId, seedData.workspaceId)).definition_ids,
      )
      .toEqual([taskMCP.id]);
  });

  test("opens the idle session MCP sheet and reports applied state", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const sessionMCP = await apiClient.createMCPServer(seedData.workspaceId, {
      runtime_name: `${TEST_RUNTIME_PREFIX}session`,
      display_name: "Mobile session MCP",
      description: "Session-only MCP addition.",
      execution_mode: "remote",
      transport: "streamable_http",
      configuration: { url: "https://mcp.example.test/mobile-session" },
      source: "custom",
    });
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile MCP session task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");
    await waitForSessionDone(apiClient, task.id, task.session_id, "Waiting for mobile MCP session");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    const settings = testPage.getByTestId("task-session-mcp-settings");
    await expect(settings).toBeVisible();
    const sessionTrigger = settings.getByRole("button", { name: "Session MCP additions" });
    await sessionTrigger.tap();
    const picker = settings.getByTestId("task-session-mcp-settings-picker");
    await picker.getByRole("button").tap();

    const drawer = testPage.locator('[data-slot="drawer-content"][data-state="open"]').last();
    await expect(drawer).toBeVisible();
    await expect(drawer.locator(".overflow-y-auto")).toHaveCount(1);
    await assertLocatorWithinViewportX(drawer, "mobile session MCP drawer");
    await assertNoDocumentHorizontalOverflow(testPage, "mobile session MCP drawer");
    await drawer.getByText("Mobile session MCP", { exact: true }).tap();
    await testPage.keyboard.press("Escape");

    await expect
      .poll(async () =>
        apiClient.getMCPSelections("task_session", task.session_id!, seedData.workspaceId),
      )
      .toMatchObject({
        definition_ids: [sessionMCP.id],
        mcp_state: expect.objectContaining({ apply_state: "applied" }),
      });
    await expect(settings).toContainText("Applied");
  });
});
