import { expect, test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";

async function seedWalkthroughTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<void> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile Walkthrough E2E",
    seedData.agentProfileId,
    {
      description: "/e2e:walkthrough-basic",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.chat.getByText("5-step tour", { exact: false })).toBeVisible({
    timeout: 45_000,
  });
}

test.describe("Mobile code walkthrough", () => {
  test.describe.configure({ retries: 2, timeout: 120_000 });

  test("opens the walkthrough as a bottom-sheet panel", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedWalkthroughTask(testPage, apiClient, seedData);

    await testPage.getByTestId("walkthrough-launcher").click();
    const card = testPage.getByTestId("walkthrough-floating");
    await expect(card).toBeVisible({ timeout: 30_000 });
    await expect(card).toHaveAttribute("data-mobile-variant", "bottom-sheet");

    const box = await card.boundingBox();
    const viewport = testPage.viewportSize();
    if (!box || !viewport) throw new Error("walkthrough geometry unavailable");

    expect(box.x).toBeLessThanOrEqual(12);
    expect(box.width).toBeGreaterThanOrEqual(viewport.width - 24);
    expect(box.y + box.height).toBeGreaterThanOrEqual(viewport.height - 16);

    await expect(testPage.getByTestId("walkthrough-editor-range")).toBeVisible({
      timeout: 15_000,
    });
  });
});
