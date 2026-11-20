import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { replacePromptEditor } from "../../helpers/settings-prompt-editor";
import {
  cleanUpPromptSync,
  configureThroughDialog,
  promptRow,
  saveVersions,
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
    // Prompts that only came from the repository are kept unless the user
    // asks to delete them.
    await expect(dialog.getByTestId("prompt-removal-repository-only-names")).toHaveText(
      "@principles, @review-rules",
    );
    await expect(dialog.getByTestId("prompt-removal-delete-repository-only")).toHaveAttribute(
      "aria-checked",
      "false",
    );
    const removed = waitForHttp(testPage, "DELETE", /^\/api\/v1\/prompt-sync\/config$/);
    await dialog.getByTestId("prompt-sync-removal-confirm").click();
    expect((await removed).ok()).toBe(true);
    await expect(principles.getByTestId("prompt-synced-badge")).toHaveCount(0);
    await expect(principles.getByTestId("prompt-edit-button")).toBeVisible();
    await testPage.reload();
    await expect(principles.getByTestId("prompt-synced-badge")).toHaveCount(0);
    await expect(principles.getByTestId("prompt-edit-button")).toBeVisible();
    await expect(testPage.getByTestId("prompt-sync-status")).toHaveCount(0);
    await apiClient.deleteWorkflow(workflow.id);
  });

  // @covers AC-INTEGRATIONS-PROMPT-SYNC-007.1 AC-INTEGRATIONS-PROMPT-SYNC-007.2 AC-INTEGRATIONS-PROMPT-SYNC-007.3 AC-INTEGRATIONS-PROMPT-SYNC-007.5 AC-INTEGRATIONS-PROMPT-SYNC-009.2
  test("toggles the Local override switch as a pending edit saved from the save bar", async ({
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
    const toggle = hook.getByRole("switch", { name: "Local override" });
    const saveBar = testPage.getByTestId("settings-floating-save");
    await expect(toggle).toHaveAttribute("aria-checked", "false");
    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-checked", "true");
    await expect(hook.getByTestId("prompt-synced-badge")).toBeVisible();
    await replacePromptEditor(testPage, hook.getByTestId("prompt-content-input"), "My reviewer");
    await saveVersions(testPage);
    await expect(hook.getByTestId("prompt-override-badge")).toBeVisible();
    await expect(hook.getByTestId("prompt-synced-badge")).toHaveCount(0);
    await expect(hook).toContainText("My reviewer");

    await seedPromptFiles(apiClient, seedData.workspaceId, {
      "post-pr-review.md": "Shared reviewer rev 2",
    });
    await syncNow(testPage);
    await expect(hook).toContainText("My reviewer");
    await expect(testPage.getByTestId("prompt-sync-overridden")).toContainText("@post-pr-review");
    await expect(testPage.getByTestId("prompt-sync-warnings")).toHaveCount(0);

    await toggle.click();
    await expect(hook.getByTestId("prompt-content-input")).toContainText("rev 2");
    await saveVersions(testPage);
    await expect(hook.getByTestId("prompt-synced-badge")).toBeVisible();
    await expect(hook).toContainText("rev 2");
    await expect(testPage.getByTestId("prompt-sync-overridden")).toHaveCount(0);

    await hook.getByRole("switch", { name: "Local override" }).click();
    await expect(hook.getByTestId("prompt-content-input")).toContainText("My reviewer");
    await saveBar.getByRole("button", { name: "Reset" }).click();
    await expect(hook.getByRole("switch", { name: "Local override" })).toHaveAttribute(
      "aria-checked",
      "false",
    );
    await expect(hook.getByTestId("prompt-synced-badge")).toBeVisible();
  });
});
