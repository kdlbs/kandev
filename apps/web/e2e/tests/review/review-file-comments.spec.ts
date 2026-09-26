import { test, expect } from "../../fixtures/test-base";
import {
  seedReviewTask,
  loadSession,
  openDialogWithChanges,
} from "./review-fix-comments-popover-flow";
import { exerciseFileComment } from "./review-file-comments-flow";

test.setTimeout(120_000);

test("whole-file review feedback survives reload and can be deleted", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await seedReviewTask(testPage, apiClient, seedData);
  await loadSession(testPage, task.id);
  let dialog = await openDialogWithChanges(testPage);
  await exerciseFileComment(testPage, dialog, false);
  await testPage.screenshot({ path: test.info().outputPath("file-comments.png") });
  await testPage.reload();
  await loadSession(testPage, task.id);
  dialog = await openDialogWithChanges(testPage);
  const card = dialog.getByTestId("review-file-comment-card");
  await expect(card).toContainText("Updated whole-file feedback");
  await card.getByRole("button", { name: "Delete comment", exact: true }).click();
  await expect(card).toHaveCount(0);
  await expect(dialog.getByTestId("review-fix-comments-button")).toHaveCount(0);
});
