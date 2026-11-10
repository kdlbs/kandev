import { test, expect } from "../../fixtures/test-base";
import {
  CHANGED_PIXELS,
  chooseZoom,
  expectThumbnailsShareRow,
  REVIEW_IMAGE_SIZE,
  expectNoHorizontalOverflow,
  openReviewQuestions,
  recordImageRequests,
  reviewImages,
  reviewScript,
  seedReviewImages,
} from "./question-image-review-helpers";

test("reviews images in a large view that shares the answer form", async ({
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
  const overlay = session.clarificationOverlay();
  await expectThumbnailsShareRow(overlay);
  const requested = recordImageRequests(testPage);
  const open = overlay.getByTestId("question-review-open");
  await open.click();

  const dialog = testPage.getByTestId("question-review-dialog");
  await expect(dialog).toBeVisible();
  await expect(overlay.getByTestId("question-review-placeholder")).toBeVisible();
  const stage = dialog.getByTestId("question-review-stage");
  const card = dialog.getByTestId("clarification-question-card");
  await expect(dialog.getByRole("img", { name: reviewImages.before.alt })).toBeVisible();
  await expect(stage).toBeInViewport();
  await expect(card.getByText("Is the new header spacing acceptable?")).toBeInViewport();
  expect((await stage.boundingBox())!.y).toBeLessThan((await card.boundingBox())!.y);
  expect(requested).not.toContain(reviewImages.spacing.id);
  await prCapture.screenshot("desktop-question-review", {
    caption: "Large image review above the question",
  });

  await card.getByTestId("clarification-input").fill("Tighten the header.");
  await stage.focus();
  await stage.press("2");
  await stage.press("ArrowRight");
  await expect(card).toHaveAttribute("data-question-id", "header");
  await expect(card.getByTestId("clarification-option").first()).not.toHaveAttribute(
    "data-selected",
    "true",
  );

  await chooseZoom(testPage, dialog, "question-review-actual");
  await expect(dialog.getByTestId("question-review-zoom-level")).toHaveText("100%");
  await stage.press("+");
  await expect(dialog.getByTestId("question-review-zoom-level")).toHaveText("200%");
  await expect(dialog.getByRole("img", { name: reviewImages.before.alt })).toHaveCSS(
    "image-rendering",
    "pixelated",
  );

  await stage.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(open).toBeFocused();
  await expect(overlay.getByTestId("clarification-input")).toHaveValue("Tighten the header.");

  await open.click();
  const segments = dialog.getByTestId("question-review-segment");
  await expect(segments).toHaveCount(2);
  await segments.nth(1).click();
  await expect(dialog.getByRole("img", { name: reviewImages.spacing.alt })).toBeVisible();
  expect(requested).toContain(reviewImages.spacing.id);
  await expect(card).toHaveAttribute("data-question-id", "spacing");
  await card.getByTestId("clarification-option").nth(1).click();
  await segments.nth(0).click();
  await expect(card.getByTestId("clarification-input")).toHaveValue("Tighten the header.");
  await dialog.getByTestId("question-review-submit").click();
  await expect(dialog).toHaveCount(0);
  await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
  await expect(session.chat).toContainText("Review questions complete.");
  await expectNoHorizontalOverflow(testPage);

  const { messages } = await apiClient.listSessionMessages(task.session_id!);
  const questions = messages.filter((message) => message.type === "clarification_request");
  expect(questions.every((message) => message.metadata?.status === "answered")).toBe(true);
});

test("compares a baseline and a candidate in all five modes", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(180_000);
  seedReviewImages(seedData.repositoryPath);
  const { session } = await openReviewQuestions(testPage, apiClient, seedData, reviewScript());
  await session.clarificationOverlay().getByTestId("question-review-open").click();
  const dialog = testPage.getByTestId("question-review-dialog");
  const modes = dialog.getByTestId("question-review-modes");
  await expect(dialog.getByRole("img", { name: reviewImages.before.alt })).toBeVisible();
  const requested = recordImageRequests(testPage);

  await dialog.getByTestId("question-review-flip").click();
  await expect(dialog.getByRole("img", { name: reviewImages.after.alt })).toBeVisible();
  await expect(dialog.getByRole("img", { name: reviewImages.before.alt })).toBeHidden();

  await chooseZoom(testPage, dialog, "question-review-actual");
  await dialog.getByTestId("question-review-zoom-in").click();
  const level = dialog.getByTestId("question-review-zoom-level");
  await expect(level).toHaveText("200%");
  for (const mode of ["Wipe", "Side by side", "Onion skin", "Toggle"]) {
    await modes.getByText(mode, { exact: true }).click();
    await expect(level).toHaveText("200%");
  }

  await modes.getByText("Wipe", { exact: true }).click();
  const handle = dialog.getByTestId("question-review-wipe-handle");
  await handle.focus();
  await handle.press("ArrowRight");
  await expect(handle).toHaveAttribute("aria-valuenow", "55");
  await expect(dialog.getByTestId("clarification-question-card")).toHaveAttribute(
    "data-question-id",
    "header",
  );

  await modes.getByText("Side by side", { exact: true }).click();
  await expect(dialog.getByTestId("question-review-stage-b")).toBeVisible();

  await modes.getByText("Difference", { exact: true }).click();
  await expect(dialog.getByTestId("question-review-diff-status")).toHaveText(
    `${CHANGED_PIXELS} pixels changed (0.25%)`,
  );
  await expect(dialog.getByTestId("question-review-diff-mask")).toBeVisible();
  await prCapture.screenshot("desktop-question-review-difference", {
    caption: "Difference mode highlights the changed pixels",
  });
  expect(requested).toEqual([]);
  await expectNoHorizontalOverflow(testPage);
});

test("reports different image sizes and counts pixels outside the shared area", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  seedReviewImages(seedData.repositoryPath);
  const { session } = await openReviewQuestions(
    testPage,
    apiClient,
    seedData,
    reviewScript([reviewImages.before, reviewImages.taller]),
  );
  await session.clarificationOverlay().getByTestId("question-review-open").click();
  const dialog = testPage.getByTestId("question-review-dialog");
  const { width, height } = REVIEW_IMAGE_SIZE;
  await expect(dialog.getByTestId("question-review-sizes-differ")).toHaveText(
    `Sizes differ: A ${width} × ${height} px, B ${width} × ${height + 4} px`,
  );
  await dialog
    .getByTestId("question-review-modes")
    .getByText("Difference", { exact: true })
    .click();
  await expect(dialog.getByTestId("question-review-diff-status")).toContainText(
    `${width * 4} pixels changed`,
  );
});

test("marks a region on an image and delivers it with the answer", async ({
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
  await session.clarificationOverlay().getByTestId("question-review-open").click();
  const dialog = testPage.getByTestId("question-review-dialog");
  await expect(dialog.getByRole("img", { name: reviewImages.before.alt })).toBeVisible();

  await dialog.getByTestId("question-review-flip").click();
  await dialog.getByTestId("question-review-annotate").click();
  const stage = dialog.getByTestId("question-review-stage");
  const box = (await stage.boundingBox())!;
  await testPage.mouse.move(box.x + box.width * 0.3, box.y + box.height * 0.3);
  await testPage.mouse.down();
  await testPage.mouse.move(box.x + box.width * 0.6, box.y + box.height * 0.6, { steps: 4 });
  await testPage.mouse.up();
  await expect(dialog.getByTestId("question-review-region")).toHaveCount(1);
  const popover = dialog.getByTestId("question-review-region-popover");
  await expect(popover).toBeVisible();
  const region = (await dialog.getByTestId("question-review-region").boundingBox())!;
  const popoverBox = (await popover.boundingBox())!;
  expect(Math.abs(popoverBox.y - (region.y + region.height))).toBeLessThan(260);
  await expect(dialog.getByTestId("question-review-region-input")).toBeFocused();
  await dialog.getByTestId("question-review-region-input").fill("Label contrast too low");
  await dialog.getByTestId("question-review-region-add").click();
  await expect(popover).toHaveCount(0);
  const item = dialog.getByTestId("question-review-region-item");
  await expect(item).toContainText(reviewImages.after.alt);
  await expect(item.getByTestId("question-review-region-text")).toHaveText(
    "Label contrast too low",
  );
  await prCapture.screenshot("desktop-question-review-region", {
    caption: "A marked region with its comment",
  });

  const card = dialog.getByTestId("clarification-question-card");
  await card.getByTestId("clarification-option").nth(1).click();
  await expect(card).toHaveAttribute("data-question-id", "header");
  await expect(dialog.getByTestId("clarification-next")).toHaveAttribute("data-emphasized", "true");
  await dialog.getByTestId("clarification-next").click();
  await expect(card).toHaveAttribute("data-question-id", "spacing");
  await card.getByTestId("clarification-option").first().click();
  await dialog.getByTestId("question-review-submit").click();
  await expect(dialog).toHaveCount(0);
  await expect(session.chat).toContainText("Review questions complete.", { timeout: 30_000 });

  const { messages } = await apiClient.listSessionMessages(task.session_id!);
  const header = messages.find(
    (message) =>
      message.type === "clarification_request" && message.metadata?.question_id === "header",
  );
  const annotations = (header?.metadata?.response as { annotations?: unknown[] })?.annotations;
  expect(annotations).toHaveLength(1);
  expect(annotations![0]).toMatchObject({
    image_id: reviewImages.after.id,
    image_width: REVIEW_IMAGE_SIZE.width,
    image_height: REVIEW_IMAGE_SIZE.height,
    comment: "Label contrast too low",
  });
  await expect(session.chat.getByTestId("clarification-answer-regions").first()).toContainText(
    `Region 1 on ${reviewImages.after.alt}: Label contrast too low`,
  );
});

test("steps candidates, keeps the mode, zooms to changes, and holds to compare", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  seedReviewImages(seedData.repositoryPath);
  const { session } = await openReviewQuestions(
    testPage,
    apiClient,
    seedData,
    reviewScript(
      [reviewImages.before, reviewImages.after, reviewImages.taller],
      [reviewImages.before, reviewImages.after],
    ),
  );
  await session.clarificationOverlay().getByTestId("question-review-open").click();
  const dialog = testPage.getByTestId("question-review-dialog");
  await expect(dialog.getByRole("img", { name: reviewImages.before.alt })).toBeVisible();
  await expect(dialog.getByTestId("question-review-pick-b")).toContainText("after");
  await dialog.getByTestId("question-review-pick-a").click();
  const optionsA = testPage.getByTestId("question-review-pick-a-option");
  await expect(optionsA).toHaveCount(3);
  await expect(optionsA.nth(1)).toContainText(reviewImages.after.alt);
  await expect(optionsA.nth(1)).toContainText("In B");
  await expect(optionsA.nth(0).locator("img")).toBeVisible();
  await testPage.keyboard.press("Escape");
  await expect(optionsA).toHaveCount(0);

  const stepLabel = dialog.getByTestId("question-review-step-label");
  await expect(stepLabel).toHaveText("B: after (1 of 2)");
  await dialog.getByTestId("question-review-step-next").click();
  await expect(stepLabel).toHaveText("B: taller (2 of 2)");
  await expect(dialog.getByRole("img", { name: reviewImages.taller.alt })).toBeVisible();
  const stage = dialog.getByTestId("question-review-stage");
  await stage.focus();
  await stage.press("[");
  await expect(stepLabel).toHaveText("B: after (1 of 2)");

  await stage.focus();
  await testPage.keyboard.down(" ");
  await expect(dialog.getByRole("img", { name: reviewImages.before.alt })).toBeVisible();
  await expect(dialog.getByRole("img", { name: reviewImages.after.alt })).toBeHidden();
  await testPage.keyboard.up(" ");
  await expect(dialog.getByRole("img", { name: reviewImages.after.alt })).toBeVisible();

  const level = dialog.getByTestId("question-review-zoom-level");
  const fitLevel = Number.parseInt((await level.textContent())!, 10);
  await dialog.getByTestId("question-review-zoom-changes").click();
  await expect
    .poll(async () => Number.parseInt((await level.textContent())!, 10))
    .toBeGreaterThan(fitLevel);
  await expect(dialog.getByTestId("question-review-change-outline")).toBeVisible();
  await expect(dialog.getByTestId("question-review-change-outline")).toHaveCount(0);

  await dialog
    .getByTestId("question-review-modes")
    .getByText("Difference", { exact: true })
    .click();
  await expect(dialog.getByTestId("question-review-diff-status")).toHaveText(
    `${CHANGED_PIXELS} pixels changed (0.25%)`,
  );
  await expect(dialog.getByTestId("question-review-change-outline")).toBeVisible();

  await dialog.getByTestId("question-review-segment").nth(1).click();
  await expect(dialog.getByTestId("clarification-question-card")).toHaveAttribute(
    "data-question-id",
    "spacing",
  );
  await expect(dialog.getByTestId("question-review-tolerance")).toBeVisible();
  await expect(dialog.getByTestId("question-review-diff-status")).toHaveText(
    `${CHANGED_PIXELS} pixels changed (0.25%)`,
  );
});

test("marking is obvious, regions open on click, and Escape leaves marking first", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  seedReviewImages(seedData.repositoryPath);
  const { session } = await openReviewQuestions(testPage, apiClient, seedData, reviewScript());
  await session.clarificationOverlay().getByTestId("question-review-open").click();
  const dialog = testPage.getByTestId("question-review-dialog");
  await expect(dialog.getByRole("img", { name: reviewImages.before.alt })).toBeVisible();
  const mark = dialog.getByTestId("question-review-annotate");
  await expect(mark).toHaveText("Mark area");
  await mark.click();
  await expect(mark).toHaveAttribute("aria-pressed", "true");
  await expect(mark).toHaveText("Marking areas");
  await expect(dialog.getByTestId("question-review-annotate-hint")).toBeVisible();
  await expect(dialog.getByTestId("question-review-regions-panel")).toBeVisible();

  const stage = dialog.getByTestId("question-review-stage");
  const box = (await stage.boundingBox())!;
  await testPage.mouse.move(box.x + box.width * 0.35, box.y + box.height * 0.35);
  await testPage.mouse.down();
  await testPage.mouse.move(box.x + box.width * 0.55, box.y + box.height * 0.6, { steps: 4 });
  await testPage.mouse.up();
  await dialog.getByTestId("question-review-region-input").fill("Label too faint");
  await dialog.getByTestId("question-review-region-add").click();
  await testPage.keyboard.press("Escape");
  await expect(mark).toHaveAttribute("aria-pressed", "false");
  await expect(dialog).toBeVisible();

  await dialog.getByTestId("question-review-region").click();
  await expect(dialog.getByTestId("question-review-region-input")).toHaveValue("Label too faint");
  await testPage.keyboard.press("Escape");
  await expect(dialog.getByTestId("question-review-region-popover")).toHaveCount(0);
  await expect(dialog).toBeVisible();
  await testPage.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
});

test("the thumbnail inspector steps between a question's images", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  seedReviewImages(seedData.repositoryPath);
  const { session } = await openReviewQuestions(
    testPage,
    apiClient,
    seedData,
    reviewScript([reviewImages.before, reviewImages.after, reviewImages.taller]),
  );
  const overlay = session.clarificationOverlay();
  const second = overlay.getByRole("button", { name: new RegExp(reviewImages.after.alt) });
  await second.click();
  const viewer = testPage.getByRole("dialog");
  const counter = viewer.getByTestId("question-image-counter");
  await expect(counter).toHaveText("after, 2 of 3");
  const canvasBox = (await viewer.getByTestId("question-image-canvas").boundingBox())!;
  const nextBox = (await viewer.getByTestId("question-image-next").boundingBox())!;
  expect(nextBox.x + nextBox.width).toBeLessThanOrEqual(canvasBox.x + canvasBox.width + 1);
  expect(nextBox.x).toBeGreaterThan(canvasBox.x + canvasBox.width / 2);
  await viewer.getByTestId("question-image-next").click();
  await expect(counter).toHaveText("taller, 3 of 3");
  await expect(viewer.getByTestId("question-image-next")).toBeDisabled();
  await viewer.getByTestId("question-image-canvas").focus();
  await testPage.keyboard.press("ArrowLeft");
  await testPage.keyboard.press("ArrowLeft");
  await expect(counter).toHaveText("before, 1 of 3");
  await testPage.keyboard.press("Escape");
  await expect(viewer).toHaveCount(0);
  await expect(
    overlay.getByRole("button", { name: new RegExp(reviewImages.before.alt) }),
  ).toBeFocused();
});
