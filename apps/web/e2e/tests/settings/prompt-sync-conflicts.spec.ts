import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import {
  CONFIG_PATH,
  cleanUpPromptSync,
  configureThroughDialog,
  fillSyncDialog,
  promptRow,
  saveToReview,
  saveVersions,
  seedPromptFiles,
  syncNow,
} from "../../helpers/prompt-sync";

test.describe("Prompt Sync name conflicts", () => {
  test.afterEach(async ({ apiClient }) => {
    await cleanUpPromptSync(apiClient);
  });

  // @covers AC-INTEGRATIONS-PROMPT-SYNC-008.1 AC-INTEGRATIONS-PROMPT-SYNC-008.2 AC-INTEGRATIONS-PROMPT-SYNC-008.3 AC-INTEGRATIONS-PROMPT-SYNC-009.1 AC-INTEGRATIONS-PROMPT-SYNC-009.2 AC-INTEGRATIONS-PROMPT-SYNC-009.5 AC-INTEGRATIONS-PROMPT-SYNC-009.6
  test("reviews conflicts before saving and switches between versions without losing either", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.createPrompt("house-rules", "My house rules");
    await apiClient.createPrompt("same-rule", "Same rule");
    await seedPromptFiles(
      apiClient,
      seedData.workspaceId,
      {
        "house-rules.md": "Repository house rules",
        "fresh-rule.md": "Fresh rule",
        "same-rule.md": "Same rule",
        "repo-only.md": "Repository only",
      },
      { first: true },
    );
    await testPage.goto("/settings/prompts");

    let dialog = await fillSyncDialog(testPage, false);
    await saveToReview(testPage, dialog, false);
    // same-rule has identical text, so it is not a conflict.
    const item = dialog.getByTestId("prompt-conflict-item");
    await expect(item).toHaveCount(1);
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
    await dialog
      .getByRole("switch", { name: "Use the repository version of @house-rules" })
      .click();
    const saved = waitForHttp(testPage, "POST", CONFIG_PATH);
    await dialog.getByTestId("prompt-sync-save").click();
    expect((await saved).ok()).toBe(true);
    await expect(dialog).toHaveCount(0);
    await expect(houseRules.getByTestId("prompt-synced-badge")).toBeVisible();
    await expect(houseRules).toContainText("Repository house rules");
    await expect(
      promptRow(testPage, "fresh-rule").getByTestId("prompt-synced-badge"),
    ).toBeVisible();
    await expect(promptRow(testPage, "same-rule").getByTestId("prompt-synced-badge")).toBeVisible();

    const toggle = houseRules.getByRole("switch", {
      name: "Use the repository version of @house-rules",
    });
    await toggle.click();
    await saveVersions(testPage);
    await expect(houseRules.getByTestId("prompt-override-badge")).toBeVisible();
    await expect(houseRules).toContainText("My house rules");
    await toggle.click();
    await saveVersions(testPage);
    await expect(houseRules.getByTestId("prompt-synced-badge")).toBeVisible();
    await expect(houseRules).toContainText("Repository house rules");

    // A synced prompt overridden and then edited also has two versions.
    const freshRule = promptRow(testPage, "fresh-rule");
    await freshRule
      .getByRole("switch", { name: "Use the repository version of @fresh-rule" })
      .click();
    await saveVersions(testPage);
    await expect(freshRule.getByTestId("prompt-override-badge")).toBeVisible();
    const fresh = (await apiClient.listPrompts()).prompts.find((p) => p.name === "fresh-rule")!;
    await apiClient.updatePrompt(fresh.id, { content: "My fresh rule" });
    await expect(freshRule).toContainText("My fresh rule");

    // Removing Prompt Sync asks which version every prompt with two versions
    // keeps, starting on the version each uses now.
    await testPage.getByTestId("prompt-sync-open").click();
    dialog = testPage.getByTestId("prompt-sync-dialog");
    await dialog.getByTestId("prompt-sync-remove").click();
    const rows = dialog.getByTestId("prompt-conflict-item");
    await expect(rows).toHaveCount(2);
    const houseSwitch = dialog.getByRole("switch", {
      name: "Use the repository version of @house-rules",
    });
    const freshSwitch = dialog.getByRole("switch", {
      name: "Use the repository version of @fresh-rule",
    });
    await expect(houseSwitch).toHaveAttribute("aria-checked", "true");
    await expect(freshSwitch).toHaveAttribute("aria-checked", "false");
    await houseSwitch.click();
    await freshSwitch.click();
    // The prompt that only came from the repository can be deleted instead.
    await expect(dialog.getByTestId("prompt-removal-repository-only-names")).toHaveText(
      "@repo-only",
    );
    await dialog.getByTestId("prompt-removal-delete-repository-only").click();
    const removed = waitForHttp(testPage, "DELETE", CONFIG_PATH);
    await dialog.getByTestId("prompt-sync-removal-confirm").click();
    expect((await removed).ok()).toBe(true);
    await expect(dialog).toHaveCount(0);
    await expect(houseRules.getByTestId("prompt-edit-button")).toBeVisible();
    await expect(houseRules).toContainText("My house rules");
    await expect(freshRule).toContainText("Fresh rule");
    await expect(freshRule).not.toContainText("My fresh rule");
    await expect(promptRow(testPage, "repo-only")).toHaveCount(0);
    await expect(promptRow(testPage, "same-rule").getByTestId("prompt-edit-button")).toBeVisible();
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
