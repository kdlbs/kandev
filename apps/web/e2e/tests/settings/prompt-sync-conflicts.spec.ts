import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import {
  CONFIG_PATH,
  cleanUpPromptSync,
  configureThroughDialog,
  fillSyncDialog,
  promptRow,
  saveToReview,
  seedPromptFiles,
  syncNow,
} from "../../helpers/prompt-sync";

test.describe("Prompt Sync name conflicts", () => {
  test.afterEach(async ({ apiClient }) => {
    await cleanUpPromptSync(apiClient);
  });

  // @covers AC-INTEGRATIONS-PROMPT-SYNC-008.1 AC-INTEGRATIONS-PROMPT-SYNC-008.2 AC-INTEGRATIONS-PROMPT-SYNC-008.3 AC-INTEGRATIONS-PROMPT-SYNC-009.1 AC-INTEGRATIONS-PROMPT-SYNC-009.2 AC-INTEGRATIONS-PROMPT-SYNC-009.5
  test("reviews conflicts before saving and switches between versions without losing either", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.createPrompt("house-rules", "My house rules");
    await seedPromptFiles(
      apiClient,
      seedData.workspaceId,
      { "house-rules.md": "Repository house rules", "fresh-rule.md": "Fresh rule" },
      { first: true },
    );
    await testPage.goto("/settings/prompts");

    let dialog = await fillSyncDialog(testPage, false);
    await saveToReview(testPage, dialog, false);
    const item = dialog.getByTestId("prompt-conflict-item");
    await expect(item).toHaveAttribute("data-prompt-name", "house-rules");
    await item.getByTestId("prompt-conflict-compare").click();
    await expect(item.getByTestId("prompt-conflict-local-content")).toHaveText("My house rules");
    await expect(item.getByTestId("prompt-conflict-repository-content")).toHaveText(
      "Repository house rules",
    );
    await testPage.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);
    await expect(testPage.getByTestId("prompt-sync-status")).toHaveCount(0);
    const houseRules = promptRow(testPage, "house-rules");
    await expect(houseRules.getByTestId("prompt-override-switch")).toHaveCount(0);
    await expect(promptRow(testPage, "fresh-rule")).toHaveCount(0);

    dialog = await fillSyncDialog(testPage, false);
    await saveToReview(testPage, dialog, false);
    await dialog.getByTestId("prompt-conflict-choice-repository").click();
    const saved = waitForHttp(testPage, "POST", CONFIG_PATH);
    await dialog.getByTestId("prompt-sync-save").click();
    expect((await saved).ok()).toBe(true);
    await expect(dialog).toHaveCount(0);
    await expect(houseRules.getByTestId("prompt-synced-badge")).toBeVisible();
    await expect(houseRules).toContainText("Repository house rules");
    await expect(
      promptRow(testPage, "fresh-rule").getByTestId("prompt-synced-badge"),
    ).toBeVisible();

    const toggle = houseRules.getByRole("switch", { name: "Local override" });
    await toggle.click();
    await expect(houseRules.getByTestId("prompt-override-badge")).toBeVisible();
    await expect(houseRules).toContainText("My house rules");
    await toggle.click();
    await houseRules.getByTestId("prompt-revert-confirm").click();
    await expect(houseRules.getByTestId("prompt-synced-badge")).toBeVisible();
    await expect(houseRules).toContainText("Repository house rules");

    // Removing Prompt Sync asks which version a prompt with a kept local
    // version keeps.
    await testPage.getByTestId("prompt-sync-open").click();
    dialog = testPage.getByTestId("prompt-sync-dialog");
    await dialog.getByTestId("prompt-sync-remove").click();
    await expect(dialog.getByTestId("prompt-conflict-item")).toHaveAttribute(
      "data-prompt-name",
      "house-rules",
    );
    await dialog.getByTestId("prompt-conflict-choice-local").click();
    const removed = waitForHttp(testPage, "DELETE", CONFIG_PATH);
    await dialog.getByTestId("prompt-sync-removal-confirm").click();
    expect((await removed).ok()).toBe(true);
    await expect(dialog).toHaveCount(0);
    await expect(houseRules.getByTestId("prompt-edit-button")).toBeVisible();
    await expect(houseRules).toContainText("My house rules");
    await expect(promptRow(testPage, "fresh-rule")).toContainText("Fresh rule");
  });

  // @covers AC-INTEGRATIONS-PROMPT-SYNC-008.4 AC-INTEGRATIONS-PROMPT-SYNC-008.5 AC-INTEGRATIONS-PROMPT-SYNC-008.6
  test("flags a conflict found by a later sync until the user resolves it", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedPromptFiles(
      apiClient,
      seedData.workspaceId,
      { "first-rule.md": "First" },
      { first: true },
    );
    await testPage.goto("/settings/prompts");
    await configureThroughDialog(testPage, false);
    await expect(
      promptRow(testPage, "first-rule").getByTestId("prompt-synced-badge"),
    ).toBeVisible();

    await apiClient.createPrompt("late-rule", "My late rule");
    await seedPromptFiles(apiClient, seedData.workspaceId, {
      "first-rule.md": "First",
      "late-rule.md": "Repository late rule",
    });
    await syncNow(testPage);

    const lateRule = promptRow(testPage, "late-rule");
    await expect(lateRule.getByTestId("prompt-conflict-badge")).toBeVisible();
    await expect(lateRule).toContainText("My late rule");
    await expect(testPage.getByTestId("prompt-conflict-banner")).toBeVisible();
    await expect(testPage.getByTestId("prompt-conflict-indicator").first()).toBeVisible();

    await testPage.getByTestId("prompt-conflict-banner-review").click();
    const review = testPage.getByTestId("prompt-conflict-dialog");
    await expect(review.getByTestId("prompt-conflict-item")).toHaveAttribute(
      "data-prompt-name",
      "late-rule",
    );
    await review.getByTestId("prompt-conflict-apply").click();
    await expect(review).toHaveCount(0);
    await expect(testPage.getByTestId("prompt-conflict-banner")).toHaveCount(0);
    await expect(lateRule.getByTestId("prompt-conflict-badge")).toHaveCount(0);
    await expect(lateRule.getByTestId("prompt-override-badge")).toBeVisible();
    await expect(lateRule).toContainText("My late rule");
    await expect(testPage.getByTestId("prompt-conflict-indicator")).toHaveCount(0);
  });
});
