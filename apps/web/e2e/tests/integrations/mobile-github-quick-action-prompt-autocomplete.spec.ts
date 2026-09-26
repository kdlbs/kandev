import { test, expect } from "../../fixtures/test-base";
import {
  acceptPromptCompletion,
  promptEditorText,
  replacePromptEditor,
  waitForPromptInStore,
} from "../../helpers/settings-prompt-editor";

test.describe("GitHub quick action prompt autocomplete on mobile", () => {
  test("supports touch editing without horizontal document overflow", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(90_000);
    const prompt = await apiClient.createPrompt(
      `Daily Summary ${Date.now()}`,
      "Reusable mobile review guidance",
    );

    try {
      const settingsPath = `/settings/workspaces/${seedData.workspaceId}/integrations/github`;
      await testPage.goto(settingsPath);

      const quickActions = testPage
        .getByRole("heading", { name: "Quick actions", exact: true })
        .locator("xpath=ancestor::section[1]");
      await expect(quickActions).toBeVisible();
      await quickActions.getByRole("tab", { name: "Pull requests" }).tap();
      await quickActions.getByRole("button", { name: "Edit prompt" }).first().tap();
      await waitForPromptInStore(testPage, prompt.name);

      const editor = quickActions.locator('[data-testid^="github-action-prompt-editor-"]').first();
      await replacePromptEditor(testPage, editor, "Review @Daily ", { touch: true });
      await acceptPromptCompletion(testPage, prompt.name, { touch: true });
      await testPage.keyboard.type(" {{");
      await acceptPromptCompletion(testPage, "{{title}}", { touch: true });

      await expect(promptEditorText(editor)).toContainText(`@${prompt.name}`);
      await expect(promptEditorText(editor)).toContainText("{{title}}");

      const editorBox = await editor.boundingBox();
      expect(editorBox).not.toBeNull();
      expect(editorBox!.width).toBeLessThanOrEqual(testPage.viewportSize()!.width);
      expect(
        await testPage.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      ).toBe(true);
      await prCapture.screenshot("mobile-github-action-prompt-editor", {
        caption: "Mobile GitHub quick action prompt editor with touch completion controls.",
      });
    } finally {
      await apiClient.deletePrompt(prompt.id).catch(() => undefined);
    }
  });
});
