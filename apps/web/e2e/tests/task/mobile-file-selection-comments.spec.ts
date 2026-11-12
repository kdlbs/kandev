import fs from "node:fs";
import { test, expect } from "../../fixtures/test-base";
import {
  CODE_SELECTION_QUOTE,
  CODE_SELECTION_LINE,
  expectStoredCodeComment,
  openCodeFileForSelectionComment,
  restoreCodeEditorProvider,
  selectFinalCodeLine,
  type CodeEditorProvider,
} from "../../helpers/file-selection-comments";
import {
  restoreSelectionCommentVisualViewport,
  setSelectionCommentVisualViewport,
} from "../../helpers/selection-comment-visual-viewport";

const COMMENT_TEXT = "Clarify the final line.";

async function expectDockWithinEditor(
  dock: import("@playwright/test").Locator,
  owner: import("@playwright/test").Locator,
) {
  await expect
    .poll(async () => {
      const [dockBox, ownerBox] = await Promise.all([dock.boundingBox(), owner.boundingBox()]);
      if (!dockBox || !ownerBox) return false;
      return (
        dockBox.x >= ownerBox.x - 1 &&
        dockBox.x + dockBox.width <= ownerBox.x + ownerBox.width + 1 &&
        dockBox.y >= ownerBox.y - 1 &&
        dockBox.y + dockBox.height <= ownerBox.y + ownerBox.height + 1
      );
    })
    .toBe(true);
}

test.describe("File selection comments on mobile", () => {
  for (const provider of ["codemirror", "monaco"] as const satisfies CodeEditorProvider[]) {
    test(`${provider} docks touch selection and adds a pending final-line comment`, async ({
      testPage,
      apiClient,
      seedData,
    }, testInfo) => {
      test.setTimeout(90_000);
      const fileName = `mobile-selection-comment-${provider}-${Date.now()}.ts`;
      const { session, sessionId, diskPath, repositoryName, owner } =
        await openCodeFileForSelectionComment({
          page: testPage,
          apiClient,
          seedData,
          fileName,
          title: `Mobile ${provider} selection comment`,
          provider,
        });
      try {
        await selectFinalCodeLine({ page: testPage, owner, provider, touch: true });
        const dock = testPage.getByTestId("selection-comment-dock");
        const action = testPage.getByTestId(`${provider}-comment-button`);
        await expect(action).toBeVisible({ timeout: 5_000 });
        await expect(dock).toHaveCount(1);
        const actionBox = await action.boundingBox();
        expect(actionBox).not.toBeNull();
        if (!actionBox) throw new Error("Expected editor comment action geometry");
        expect(actionBox.height).toBeGreaterThanOrEqual(44);
        if (provider === "codemirror") {
          const reservedPadding = await owner
            .locator(".cm-scroller")
            .evaluate((element) =>
              Number.parseFloat(window.getComputedStyle(element).paddingBottom),
            );
          expect(reservedPadding).toBeGreaterThanOrEqual(56);
        }
        await expectDockWithinEditor(dock, owner);

        const dockBox = await dock.boundingBox();
        expect(dockBox).not.toBeNull();
        if (!dockBox) throw new Error("Expected dock geometry");
        const beforeViewportChange = await owner.evaluate((element) => {
          const rect = element.getBoundingClientRect();
          return { innerHeight: window.innerHeight, top: rect.top, bottom: rect.bottom };
        });
        const clippedViewport = Math.max(320, Math.floor(beforeViewportChange.innerHeight * 0.62));
        await setSelectionCommentVisualViewport(testPage, {
          height: clippedViewport,
          offsetTop: 24,
        });
        await expect(action).toBeVisible();
        const afterViewportChange = await owner.evaluate((element) => {
          const rect = element.getBoundingClientRect();
          return { innerHeight: window.innerHeight, top: rect.top, bottom: rect.bottom };
        });
        expect(afterViewportChange.innerHeight).toBe(beforeViewportChange.innerHeight);
        expect(afterViewportChange.top).toBeCloseTo(beforeViewportChange.top, 1);
        expect(afterViewportChange.bottom).toBeCloseTo(beforeViewportChange.bottom, 1);

        const finalLine =
          provider === "codemirror"
            ? owner.locator(".cm-line").filter({ hasText: CODE_SELECTION_QUOTE }).last()
            : owner.locator(".view-line").filter({ hasText: CODE_SELECTION_QUOTE }).last();
        if (provider === "codemirror") {
          await owner.locator(".cm-scroller").evaluate((element) => {
            element.scrollTop = element.scrollHeight;
          });
        } else {
          await owner
            .locator(".monaco-scrollable-element")
            .first()
            .evaluate((element) => {
              element.scrollTop = element.scrollHeight;
            });
        }
        await expect(finalLine).toBeVisible();
        await finalLine.scrollIntoViewIfNeeded();
        const clippedDockBox = await dock.boundingBox();
        const lineBox = await finalLine.boundingBox();
        expect(clippedDockBox).not.toBeNull();
        expect(lineBox).not.toBeNull();
        if (!clippedDockBox || !lineBox) throw new Error("Expected clipped final-line geometry");
        expect(lineBox.y + lineBox.height).toBeLessThanOrEqual(clippedDockBox.y + 1);
        await testPage.screenshot({
          path: testInfo.outputPath(`${provider}-selection-comment-dock.png`),
        });
        await restoreSelectionCommentVisualViewport(testPage);
        await expectDockWithinEditor(dock, owner);

        await action.tap();
        if (provider === "codemirror") {
          const textarea = testPage.getByPlaceholder("Add your comment or instruction...");
          const popover = testPage.locator("div.fixed.z-50").filter({ has: textarea }).first();
          await expect(popover).toBeVisible();
          await expect(popover.locator("pre")).toContainText(CODE_SELECTION_QUOTE);
          await expect(popover).toContainText(`${CODE_SELECTION_LINE}`);
          await textarea.fill(COMMENT_TEXT);
          await popover.getByRole("button", { name: "Add", exact: true }).tap();
        } else {
          const form = owner.locator("[data-comment-zone]").last();
          const textarea = form.locator("textarea");
          await expect(form).toBeVisible();
          await textarea.fill(COMMENT_TEXT);
          const addButton = form.locator("[data-comment-actions] button").nth(1);
          await expect(addButton).toBeVisible();
          await addButton.scrollIntoViewIfNeeded();
          await addButton.click();
        }

        await expectStoredCodeComment({
          page: testPage,
          sessionId,
          fileName,
          repositoryName,
          commentText: COMMENT_TEXT,
        });
        expect(fs.readFileSync(diskPath, "utf8")).toContain(CODE_SELECTION_QUOTE);
        await testPage.getByRole("button", { name: "Chat", exact: true }).tap();
        await expect(
          session.activeChat().getByText(`${fileName} (1)`, { exact: true }),
        ).toBeVisible();
      } finally {
        await restoreSelectionCommentVisualViewport(testPage);
        fs.rmSync(diskPath, { force: true });
        await restoreCodeEditorProvider(testPage);
      }
    });
  }
});
