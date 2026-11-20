import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import {
  cleanUpPromptSync,
  configureThroughDialog,
  promptRow,
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
    await expect(row.getByTestId("prompt-override-badge")).toBeVisible();
    await row.getByTestId("prompt-override-switch-label").tap();
    const reverted = waitForHttp(
      testPage,
      "POST",
      /^\/api\/v1\/prompt-sync\/prompts\/[^/]+\/revert$/,
    );
    await testPage.getByTestId("prompt-revert-confirm").tap();
    expect((await reverted).ok()).toBe(true);
    await expect(row.getByTestId("prompt-synced-badge")).toBeVisible();
  });
});
