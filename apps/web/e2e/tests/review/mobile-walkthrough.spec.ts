import { expect, test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import { expectWalkthroughBehindDialog } from "./walkthrough-layering";

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
    prCapture,
  }) => {
    await seedWalkthroughTask(testPage, apiClient, seedData);

    await testPage.getByTestId("walkthrough-launcher").click();
    const card = testPage.getByTestId("walkthrough-floating");
    await expect(card).toBeVisible({ timeout: 30_000 });
    await expect(card).toHaveAttribute("data-mobile-variant", "bottom-sheet");
    await expect(card.getByRole("button", { name: "Cancel", exact: true })).toHaveCount(0);
    await expect(card.getByRole("button", { name: "Add", exact: true })).toBeVisible();
    await expect(card.getByRole("button", { name: "Run", exact: true })).toBeVisible();
    await card.getByRole("textbox").fill("Why does this line exist?");
    await expect(card.getByRole("button", { name: "Add", exact: true })).toBeEnabled();
    await expect(card.getByRole("button", { name: "Run", exact: true })).toBeEnabled();
    await prCapture.screenshot("mobile-walkthrough-feedback-controls", {
      caption: "Mobile walkthrough keeps Add and Run without an inert Cancel action",
    });

    const box = await card.boundingBox();
    const viewport = testPage.viewportSize();
    if (!box || !viewport) throw new Error("walkthrough geometry unavailable");

    expect(box.x).toBeLessThanOrEqual(12);
    expect(box.width).toBeGreaterThanOrEqual(viewport.width - 24);
    expect(box.y + box.height).toBeGreaterThanOrEqual(viewport.height - 16);

    await expect(testPage.getByTestId("walkthrough-editor-range")).toBeVisible({
      timeout: 15_000,
    });

    await testPage.evaluate(() => window.dispatchEvent(new CustomEvent("open-review-dialog")));
    const reviewDialog = testPage.getByRole("dialog", { name: "Review Changes" });
    await expect(reviewDialog).toBeVisible({ timeout: 15_000 });
    await expect(reviewDialog.locator('[data-walkthrough-active="true"]')).toHaveCount(0);
    await expect(reviewDialog.getByText(/^0 of \d+ files reviewed$/)).toBeVisible();
    await expectWalkthroughBehindDialog(testPage, reviewDialog, [
      { locator: card, name: "walkthrough window" },
      {
        locator: testPage.getByTestId("walkthrough-launcher").locator(".."),
        name: "walkthrough launcher",
      },
    ]);
    await prCapture.screenshot("mobile-review-with-walkthrough", {
      caption: "Mobile Review stays isolated from the task walkthrough",
    });
  });

  test("requests a walkthrough from Changes and opens the generated bottom sheet", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await seedWalkthroughTask(testPage, apiClient, seedData, "walkthrough-setup", "changes ready");

    await testPage.getByRole("button", { name: "Changes" }).click();
    const changes = testPage.getByTestId("mobile-changes-panel");
    await expect(changes).toBeVisible({ timeout: 15_000 });
    const request = changes.getByTestId("changes-request-walkthrough");
    await expect(request).toBeEnabled({ timeout: 30_000 });
    await expect(request.getByText("Walkthrough", { exact: true })).toBeVisible();
    const [changesBox, requestBox] = await Promise.all([
      changes.boundingBox(),
      request.boundingBox(),
    ]);
    if (!changesBox || !requestBox) throw new Error("Mobile Changes toolbar geometry unavailable");
    expect(requestBox.x).toBeGreaterThanOrEqual(changesBox.x);
    expect(requestBox.x + requestBox.width).toBeLessThanOrEqual(
      changesBox.x + changesBox.width + 1,
    );
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
    await prCapture.screenshot("mobile-changes-walkthrough", {
      caption: "Mobile Changes keeps the Walkthrough action within the viewport",
    });
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

    const initialNavigation = await card.getByTestId("walkthrough-navigation").boundingBox();
    if (!initialNavigation) throw new Error("walkthrough navigation missing before step change");
    await card.getByTestId("walkthrough-next").tap();
    await expect(card.getByTestId("walkthrough-step-header")).toContainText("Step 2 / 5");
    const nextNavigation = await card.getByTestId("walkthrough-navigation").boundingBox();
    if (!nextNavigation) throw new Error("walkthrough navigation missing after step change");
    expect(Math.abs(nextNavigation.y - initialNavigation.y)).toBeLessThan(1);
    const viewport = testPage.viewportSize();
    if (!viewport) throw new Error("walkthrough viewport unavailable");
    expect(nextNavigation.y).toBeGreaterThanOrEqual(0);
    expect(nextNavigation.y + nextNavigation.height).toBeLessThanOrEqual(viewport.height);
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
