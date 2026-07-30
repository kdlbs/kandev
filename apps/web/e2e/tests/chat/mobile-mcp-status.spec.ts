import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test("mobile MCP status opens in a contained Drawer", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}, testInfo) => {
  test.skip(testInfo.project.name !== "mobile-chrome", "mobile-only status Drawer");
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile MCP status test",
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

  const trigger = testPage.getByTestId("mcp-status-trigger");
  await expect(trigger).toBeVisible();
  await trigger.tap();
  const drawer = testPage.getByTestId("mcp-status-drawer");
  await expect(drawer).toBeVisible();
  await expect(drawer.getByText("MCP servers", { exact: true })).toBeVisible();
  await expect(drawer.getByText("kandev", { exact: true })).toBeVisible();
  await expect(
    drawer.getByText(
      /^(Unknown|Delivered — connection unverified|Connected|Active|Failed|Filtered|Unavailable)$/,
    ),
  ).toBeVisible();
  await expect
    .poll(() =>
      testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    )
    .toBe(true);
  await prCapture.screenshot("mobile-mcp-status", {
    caption: "Mobile MCP connection status Drawer",
  });
  prCapture.flush();
});
