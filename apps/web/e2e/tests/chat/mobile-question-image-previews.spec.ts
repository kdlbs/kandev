import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { exerciseChoiceImageQuestion } from "./choice-image-helpers";
import {
  exerciseImageCollapse,
  inspectAndAnswerImages,
  openImageQuestions,
  seedQuestionImage,
} from "./question-image-helpers";

test("phone choice images inspect without selecting", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(180_000);
  await exerciseChoiceImageQuestion(testPage, apiClient, seedData, true, prCapture);
});

test("phone inspection preserves the question draft and all answer controls", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  seedQuestionImage(seedData.repositoryPath);
  const { session } = await openImageQuestions(testPage, apiClient, seedData);
  const overlay = session.clarificationOverlay();
  await expect(overlay.getByText("Approve this layout?", { exact: true })).toBeInViewport({
    ratio: 1,
  });
  const thumbnail = await overlay
    .getByRole("img", { name: "Proposed layout", exact: true })
    .boundingBox();
  expect(thumbnail!.height).toBeLessThanOrEqual(96);
  await prCapture.screenshot("phone-question-images", {
    caption: "Workspace image beside a blocking question",
  });
  if (prCapture.capturing) {
    await session
      .clarificationOverlay()
      .getByTestId("clarification-input")
      .scrollIntoViewIfNeeded();
    await prCapture.screenshot("phone-question-answer-controls", {
      caption: "The same clarification card scrolled to its choices and text answer",
    });
  }
  await inspectAndAnswerImages(testPage, session, true, () =>
    prCapture.screenshot("phone-question-image-viewer", {
      caption: "Image inspection preserves the answer draft",
    }),
  );
});

test("phone collapse releases previews and preserves the answer draft", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  seedQuestionImage(seedData.repositoryPath);
  const { session } = await openImageQuestions(testPage, apiClient, seedData);
  await exerciseImageCollapse(testPage, session, true);
  await prCapture.screenshot("phone-question-images-reopened", {
    caption: "Reopened clarification reloads its image and keeps the answer draft",
  });
  const skipped = waitForHttp(testPage, "POST", /\/api\/v1\/clarification\/[^/]+\/respond$/, {
    predicate: (response) => response.ok(),
  });
  await session.clarificationSkip().click();
  await skipped;
  await expect(session.idleInput()).toBeVisible();
});
