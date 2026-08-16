import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test("mobile MCP explorer uses a full-height list-to-detail drawer", async ({
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
  const triggerBox = await trigger.boundingBox();
  expect(triggerBox?.height).toBeGreaterThanOrEqual(44);
  await trigger.tap();

  const drawer = testPage.getByTestId("mcp-server-explorer");
  await expect(drawer).toBeVisible();
  const drawerBox = await drawer.boundingBox();
  const viewport = testPage.viewportSize();
  expect(drawerBox?.height).toBeGreaterThanOrEqual((viewport?.height ?? 0) * 0.9);
  await expect(drawer.getByTestId("mcp-server-list")).toBeVisible();
  await expect(drawer.getByTestId("mcp-server-row-kandev")).toBeVisible();

  await drawer.getByTestId("mcp-server-row-kandev").tap();
  const detail = drawer.getByTestId("mcp-server-detail");
  await expect(detail).toBeVisible();
  await expect(detail.getByTestId("mcp-tool-row-create_task_kandev")).toBeVisible({
    timeout: 30_000,
  });
  const detailScroll = drawer.getByTestId("mcp-server-detail-scroll");
  await expect
    .poll(() => detailScroll.evaluate((element) => element.scrollHeight > element.clientHeight))
    .toBe(true);
  const back = detail.getByRole("button", { name: "Back to servers" });
  await expect(back).toBeVisible();
  const backBox = await back.boundingBox();
  expect(backBox?.height).toBeGreaterThanOrEqual(44);
  await prCapture.screenshot("mobile-mcp-explorer-kandev", {
    caption: "Mobile MCP server explorer with a focused Kandev tool catalog",
  });

  await back.tap();
  await expect(drawer.getByTestId("mcp-server-list")).toBeVisible();
  await expect(drawer.getByTestId("mcp-server-detail")).toHaveCount(0);
  expect(
    await testPage.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
  prCapture.flush();
});
