import { test, expect } from "../../fixtures/test-base";
import {
  openLateAnswerForm,
  openReviewQuestions,
  removeReviewImages,
  reviewImages,
  reviewScript,
  seedReviewImages,
} from "./question-image-review-helpers";

test.afterEach(({ seedData }) => removeReviewImages(seedData.repositoryPath));

test("reviews images from the late-answer form and sends a new message", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(180_000);
  seedReviewImages(seedData.repositoryPath);
  const { task, session } = await openReviewQuestions(
    testPage,
    apiClient,
    seedData,
    reviewScript(),
  );
  const sessionId = task.session_id!;
  const form = await openLateAnswerForm(testPage, session);

  const open = form.getByTestId("question-review-open");
  await expect(open).toBeVisible();
  await open.click();
  const dialog = testPage.getByTestId("question-review-dialog");
  await expect(dialog.getByRole("img", { name: reviewImages.before.alt })).toBeVisible();
  const submit = dialog.getByTestId("question-review-submit");
  await expect(submit).toHaveText(/Send as new message/);
  await expect(submit).toBeDisabled();

  // Escape closes the view without dismissing the late-answer form.
  await dialog.getByTestId("question-review-stage").focus();
  await testPage.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(form.getByTestId("clarification-late-submit")).toBeVisible();

  await open.click();
  const card = dialog.getByTestId("clarification-question-card");
  await card.getByTestId("clarification-option").first().click();
  await dialog.getByTestId("clarification-next").click();
  await card.getByTestId("clarification-option").nth(1).click();
  await expect(submit).toBeEnabled();
  await prCapture.screenshot("desktop-question-review-late", {
    caption: "Review images from the late-answer form",
  });
  await submit.click();

  await expect(dialog).toHaveCount(0);
  await expect(testPage.getByTestId("clarification-late-success")).toBeVisible();
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        return messages.some(
          (message) => message.author_type === "user" && message.content.includes("Approve"),
        );
      },
      { timeout: 30_000, message: "late answer from the review view should be a new message" },
    )
    .toBe(true);
});
