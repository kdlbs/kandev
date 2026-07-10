import { expect, test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";

async function seedWalkthroughTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  scenario = "walkthrough-basic",
  doneText = "5-step tour",
): Promise<void> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile Walkthrough E2E",
    seedData.agentProfileId,
    {
      description: `/e2e:${scenario}`,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.chat.getByText(doneText, { exact: false })).toBeVisible({
    timeout: 45_000,
  });
  await session.waitForChatIdle();
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

  test("requests a walkthrough from Changes and opens the generated bottom sheet", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedWalkthroughTask(testPage, apiClient, seedData, "walkthrough-setup", "changes ready");

    await testPage.getByRole("button", { name: "Changes" }).click();
    const changes = testPage.getByTestId("mobile-changes-panel");
    await expect(changes).toBeVisible({ timeout: 15_000 });
    const request = changes.getByTestId("changes-request-walkthrough");
    await expect(request).toBeEnabled({ timeout: 30_000 });
    await request.click();

    await testPage.getByRole("button", { name: "Chat" }).click();
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.activeChat()).toContainText("Walkthrough: Tour of the change", {
      timeout: 45_000,
    });
    await expect(session.activeChat()).toContainText("walkthrough-request complete", {
      timeout: 45_000,
    });

    await session.walkthroughLauncher().click();
    const card = session.walkthroughFloating();
    await expect(card).toBeVisible({ timeout: 30_000 });
    await expect(card).toHaveAttribute("data-mobile-variant", "bottom-sheet");
    await expect(card.getByTestId("walkthrough-step-title")).toContainText("Overview");
    await expect(card.getByTestId("walkthrough-step-body")).toContainText("ELI5:");
    await expect(session.walkthroughEditorRange()).toBeVisible({ timeout: 15_000 });
  });

  test("can discard a walkthrough from the touch launcher", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedWalkthroughTask(testPage, apiClient, seedData);
    const session = new SessionPage(testPage);

    await expect(session.walkthroughLauncher()).toBeVisible({ timeout: 30_000 });
    await expect(session.walkthroughDiscardButton()).toBeVisible({ timeout: 5_000 });
    await session.walkthroughDiscardButton().click();
    await expect(session.walkthroughDiscardDialog()).toBeVisible();
    await session
      .walkthroughDiscardDialog()
      .getByRole("button", { name: "Discard walkthrough" })
      .click();

    await expect(session.walkthroughLauncher()).toHaveCount(0);
    await expect(session.walkthroughFloating()).toHaveCount(0);
  });
});
