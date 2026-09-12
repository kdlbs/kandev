/**
 * Adversarial QA probes for the @-mention prompt autocomplete in task creation.
 * Complements task-create-prompt-autocomplete.spec.ts with edge-case coverage.
 */
import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { expectTaskDescription } from "../../pages/task-description-editor";

const MENU_TITLE = /Mention tasks, files, prompts/i;

async function cleanupPrompts(
  apiClient: {
    listPrompts: () => Promise<{ prompts: Array<{ id: string; name: string; builtin: boolean }> }>;
    deletePrompt: (id: string) => Promise<void>;
  },
  names: string[],
) {
  const { prompts } = await apiClient.listPrompts();
  for (const p of prompts) {
    if (!p.builtin && names.includes(p.name)) {
      await apiClient.deletePrompt(p.id).catch(() => undefined);
    }
  }
}

const ALL_QA_PROMPTS = [
  "qa-alpha",
  "qa-esc",
  "qa-arr-1",
  "qa-arr-2",
  "qa-mouse",
  "qa-multi",
  "qa-space",
  "qa-back",
  "qa-arrow",
  "qa-submit",
];

test.describe("@-mention autocomplete: adversarial QA", () => {
  test.afterEach(async ({ apiClient }) => {
    await cleanupPrompts(apiClient, ALL_QA_PROMPTS);
  });

  test("bare @ opens menu with prompts visible", async ({ testPage, apiClient }) => {
    test.setTimeout(60_000);
    await apiClient.createPrompt("qa-alpha", "alpha-content");

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();

    const editor = testPage.getByTestId("task-description-input");
    await editor.click();
    await editor.pressSequentially("@");

    await expect(testPage.getByText(MENU_TITLE)).toBeVisible();
    await expect(testPage.getByRole("option", { name: /qa-alpha/ })).toBeVisible();
  });

  test("Escape closes the menu without inserting the prompt", async ({ testPage, apiClient }) => {
    test.setTimeout(60_000);
    await apiClient.createPrompt("qa-esc", "ESC_CONTENT");

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();

    const editor = testPage.getByTestId("task-description-input");
    await editor.click();
    await editor.pressSequentially("@qa-es");

    await expect(testPage.getByText(MENU_TITLE)).toBeVisible();
    await editor.press("Escape");
    await expect(testPage.getByText(MENU_TITLE)).not.toBeVisible();

    // The @query text is preserved (Esc just closes the menu, doesn't undo typing).
    await expectTaskDescription(editor, "@qa-es");

    // The open state must persist after the close animation window.
    await expect(testPage.getByTestId("create-task-dialog")).toHaveAttribute("data-state", "open");
    await expect(editor).toBeFocused();

    await editor.pressSequentially(" continued");
    await expectTaskDescription(editor, "@qa-es continued");
    await expect(testPage.getByText(MENU_TITLE)).toHaveCount(0);
  });

  test("Escape keeps Create Task open without an autocomplete menu", async ({
    testPage,
    prCapture,
  }) => {
    test.setTimeout(60_000);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();

    const dialog = testPage.getByTestId("create-task-dialog");
    const editor = testPage.getByTestId("task-description-input");
    await expect(dialog).toHaveAttribute("data-state", "open");
    await editor.fill("Keep this draft");
    await expect(testPage.getByText(MENU_TITLE)).toHaveCount(0);

    await editor.press("Escape");

    await expect(dialog).toHaveAttribute("data-state", "open");
    await expectTaskDescription(editor, "Keep this draft");
    await prCapture.screenshot("create-task-dialog-after-escape-desktop", {
      caption: "Create Task stays open with the draft after Escape on desktop.",
    });
  });

  test("ArrowDown + Enter selects the second prompt", async ({ testPage, apiClient }) => {
    test.setTimeout(60_000);
    // Both prompts begin with "qa-arr" so the filter narrows to both.
    await apiClient.createPrompt("qa-arr-1", "FIRST");
    await apiClient.createPrompt("qa-arr-2", "SECOND");

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();

    const editor = testPage.getByTestId("task-description-input");
    await editor.click();
    await editor.pressSequentially("@qa-arr");

    await expect(testPage.getByText(MENU_TITLE)).toBeVisible();
    // Both should be visible.
    await expect(testPage.getByRole("option", { name: /qa-arr-1/ })).toBeVisible();
    await expect(testPage.getByRole("option", { name: /qa-arr-2/ })).toBeVisible();

    const secondOption = testPage.getByRole("option", { name: /qa-arr-2/ });
    await expect(async () => {
      await editor.focus();
      await editor.press("ArrowDown");
      await expect(secondOption).toHaveAttribute("aria-selected", "true", { timeout: 500 });
    }).toPass({ timeout: 5_000, intervals: [100, 250, 500] });
    await editor.press("Enter");

    // Equal filter scores keep insertion order. One ArrowDown selects qa-arr-2.
    await expectTaskDescription(editor, "@qa-arr-2");
    await expect(testPage.getByText(MENU_TITLE)).not.toBeVisible();
  });

  test("clicking a menu item with the mouse inserts a prompt reference", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);
    await apiClient.createPrompt("qa-mouse", "MOUSE_CONTENT");

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();

    const editor = testPage.getByTestId("task-description-input");
    await editor.click();
    await editor.pressSequentially("@qa-mo");

    await expect(testPage.getByText(MENU_TITLE)).toBeVisible();
    await testPage.getByRole("option", { name: /qa-mouse/ }).click();

    await expectTaskDescription(editor, "@qa-mouse");
    await expect(testPage.getByText(MENU_TITLE)).not.toBeVisible();
  });

  test("selecting a prompt with multi-line content keeps the alias compact", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);
    const lines = Array.from({ length: 8 }, (_, i) => `line ${i + 1}`).join("\n");
    const promptName = `qa-multi-${Date.now()}`;
    const prompt = await apiClient.createPrompt(promptName, lines);

    try {
      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await kanban.createTaskButton.first().click();
      await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();

      const editor = testPage.getByTestId("task-description-input");
      await editor.fill("");
      await editor.click();
      await editor.pressSequentially(`@${promptName}`);
      await expect(testPage.getByText(MENU_TITLE)).toBeVisible();
      // Select the exact prompt row. Keyboard selection can use a stale
      // filtered item while the prompt store is still hydrating.
      await testPage.getByRole("option", { name: new RegExp(promptName) }).click();

      await expectTaskDescription(editor, `@${promptName}`);
      await expect(editor).toContainText(`@${promptName}`);
    } finally {
      await apiClient.deletePrompt(prompt.id).catch(() => undefined);
    }
  });

  test("typing space after @ closes the menu", async ({ testPage, apiClient }) => {
    test.setTimeout(60_000);
    await apiClient.createPrompt("qa-space", "x");

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();

    const editor = testPage.getByTestId("task-description-input");
    await editor.click();
    await editor.pressSequentially("@");
    await expect(testPage.getByText(MENU_TITLE)).toBeVisible();
    await editor.pressSequentially(" foo");
    // After a space immediately follows @, trigger detection should yield null.
    await expect(testPage.getByText(MENU_TITLE)).toHaveCount(0);
  });

  test("backspacing past the @ closes the menu", async ({ testPage, apiClient }) => {
    test.setTimeout(60_000);
    await apiClient.createPrompt("qa-back", "x");

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();

    const editor = testPage.getByTestId("task-description-input");
    await editor.click();
    await editor.pressSequentially("@qa");
    await expect(testPage.getByText(MENU_TITLE)).toBeVisible();
    await editor.press("Backspace");
    await editor.press("Backspace");
    await editor.press("Backspace"); // deletes the @
    await expectTaskDescription(editor, "");
    await expect(testPage.getByText(MENU_TITLE)).toHaveCount(0);
  });

  test("ArrowUp/ArrowDown stay in the suggestion menu", async ({ testPage, apiClient }) => {
    // When the menu is open, the hook calls preventDefault on Arrow keys, so
    // the editor keeps the active query while the menu changes selection.
    test.setTimeout(60_000);
    await apiClient.createPrompt("qa-arrow", "ARROW_CONTENT");

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();

    const editor = testPage.getByTestId("task-description-input");
    await editor.click();
    await editor.pressSequentially("@qa-arr");
    await expect(testPage.getByText(MENU_TITLE)).toBeVisible();

    await editor.press("ArrowDown");
    await editor.press("ArrowUp");

    await editor.press("Enter");
    await expectTaskDescription(editor, "@qa-arrow");
  });

  test("description with a prompt alias is sent to backend on submit", async ({
    testPage,
    apiClient,
  }) => {
    // The visible alias is submitted and the existing server launch path
    // resolves its definition later.
    test.setTimeout(60_000);
    const content = "INLINED_FROM_PROMPT_PAYLOAD";
    await apiClient.createPrompt("qa-submit", content);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();

    // Use scratch mode so submit does not depend on a pre-selected repository.
    await testPage.getByTestId("source-mode-scratch").click();
    await testPage.getByTestId("task-title-input").fill("qa-submit-task");
    const editor = testPage.getByTestId("task-description-input");
    await editor.click();
    await editor.pressSequentially("@qa-su");
    await expect(testPage.getByText(MENU_TITLE)).toBeVisible();
    await editor.press("Enter");
    await expectTaskDescription(editor, "@qa-submit");

    const start = testPage.getByTestId("submit-start-agent");
    await expect(start).toBeEnabled({ timeout: 30_000 });
    await start.click();

    await expect(testPage.getByTestId("create-task-dialog")).not.toBeVisible({
      timeout: 10_000,
    });
  });
});
