import { test, expect } from "../../fixtures/test-base";
import {
  choosePhoneMode,
  expectNoHorizontalOverflow,
  expectThumbnailsShareRow,
  expectTouchTargets,
  openPhoneMore,
  openReviewQuestions,
  reviewImages,
  reviewScript,
  seedReviewImages,
  touchDrag,
} from "./question-image-review-helpers";

const threeImages = [reviewImages.before, reviewImages.after, reviewImages.taller];

test("phone review uses a top row, the image, the form, and a thumb-zone bar", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(180_000);
  seedReviewImages(seedData.repositoryPath);
  const { session } = await openReviewQuestions(
    testPage,
    apiClient,
    seedData,
    reviewScript(threeImages),
  );
  const overlay = session.clarificationOverlay();
  await expectThumbnailsShareRow(overlay);
  await overlay.getByTestId("question-review-open").tap();

  const dialog = testPage.getByTestId("question-review-dialog");
  const viewport = testPage.viewportSize()!;
  expect((await dialog.boundingBox())!.height).toBeGreaterThan(viewport.height - 3);
  const stage = dialog.getByTestId("question-review-stage");
  const card = dialog.getByTestId("clarification-question-card");
  const bar = dialog.getByTestId("question-review-bottom-bar");
  await expect(dialog.getByRole("img", { name: reviewImages.before.alt })).toBeVisible();
  await expect(card.getByText("Is the new header spacing acceptable?")).toBeInViewport();
  const stageBox = (await stage.boundingBox())!;
  const barBox = (await bar.boundingBox())!;
  expect(stageBox.y).toBeLessThan((await card.boundingBox())!.y);
  expect(barBox.y + barBox.height).toBeGreaterThan(viewport.height - 60);
  await expectTouchTargets(bar);
  await expectNoHorizontalOverflow(testPage);
  await prCapture.screenshot("phone-question-review", {
    caption: "Phone image review with frequent actions in the bottom bar",
  });

  await bar.getByTestId("question-review-flip").tap();
  await expect(dialog.getByRole("img", { name: reviewImages.after.alt })).toBeVisible();
  await bar.getByTestId("question-review-step-next").tap();
  await expect(dialog.getByRole("img", { name: reviewImages.taller.alt })).toBeVisible();
  await expect(bar.getByTestId("question-review-picker-button")).toContainText(
    "A: before, B: taller",
  );
  await bar.getByTestId("question-review-picker-button").tap();
  const picker = testPage.getByTestId("question-review-picker-sheet");
  const optionB = picker.getByTestId("question-review-pick-b-option").nth(1);
  await expect(optionB).toContainText(reviewImages.after.alt);
  await expect(optionB.locator("img")).toBeVisible();
  expect((await optionB.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await optionB.tap();
  await testPage.keyboard.press("Escape");
  await expect(bar.getByTestId("question-review-picker-button")).toContainText("B: after");

  const chip = dialog.getByTestId("question-review-zoom-chip");
  const fitLevel = await chip.textContent();
  await chip.tap();
  await expect(chip).toHaveText("100%");
  await chip.tap();
  await expect(chip).toHaveText(fitLevel!);

  await card.getByTestId("clarification-input").tap();
  await expect(bar).toBeHidden();
  await card.getByTestId("clarification-input").blur();
  await expect(bar).toBeVisible();

  await card.getByTestId("clarification-option").first().tap();
  await expect(card).toHaveAttribute("data-question-id", "header");
  await dialog.getByTestId("clarification-next").tap();
  await card.getByTestId("clarification-option").first().tap();
  await dialog.getByTestId("question-review-submit").tap();
  await expect(dialog).toHaveCount(0);
  await expect(session.chat).toContainText("Review questions complete.", { timeout: 30_000 });
});

test("phone wipe pins its line and scrubs the images; Side by side fits", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  seedReviewImages(seedData.repositoryPath);
  const { session } = await openReviewQuestions(testPage, apiClient, seedData, reviewScript());
  await session.clarificationOverlay().getByTestId("question-review-open").tap();
  const dialog = testPage.getByTestId("question-review-dialog");
  await expect(dialog.getByRole("img", { name: reviewImages.before.alt })).toBeVisible();
  await choosePhoneMode(testPage, dialog, "Wipe");
  const handle = dialog.getByTestId("question-review-wipe-handle");
  await expect(handle).toHaveAttribute("data-pinned", "true");
  await expect(handle).toHaveAttribute("aria-valuenow", "50");
  const stage = dialog.getByTestId("question-review-stage");
  const box = (await stage.boundingBox())!;
  const center = box.x + box.width / 2;
  const width = testPage.viewportSize()!.width;
  await touchDrag(testPage, stage, center + Math.min(box.width / 2, width / 2 - 60));
  await touchDrag(testPage, stage, center + Math.min(box.width / 2, width / 2 - 60));
  await expect(handle).toHaveAttribute("aria-valuenow", "0");
  for (let attempt = 0; attempt < 3; attempt++) {
    await touchDrag(testPage, stage, center - Math.min(box.width / 2, width / 2 - 60));
  }
  await expect(handle).toHaveAttribute("aria-valuenow", "100");

  await choosePhoneMode(testPage, dialog, "Side by side");
  for (const id of ["question-review-stage", "question-review-stage-b"]) {
    const pane = dialog.getByTestId(id);
    const paneBox = (await pane.boundingBox())!;
    const image = (await pane.getByTestId("question-review-image").boundingBox())!;
    expect(image.y).toBeGreaterThanOrEqual(paneBox.y - 1);
    expect(image.y + image.height).toBeLessThanOrEqual(paneBox.y + paneBox.height + 1);
  }
  await expectNoHorizontalOverflow(testPage);
});

test("phone regions: mark from More, comment in a drawer, review in a sheet", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  seedReviewImages(seedData.repositoryPath);
  const { session } = await openReviewQuestions(testPage, apiClient, seedData, reviewScript());
  await session.clarificationOverlay().getByTestId("question-review-open").tap();
  const dialog = testPage.getByTestId("question-review-dialog");
  await expect(dialog.getByRole("img", { name: reviewImages.before.alt })).toBeVisible();
  const more = await openPhoneMore(testPage, dialog);
  await more.getByTestId("question-review-annotate").tap();
  await expect(dialog.getByTestId("question-review-annotate-hint")).toBeVisible();
  await dialog.getByTestId("question-review-stage").focus();
  await testPage.keyboard.press("n");
  const input = testPage.getByTestId("question-review-region-input");
  await expect(input).toBeVisible();
  await input.fill("Too tight");
  await testPage.getByTestId("question-review-region-add").tap();
  await dialog.getByTestId("question-review-annotate-done").tap();
  await dialog.getByTestId("question-review-regions-button").tap();
  const sheet = testPage.getByTestId("question-review-regions-sheet");
  const item = sheet.getByTestId("question-review-region-item");
  await expect(item.getByTestId("question-review-region-text")).toHaveText("Too tight");
  await expectTouchTargets(item);
  await item.getByTestId("question-review-region-delete").tap();
  await expect(item).toHaveCount(0);
  await expectNoHorizontalOverflow(testPage);
});

test("phone thumbnail inspector swipes between a question's images", async ({
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
    reviewScript(threeImages),
  );
  const overlay = session.clarificationOverlay();
  await overlay.getByRole("button", { name: new RegExp(reviewImages.before.alt) }).tap();
  const viewer = testPage.getByRole("dialog");
  const counter = viewer.getByTestId("question-image-counter");
  await expect(counter).toHaveText("before, 1 of 3");
  const canvas = viewer.getByTestId("question-image-canvas");
  const box = (await canvas.boundingBox())!;
  await touchDrag(testPage, canvas, box.x + 40);
  await expect(counter).toHaveText("after, 2 of 3");
  await expectTouchTargets(viewer);
});
