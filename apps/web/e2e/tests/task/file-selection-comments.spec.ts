import fs from "node:fs";
import { test, expect } from "../../fixtures/test-base";
import {
  CODE_SELECTION_QUOTE,
  CODE_SELECTION_SENTINEL,
  expectStoredCodeComment,
  openCodeFileForSelectionComment,
  restoreCodeEditorProvider,
  selectFinalCodeLine,
  type CodeEditorProvider,
} from "../../helpers/file-selection-comments";

const COMMENT_TEXT = "Clarify the final line.";

test.describe("File selection comments", () => {
  for (const provider of ["codemirror", "monaco"] as const satisfies CodeEditorProvider[]) {
    test(`${provider} keeps desktop placement and adds the expected final-line comment`, async ({
      testPage,
      apiClient,
      seedData,
    }) => {
      test.setTimeout(90_000);
      const fileName = `selection-comment-${provider}-${Date.now()}.ts`;
      const { session, sessionId, diskPath, repositoryName, owner } =
        await openCodeFileForSelectionComment({
          page: testPage,
          apiClient,
          seedData,
          fileName,
          title: `${provider} selection comment`,
          provider,
        });
      try {
        await selectFinalCodeLine({ page: testPage, owner, provider, touch: false });
        const action = testPage.locator(".floating-comment-btn");
        await expect(action).toBeVisible({ timeout: 5_000 });
        await expect(testPage.getByTestId("selection-comment-dock")).toHaveCount(0);
        await expect(action).toContainText("Comment");
        if (provider === "codemirror") {
          await expect(owner.getByText(CODE_SELECTION_SENTINEL).last()).toBeVisible();
        }
        await action.click();

        if (provider === "codemirror") {
          const textarea = testPage.getByPlaceholder("Add your comment or instruction...");
          const popover = testPage.locator("div.fixed.z-50").filter({ has: textarea }).first();
          await expect(popover.locator("pre")).toContainText(CODE_SELECTION_QUOTE);
          await textarea.fill(COMMENT_TEXT);
          await popover.getByRole("button", { name: "Add", exact: true }).click();
        } else {
          const form = owner.locator("[data-comment-zone]").last();
          await expect(form).toBeVisible();
          await form.locator("textarea").fill(COMMENT_TEXT);
          await form.locator("[data-comment-actions] button").nth(1).click();
        }

        await expectStoredCodeComment({
          page: testPage,
          sessionId,
          fileName,
          repositoryName,
          commentText: COMMENT_TEXT,
        });
        expect(fs.readFileSync(diskPath, "utf8")).toContain(CODE_SELECTION_QUOTE);
        await session.clickSessionChatTab();
        await expect(
          session.activeChat().getByText(`${fileName} (1)`, { exact: true }),
        ).toBeVisible();
      } finally {
        fs.rmSync(diskPath, { force: true });
        await restoreCodeEditorProvider(testPage);
      }
    });
  }
});
