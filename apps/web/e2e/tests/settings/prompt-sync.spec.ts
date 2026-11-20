import type { Locator } from "@playwright/test";
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

function repositorySwitch(row: Locator, name: string) {
  return row.getByRole("switch", { name: `Use the repository version of @${name}` });
}

// editorBackground reads the painted background of a prompt editor.
async function editorBackground(editor: Locator) {
  return editor.evaluate((element) => {
    const surface =
      element.querySelector(".monaco-editor-background") ?? element.querySelector(".monaco-editor");
    return surface ? getComputedStyle(surface).backgroundColor : "";
  });
}

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

  // @covers AC-INTEGRATIONS-PROMPT-SYNC-004.3 AC-INTEGRATIONS-PROMPT-SYNC-007.1 AC-INTEGRATIONS-PROMPT-SYNC-007.2 AC-INTEGRATIONS-PROMPT-SYNC-007.3 AC-INTEGRATIONS-PROMPT-SYNC-007.5 AC-INTEGRATIONS-PROMPT-SYNC-009.2
  test("switches several prompts between versions as pending edits saved together", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedPromptFiles(
      apiClient,
      seedData.workspaceId,
      { "post-pr-review.md": "Shared reviewer rev 1", "principles.md": "Shared principles" },
      { first: true },
    );
    await testPage.goto("/settings/prompts");
    await configureThroughDialog(testPage, false);
    await syncNow(testPage);

    const hook = promptRow(testPage, "post-pr-review");
    const principles = promptRow(testPage, "principles");
    const hookSwitch = repositorySwitch(hook, "post-pr-review");
    const principlesSwitch = repositorySwitch(principles, "principles");
    await expect(hookSwitch).toHaveAttribute("aria-checked", "true");
    await hookSwitch.click();
    await principlesSwitch.click();
    await expect(hookSwitch).toHaveAttribute("aria-checked", "false");
    await expect(hook.getByTestId("prompt-synced-badge")).toBeVisible();
    await expect(principles.getByTestId("prompt-version-preview")).toContainText(
      "Shared principles",
    );

    // The preview is read-only on a muted background; the local editor is not.
    await hook.getByTestId("prompt-edit-button").click();
    const editor = hook.getByTestId("prompt-content-input");
    expect(await editorBackground(principles.getByTestId("prompt-version-preview"))).not.toBe(
      await editorBackground(editor),
    );
    await replacePromptEditor(testPage, editor, "My reviewer");
    await saveVersions(testPage, { count: 2 });
    await expect(hook.getByTestId("prompt-override-badge")).toBeVisible();
    await expect(principles.getByTestId("prompt-override-badge")).toBeVisible();
    await expect(hook).toContainText("My reviewer");

    await seedPromptFiles(apiClient, seedData.workspaceId, {
      "post-pr-review.md": "Shared reviewer rev 2",
      "principles.md": "Shared principles",
    });
    await syncNow(testPage);
    await expect(hook).toContainText("My reviewer");
    await expect(testPage.getByTestId("prompt-sync-overridden")).toContainText("@post-pr-review");
    await expect(testPage.getByTestId("prompt-sync-warnings")).toHaveCount(0);

    await hookSwitch.click();
    await expect(hook.getByTestId("prompt-version-preview")).toContainText("rev 2");
    await saveVersions(testPage);
    await expect(hook.getByTestId("prompt-synced-badge")).toBeVisible();
    await expect(hook).toContainText("rev 2");

    await hookSwitch.click();
    await expect(hook.getByTestId("prompt-version-preview")).toContainText("My reviewer");
    await repositorySwitch(principles, "principles").click();
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Reset" })
      .click();
    await expect(hookSwitch).toHaveAttribute("aria-checked", "true");
    await expect(repositorySwitch(principles, "principles")).toHaveAttribute(
      "aria-checked",
      "false",
    );
    await expect(testPage.getByTestId("prompt-version-preview")).toHaveCount(0);
  });
});
