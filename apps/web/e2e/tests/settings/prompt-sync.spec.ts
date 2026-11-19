import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import {
  cleanUpPromptSync,
  configureThroughDialog,
  promptRow,
  seedPromptFiles,
  syncNow,
} from "../../helpers/prompt-sync";

test.describe("Prompt Sync", () => {
  test.afterEach(async ({ apiClient }) => {
    await cleanUpPromptSync(apiClient);
  });

  test("syncs prompts read-only, keeps referenced removals, and releases on removal", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedPromptFiles(
      apiClient,
      seedData.workspaceId,
      {
        "principles.md": "Prefer the simplest design. rev 1",
        "review-rules.md": "Check every edge case.",
      },
      { first: true },
    );
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Prompt sync refs");
    const step = await apiClient.createWorkflowStep(workflow.id, "Review", 0, {
      is_start_step: true,
    });
    await apiClient.updateWorkflowStep(step.id, { prompt: "Apply @review-rules now" });

    await testPage.goto("/settings/prompts");
    await configureThroughDialog(testPage, false);
    await syncNow(testPage);

    const principles = promptRow(testPage, "principles");
    await expect(principles.getByTestId("prompt-synced-badge")).toBeVisible();
    await expect(principles.getByTestId("prompt-edit-button")).toHaveCount(0);
    await expect(principles.getByTestId("prompt-delete-button")).toHaveCount(0);

    await seedPromptFiles(apiClient, seedData.workspaceId, {
      "principles.md": "Prefer the simplest design. rev 2",
    });
    await syncNow(testPage);
    await expect(principles).toContainText("rev 2");
    await expect(testPage.getByTestId("prompt-sync-warnings")).toContainText("review-rules");
    await expect(promptRow(testPage, "review-rules")).toBeVisible();

    await testPage.getByTestId("prompt-sync-open").click();
    const dialog = testPage.getByTestId("prompt-sync-dialog");
    await dialog.getByTestId("prompt-sync-remove").click();
    const removed = waitForHttp(testPage, "DELETE", /^\/api\/v1\/prompt-sync\/config$/);
    await testPage.getByTestId("prompt-sync-remove-confirm").click();
    expect((await removed).ok()).toBe(true);
    await expect(principles.getByTestId("prompt-synced-badge")).toHaveCount(0);
    await expect(principles.getByTestId("prompt-edit-button")).toBeVisible();
    await apiClient.deleteWorkflow(workflow.id);
  });
});
