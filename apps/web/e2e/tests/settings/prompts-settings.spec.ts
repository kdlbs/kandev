import { test, expect } from "../../fixtures/test-base";
import {
  acceptPromptCompletion,
  promptEditorText,
  replacePromptEditor,
  waitForPromptInStore,
} from "../../helpers/settings-prompt-editor";

// Custom prompts share a UNIQUE name index with built-in prompts. Prior to the
// 409-mapping fix, posting a duplicate name returned 500 and the frontend
// silently swallowed the error. These tests guard the toast contract so a
// regression cannot ship unnoticed.
test.describe("Prompts settings — duplicate name handling", () => {
  test.afterEach(async ({ apiClient }) => {
    const { prompts } = await apiClient.listPrompts();
    for (const p of prompts) {
      if (!p.builtin) {
        await apiClient.deletePrompt(p.id).catch(() => undefined);
      }
    }
  });

  test("creating a prompt with a duplicate name shows an error toast and keeps the form open", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);

    await apiClient.createPrompt("dupe-prompt", "first content");

    await testPage.goto("/settings/prompts");
    await testPage.getByTestId("prompt-create-button").click();

    const form = testPage.getByTestId("prompt-create-form");
    await expect(form).toBeVisible();
    await form.getByTestId("prompt-name-input").fill("dupe-prompt");
    await replacePromptEditor(testPage, form.getByTestId("prompt-content-input"), "second content");
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .click();

    const toast = testPage.getByTestId("toast-message");
    await expect(toast).toBeVisible({ timeout: 5_000 });
    await expect(toast).toContainText(/already exists/i);

    // Form must remain open (no silent reset) so the user can fix the name.
    await expect(form).toBeVisible();
    await expect(form.getByTestId("prompt-name-input")).toHaveValue("dupe-prompt");
  });

  test("renaming a prompt to an existing name shows an error toast and does not persist the rename", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);

    await apiClient.createPrompt("alpha", "a");
    await apiClient.createPrompt("beta", "b");

    await testPage.goto("/settings/prompts");

    const betaRow = testPage.locator('[data-testid="prompt-list-item"][data-prompt-name="beta"]');
    await expect(betaRow).toBeVisible();
    await betaRow.getByTestId("prompt-edit-button").click();

    const nameInput = betaRow.getByTestId("prompt-name-input");
    await nameInput.fill("alpha");
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .click();

    const toast = testPage.getByTestId("toast-message");
    await expect(toast).toBeVisible({ timeout: 5_000 });
    await expect(toast).toContainText(/already exists/i);

    // Backend rejected. Cancel the dirty draft and confirm the original row
    // remains visible without forcing a guarded page reload.
    await betaRow.getByRole("button", { name: "Cancel" }).click();
    await expect(
      testPage.locator('[data-testid="prompt-list-item"][data-prompt-name="beta"]'),
    ).toBeVisible();
  });

  test("cancelling a prompt delete keeps the local editor draft", async ({
    testPage,
    apiClient,
  }) => {
    await apiClient.createPrompt("cancel-delete-prompt", "original content");

    await testPage.goto("/settings/prompts");

    const row = testPage.locator(
      '[data-testid="prompt-list-item"][data-prompt-name="cancel-delete-prompt"]',
    );
    await row.getByTestId("prompt-edit-button").click();
    const editor = row.getByTestId("prompt-content-input");
    await replacePromptEditor(testPage, editor, "unsaved editor draft");
    await row.getByTestId("prompt-delete-button").click();

    const confirmation = testPage.getByTestId("prompt-delete-confirm-popover");
    await expect(confirmation).toBeVisible();
    await expect(testPage.getByRole("alertdialog")).toHaveCount(0);
    await confirmation.getByRole("button", { name: "Cancel" }).click();

    await expect(promptEditorText(editor)).toContainText("unsaved editor draft");
  });

  test("deleting one prompt confirms in its row and removes only that prompt", async ({
    testPage,
    apiClient,
  }) => {
    await apiClient.createPrompt("delete-prompt", "delete me");
    await apiClient.createPrompt("keep-prompt", "keep me");

    await testPage.goto("/settings/prompts");

    const row = testPage.locator(
      '[data-testid="prompt-list-item"][data-prompt-name="delete-prompt"]',
    );
    const keptRow = testPage.locator(
      '[data-testid="prompt-list-item"][data-prompt-name="keep-prompt"]',
    );
    await row.getByTestId("prompt-delete-button").click();

    const confirmation = testPage.getByTestId("prompt-delete-confirm-popover");
    await expect(confirmation).toBeVisible();
    await confirmation.getByTestId("prompt-delete-confirm").click();

    await expect(confirmation).toHaveCount(0);
    await expect(row).toHaveCount(0);
    await expect(keptRow).toBeVisible();
  });

  test("creating a prompt with a unique name succeeds and appears in the list", async ({
    testPage,
  }) => {
    test.setTimeout(60_000);

    await testPage.goto("/settings/prompts");
    await testPage.getByTestId("prompt-create-button").click();

    const form = testPage.getByTestId("prompt-create-form");
    await form.getByTestId("prompt-name-input").fill("e2e-fresh-prompt");
    await replacePromptEditor(testPage, form.getByTestId("prompt-content-input"), "hello world");
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .click();

    await expect(
      testPage.locator('[data-testid="prompt-list-item"][data-prompt-name="e2e-fresh-prompt"]'),
    ).toBeVisible({ timeout: 10_000 });
    // Form should be reset / closed on success.
    await expect(testPage.getByTestId("prompt-create-form")).toHaveCount(0);
  });

  test("does not suggest the prompt currently being edited", async ({ testPage, apiClient }) => {
    test.setTimeout(60_000);

    const current = await apiClient.createPrompt("e2e-current-prompt", "Current prompt content");
    const other = await apiClient.createPrompt("e2e-other-prompt", "Reusable prompt content");

    try {
      await testPage.goto("/settings/prompts");
      await waitForPromptInStore(testPage, other.name);
      const row = testPage.locator(
        '[data-testid="prompt-list-item"][data-prompt-name="e2e-current-prompt"]',
      );
      await expect(row).toBeVisible();
      await row.getByTestId("prompt-edit-button").click();

      const editor = row.getByTestId("prompt-content-input");
      await replacePromptEditor(testPage, editor, "Use ");
      await testPage.keyboard.type("@");

      const suggestWidget = testPage.locator(".suggest-widget:visible").last();
      await expect(suggestWidget).toBeVisible({ timeout: 5_000 });
      await expect(suggestWidget).toContainText(other.name);
      await expect(suggestWidget).not.toContainText(current.name);

      await acceptPromptCompletion(testPage, other.name);
      await expect(promptEditorText(editor)).toContainText(`@${other.name}`);
    } finally {
      await apiClient.deletePrompt(current.id).catch(() => undefined);
      await apiClient.deletePrompt(other.id).catch(() => undefined);
    }
  });
});
