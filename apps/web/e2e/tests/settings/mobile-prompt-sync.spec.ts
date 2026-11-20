import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import {
  CONFIG_PATH,
  cleanUpPromptSync,
  configureThroughDialog,
  fillSyncDialog,
  promptRow,
  saveVersions,
  saveToReview,
  seedPromptFiles,
  SYNC_PATH,
} from "../../helpers/prompt-sync";

test.describe("Prompt Sync on a phone", () => {
  test.afterEach(async ({ apiClient }) => {
    await cleanUpPromptSync(apiClient);
  });

  test("configures and syncs on a phone", async ({ testPage, apiClient, seedData }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await seedPromptFiles(
      apiClient,
      seedData.workspaceId,
      { "phone-rule.md": "Tap targets matter." },
      { first: true },
    );
    await testPage.goto("/settings/prompts");
    await configureThroughDialog(testPage, true);
    const response = waitForHttp(testPage, "POST", SYNC_PATH);
    await testPage.getByTestId("prompt-sync-now").tap();
    expect((await response).ok()).toBe(true);
    const row = promptRow(testPage, "phone-rule");
    await expect(row.getByTestId("prompt-synced-badge")).toBeVisible();
    await expect(row.getByTestId("prompt-delete-button")).toHaveCount(0);
    const view = row.getByTestId("prompt-view-button");
    const box = await view.boundingBox();
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
    await view.tap();
    await expect(row.getByTestId("prompt-view-content")).toContainText("Tap targets matter.");
    await testPage.reload();
    await expect(row.getByTestId("prompt-synced-badge")).toBeVisible();
    await expect(row.getByTestId("prompt-delete-button")).toHaveCount(0);

    const switchLabel = row.getByTestId("prompt-override-switch-label");
    expect((await switchLabel.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
    await switchLabel.tap();
    await expect(row.getByTestId("prompt-unsaved-badge")).toBeVisible();
    await expect(row.getByTestId("prompt-content-input")).toHaveCount(0);
    const edit = row.getByTestId("prompt-edit-button");
    expect((await edit.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
    await edit.tap();
    await expect(row.getByTestId("prompt-content-input")).toBeVisible();
    await saveVersions(testPage, { tap: true });
    await expect(row.getByTestId("prompt-override-badge")).toBeVisible();
  });

  // @covers AC-INTEGRATIONS-PROMPT-SYNC-008.2 AC-INTEGRATIONS-PROMPT-SYNC-008.3 AC-INTEGRATIONS-PROMPT-SYNC-009.5
  test("reviews name conflicts with touch-sized choices on a phone", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await apiClient.createPrompt("phone-house", "My phone rules");
    await seedPromptFiles(
      apiClient,
      seedData.workspaceId,
      { "phone-house.md": "Repository phone rules" },
      { first: true },
    );
    await testPage.goto("/settings/prompts");
    const dialog = await fillSyncDialog(testPage, true);
    await saveToReview(testPage, dialog, true);
    await dialog.getByTestId("prompt-sync-review-back").tap();
    await expect(dialog.getByTestId("prompt-sync-url-input")).toHaveValue("platform/prompts");
    await saveToReview(testPage, dialog, true);

    const choice = dialog.locator("label", { has: testPage.getByTestId("prompt-conflict-switch") });
    expect((await choice.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
    await choice.tap();
    const saved = waitForHttp(testPage, "POST", CONFIG_PATH);
    await dialog.getByTestId("prompt-sync-save").tap();
    expect((await saved).ok()).toBe(true);
    await expect(dialog).toHaveCount(0);
    const row = promptRow(testPage, "phone-house");
    await expect(row.getByTestId("prompt-synced-badge")).toBeVisible();

    await testPage.getByTestId("prompt-sync-open").tap();
    await dialog.getByTestId("prompt-sync-remove").tap();
    const keepMine = dialog.getByRole("switch", {
      name: "Use the repository version of @phone-house",
    });
    await expect(keepMine).toHaveAttribute("aria-checked", "true");
    await dialog
      .locator("label", {
        has: testPage.getByRole("switch", { name: "Use the repository version of @phone-house" }),
      })
      .tap();
    await expect(keepMine).toHaveAttribute("aria-checked", "false");
    const removed = waitForHttp(testPage, "DELETE", CONFIG_PATH);
    await dialog.getByTestId("prompt-sync-removal-confirm").tap();
    expect((await removed).ok()).toBe(true);
    await expect(row).toContainText("My phone rules");
  });
});
