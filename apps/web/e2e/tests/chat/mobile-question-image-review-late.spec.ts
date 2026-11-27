import { test, expect } from "../../fixtures/test-base";
import {
  expectNoHorizontalOverflow,
  expectTouchTargets,
  openLateAnswerForm,
  openReviewQuestions,
  removeReviewImages,
  reviewImages,
  reviewScript,
  seedReviewImages,
} from "./question-image-review-helpers";

test.afterEach(({ seedData }) => removeReviewImages(seedData.repositoryPath));

test("phone late-answer form opens review and sends from it", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  seedReviewImages(seedData.repositoryPath);
  await testPage.setViewportSize({ width: 320, height: 700 });
  const { session } = await openReviewQuestions(testPage, apiClient, seedData, reviewScript());
  const form = await openLateAnswerForm(testPage, session);
  // Review, Close, and Send stay on screen at 320px before the view opens.
  for (const id of [
    "question-review-open",
    "clarification-late-close",
    "clarification-late-submit",
  ]) {
    const box = (await form.getByTestId(id).boundingBox())!;
    expect(box.x, id).toBeGreaterThanOrEqual(0);
    expect(box.x + box.width, id).toBeLessThanOrEqual(320.5);
  }
  await expectNoHorizontalOverflow(testPage);

  const open = form.getByTestId("question-review-open");
  const openBox = (await open.boundingBox())!;
  expect(openBox.height).toBeGreaterThanOrEqual(44);
  await open.click();
  const dialog = testPage.getByTestId("question-review-dialog");
  await expect(dialog.getByRole("img", { name: reviewImages.before.alt })).toBeVisible();
  const submit = dialog.getByTestId("question-review-submit");
  await expect(submit).toHaveText(/Send as new message/);
  await expectTouchTargets(dialog.getByTestId("question-review-bottom-bar"));
  await expectNoHorizontalOverflow(testPage);

  const card = dialog.getByTestId("clarification-question-card");
  await card.getByTestId("clarification-option").first().click();
  await dialog.getByTestId("clarification-next").click();
  await card.getByTestId("clarification-option").first().click();
  await submit.click();
  await expect(dialog).toHaveCount(0);
  await expect(testPage.getByTestId("clarification-late-success")).toBeVisible();
});
