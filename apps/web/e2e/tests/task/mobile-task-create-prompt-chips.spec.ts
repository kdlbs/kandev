import { expect, test } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import { waitForHttp } from "../../helpers/causal-waits";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { expectTaskDescription } from "../../pages/task-description-editor";
import { useRegularMode } from "../../helpers/regular-mode";

useRegularMode();

const PROMPT_NAME = "mobile-task-reference-prompt";
const PROMPT_CONTENT = Array.from(
  { length: 28 },
  (_, index) => `Mobile preview instruction ${index + 1}: preserve this saved guidance.`,
).join("\n");

async function clearTaskCreateDrafts(page: Page) {
  await page.evaluate(() => {
    for (const key of Object.keys(window.sessionStorage)) {
      if (key.startsWith("kandev.taskCreateDraft.")) window.sessionStorage.removeItem(key);
    }
  });
}

test("edits saved-prompt chips and submits their aliases on mobile", async ({
  testPage,
  apiClient,
  prCapture,
}) => {
  test.setTimeout(120_000);
  const prompt = await apiClient.createPrompt(PROMPT_NAME, PROMPT_CONTENT);
  let taskId: string | undefined;

  try {
    await testPage.setViewportSize({ width: 390, height: 844 });
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await clearTaskCreateDrafts(testPage);
    await mobile.mobileFab.tap();

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByTestId("source-mode-scratch").tap();
    await dialog.getByTestId("task-title-input").fill("Mobile prompt reference task");

    const editor = dialog.getByTestId("task-description-input");
    await editor.fill("");
    await editor.tap();
    await editor.pressSequentially("@mobile-task-reference");

    const menu = testPage.getByRole("listbox", {
      name: /Mention tasks, files, prompts/i,
    });
    await expect(menu).toBeVisible();
    const option = menu.getByRole("option", { name: PROMPT_NAME, exact: false });
    await expect(option).toBeVisible();
    await option.tap();
    await expectTaskDescription(editor, `@${PROMPT_NAME}`);

    const chip = dialog.getByTestId("custom-prompt-mention").first();
    const remove = dialog.getByTestId("task-prompt-reference-remove").first();
    await expect(chip).toBeVisible();
    await expect(remove).toBeVisible();
    const [chipBox, removeBox] = await Promise.all([chip.boundingBox(), remove.boundingBox()]);
    expect(chipBox).not.toBeNull();
    expect(removeBox).not.toBeNull();
    expect(chipBox!.height).toBeGreaterThanOrEqual(44);
    expect(removeBox!.height).toBeGreaterThanOrEqual(44);
    expect(removeBox!.width).toBeGreaterThanOrEqual(44);
    await prCapture.screenshot("mobile-task-prompt-reference-chip", {
      caption: "A saved prompt appears as an editable touch-sized chip in task creation.",
    });

    await chip.tap();
    const drawer = testPage.locator('[data-slot="drawer-content"]:visible').last();
    await expect(drawer).toBeVisible();
    await expect(drawer).toContainText(PROMPT_CONTENT.slice(0, 45));
    const previewScroll = drawer.locator("div.overflow-y-auto").first();
    await expect(previewScroll).toBeVisible();
    const previewMetrics = await previewScroll.evaluate((element) => ({
      clientHeight: element.clientHeight,
      scrollHeight: element.scrollHeight,
      overflowY: getComputedStyle(element).overflowY,
    }));
    expect(previewMetrics.overflowY).toBe("auto");
    expect(previewMetrics.scrollHeight).toBeGreaterThanOrEqual(previewMetrics.clientHeight);
    await prCapture.screenshot("mobile-task-prompt-reference-preview", {
      caption: "Tapping a saved prompt chip opens its current definition in a mobile drawer.",
    });
    await testPage.keyboard.press("Escape");
    await expect(drawer).not.toBeVisible();
    await expectTaskDescription(editor, `@${PROMPT_NAME}`);

    await editor.focus();
    await editor.press("ControlOrMeta+End");
    await editor.pressSequentially(` and @mobile-task-reference`);
    await expect(menu).toBeVisible();
    await menu.getByRole("option", { name: PROMPT_NAME, exact: false }).tap();
    await expectTaskDescription(editor, `@${PROMPT_NAME} and @${PROMPT_NAME}`);
    await expect(dialog.getByTestId("task-prompt-reference-remove")).toHaveCount(2);
    await dialog.getByTestId("task-prompt-reference-remove").first().tap();
    await expectTaskDescription(editor, ` and @${PROMPT_NAME}`);

    const longDraft = Array.from(
      { length: 36 },
      (_, index) => `Long mobile draft line ${index + 1}: keep this text inside the editor.`,
    ).join("\n");
    await editor.fill(longDraft);
    const editorMetrics = await editor.evaluate((element) => ({
      clientHeight: element.clientHeight,
      scrollHeight: element.scrollHeight,
      overflowY: getComputedStyle(element).overflowY,
    }));
    expect(editorMetrics.overflowY).toBe("auto");
    expect(editorMetrics.scrollHeight).toBeGreaterThan(editorMetrics.clientHeight);
    const submittedDescription = `Final mobile task goal @${PROMPT_NAME}`;
    await editor.fill(submittedDescription);
    await expectTaskDescription(editor, submittedDescription);
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);

    const responsePromise = waitForHttp(testPage, "POST", /\/api\/v1\/tasks$/);
    await dialog.getByTestId("submit-start-agent").tap();
    const response = await responsePromise;
    const responseBody = await response.text();
    expect(response.status(), responseBody).toBe(200);
    taskId = (JSON.parse(responseBody) as { id: string }).id;
    await expect
      .poll(async () => (await apiClient.getTask(taskId!)).description)
      .toBe(submittedDescription);
  } finally {
    if (taskId) await apiClient.deleteTask(taskId).catch(() => undefined);
    await apiClient.deletePrompt(prompt.id).catch(() => undefined);
  }
});
