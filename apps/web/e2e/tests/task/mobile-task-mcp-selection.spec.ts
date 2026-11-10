import { expectMCPChoiceDetails } from "../../helpers/mcp-picker-consistency";
import { checkMCPOriginSections } from "../../helpers/mcp-origin-sections";
import { openMCPComposerCustomization } from "../../helpers/mcp-composer";
import { checkMCPSessionClarity } from "../../helpers/mcp-session-clarity";
import { checkMCPSelectionRefresh } from "../../helpers/mcp-selection-refresh";
import { expect, test } from "../../fixtures/test-base";
import { checkMCPSelectionRejection } from "../../helpers/mcp-selection-feedback";
import type { ApiClient } from "../../helpers/api-client";
import {
  assertLocatorWithinViewportX,
  assertNoDocumentHorizontalOverflow,
} from "../../helpers/layout-assertions";
import { useRegularMode } from "../../helpers/regular-mode";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
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
  test("groups MCP origins and enriches session choices", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await checkMCPOriginSections({ page: testPage, apiClient, seedData, mobile: true });
  });

  // @covers AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.13
  test("clarifies deferred session customization", async ({ testPage, apiClient, seedData }) => {
    await checkMCPSessionClarity({ page: testPage, apiClient, seedData });
  });
  // @covers AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.9, AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.11
  for (const stale of [false, true]) {
    test(
      stale
        ? "shows one recovery surface for stale MCP selections"
        : "keeps ordinary MCP authorization failures local",
      async ({ testPage, apiClient, seedData }) => {
        test.setTimeout(120_000);
        await checkMCPSelectionRejection({
          page: testPage,
          apiClient,
          seedData,
          stale,
          mobile: true,
        });
      },
    );
  }

  test("retains focused MCP choices through pending background refreshes", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await checkMCPSelectionRefresh({ page: testPage, apiClient, seedData, mobile: true });
  });

  let baseline: { profile: string[]; repository: string[] };
  test.beforeEach(async ({ apiClient, seedData, testPage }) => {
    void testPage;
    baseline = {
      profile: (
        await apiClient.getMCPSelections("profile", seedData.agentProfileId, seedData.workspaceId)
      ).definition_ids,
      repository: (
        await apiClient.getMCPSelections("repository", seedData.repositoryId, seedData.workspaceId)
      ).definition_ids,
    };
  });
  test.afterEach(async ({ apiClient, seedData }) => {
    if (baseline) {
      await apiClient.replaceMCPSelections(
        "profile",
        seedData.agentProfileId,
        seedData.workspaceId,
        baseline.profile,
      );
      await apiClient.replaceMCPSelections(
        "repository",
        seedData.repositoryId,
        seedData.workspaceId,
        baseline.repository,
      );
    }
    await cleanupMCPServers(apiClient, seedData.workspaceId);
  });

  test("keeps task additions inside closed Advanced settings and a touch drawer", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
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

    const pickerTrigger = mcpSelection.getByRole("button", { name: "MCP servers", exact: true });
    const priorityBox = (await dialog.getByTestId("task-create-priority-select").boundingBox())!;
    expect((await pickerTrigger.boundingBox())!.width).toBeLessThanOrEqual(priorityBox.width + 1);
    await expect(mcpSelection.locator(":scope > p")).toHaveCount(0);
    await mcpSelection.getByRole("button", { name: "About MCP servers", exact: true }).tap();
    const help = testPage.locator('[data-slot="drawer-content"][data-state="open"]');
    await expect(help).toHaveCount(1);
    await expect(help).toContainText(
      "Servers selected in its agent profile or repositories are included automatically.",
    );
    await help.getByRole("button", { name: "Close", exact: true }).tap();
    await expect(dialog).toBeVisible();
    const pickerTriggerBox = await pickerTrigger.boundingBox();
    expect(pickerTriggerBox).not.toBeNull();
    expect(pickerTriggerBox!.height).toBeGreaterThanOrEqual(44);
    await pickerTrigger.tap();

    const drawer = testPage.getByTestId("task-create-mcp-options");
    await expect(drawer).toHaveCount(1);
    await expect(drawer).toBeVisible();
    await assertLocatorWithinViewportX(drawer, "mobile task MCP drawer");
    await assertNoDocumentHorizontalOverflow(testPage, "mobile task MCP drawer");
    await expect(drawer).toContainText("Inherited MCP servers");
    await expect(drawer).toContainText("Mobile inherited MCP");
    await expectMCPChoiceDetails(
      drawer.getByTestId(`mcp-choice-${taskMCP.id}`),
      "Mobile task MCP for mobile selection coverage.",
      "Streamable HTTP",
    );
    const taskRow = drawer.getByText("Mobile task MCP", { exact: true });
    await expect(taskRow).toBeVisible();
    await taskRow.tap();
    await expect(mcpSelection).toContainText("1 selected");

    await drawer.getByRole("button", { name: "Close", exact: true }).tap();
    await expect(dialog).toBeVisible();
    await expect(pickerTrigger).toBeFocused();
    await pickerTrigger.tap();
    await expect(drawer.getByRole("checkbox", { name: /Mobile task MCP/ })).toBeChecked();
    await drawer.getByRole("button", { name: "Close", exact: true }).tap();
    const layout = dialog.getByTestId("task-create-initial-workspace-layout-setting");
    await layout.getByTestId("task-create-initial-workspace-layout-info").tap();
    const layoutHelp = testPage.getByTestId("task-create-initial-workspace-layout-help-drawer");
    await expect(layoutHelp).toBeVisible();
    await assertLocatorWithinViewportX(layoutHelp, "parent workspace help after MCP selection");
    await testPage.keyboard.press("Escape");
    await expect(layoutHelp).not.toBeVisible();
    const layoutCheckbox = layout.getByTestId("task-create-initial-workspace-layout-checkbox");
    await layoutCheckbox.tap();
    await expect(layoutCheckbox).toHaveAttribute("data-state", "checked");
    const taskTitle = "Mobile MCP task";
    await dialog.getByTestId("task-title-input").fill(taskTitle);
    await dialog.getByTestId("task-description-input").fill("Persist the mobile MCP selection.");
    if (prCapture.capturing) {
      await layout.scrollIntoViewIfNeeded();
      await expect(mcpSelection).toBeInViewport();
      await waitForFiniteAnimations(dialog);
      await prCapture.screenshot("mobile-mcp-and-parent-workspace", {
        caption: "Task MCP additions and parent workspace layout in mobile Advanced settings",
      });
    }
    await expect(dialog.getByTestId("submit-start-agent")).toBeEnabled({ timeout: 30_000 });
    await dialog.getByTestId("mobile-create-without-agent").tap();

    await expect(dialog).not.toBeVisible({ timeout: 15_000 });
    const findCreatedTask = async () => {
      const { tasks } = await apiClient.listTasks(seedData.workspaceId);
      return tasks.find((task) => task.title === taskTitle)?.id;
    };
    await expect.poll(findCreatedTask, { timeout: 30_000 }).toBeDefined();
    const taskId = await findCreatedTask();
    if (!taskId) throw new Error("Task was not created from the mobile dialog");
    await expect
      .poll(() => apiClient.getTask(taskId))
      .toMatchObject({
        initial_workspace_layout: "task_root",
      });
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
    const inheritedMCP = await apiClient.createMCPServer(seedData.workspaceId, {
      runtime_name: `${TEST_RUNTIME_PREFIX}inherited`,
      display_name: "Mobile inherited documentation",
      description: "Readable profile and repository origins.",
      execution_mode: "remote",
      transport: "streamable_http",
      configuration: { url: "https://mcp.example.test/mobile-inherited" },
      source: "custom",
    });
    await apiClient.replaceMCPSelections("profile", seedData.agentProfileId, seedData.workspaceId, [
      inheritedMCP.id,
    ]);
    await apiClient.replaceMCPSelections(
      "repository",
      seedData.repositoryId,
      seedData.workspaceId,
      [inheritedMCP.id],
    );
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

    await expect(testPage.getByTestId("task-session-mcp-settings")).toHaveCount(0);
    const sessionTrigger = testPage.getByTestId("mcp-status-trigger");
    const triggerBox = await sessionTrigger.boundingBox();
    expect(triggerBox!.height).toBeGreaterThanOrEqual(44);
    await openMCPComposerCustomization(testPage);

    const drawer = testPage.locator('[data-slot="drawer-content"][data-state="open"]').last();
    await expect(drawer).toBeVisible();
    await expect(
      drawer.getByRole("tabpanel", { name: "Customize", exact: true }).locator(".overflow-y-auto"),
    ).toHaveCount(1);
    await assertLocatorWithinViewportX(drawer, "mobile session MCP drawer");
    await assertNoDocumentHorizontalOverflow(testPage, "mobile session MCP drawer");
    await expect(drawer).toHaveCount(1);
    const profile = await apiClient.getAgentProfile(seedData.agentProfileId);
    const repository = await apiClient.getRepository(seedData.repositoryId);
    await expect(drawer).toContainText(profile.name);
    await expect(drawer).toContainText(`Repository: ${repository.name}`);
    await expect(drawer).not.toContainText(seedData.agentProfileId);
    await expect(drawer).not.toContainText(seedData.repositoryId);
    const close = drawer.getByRole("button", { name: "Close", exact: true });
    const closeBox = await close.boundingBox();
    expect(closeBox!.height).toBeGreaterThanOrEqual(44);
    expect(closeBox!.width).toBeGreaterThanOrEqual(44);
    const search = drawer.getByRole("textbox", { name: "Search MCP servers" });
    await search.fill("Mobile session");
    const checkbox = drawer
      .locator("label")
      .filter({ hasText: "Mobile session MCP" })
      .getByRole("checkbox");
    await checkbox.focus();
    await testPage.keyboard.press("Space");

    await expect
      .poll(async () =>
        apiClient.getMCPSelections("task_session", task.session_id!, seedData.workspaceId),
      )
      .toMatchObject({
        definition_ids: [sessionMCP.id],
        mcp_state: expect.objectContaining({ apply_state: "applied" }),
      });
    await expect(drawer).toContainText("Setup up to date");
    await expect(checkbox).toBeChecked();
    await close.click();
    await expect(drawer).toHaveCount(0);
    await expect(sessionTrigger).toBeFocused();
    await expect(testPage.getByTestId("mcp-selection-marker")).toHaveCount(0);
    await openMCPComposerCustomization(testPage);
    await expect(search).toHaveValue("Mobile session");
    await expect(checkbox).toBeChecked();
    await testPage.keyboard.press("Escape");
    await expect(drawer).toHaveCount(0);
    await testPage.reload();
    await expect(testPage.getByTestId("mcp-selection-marker")).toHaveCount(0);
    await openMCPComposerCustomization(testPage);
    await expect(
      drawer.locator("label").filter({ hasText: "Mobile session MCP" }).getByRole("checkbox"),
    ).toBeChecked();
  });
});
