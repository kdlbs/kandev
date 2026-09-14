/**
 * Adversarial QA probes for the @-mention prompt autocomplete in task creation.
 * Complements task-create-prompt-autocomplete.spec.ts with edge-case coverage.
 */
import { test, expect } from "../../fixtures/test-base";
import type { Locator } from "@playwright/test";
import { KanbanPage } from "../../pages/kanban-page";
import { expectTaskDescription } from "../../pages/task-description-editor";

const MENU_TITLE = /Mention tasks, files, prompts/i;
const LONG_PROMPT_NAME = "qa-long-prompt-reference-name-that-definitely-overflows-editor-width";

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
  "qa-compact",
  LONG_PROMPT_NAME,
];

type Box = { x: number; y: number; width: number; height: number };

async function readPromptReferenceMetrics(reference: Locator) {
  return reference.evaluate((element) => {
    const readBox = (target: Element): Box => {
      const rect = target.getBoundingClientRect();
      return { x: rect.x, y: rect.y, width: rect.width, height: rect.height };
    };
    const mention = element.querySelector<HTMLElement>('[data-testid="custom-prompt-mention"]');
    const remove = element.querySelector<HTMLElement>(
      '[data-testid="task-prompt-reference-remove"]',
    );
    const label = mention?.querySelector<HTMLElement>(
      '[data-testid="custom-prompt-mention-label"]',
    );
    if (!mention || !remove || !label) throw new Error("prompt reference controls are missing");

    const elementsWithGreenBorder = [
      element,
      ...Array.from(element.querySelectorAll<HTMLElement>("[class]")),
    ].filter((target) => target.getAttribute("class")?.includes("border-emerald-300/35"));
    const labelStyle = getComputedStyle(label);
    return {
      shell: readBox(element),
      mention: readBox(mention),
      label: readBox(label),
      remove: readBox(remove),
      labelText: label.textContent ?? "",
      labelClientWidth: label.clientWidth,
      labelScrollWidth: label.scrollWidth,
      labelFontSize: labelStyle.fontSize,
      labelOverflow: labelStyle.overflow,
      labelTextOverflow: labelStyle.textOverflow,
      labelWhiteSpace: labelStyle.whiteSpace,
      shellBorderWidth: getComputedStyle(element).borderTopWidth,
      mentionBorderWidth: getComputedStyle(mention).borderTopWidth,
      greenBorderCount: elementsWithGreenBorder.length,
    };
  });
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

  test("editable prompt chip is compact and contains removal", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    test.setTimeout(90_000);
    await apiClient.createPrompt("qa-compact", "Compact prompt content");

    await testPage.setViewportSize({ width: 1280, height: 900 });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();

    const dialog = testPage.getByTestId("create-task-dialog");
    const editor = dialog.getByTestId("task-description-input");
    await expect(dialog).toBeVisible();
    await editor.fill("");
    await editor.click();
    await editor.pressSequentially("@qa-com");
    await expect(testPage.getByText(MENU_TITLE)).toBeVisible();
    const promptOption = testPage.getByRole("option").filter({ hasText: "qa-compact" });
    await expect(promptOption).toBeVisible();
    await promptOption.click();
    await expect(dialog.getByTestId("task-prompt-reference")).toHaveCount(1);

    const reference = dialog.getByTestId("task-prompt-reference").first();
    const remove = reference.getByTestId("task-prompt-reference-remove");
    const metrics = await readPromptReferenceMetrics(reference);
    expect(metrics.shell.height).toBeCloseTo(24, 0);
    expect(metrics.labelFontSize).toBe("12px");
    expect(metrics.shellBorderWidth).toBe("1px");
    expect(metrics.mentionBorderWidth).toBe("0px");
    expect(metrics.greenBorderCount).toBe(1);
    expectContained(metrics.shell, metrics.mention, "preview target");
    expectContained(metrics.shell, metrics.remove, "removal target");
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );

    const defaultBackground = await remove.evaluate(
      (element) => getComputedStyle(element).backgroundColor,
    );
    await remove.hover();
    const hoverBackground = await remove.evaluate(
      (element) => getComputedStyle(element).backgroundColor,
    );
    expect(hoverBackground).not.toBe(defaultBackground);
    await remove.focus();
    await expect(remove).toBeFocused();
    const focusOutline = await remove.evaluate((element) => {
      const style = getComputedStyle(element);
      return { style: style.outlineStyle, width: style.outlineWidth };
    });
    expect(focusOutline.style).toBe("solid");
    expect(focusOutline.width).toBe("2px");
    await prCapture.screenshot("desktop-task-prompt-reference-chip", {
      caption: "The desktop task prompt reference uses one compact border around both controls.",
    });

    await remove.press("Enter");
    await expectTaskDescription(editor, "");
    await expect(testPage.getByText("Compact prompt content", { exact: false })).toHaveCount(0);
    await editor.press("ControlOrMeta+z");
    await expectTaskDescription(editor, "@qa-compact");
    await expect(dialog.getByTestId("task-prompt-reference")).toHaveCount(1);

    for (const width of [767, 768]) {
      await testPage.setViewportSize({ width, height: 900 });
      await expect(reference).toBeVisible();
      const resized = await readPromptReferenceMetrics(reference);
      if (width < 768) {
        expect(resized.shell.height).toBeGreaterThanOrEqual(44);
      } else {
        expect(resized.shell.height).toBeCloseTo(24, 0);
      }
    }
  });

  test("long prompt chips truncate and wrap without hiding removal", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    test.setTimeout(90_000);
    const promptName = LONG_PROMPT_NAME;
    await apiClient.createPrompt(promptName, "Long prompt content");

    await testPage.setViewportSize({ width: 480, height: 900 });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await testPage.getByTestId("mobile-fab").click();

    const dialog = testPage.getByTestId("create-task-dialog");
    const editor = dialog.getByTestId("task-description-input");
    await expect(dialog).toBeVisible();
    await editor.fill("");
    await editor.click();
    await editor.pressSequentially(`@${promptName}`);
    await expect(testPage.getByText(MENU_TITLE)).toBeVisible();
    const promptOption = testPage.getByRole("option").filter({ hasText: promptName });
    await expect(promptOption).toBeVisible();
    await promptOption.click();
    await editor.focus();
    await editor.press("ControlOrMeta+End");
    await editor.pressSequentially(` and @${promptName}`);
    await expect(testPage.getByText(MENU_TITLE)).toBeVisible();
    await expect(promptOption).toBeVisible();
    await promptOption.click();
    const references = dialog.getByTestId("task-prompt-reference");
    await expect(references).toHaveCount(2);
    const editorMetrics = await readEditorContentMetrics(editor);
    expect(editorMetrics.scrollWidth).toBeLessThanOrEqual(editorMetrics.clientWidth);

    for (let index = 0; index < 2; index += 1) {
      const reference = references.nth(index);
      const mention = reference.getByTestId("custom-prompt-mention");
      const remove = reference.getByTestId("task-prompt-reference-remove");
      await expect(mention).toHaveAttribute("aria-label", `Custom prompt: ${promptName}`);
      const metrics = await readPromptReferenceMetrics(reference);
      expect(metrics.labelText).toBe(`@${promptName}`);
      expect(metrics.labelScrollWidth).toBeGreaterThan(metrics.labelClientWidth);
      expect(metrics.labelOverflow).toBe("hidden");
      expect(metrics.labelTextOverflow).toBe("ellipsis");
      expect(metrics.labelWhiteSpace).toBe("nowrap");
      expectContained(editorMetrics.contentBox, metrics.shell, `editor content ${index}`);
      expectContained(metrics.shell, metrics.mention, `preview target ${index}`);
      expectContained(metrics.shell, metrics.label, `label ${index}`);
      expectContained(metrics.shell, metrics.remove, `removal target ${index}`);
      await expect(remove).toBeVisible();
    }
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );
    await prCapture.screenshot("desktop-task-prompt-reference-long-name", {
      caption: "Long task prompt references keep their removal controls visible while truncating.",
    });
  });
});
