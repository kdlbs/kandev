import { test, expect } from "../../fixtures/test-base";

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
    await form.getByTestId("prompt-content-input").fill("second content");
    await form.getByTestId("prompt-submit").click();

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
    await betaRow.getByTestId("prompt-submit").click();

    const toast = testPage.getByTestId("toast-message");
    await expect(toast).toBeVisible({ timeout: 5_000 });
    await expect(toast).toContainText(/already exists/i);

    // Backend rejected — the row must still exist under its original name.
    await testPage.reload();
    await expect(
      testPage.locator('[data-testid="prompt-list-item"][data-prompt-name="beta"]'),
    ).toBeVisible();
  });

  test("creating a prompt with a unique name succeeds and appears in the list", async ({
    testPage,
  }) => {
    test.setTimeout(60_000);

    await testPage.goto("/settings/prompts");
    await testPage.getByTestId("prompt-create-button").click();

    const form = testPage.getByTestId("prompt-create-form");
    await form.getByTestId("prompt-name-input").fill("e2e-fresh-prompt");
    await form.getByTestId("prompt-content-input").fill("hello world");
    await form.getByTestId("prompt-submit").click();

    await expect(
      testPage.locator('[data-testid="prompt-list-item"][data-prompt-name="e2e-fresh-prompt"]'),
    ).toBeVisible({ timeout: 10_000 });
    // Form should be reset / closed on success.
    await expect(testPage.getByTestId("prompt-create-form")).toHaveCount(0);
  });
});
