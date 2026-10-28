import { expect, test } from "../../fixtures/test-base";
import { seedReviewThread, ROOT_TEXT, REPLY_TEXT } from "../../helpers/review-thread-context";
import { assertNoDocumentHorizontalOverflow, requireBox } from "../../helpers/layout-assertions";

test("adds a mixed review thread to pending context on phone", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  const task = await seedReviewThread(apiClient, seedData);
  await testPage.goto(`/t/${task.id}`);
  await testPage.getByRole("button", { name: "Review", exact: true }).tap();
  const detail = testPage.getByTestId("mobile-review-panel").getByTestId("change-request-detail");
  await expect(detail.getByText(ROOT_TEXT)).toBeVisible();
  await expect(detail.getByText(REPLY_TEXT)).toHaveCount(0);
  const action = detail.getByTestId("change-request-thread-context-8211").getByRole("button");
  await expect(action).toBeVisible();
  const box = await requireBox(action, "whole thread context action");
  expect(box.width).toBeGreaterThanOrEqual(44);
  expect(box.height).toBeGreaterThanOrEqual(44);
  await prCapture.screenshot("mobile-review-thread-context", {
    caption: "Phone review thread context action",
  });
  await action.tap();
  await testPage.getByRole("button", { name: "Chat", exact: true }).tap();
  const chip = testPage.getByRole("button", { name: "1 PR feedback", exact: true });
  await expect(chip).toBeVisible();
  await chip.tap();
  const drawer = testPage.getByRole("dialog");
  await expect(drawer.getByText(ROOT_TEXT, { exact: false })).toBeVisible();
  await expect(drawer.getByText(REPLY_TEXT, { exact: false })).toBeVisible();
  await assertNoDocumentHorizontalOverflow(testPage, "mobile review thread context");
});
