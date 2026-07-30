import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { Page } from "@playwright/test";
import { rewriteBackendHostOS } from "../../helpers/boot-payload";
import { SessionPage } from "../../pages/session-page";

async function seedTask(
  page: Page,
  apiClient: ApiClient,
  seedData: {
    workspaceId: string;
    workflowId: string;
    startStepId: string;
    agentProfileId: string;
  },
) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile host platform editor availability",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [],
    },
  );
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
}

test("Windows host keeps the intentional mobile topbar without desktop editor controls", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  await rewriteBackendHostOS(testPage, "windows");
  await seedTask(testPage, apiClient, seedData);

  await expect(testPage.getByTestId("mobile-session-menu")).toBeVisible();
  await expect(testPage.getByTestId("editors-menu-list")).toHaveCount(0);
  const width = await testPage.evaluate(() => ({
    client: document.documentElement.clientWidth,
    scroll: document.documentElement.scrollWidth,
  }));
  expect(width.scroll).toBeLessThanOrEqual(width.client + 1);
  await prCapture.screenshot("windows-host-editor-menu-mobile", {
    caption: "Mobile task topbar remains compact on a Windows Kandev host",
  });
});
