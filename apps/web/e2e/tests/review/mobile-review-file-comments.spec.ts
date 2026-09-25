import { test, expect } from "../../fixtures/test-base";
import {
  seedReviewTask,
  loadSession,
  openDialogWithChanges,
} from "./review-fix-comments-popover-flow";
import { watchWs } from "../../helpers/causal-waits";
import { exerciseFileComment, openFileComment } from "./review-file-comments-flow";

test.setTimeout(120_000);

test("phone file menu creates editable whole-file feedback", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const ws = watchWs(testPage);
  const task = await seedReviewTask(testPage, apiClient, seedData);
  await loadSession(testPage, task.id);
  const dialog = await openDialogWithChanges(testPage);
  await exerciseFileComment(testPage, dialog, true);
  const phoneViewport = testPage.viewportSize()!;
  for (const width of [767, 768]) {
    await dialog.getByRole("button", { name: "Close review", exact: true }).click();
    await testPage.setViewportSize({ width, height: phoneViewport.height });
    await expect(
      testPage.getByTestId(width < 768 ? "mobile-task-layout" : "tablet-task-layout"),
    ).toBeVisible();
    await testPage.evaluate(() => window.dispatchEvent(new CustomEvent("open-review-dialog")));
    await expect(dialog).toBeVisible();
    const region = await openFileComment(testPage, dialog, width < 768);
    const cancel = region.getByRole("button", { name: "Cancel", exact: true });
    expect((await cancel.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await cancel.click();
    expect(await testPage.locator("html").evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(
      true,
    );
  }
  await dialog.getByRole("button", { name: "Close review", exact: true }).click();
  await testPage.setViewportSize(phoneViewport);
  await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();
  await testPage.evaluate(() => window.dispatchEvent(new CustomEvent("open-review-dialog")));
  await expect(dialog).toBeVisible();
  await testPage.screenshot({ path: test.info().outputPath("file-comments.png") });
  await dialog.getByTestId("review-fix-comments-button").tap();
  const overview = testPage.getByTestId("review-comments-overview");
  await expect(overview).toContainText("File comment");
  await expect(overview).toContainText("Updated whole-file feedback");
  const sent = ws.waitForResponse("message.add");
  await dialog.getByTestId("review-fix-comments-button").tap();
  await sent;
  await expect(dialog).not.toBeVisible();
  await expect
    .poll(async () =>
      (await apiClient.listSessionMessages(task.session_id!)).messages.some(
        (message) =>
          message.author_type === "user" &&
          message.content.includes("Updated whole-file feedback") &&
          !message.content.includes(":undefined"),
      ),
    )
    .toBe(true);
});
