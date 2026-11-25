import type { Locator } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { replacePromptEditor } from "../../helpers/settings-prompt-editor";
import {
  CONFIG_PATH,
  blockNextPromptSyncWithQuota,
  cleanUpPromptSync,
  configureThroughDialog,
  promptRow,
  saveVersions,
  seedPromptFiles,
  SYNC_PATH,
  syncNow,
} from "../../helpers/prompt-sync";

function repositorySwitch(row: Locator, name: string) {
  return row.getByRole("switch", { name: `Use the repository version of @${name}` });
}

// editorColors reads the painted background and text color of a prompt editor.
async function editorColors(editor: Locator) {
  return editor.evaluate((element) => {
    const surface =
      element.querySelector(".monaco-editor-background") ?? element.querySelector(".monaco-editor");
    const text =
      element.querySelector(".view-line span span") ?? element.querySelector(".view-line");
    return {
      background: surface ? getComputedStyle(surface).backgroundColor : "",
      text: text ? getComputedStyle(text).color : "",
    };
  });
}

function luminance(color: string) {
  const [r, g, b] = (color.match(/\d+(\.\d+)?/g) ?? []).slice(0, 3).map((value) => {
    const channel = Number(value) / 255;
    return channel <= 0.03928 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * (r ?? 0) + 0.7152 * (g ?? 0) + 0.0722 * (b ?? 0);
}

// contrastRatio is the WCAG contrast ratio of two rgb() colors.
function contrastRatio(a: string, b: string) {
  const [light, dark] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (light! + 0.05) / (dark! + 0.05);
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
  test("switches and edits several prompts before one Save, keeping each row open or closed", async ({
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

    // Switching never opens a row; each switched row is marked unsaved.
    await hookSwitch.click();
    await principlesSwitch.click();
    await expect(hookSwitch).toHaveAttribute("aria-checked", "false");
    await expect(hook.getByTestId("prompt-synced-badge")).toBeVisible();
    await expect(hook.getByTestId("prompt-content-input")).toHaveCount(0);
    await expect(hook.getByTestId("prompt-unsaved-badge")).toBeVisible();
    await expect(principles.getByTestId("prompt-unsaved-badge")).toBeVisible();

    // Edit opens each Local row editable at once; both edits save together.
    await hook.getByTestId("prompt-edit-button").click();
    await replacePromptEditor(testPage, hook.getByTestId("prompt-content-input"), "My reviewer");
    await principles.getByTestId("prompt-edit-button").click();
    await replacePromptEditor(
      testPage,
      principles.getByTestId("prompt-content-input"),
      "My principles",
    );
    await saveVersions(testPage, { count: 2 });
    await expect(hook.getByTestId("prompt-override-badge")).toBeVisible();
    await expect(principles.getByTestId("prompt-override-badge")).toBeVisible();
    await expect(hook).toContainText("My reviewer");
    await expect(principles).toContainText("My principles");
    await expect(testPage.getByTestId("prompt-unsaved-badge")).toHaveCount(0);

    await seedPromptFiles(apiClient, seedData.workspaceId, {
      "post-pr-review.md": "Shared reviewer rev 2",
      "principles.md": "Shared principles",
    });
    await syncNow(testPage);
    await expect(hook).toContainText("My reviewer");
    await expect(testPage.getByTestId("prompt-sync-overridden")).toContainText("@post-pr-review");
    await expect(testPage.getByTestId("prompt-sync-warnings")).toHaveCount(0);

    // An open row stays open when switched, now showing the repository version read-only.
    await hook.getByTestId("prompt-edit-button").click();
    await expect(hook.getByTestId("prompt-content-input")).toContainText("My reviewer");
    await hookSwitch.click();
    const view = hook.getByTestId("prompt-view-content");
    await expect(view).toContainText("rev 2");
    await expect(hook.getByTestId("prompt-view-button")).toHaveAttribute("aria-expanded", "true");

    // Read-only text keeps a strong contrast on a surface unlike the editable one.
    await principles.getByTestId("prompt-edit-button").click();
    const readOnly = await editorColors(view);
    const editable = await editorColors(principles.getByTestId("prompt-content-input"));
    expect(readOnly.background).not.toBe(editable.background);
    expect(contrastRatio(readOnly.text, readOnly.background)).toBeGreaterThan(4.5);

    await saveVersions(testPage);
    await expect(hook.getByTestId("prompt-synced-badge")).toBeVisible();
    await expect(hook).toContainText("rev 2");

    await hookSwitch.click();
    await expect(hook.getByTestId("prompt-content-input")).toHaveCount(0);
    await principlesSwitch.click();
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Reset" })
      .click();
    await expect(hookSwitch).toHaveAttribute("aria-checked", "true");
    await expect(principlesSwitch).toHaveAttribute("aria-checked", "false");
    await expect(testPage.getByTestId("prompt-unsaved-badge")).toHaveCount(0);
  });

  test("reports a GitHub quota block and retries on Sync now", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedPromptFiles(
      apiClient,
      seedData.workspaceId,
      { "quota-rule.md": "Quota retry." },
      { first: true },
    );
    await testPage.goto("/settings/prompts");
    await configureThroughDialog(testPage, false);
    await blockNextPromptSyncWithQuota(testPage, apiClient);

    const blocked = waitForHttp(testPage, "POST", SYNC_PATH);
    await testPage.getByTestId("prompt-sync-now").click();
    expect((await blocked).ok()).toBe(true);
    const status = testPage.getByTestId("prompt-sync-status");
    await expect(status).toHaveAttribute("data-state", "failed");
    await expect(status).toContainText(
      "GitHub quota is unavailable. Try again after quota resets.",
    );

    const retried = waitForHttp(testPage, "POST", SYNC_PATH);
    await testPage.getByTestId("prompt-sync-now").click();
    expect((await retried).ok()).toBe(true);
    await expect(status).toHaveAttribute("data-state", "ok");
    await expect(status).not.toContainText("GitHub quota");
  });

  test("restores an override file after removal", async ({ testPage, apiClient, seedData }) => {
    await seedPromptFiles(
      apiClient,
      seedData.workspaceId,
      { "recovery-rule.md": "Repository version one." },
      { first: true },
    );
    await testPage.goto("/settings/prompts");
    await configureThroughDialog(testPage, false);
    const row = promptRow(testPage, "recovery-rule");
    const versionSwitch = repositorySwitch(row, "recovery-rule");
    await versionSwitch.click();
    await row.getByTestId("prompt-edit-button").click();
    await replacePromptEditor(
      testPage,
      row.getByTestId("prompt-content-input"),
      "My local override.",
    );
    await saveVersions(testPage);
    await expect(row.getByTestId("prompt-override-badge")).toBeVisible();

    await seedPromptFiles(apiClient, seedData.workspaceId, {});
    await syncNow(testPage);
    await expect(row.getByTestId("prompt-repository-removed")).toHaveText(
      "Removed from repository",
    );
    await expect(versionSwitch).toHaveCount(0);
    await testPage.reload();
    await expect(row.getByTestId("prompt-repository-removed")).toHaveText(
      "Removed from repository",
    );
    await expect(row.getByTestId("prompt-override-badge")).toBeVisible();
    await expect(versionSwitch).toHaveCount(0);
    let savedPrompts = await apiClient.listPrompts();
    expect(
      savedPrompts.prompts.find((prompt) => prompt.name === "recovery-rule")?.source_path,
    ).toBe("");
    await expect(row).toContainText("My local override.");
    await row.getByTestId("prompt-edit-button").click();
    await replacePromptEditor(
      testPage,
      row.getByTestId("prompt-content-input"),
      "My local override after removal.",
    );
    await saveVersions(testPage);

    const recoveredDirectory = ".kandev/recovered-prompts";
    await seedPromptFiles(
      apiClient,
      seedData.workspaceId,
      { "recovery-rule.md": "Repository version two." },
      { path: recoveredDirectory },
    );
    await testPage.getByTestId("prompt-sync-open").click();
    const dialog = testPage.getByTestId("prompt-sync-dialog");
    await dialog.getByTestId("prompt-sync-directory-input").fill(recoveredDirectory);
    const configSaved = waitForHttp(testPage, "POST", CONFIG_PATH);
    await dialog.getByTestId("prompt-sync-save").click();
    expect((await configSaved).ok()).toBe(true);
    await expect(dialog).toHaveCount(0);
    await testPage.reload();
    savedPrompts = await apiClient.listPrompts();
    expect(
      savedPrompts.prompts.find((prompt) => prompt.name === "recovery-rule")?.source_path,
    ).toBe(`${recoveredDirectory}/recovery-rule.md`);
    await expect(versionSwitch).toHaveAttribute("aria-checked", "false");
    await expect(row.getByTestId("prompt-override-badge")).toBeVisible();
    await expect(row).toContainText("My local override after removal.");
    await versionSwitch.click();
    await saveVersions(testPage);
    await row.getByTestId("prompt-view-button").click();
    await expect(row.getByTestId("prompt-view-content")).toContainText("Repository version two.");
  });
});
