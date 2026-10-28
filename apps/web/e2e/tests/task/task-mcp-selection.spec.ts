import { expectMCPChoiceDetails } from "../../helpers/mcp-picker-consistency";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { checkMCPOriginSections } from "../../helpers/mcp-origin-sections";
import { openMCPComposerCustomization } from "../../helpers/mcp-composer";
import { checkMCPSessionClarity } from "../../helpers/mcp-session-clarity";
import { checkMCPSelectionRefresh } from "../../helpers/mcp-selection-refresh";
import { expect, test } from "../../fixtures/test-base";
import { checkMCPSelectionRejection } from "../../helpers/mcp-selection-feedback";
import type { ApiClient } from "../../helpers/api-client";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import { useRegularMode } from "../../helpers/regular-mode";

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
  test("groups MCP origins and enriches session choices", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await checkMCPOriginSections({ page: testPage, apiClient, seedData, mobile: false });
  });

  test("groups MCP origins on a narrow fine-pointer phone", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await testPage.setViewportSize({ width: 767, height: 900 });
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
          mobile: false,
        });
      },
    );
  }

  test("retains focused MCP choices through pending background refreshes", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await checkMCPSelectionRefresh({ page: testPage, apiClient, seedData, mobile: false });
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

    await expect(testPage.getByTestId("task-session-mcp-settings")).toHaveCount(0);
    const trigger = testPage.getByTestId("mcp-status-trigger");
    const triggerBox = await trigger.boundingBox();
    expect(triggerBox!.height).toBeCloseTo(28, 0);
    const settings = await openMCPComposerCustomization(testPage);

    const picker = settings.getByTestId("mcp-session-customization-picker");
    await expect(picker.getByRole("button", { name: /^MCP servers/ })).toHaveCount(0);
    await expect(
      picker.getByRole("heading", { name: "Selected for this task", exact: true }),
    ).toHaveCount(1);
    await expect(picker).toContainText("Profile MCP");
    const profile = await apiClient.getAgentProfile(seedData.agentProfileId);
    const repository = await apiClient.getRepository(seedData.repositoryId);
    await expect(picker).toContainText(profile.name);
    await expect(picker).not.toContainText(seedData.agentProfileId);
    await expect(picker).toContainText("Repository MCP");
    await expect(picker).toContainText(`Repository: ${repository.name}`);
    await expect(picker).toContainText("Task MCP");
    await expect(
      picker.getByRole("heading", { name: "Selected for this task", exact: true }),
    ).toHaveCount(1);
    await expect(picker).toContainText("Shared MCP");
    const sharedOrigin = picker.getByTestId(`mcp-inherited-server-${sharedMCP.id}`);
    await expect(sharedOrigin).toContainText(profile.name);
    await expect(sharedOrigin).toContainText(`Repository: ${repository.name}`);
    await expect(sharedOrigin).toContainText("Also selected by");

    const search = picker.getByRole("textbox", { name: "Search MCP servers" });
    await search.fill("Session MCP");
    const checkbox = picker
      .locator("label")
      .filter({ hasText: "Session MCP" })
      .getByRole("checkbox");
    await checkbox.focus();
    await testPage.keyboard.press("Space");
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
    await expect(settings).toContainText("Setup up to date");
    await expect(checkbox).toBeChecked();
    await settings.getByTestId("mcp-explorer-close").click();
    await openMCPComposerCustomization(testPage);
    await expect(search).toHaveValue("Session MCP");
    await expect(checkbox).toBeChecked();
    await testPage.reload();
    await openMCPComposerCustomization(testPage);
    await expect(checkbox).toBeChecked();
    // Saved selections survive the task layout's desktop/phone remount.
    await testPage.setViewportSize({ width: 390, height: 844 });
    await openMCPComposerCustomization(testPage);
    const drawer = testPage.locator('[data-slot="drawer-content"][data-state="open"]');
    await expect(drawer).toHaveCount(1);
    await expect(
      drawer.locator("label").filter({ hasText: "Session MCP" }).getByRole("checkbox"),
    ).toBeChecked();
    await testPage.keyboard.press("Escape");
    await testPage.setViewportSize({ width: 1280, height: 720 });
    await openMCPComposerCustomization(testPage);
    await expect(checkbox).toBeChecked();
  });
  test.describe("coarse-pointer tablet", () => {
    test.use({ hasTouch: true, viewport: { width: 900, height: 1000 } });
    test("uses a bounded touch drawer while selection updates outside it", async ({
      testPage,
      apiClient,
      seedData,
    }) => {
      await checkMCPSelectionRefresh({ page: testPage, apiClient, seedData, mobile: false });
      const trigger = await testPage.getByTestId("mcp-status-trigger").boundingBox();
      expect(trigger!.height).toBeGreaterThanOrEqual(44);
      const drawer = testPage.locator('[data-slot="drawer-content"][data-state="open"]');
      await expect(drawer).toHaveCount(1);
      await waitForFiniteAnimations(drawer);
      const box = await drawer.boundingBox();
      // Browser transforms can produce fractional-pixel rounding at the 80vh limit.
      expect(box!.height).toBeLessThanOrEqual(testPage.viewportSize()!.height * 0.8 + 1);
      const close = await drawer.getByTestId("mcp-explorer-close").boundingBox();
      expect(close!.width).toBeGreaterThanOrEqual(44);
      expect(close!.height).toBeGreaterThanOrEqual(44);
    });
  });
});

// @covers AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.4, AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.8
test.describe("Advanced MCP control", () => {
  useRegularMode();

  test("matches neighboring Advanced settings and preserves task additions", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const previous = await apiClient.getMCPSelections(
      "profile",
      seedData.agentProfileId,
      seedData.workspaceId,
    );
    const definitions = [];
    try {
      for (const [index, display_name] of [
        "Inherited documentation",
        "Task issue tracker",
      ].entries())
        definitions.push(
          await apiClient.createMCPServer(seedData.workspaceId, {
            runtime_name: `e2e-advanced-mcp-${index}`,
            display_name,
            description: "Find and update project issues.",
            source: "custom",
            execution_mode: "remote",
            transport: "sse",
            configuration: { url: `https://mcp.example.test/advanced-${index}` },
          }),
        );
      await apiClient.replaceMCPSelections(
        "profile",
        seedData.agentProfileId,
        seedData.workspaceId,
        [definitions[0].id],
      );
      await testPage.goto("/");
      const create = testPage.getByRole("button", { name: "New Task", exact: true });
      await expect(create).toHaveCount(1);
      await create.click();
      const dialog = testPage.getByTestId("create-task-dialog");
      const advanced = dialog.getByTestId("task-create-advanced-settings-trigger");
      await expect(advanced).toHaveAttribute("aria-expanded", "false");
      await expect(dialog.getByTestId("task-create-mcp-selection")).toHaveCount(0);
      await advanced.click();
      const selection = dialog.getByTestId("task-create-mcp-selection");
      const trigger = selection.getByRole("button", { name: /^MCP servers/ });
      const [mcpBox, priorityBox] = await Promise.all([
        trigger.boundingBox(),
        dialog.getByTestId("task-create-priority-select").boundingBox(),
      ]);
      expect(mcpBox!.width).toBeLessThanOrEqual(priorityBox!.width + 1);
      expect(mcpBox!.height).toBeCloseTo(28, 0);
      await expect(selection.locator(":scope > p")).toHaveCount(0);
      await expect(selection.getByText("MCP servers", { exact: true })).toBeVisible();
      const help = selection.getByRole("button", { name: "About MCP servers", exact: true });
      await help.hover();
      await expect(testPage.getByRole("tooltip")).toContainText(
        "Servers selected in its agent profile or repositories are included automatically.",
      );
      await trigger.focus();
      await testPage.keyboard.press("Enter");
      const picker = dialog.getByTestId("task-create-mcp-options");
      await expect(picker).toBeVisible();
      await expect(picker).toContainText("Inherited documentation");
      const search = picker.getByRole("textbox", { name: "Search MCP servers" });
      await search.fill("Task issue");
      await expectMCPChoiceDetails(
        picker.getByTestId(`mcp-choice-${definitions[1].id}`),
        "Find and update project issues.",
      );
      await picker.getByRole("checkbox").press("Space");
      await expect(trigger).toHaveText("1 selected");
      await testPage.keyboard.press("Escape");
      await expect(picker).not.toBeVisible();
      await expect(dialog).toBeVisible();
      await expect(trigger).toBeFocused();
      await trigger.click();
      await expect(search).toHaveValue("Task issue");
      await expect(picker.getByRole("checkbox")).toBeChecked();
      await testPage.keyboard.press("Escape");
      const layout = dialog.getByTestId("task-create-initial-workspace-layout-setting");
      await layout.getByTestId("task-create-initial-workspace-layout-checkbox").click();
      await dialog.getByTestId("task-title-input").fill("Advanced MCP task");
      await dialog
        .getByTestId("task-description-input")
        .fill("Verify task additions from a compact control.");
      await expect(dialog.getByTestId("submit-start-agent")).toBeEnabled({ timeout: 30_000 });
      await dialog.getByTestId("submit-start-agent-chevron").click();
      await testPage.getByTestId("submit-create-without-agent").click();
      await expect(dialog).not.toBeVisible();
      const findTask = async () =>
        (await apiClient.listTasks(seedData.workspaceId)).tasks.find(
          (task) => task.title === "Advanced MCP task",
        )?.id;
      await expect.poll(findTask).toBeDefined();
      const taskId = (await findTask())!;
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
        .toEqual([definitions[1].id]);
      await testPage.reload();
      expect(
        (await apiClient.getMCPSelections("task", taskId, seedData.workspaceId)).definition_ids,
      ).toEqual([definitions[1].id]);
    } finally {
      await apiClient.replaceMCPSelections(
        "profile",
        seedData.agentProfileId,
        seedData.workspaceId,
        previous.definition_ids,
      );
      for (const definition of definitions.reverse())
        await apiClient.deleteMCPServer(seedData.workspaceId, definition.id, definition.revision);
    }
  });
});
