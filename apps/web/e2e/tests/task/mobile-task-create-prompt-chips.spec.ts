import { expect, test } from "../../fixtures/test-base";
import type { Locator, Page } from "@playwright/test";
import { waitForHttp } from "../../helpers/causal-waits";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { expectTaskDescription } from "../../pages/task-description-editor";
import { useRegularMode } from "../../helpers/regular-mode";

useRegularMode();

const PROMPT_NAME = "mobile-task-reference-prompt";
const LONG_PROMPT_NAME =
  "mobile-task-reference-with-a-name-that-definitely-overflows-the-editor-width";
const PROMPT_CONTENT = Array.from(
  { length: 28 },
  (_, index) => `Mobile preview instruction ${index + 1}: preserve this saved guidance.`,
).join("\n");

type Box = { x: number; y: number; width: number; height: number };

function expectContained(parent: Box, child: Box, label: string) {
  const epsilon = 1;
  expect(child.x, `${label} left edge`).toBeGreaterThanOrEqual(parent.x - epsilon);
  expect(child.y, `${label} top edge`).toBeGreaterThanOrEqual(parent.y - epsilon);
  expect(child.x + child.width, `${label} right edge`).toBeLessThanOrEqual(
    parent.x + parent.width + epsilon,
  );
  expect(child.y + child.height, `${label} bottom edge`).toBeLessThanOrEqual(
    parent.y + parent.height + epsilon,
  );
}

async function readEditorContentMetrics(editor: Locator) {
  return editor.evaluate((element) => {
    const rect = element.getBoundingClientRect();
    const style = getComputedStyle(element);
    const paddingLeft = Number.parseFloat(style.paddingLeft) || 0;
    const paddingRight = Number.parseFloat(style.paddingRight) || 0;
    const paddingTop = Number.parseFloat(style.paddingTop) || 0;
    const paddingBottom = Number.parseFloat(style.paddingBottom) || 0;
    return {
      contentBox: {
        x: rect.x + paddingLeft,
        y: rect.y + paddingTop,
        width: Math.max(0, rect.width - paddingLeft - paddingRight),
        height: Math.max(0, rect.height - paddingTop - paddingBottom),
      },
      clientWidth: element.clientWidth,
      scrollWidth: element.scrollWidth,
    };
  });
}

async function clearTaskCreateDrafts(page: Page) {
  await page.evaluate(() => {
    for (const key of Object.keys(window.sessionStorage)) {
      if (key.startsWith("kandev.taskCreateDraft.")) window.sessionStorage.removeItem(key);
    }
  });
}

async function openTaskCreateDialogAtWidth(page: Page, width: number) {
  await page.setViewportSize({ width, height: 844 });
  await page.goto("/");
  await page.getByTestId("kanban-board").waitFor({ state: "visible" });
  await clearTaskCreateDrafts(page);

  const mobileFab = page.getByTestId("mobile-fab");
  if (await mobileFab.isVisible()) {
    await mobileFab.tap();
  } else {
    await page.getByTestId("create-task-button").first().click();
  }

  const dialog = page.getByTestId("create-task-dialog");
  await expect(dialog).toBeVisible();
  return { dialog, editor: dialog.getByTestId("task-description-input") };
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

    const reference = dialog.getByTestId("task-prompt-reference").first();
    const chip = reference.getByTestId("custom-prompt-mention");
    const remove = reference.getByTestId("task-prompt-reference-remove");
    await expect(chip).toBeVisible();
    await expect(remove).toBeVisible();
    const [referenceBox, chipBox, removeBox] = await Promise.all([
      reference.boundingBox(),
      chip.boundingBox(),
      remove.boundingBox(),
    ]);
    expect(referenceBox).not.toBeNull();
    expect(chipBox).not.toBeNull();
    expect(removeBox).not.toBeNull();
    expect(referenceBox!.height).toBeGreaterThanOrEqual(44);
    expect(chipBox!.width).toBeGreaterThanOrEqual(44);
    expect(chipBox!.height).toBeGreaterThanOrEqual(44);
    expectContained(referenceBox!, chipBox!, "preview target");
    expectContained(referenceBox!, removeBox!, "removal target");
    expect(removeBox!.height).toBeGreaterThanOrEqual(44);
    expect(removeBox!.width).toBeGreaterThanOrEqual(44);
    const editorContentMetrics = await readEditorContentMetrics(editor);
    expectContained(editorContentMetrics.contentBox, referenceBox!, "editor content");
    expect(editorContentMetrics.scrollWidth).toBeLessThanOrEqual(editorContentMetrics.clientWidth);
    await prCapture.screenshot("mobile-task-prompt-reference-chip", {
      caption: "A saved prompt appears as an editable touch-sized chip in task creation.",
    });

    await chip.tap();
    const drawer = testPage.locator('[data-slot="drawer-content"]:visible').last();
    await expect(drawer).toBeVisible();
    await expect(drawer.locator('[data-slot="drawer-description"]')).toHaveText("Prompt");
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
    await expect(testPage.locator('[data-slot="drawer-content"]:visible')).toHaveCount(0);

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

test("long prompt chips remain usable across coarse-pointer widths", async ({
  testPage,
  apiClient,
  prCapture,
}) => {
  test.setTimeout(120_000);
  const prompt = await apiClient.createPrompt(LONG_PROMPT_NAME, "Long mobile prompt content");

  try {
    for (const width of [390, 767, 768, 900]) {
      const { dialog, editor } = await openTaskCreateDialogAtWidth(testPage, width);
      await editor.fill("");
      await editor.tap();
      await editor.pressSequentially(`@${LONG_PROMPT_NAME}`);
      const menu = testPage.getByRole("listbox", {
        name: /Mention tasks, files, prompts/i,
      });
      await expect(menu).toBeVisible();
      const promptOption = menu.getByRole("option", { name: LONG_PROMPT_NAME, exact: false });
      await expect(promptOption).toBeVisible();
      await promptOption.tap();
      await expectTaskDescription(editor, `@${LONG_PROMPT_NAME}`);
      await editor.focus();
      await editor.press("ControlOrMeta+End");
      await editor.pressSequentially(` and @${LONG_PROMPT_NAME}`);
      await expect(menu).toBeVisible();
      await expect(promptOption).toBeVisible();
      await promptOption.tap();

      const references = dialog.getByTestId("task-prompt-reference");
      await expect(references).toHaveCount(2);

      for (let index = 0; index < 2; index += 1) {
        const reference = references.nth(index);
        const mention = reference.getByTestId("custom-prompt-mention");
        const label = mention.getByTestId("custom-prompt-mention-label");
        const remove = reference.getByTestId("task-prompt-reference-remove");
        await expect(mention).toHaveAttribute("aria-label", `Custom prompt: ${LONG_PROMPT_NAME}`);
        await expect(label).toHaveText(`@${LONG_PROMPT_NAME}`);
        const [referenceBox, mentionBox, labelBox, removeBox] = await Promise.all([
          reference.boundingBox(),
          mention.boundingBox(),
          label.boundingBox(),
          remove.boundingBox(),
        ]);
        expect(referenceBox).not.toBeNull();
        expect(mentionBox).not.toBeNull();
        expect(labelBox).not.toBeNull();
        expect(removeBox).not.toBeNull();
        expect(mentionBox!.width).toBeGreaterThanOrEqual(44);
        expect(mentionBox!.height).toBeGreaterThanOrEqual(44);
        expect(removeBox!.width).toBeGreaterThanOrEqual(44);
        expect(removeBox!.height).toBeGreaterThanOrEqual(44);
        expect(mentionBox!.x).toBeGreaterThanOrEqual(referenceBox!.x - 1);
        expect(mentionBox!.x + mentionBox!.width).toBeLessThanOrEqual(
          referenceBox!.x + referenceBox!.width + 1,
        );
        expect(removeBox!.x).toBeGreaterThanOrEqual(referenceBox!.x - 1);
        expect(removeBox!.x + removeBox!.width).toBeLessThanOrEqual(
          referenceBox!.x + referenceBox!.width + 1,
        );
        expectContained(referenceBox!, labelBox!, `label ${index}`);
        const labelMetrics = await label.evaluate((element) => {
          const computed = getComputedStyle(element);
          return {
            clientWidth: element.clientWidth,
            scrollWidth: element.scrollWidth,
            overflow: computed.overflow,
            textOverflow: computed.textOverflow,
            whiteSpace: computed.whiteSpace,
          };
        });
        expect(labelMetrics.overflow).toBe("hidden");
        expect(labelMetrics.textOverflow).toBe("ellipsis");
        expect(labelMetrics.whiteSpace).toBe("nowrap");
        if (width === 390) {
          expect(labelMetrics.scrollWidth).toBeGreaterThan(labelMetrics.clientWidth);
        }
        const editorMetrics = await readEditorContentMetrics(editor);
        expectContained(editorMetrics.contentBox, referenceBox!, `editor content ${index}`);
        expect(editorMetrics.scrollWidth).toBeLessThanOrEqual(editorMetrics.clientWidth);
      }

      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth),
      ).toBeLessThanOrEqual(await testPage.evaluate(() => document.documentElement.clientWidth));

      if (width === 390) {
        await prCapture.screenshot("mobile-task-prompt-reference-long-name", {
          caption: "Long saved-prompt references remain contained and touch-sized on a phone.",
        });
      }
    }
  } finally {
    await apiClient.deletePrompt(prompt.id).catch(() => undefined);
  }
});
