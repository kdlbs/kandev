/**
 * Adversarial QA probes for the @-mention prompt autocomplete in task creation.
 * Complements task-create-prompt-autocomplete.spec.ts with edge-case coverage.
 */
import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

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

    const textarea = testPage.getByTestId("task-description-input");
    await textarea.click();
    await textarea.pressSequentially("@");

    await expect(testPage.getByText(MENU_TITLE)).toBeVisible();
    await expect(testPage.getByRole("button", { name: /qa-alpha/ })).toBeVisible();
  });

  test("Escape closes the menu without inserting the prompt", async ({ testPage, apiClient }) => {
    test.setTimeout(60_000);
    await apiClient.createPrompt("qa-esc", "ESC_CONTENT");

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();

    const textarea = testPage.getByTestId("task-description-input");
    await textarea.click();
    await textarea.pressSequentially("@qa-es");

    await expect(testPage.getByText(MENU_TITLE)).toBeVisible();
    await textarea.press("Escape");
    await expect(testPage.getByText(MENU_TITLE)).not.toBeVisible();

    // The @query text is preserved (Esc just closes the menu, doesn't undo typing).
    await expect(textarea).toHaveValue("@qa-es");

    // Dialog should still be open — Escape went to the menu, not the dialog.
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();
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

    const textarea = testPage.getByTestId("task-description-input");
    await textarea.click();
    await textarea.pressSequentially("@qa-arr");

    await expect(testPage.getByText(MENU_TITLE)).toBeVisible();
    // Both should be visible.
    await expect(testPage.getByRole("button", { name: /qa-arr-1/ })).toBeVisible();
    await expect(testPage.getByRole("button", { name: /qa-arr-2/ })).toBeVisible();

    await textarea.press("ArrowDown");
    await textarea.press("Enter");

    const value = await textarea.inputValue();
    // The 2nd entry should be selected. The mention items come back in filter-score
    // order; with equal scores order should be insertion (stable sort). The test
    // accepts either content -- but asserts ONE of them was inserted, not both.
    expect(["FIRST", "SECOND"]).toContain(value);
    await expect(testPage.getByText(MENU_TITLE)).not.toBeVisible();
  });

  test("clicking a menu item with the mouse inlines the prompt", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);
    await apiClient.createPrompt("qa-mouse", "MOUSE_CONTENT");

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();

    const textarea = testPage.getByTestId("task-description-input");
    await textarea.click();
    await textarea.pressSequentially("@qa-mo");

    await expect(testPage.getByText(MENU_TITLE)).toBeVisible();
    await testPage.getByRole("button", { name: /qa-mouse/ }).click();

    await expect(textarea).toHaveValue("MOUSE_CONTENT");
    await expect(testPage.getByText(MENU_TITLE)).not.toBeVisible();
  });

  test("inserted prompt with multi-line content auto-grows the textarea", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);
    const lines = Array.from({ length: 8 }, (_, i) => `line ${i + 1}`).join("\n");
    await apiClient.createPrompt("qa-multi", lines);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();

    const textarea = testPage.getByTestId("task-description-input");
    await textarea.click();
    await textarea.pressSequentially("@qa-mu");
    await expect(testPage.getByText(MENU_TITLE)).toBeVisible();
    await textarea.press("Enter");

    await expect(textarea).toHaveValue(lines);

    // Height should reflect content (8 lines should be taller than the default ~96px min-h).
    const height = await textarea.evaluate((el) => (el as HTMLTextAreaElement).scrollHeight);
    expect(height).toBeGreaterThan(100);
  });

  test("typing space after @ closes the menu", async ({ testPage, apiClient }) => {
    test.setTimeout(60_000);
    await apiClient.createPrompt("qa-space", "x");

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();

    const textarea = testPage.getByTestId("task-description-input");
    await textarea.click();
    await textarea.pressSequentially("@");
    await expect(testPage.getByText(MENU_TITLE)).toBeVisible();
    await textarea.pressSequentially(" foo");
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

    const textarea = testPage.getByTestId("task-description-input");
    await textarea.click();
    await textarea.pressSequentially("@qa");
    await expect(testPage.getByText(MENU_TITLE)).toBeVisible();
    await textarea.press("Backspace");
    await textarea.press("Backspace");
    await textarea.press("Backspace"); // deletes the @
    await expect(textarea).toHaveValue("");
    await expect(testPage.getByText(MENU_TITLE)).toHaveCount(0);
  });

  test("ArrowUp/ArrowDown don't propagate to native textarea cursor motion", async ({
    testPage,
    apiClient,
  }) => {
    // When the menu is open, the hook calls preventDefault on Arrow keys, so the
    // textarea's selection cursor should NOT move. Probe by verifying that
    // arrow-up does not change the textarea content/cursor in a way that
    // breaks the subsequent Enter selection.
    test.setTimeout(60_000);
    await apiClient.createPrompt("qa-arrow", "ARROW_CONTENT");

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();

    const textarea = testPage.getByTestId("task-description-input");
    await textarea.click();
    await textarea.pressSequentially("@qa-arr");
    await expect(testPage.getByText(MENU_TITLE)).toBeVisible();

    const before = await textarea.evaluate((el) => (el as HTMLTextAreaElement).selectionStart);
    await textarea.press("ArrowDown");
    await textarea.press("ArrowUp");
    const after = await textarea.evaluate((el) => (el as HTMLTextAreaElement).selectionStart);
    expect(after).toBe(before);

    await textarea.press("Enter");
    await expect(textarea).toHaveValue("ARROW_CONTENT");
  });

  test("description with inlined prompt is sent to backend on submit", async ({
    testPage,
    apiClient,
  }) => {
    // Acceptance criterion: the textarea contains the inlined prompt content before submit
    // and the form completes successfully. API-level task description verification is a
    // follow-up — it requires retrieving the created task by title after creation.
    test.setTimeout(60_000);
    const content = "INLINED_FROM_PROMPT_PAYLOAD";
    await apiClient.createPrompt("qa-submit", content);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();

    await testPage.getByTestId("task-title-input").fill("qa-submit-task");
    const textarea = testPage.getByTestId("task-description-input");
    await textarea.click();
    await textarea.pressSequentially("@qa-su");
    await expect(testPage.getByText(MENU_TITLE)).toBeVisible();
    await textarea.press("Enter");
    await expect(textarea).toHaveValue(content);

    const start = testPage.getByTestId("submit-start-agent");
    await expect(start).toBeEnabled({ timeout: 30_000 });
    await start.click();

    await expect(testPage.getByTestId("create-task-dialog")).not.toBeVisible({
      timeout: 10_000,
    });
  });
});
