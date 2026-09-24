import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { assertNoDocumentHorizontalOverflow, requireBox } from "../../helpers/layout-assertions";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { openTaskTools } from "../../helpers/task-tools";

test("task chrome keeps diagnostics and workspace tools behind labelled disclosures", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  // @covers AC-UI-TASK-TOPBAR-001.1, AC-UI-TASK-TOPBAR-001.2, AC-UI-TASK-TOPBAR-001.3
  const settings = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
    app_status_bar_enabled: false,
    system_metrics_display: { show_in_topbar: true, simplified: false },
  });
  expect(settings.ok).toBe(true);
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Keep checkout progress visible during payment confirmation",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await testPage.setViewportSize({ width: 1000, height: 800 });
  await testPage.goto(`/t/${task.id}`);
  await new SessionPage(testPage).waitForLoad();

  const header = testPage.getByTestId("task-topbar");
  await expect(header.getByTestId("layout-preset-trigger")).not.toBeVisible();
  await expect(header.getByTestId("system-metric-meter")).toHaveCount(0);
  const metricsTrigger = header.getByRole("button", { name: "System metrics", exact: true });
  await metricsTrigger.focus();
  await metricsTrigger.press("Enter");
  const metrics = testPage.getByRole("dialog", { name: "System metrics", exact: true });
  await expect(metrics).toBeFocused();
  await expect(metrics.getByLabel(/^CPU /)).toBeVisible();
  await expect(metrics.getByLabel(/^Memory /)).toBeVisible();
  await testPage.keyboard.press("Escape");
  await expect(metricsTrigger).toBeFocused();

  const toolsTrigger = header.getByRole("button", { name: "Task tools", exact: true });
  await toolsTrigger.focus();
  await toolsTrigger.press("Enter");
  const tools = testPage.getByRole("dialog", { name: "Task tools", exact: true });
  await expect(tools).toBeFocused();
  await expect(tools.getByTestId("editors-menu-list")).toBeVisible();
  await expect(tools.getByTestId("open-task-folder")).toBeVisible();
  await testPage.keyboard.press("Escape");
  await expect(toolsTrigger).toBeFocused();
  await toolsTrigger.click();
  await testPage.keyboard.press("Tab");
  await expect(tools.getByTestId("layout-preset-trigger")).toBeFocused();
  await tools.getByTestId("layout-preset-trigger").click();
  await expect(testPage.getByRole("menu")).toBeVisible();
  await testPage.keyboard.press("Escape");
  await expect(testPage.getByRole("menu")).not.toBeVisible();
  await expect(tools.getByTestId("layout-preset-trigger")).toBeFocused();
  await expect(tools).toBeVisible();
  await toolsTrigger.click();
  await expect(tools).not.toBeVisible();

  await toolsTrigger.click();
  await tools.getByTestId("layout-preset-trigger").click();
  await testPage.locator('[data-testid="layout-preset-item"][data-preset-id="default"]').click();
  await expect(tools).not.toBeVisible();

  await openTaskTools(testPage);
  await tools.getByTestId("layout-preset-trigger").click();
  await testPage.getByRole("menuitem", { name: "Save current layout...", exact: true }).click();
  const saveDialog = testPage.getByRole("dialog", { name: "Save Current Layout", exact: true });
  await saveDialog.getByLabel("Name", { exact: true }).fill("Review desk");
  await saveDialog.getByRole("button", { name: "Save", exact: true }).click();
  await expect(saveDialog).not.toBeVisible();
  await expect
    .poll(async () =>
      (await apiClient.getUserSettings()).settings.saved_layouts?.some(
        (layout) => layout.name === "Review desk",
      ),
    )
    .toBe(true);
  if ((await toolsTrigger.getAttribute("aria-expanded")) === "true") await toolsTrigger.click();

  for (const width of [900, 1280, 1536]) {
    await testPage.setViewportSize({ width, height: 900 });
    await assertNoDocumentHorizontalOverflow(testPage);
    const title = await requireBox(header.getByTestId("task-topbar-title"), "task title");
    const workflow = await requireBox(header.getByTestId("workflow-stepper"), "workflow");
    const toolsBox = await requireBox(toolsTrigger, "task tools");
    expect(title.width).toBeGreaterThan(160);
    expect(title.x + title.width).toBeLessThanOrEqual(workflow.x + 1);
    expect(toolsBox.x + toolsBox.width).toBeLessThanOrEqual(width);
  }

  await apiClient.saveUserSettings({ app_status_bar_enabled: true });
  await testPage.reload();
  await expect(header.getByRole("button", { name: "System metrics", exact: true })).toHaveCount(0);
  await expect(testPage.getByTestId("app-status-bar")).toBeVisible();
});

test("wide touch chrome uses contained drawers with usable controls", async ({
  browser,
  apiClient,
  seedData,
  backend,
}) => {
  // @covers AC-UI-TASK-TOPBAR-001.5
  const context = await browser.newContext({
    baseURL: backend.baseUrl,
    viewport: { width: 1280, height: 900 },
    hasTouch: true,
  });
  const page = await context.newPage();
  try {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Make checkout recovery clear on every device",
      seedData.agentProfileId,
      { description: "/e2e:simple-message", workflow_id: seedData.workflowId },
    );
    await page.goto(`/t/${task.id}`);
    await new SessionPage(page).waitForLoad();
    const trigger = page.getByRole("button", { name: "Task tools", exact: true });
    expect((await requireBox(trigger, "touch tools trigger")).height).toBeGreaterThanOrEqual(44);
    await trigger.tap();
    const drawer = page.getByRole("dialog", { name: "Task tools", exact: true });
    await expect(drawer).toHaveAttribute("data-slot", "drawer-content");
    await waitForFiniteAnimations(drawer);
    const layout = drawer.getByTestId("layout-preset-trigger");
    expect((await requireBox(layout, "touch layout picker")).height).toBeGreaterThanOrEqual(44);
    await layout.tap();
    await expect(page.getByRole("menu")).toBeVisible();
    await waitForFiniteAnimations(page.getByRole("menu"));
    for (const item of await page.getByRole("menuitem").all()) {
      const box = await requireBox(item, "touch layout option");
      expect(box.height).toBeGreaterThanOrEqual(44);
      expect(box.width).toBeGreaterThanOrEqual(44);
    }
    await page.keyboard.press("Escape");
    const editor = drawer.getByTestId("editors-menu-list");
    const editorGroup = editor.locator("..");
    const editorBox = await requireBox(editor, "touch editor button");
    const groupBox = await requireBox(editorGroup, "touch editor group");
    expect(editorBox.height).toBeGreaterThanOrEqual(44);
    expect(editorBox.y).toBeGreaterThanOrEqual(groupBox.y + 1);
    expect(editorBox.y + editorBox.height).toBeLessThanOrEqual(groupBox.y + groupBox.height - 1);
    await editor.tap();
    await expect(page.getByRole("menu")).toBeVisible();
    await waitForFiniteAnimations(page.getByRole("menu"));
    for (const item of await page.getByRole("menuitem").all()) {
      const box = await requireBox(item, "touch editor option");
      expect(box.height).toBeGreaterThanOrEqual(44);
      expect(box.width).toBeGreaterThanOrEqual(44);
    }
    await assertNoDocumentHorizontalOverflow(page);
  } finally {
    await context.close();
  }
});
