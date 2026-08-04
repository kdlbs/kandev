import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

const PROMPT_NAME = "e2e-bug-template";
const PROMPT_CONTENT = "Reproduce, isolate, fix with a regression test.";
const MENU_TITLE = /Mention tasks, files, prompts/i;

test.describe("Task creation: custom prompt autocomplete", () => {
  test.afterEach(async ({ apiClient }) => {
    const { prompts } = await apiClient.listPrompts();
    for (const p of prompts) {
      if (!p.builtin && p.name === PROMPT_NAME) {
        await apiClient.deletePrompt(p.id).catch(() => undefined);
      }
    }
  });

  test("typing @<name> opens the menu and selecting inlines the prompt content", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);

    await apiClient.createPrompt(PROMPT_NAME, PROMPT_CONTENT);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    const textarea = testPage.getByTestId("task-description-input");
    await textarea.click();
    await textarea.pressSequentially("@e2e-bu");

    const menu = testPage.getByText(MENU_TITLE);
    await expect(menu).toBeVisible({ timeout: 5_000 });
    await expect(testPage.getByRole("option", { name: new RegExp(PROMPT_NAME) })).toBeVisible();

    await textarea.press("Enter");

    await expect(textarea).toHaveValue(PROMPT_CONTENT);
    await expect(menu).not.toBeVisible();
  });

  test("typing @ inside a word does NOT open the menu", async ({ testPage, apiClient }) => {
    test.setTimeout(60_000);

    await apiClient.createPrompt(PROMPT_NAME, PROMPT_CONTENT);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();

    const textarea = testPage.getByTestId("task-description-input");
    await textarea.click();
    // No preceding whitespace before @ — the trigger is invalid.
    await textarea.pressSequentially("foo@bar");

    await expect(textarea).toHaveValue("foo@bar");
    await expect(testPage.getByText(MENU_TITLE)).toHaveCount(0);
  });

  test("Enter inside the menu does NOT submit the form", async ({ testPage, apiClient }) => {
    test.setTimeout(60_000);

    await apiClient.createPrompt(PROMPT_NAME, PROMPT_CONTENT);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    // Fill title so the form would be otherwise submittable.
    await testPage.getByTestId("task-title-input").fill("autocomplete-enter-test");
    const textarea = testPage.getByTestId("task-description-input");
    await textarea.click();
    await textarea.pressSequentially("@e2e-bu");

    // Wait for the menu AND its option to actually populate before pressing
    // Enter. Under CI shard load the autocomplete popup can open a beat before
    // its options hydrate; pressing Enter against an empty/half-open menu lets
    // the key fall through to the form submit, which closes the dialog and fails
    // the assertions below. Gating on the option (not just the title) with a
    // generous timeout makes the selection deterministic.
    await expect(testPage.getByText(MENU_TITLE)).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByRole("option", { name: new RegExp(PROMPT_NAME) })).toBeVisible({
      timeout: 15_000,
    });
    await textarea.press("Enter");

    // Dialog must still be open — Enter selected the menu item, not the form submit.
    await expect(dialog).toBeVisible();
    await expect(textarea).toHaveValue(PROMPT_CONTENT);
  });
});
