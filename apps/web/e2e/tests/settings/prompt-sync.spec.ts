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
    await principles.getByTestId("prompt-view-button").click();
    await expect(principles.getByTestId("prompt-view-content")).toContainText("rev 1");
    await expect(principles.getByTestId("prompt-view-source-note")).toContainText(
      ".kandev/prompts/principles.md",
    );
    await principles.getByTestId("prompt-view-button").click();
    await expect(principles.getByTestId("prompt-view-content")).toHaveCount(0);

    // Synced state and ownership survive a reload (hydration, not local state).
    await testPage.reload();
    await expect(principles.getByTestId("prompt-synced-badge")).toBeVisible();
    await expect(principles.getByTestId("prompt-edit-button")).toHaveCount(0);
    await expect(testPage.getByTestId("prompt-sync-status")).toBeVisible();

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
    await testPage.reload();
    await expect(principles.getByTestId("prompt-synced-badge")).toHaveCount(0);
    await expect(principles.getByTestId("prompt-edit-button")).toBeVisible();
    await expect(testPage.getByTestId("prompt-sync-status")).toHaveCount(0);
    await apiClient.deleteWorkflow(workflow.id);
  });

  // @covers AC-INTEGRATIONS-PROMPT-SYNC-007.1 AC-INTEGRATIONS-PROMPT-SYNC-007.2 AC-INTEGRATIONS-PROMPT-SYNC-007.3 AC-INTEGRATIONS-PROMPT-SYNC-007.5
  test("overrides a synced prompt locally and reverts to the repository version", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedPromptFiles(
      apiClient,
      seedData.workspaceId,
      { "post-pr-review.md": "Shared reviewer rev 1" },
      { first: true },
    );
    await testPage.goto("/settings/prompts");
    await configureThroughDialog(testPage, false);
    await syncNow(testPage);

    const hook = promptRow(testPage, "post-pr-review");
    await hook.getByTestId("prompt-override-button").click();
    await expect(hook.getByTestId("prompt-override-badge")).toBeVisible();
    await expect(hook.getByTestId("prompt-edit-button")).toBeVisible();
    await expect(hook.getByTestId("prompt-synced-badge")).toHaveCount(0);

    await seedPromptFiles(apiClient, seedData.workspaceId, {
      "post-pr-review.md": "Shared reviewer rev 2",
    });
    await syncNow(testPage);
    await expect(hook).toContainText("rev 1");
    await expect(testPage.getByTestId("prompt-sync-overridden")).toContainText("@post-pr-review");
    await expect(testPage.getByTestId("prompt-sync-warnings")).toHaveCount(0);

    const reverted = waitForHttp(
      testPage,
      "POST",
      /^\/api\/v1\/prompt-sync\/prompts\/[^/]+\/revert$/,
    );
    await hook.getByTestId("prompt-revert-button").click();
    expect((await reverted).ok()).toBe(true);
    await expect(hook.getByTestId("prompt-synced-badge")).toBeVisible();
    await expect(hook).toContainText("rev 2");
    await expect(testPage.getByTestId("prompt-sync-overridden")).toHaveCount(0);
  });
});
