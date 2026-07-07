import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import type { Page, Locator } from "@playwright/test";

async function seedWalkthroughTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  scenario: string,
  doneText: string,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Walkthrough E2E",
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
  await expect(session.chat.getByText(doneText, { exact: false })).toBeVisible({ timeout: 45_000 });
  return session;
}

/** Open the floating walkthrough card via the launcher pill. */
async function openWalkthrough(testPage: Page): Promise<Locator> {
  const launcher = testPage.getByTestId("walkthrough-launcher");
  await expect(launcher).toBeVisible({ timeout: 30_000 });
  await launcher.click();
  const card = testPage.getByTestId("walkthrough-floating");
  await expect(card).toBeVisible({ timeout: 30_000 });
  return card;
}

test.describe("Code walkthrough", () => {
  test.describe.configure({ retries: 2, timeout: 120_000 });

  test("floating card walks all steps across changed and unchanged files", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedWalkthroughTask(testPage, apiClient, seedData, "walkthrough-basic", "5-step tour");
    const card = await openWalkthrough(testPage);
    const header = card.getByTestId("walkthrough-step-header");
    const body = card.getByTestId("walkthrough-step-body");

    await expect(header).toContainText("Step 1 / 5");
    await expect(body).toContainText("Step 1");
    await expect(card.getByTestId("walkthrough-step-file")).toContainText("walkthrough_a.txt");
    await expect(card.getByTestId("walkthrough-prev")).toBeDisabled();

    const expectStep = async (n: number, text: string) => {
      await card.getByTestId("walkthrough-next").click();
      await expect(header).toContainText(`Step ${n} / 5`);
      await expect(body).toContainText(text);
    };
    await expectStep(2, "WALKTHROUGH_CHANGE_A");
    await expectStep(3, "WALKTHROUGH_CHANGE_B");
    await expectStep(4, "WALKTHROUGH_CHANGE_C");
    await expectStep(5, "WALKTHROUGH_UNCHANGED");
    await expect(card.getByTestId("walkthrough-next")).toBeDisabled();

    // The unchanged/base file is opened in an editor tab, but it does not belong
    // in the Review diff because it was not changed by this task.
    await expect(testPage.locator('.dv-default-tab:has-text("walkthrough_base.txt")')).toBeVisible({
      timeout: 15_000,
    });
    await testPage.evaluate(() => window.dispatchEvent(new CustomEvent("open-review-dialog")));
    const dialog = testPage.getByRole("dialog", { name: "Review Changes" });
    await expect(dialog).toBeVisible({ timeout: 15_000 });
    await expect(dialog.getByText("walkthrough_base.txt")).toHaveCount(0);
  });

  test("editor walkthrough shows range marker, connector, and supports dragging", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedWalkthroughTask(testPage, apiClient, seedData, "walkthrough-basic", "5-step tour");
    const card = await openWalkthrough(testPage);

    await expect(testPage.getByTestId("walkthrough-editor-range")).toBeVisible({
      timeout: 15_000,
    });
    await expect(testPage.getByTestId("walkthrough-connector")).toBeVisible({ timeout: 15_000 });

    await card.getByTestId("walkthrough-next").click();
    await expect(card.getByTestId("walkthrough-step-header")).toContainText("Step 2 / 5");
    await expect(testPage.getByTestId("walkthrough-editor-range")).toHaveAttribute(
      "data-line-range",
      "2-3",
      { timeout: 15_000 },
    );

    const before = await card.boundingBox();
    if (!before) throw new Error("walkthrough card missing before drag");
    const dragHandle = card.getByTestId("walkthrough-drag-handle");
    const handleBox = await dragHandle.boundingBox();
    if (!handleBox) throw new Error("walkthrough drag handle missing");
    await testPage.mouse.move(
      handleBox.x + handleBox.width / 2,
      handleBox.y + handleBox.height / 2,
    );
    await testPage.mouse.down();
    await testPage.mouse.move(handleBox.x - 120, handleBox.y + 80, { steps: 6 });
    await testPage.mouse.up();

    const after = await card.boundingBox();
    if (!after) throw new Error("walkthrough card missing after drag");
    expect(Math.abs(after.x - before.x)).toBeGreaterThan(40);
    expect(Math.abs(after.y - before.y)).toBeGreaterThan(30);
  });

  test("step file label is shown and opens the file", async ({ testPage, apiClient, seedData }) => {
    await seedWalkthroughTask(testPage, apiClient, seedData, "walkthrough-basic", "5-step tour");
    const card = await openWalkthrough(testPage);

    const fileLabel = card.getByTestId("walkthrough-step-file");
    await expect(fileLabel).toContainText("walkthrough_a.txt");
    await fileLabel.click();
    await expect(testPage.locator('.dv-default-tab:has-text("walkthrough_a.txt")')).toBeVisible({
      timeout: 15_000,
    });
  });

  test("ask box offers Add (queue) and Run (ask now)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedWalkthroughTask(testPage, apiClient, seedData, "walkthrough-basic", "5-step tour");
    const card = await openWalkthrough(testPage);

    await card.getByRole("textbox").fill("Why does this line exist?");
    await expect(card.getByRole("button", { name: "Add" })).toBeEnabled();
    await expect(card.getByRole("button", { name: "Run" })).toBeEnabled();
    await card.getByRole("button", { name: "Run" }).click();
    await expect(card.getByRole("textbox")).toHaveValue("");
  });

  test("closing minimizes the card but keeps the launcher; reopen restores it", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedWalkthroughTask(testPage, apiClient, seedData, "walkthrough-basic", "5-step tour");
    const card = await openWalkthrough(testPage);

    await card.getByTestId("walkthrough-close").click();
    await expect(card).toBeHidden({ timeout: 5_000 });
    // The launcher persists — the tour is not lost, just minimized.
    const launcher = testPage.getByTestId("walkthrough-launcher");
    await expect(launcher).toBeVisible();
    await launcher.click();
    await expect(testPage.getByTestId("walkthrough-floating")).toBeVisible({ timeout: 10_000 });
  });

  test("a re-emitted walkthrough is shown without a page reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // The agent emits a 2-step tour, then a different 3-step tour. Opening the
    // card refetches the latest, so the re-emit shows without reloading.
    await seedWalkthroughTask(
      testPage,
      apiClient,
      seedData,
      "walkthrough-reemit",
      "reemit-second-done",
    );
    const card = await openWalkthrough(testPage);

    await expect(card.getByTestId("walkthrough-step-header")).toContainText("Step 1 / 3");
    await expect(card.getByTestId("walkthrough-step-body")).toContainText("REEMIT_SECOND");
  });
});
