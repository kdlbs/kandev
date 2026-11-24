import { expect, test } from "../../fixtures/test-base";
import { seedReviewThread, ROOT_TEXT, REPLY_TEXT } from "../../helpers/review-thread-context";
import { SessionPage } from "../../pages/session-page";

test("adds a mixed review thread to pending context on desktop", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  const task = await seedReviewThread(apiClient, seedData);
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.prTopbarButton()).toBeVisible();
  await session.prTopbarButton().click();
  const detail = session.prDetailPanel().getByTestId("change-request-detail");
  await expect(detail.getByText(ROOT_TEXT)).toBeVisible();
  await expect(detail.getByText(REPLY_TEXT)).toHaveCount(0);
  const action = detail.getByTestId("change-request-thread-context-8211").getByRole("button");
  await expect(action).toBeVisible();
  await prCapture.screenshot("desktop-review-thread-context", {
    caption: "Desktop review thread context action",
  });
  await action.click();
  await session.clickSessionChatTab();
  const chip = session.activeChat().getByRole("button", { name: "1 PR feedback", exact: true });
  await expect(chip).toBeVisible();
  await chip.click();
  await expect(testPage.getByText(ROOT_TEXT, { exact: false }).last()).toBeVisible();
  await expect(testPage.getByText(REPLY_TEXT, { exact: false })).toBeVisible();
});
