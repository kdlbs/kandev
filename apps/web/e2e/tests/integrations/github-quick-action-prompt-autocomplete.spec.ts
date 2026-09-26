import { test, expect } from "../../fixtures/test-base";
import {
  acceptPromptCompletion,
  promptEditorText,
  replacePromptEditor,
  waitForPromptInStore,
} from "../../helpers/settings-prompt-editor";

test.describe("GitHub quick action prompt autocomplete", () => {
  test("inserts saved prompt mentions and GitHub action placeholders, then persists them", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(90_000);
    const prompt = await apiClient.createPrompt(
      `e2e-github-action-prompt-${Date.now()}`,
      "Reusable review guidance",
    );

    try {
      const settingsPath = `/settings/workspaces/${seedData.workspaceId}/integrations/github`;
      await testPage.goto(settingsPath);

      const quickActions = testPage
        .getByRole("heading", { name: "Quick actions", exact: true })
        .locator("xpath=ancestor::section[1]");
      await expect(quickActions).toBeVisible();
      await quickActions.getByRole("tab", { name: "Pull requests" }).click();
      await quickActions.getByRole("button", { name: "Edit prompt" }).first().click();
      await waitForPromptInStore(testPage, prompt.name);

      const editor = quickActions.locator('[data-testid^="github-action-prompt-editor-"]').first();
      await replacePromptEditor(testPage, editor, "Review @");
      await acceptPromptCompletion(testPage, prompt.name);
      await testPage.keyboard.type(" {{");
      await acceptPromptCompletion(testPage, "{{title}}");

      await expect(promptEditorText(editor)).toContainText(`@${prompt.name}`);
      await expect(promptEditorText(editor)).toContainText("{{title}}");
      await expect(testPage.getByTestId("settings-floating-save")).toBeVisible();
      await testPage
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: "Save changes" })
        .click();
      await expect(testPage.getByTestId("settings-floating-save")).not.toBeVisible({
        timeout: 15_000,
      });

      await testPage.goto(settingsPath);
      const reloadedQuickActions = testPage
        .getByRole("heading", { name: "Quick actions", exact: true })
        .locator("xpath=ancestor::section[1]");
      await reloadedQuickActions.getByRole("tab", { name: "Pull requests" }).click();
      await reloadedQuickActions.getByRole("button", { name: "Edit prompt" }).first().click();
      await waitForPromptInStore(testPage, prompt.name);
      const reloadedEditor = reloadedQuickActions
        .locator('[data-testid^="github-action-prompt-editor-"]')
        .first();
      await expect(promptEditorText(reloadedEditor)).toContainText(`@${prompt.name}`);
      await expect(promptEditorText(reloadedEditor)).toContainText("{{title}}");
      await prCapture.screenshot("github-action-prompt-editor", {
        caption: "GitHub quick action prompt editor with saved-prompt and placeholder completions.",
      });
    } finally {
      await apiClient.deletePrompt(prompt.id).catch(() => undefined);
      await apiClient.resetGitHubActionPresets(seedData.workspaceId).catch(() => undefined);
    }
  });
});
