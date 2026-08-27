import { type Locator } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

async function expectMinTouchHeight(control: Locator) {
  const box = await control.boundingBox();
  expect(box?.height).toBeGreaterThanOrEqual(44);
}

async function expectExplorerTypography(elements: Locator[]) {
  const fontSizes = await Promise.all(
    elements.map((element) => element.evaluate((node) => getComputedStyle(node).fontSize)),
  );
  expect(new Set(fontSizes)).toEqual(new Set(["13px"]));
}

test("mobile MCP explorer uses servers, tools, and tool detail pages", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}, testInfo) => {
  test.skip(testInfo.project.name !== "mobile-chrome", "mobile-only MCP explorer");
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile MCP explorer test",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  await session.sendMessageViaButton("e2e:mcp:kandev:list_workspaces_kandev({})");
  await session.waitForChatIdle({ timeout: 30_000 });

  const trigger = testPage.getByTestId("mcp-status-trigger");
  await expect(trigger).toBeVisible();
  await expectMinTouchHeight(trigger);
  await trigger.tap();

  const drawer = testPage.getByTestId("mcp-server-explorer");
  await expect(drawer).toBeVisible();
  const viewport = testPage.viewportSize();
  await expect
    .poll(async () => {
      const box = await drawer.boundingBox();
      return (box?.y ?? 0) + (box?.height ?? 0);
    })
    .toBeLessThanOrEqual(viewport?.height ?? 0);
  const drawerBox = await drawer.boundingBox();
  expect(drawerBox?.height).toBeGreaterThanOrEqual((viewport?.height ?? 0) * 0.9);
  expect((drawerBox?.y ?? 0) + (drawerBox?.height ?? 0)).toBeLessThanOrEqual(viewport?.height ?? 0);

  const serverRow = drawer.getByTestId("mcp-server-row-kandev");
  await expect(serverRow).toBeVisible();
  await expectMinTouchHeight(serverRow);
  await serverRow.tap();

  const toolList = drawer.getByTestId("mcp-tool-list");
  await expect(toolList).toBeVisible();
  const backToServers = toolList.getByRole("button", { name: "Back to servers" });
  await expectMinTouchHeight(backToServers);
  const toolScroll = toolList.getByTestId("mcp-tool-list-scroll");
  const maxScroll = await toolScroll.evaluate(
    (element) => element.scrollHeight - element.clientHeight,
  );
  expect(maxScroll).toBeGreaterThan(0);

  const createTask = toolList.getByTestId("mcp-tool-row-create_task_kandev");
  await createTask.scrollIntoViewIfNeeded();
  await expectMinTouchHeight(createTask);
  await expectExplorerTypography([
    drawer.locator('[data-slot="drawer-description"]'),
    toolList.getByTestId("mcp-server-detail").locator("h3"),
    createTask,
    createTask.getByText(/^~\d+ tokens?$/),
  ]);
  await createTask.tap();
  const detail = drawer.getByTestId("mcp-tool-detail");
  await expect(detail).toBeVisible();
  await expect(detail.getByText(/^~\d+ tokens?$/)).toBeVisible();
  await expect(detail.getByText("title", { exact: true })).toBeVisible();
  await expectExplorerTypography([
    drawer.locator('[data-slot="drawer-description"]'),
    detail.locator("h3"),
    detail.locator("section").first().locator("p"),
    detail.getByText("title", { exact: true }),
  ]);
  const backToTools = detail.getByRole("button", { name: "Back to tools" });
  await expectMinTouchHeight(backToTools);
  await prCapture.screenshot("mobile-mcp-explorer-tool-detail", {
    caption: "Mobile MCP tool detail with focused description and arguments",
  });

  await backToTools.tap();
  await expect(toolList).toBeVisible();
  await backToServers.tap();
  await expect(drawer.getByTestId("mcp-server-list")).toBeVisible();
  await expect(drawer.getByTestId("mcp-tool-list")).toHaveCount(0);
  expect(
    await testPage.evaluate(
      () =>
        document.documentElement.scrollWidth <= document.documentElement.clientWidth &&
        document.documentElement.scrollHeight <= document.documentElement.clientHeight,
    ),
  ).toBe(true);
  prCapture.flush();
});
