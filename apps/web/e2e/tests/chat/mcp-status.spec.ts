import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

async function openChat(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "MCP status test",
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
  return session;
}

test("desktop MCP status is available from the neutral composer trigger", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}, testInfo) => {
  test.skip(testInfo.project.name !== "chromium", "desktop-only status disclosure");
  await openChat(testPage, apiClient, seedData);

  const trigger = testPage.getByTestId("mcp-status-trigger");
  await expect(trigger).toBeVisible();
  await trigger.hover();
  await expect(testPage.getByTestId("mcp-status-popover")).toBeVisible();
  const popover = testPage.locator('[data-testid="mcp-status-popover"]:visible');
  await expect(popover.getByText("kandev", { exact: true }).first()).toBeVisible();
  await expect(
    popover
      .getByText(
        /^(Unknown|Delivered — connection unverified|Connected|Active|Failed|Filtered|Unavailable)$/,
      )
      .first(),
  ).toBeVisible();
  await prCapture.screenshot("desktop-mcp-status", {
    caption: "Desktop MCP connection status disclosure",
  });
  prCapture.flush();
});
