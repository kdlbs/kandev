import { test, expect } from "../../fixtures/test-base";
import {
  expectNoHorizontalOverflow,
  expectThumbnailsShareRow,
  touchDrag,
  openReviewQuestions,
  reviewImages,
  reviewScript,
  seedReviewImages,
} from "./question-image-review-helpers";

test("phone review keeps the stage above a scrolling answer form", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(180_000);
  seedReviewImages(seedData.repositoryPath);
  const { session } = await openReviewQuestions(testPage, apiClient, seedData, reviewScript());
  const overlay = session.clarificationOverlay();
  await expectThumbnailsShareRow(overlay);
  await overlay.getByTestId("question-review-open").tap();

  const dialog = testPage.getByTestId("question-review-dialog");
  await expect(dialog).toBeVisible();
  const viewport = testPage.viewportSize()!;
  expect((await dialog.boundingBox())!.height).toBeGreaterThan(viewport.height - 3);
  const stage = dialog.getByTestId("question-review-stage");
  const card = dialog.getByTestId("clarification-question-card");
  await expect(dialog.getByRole("img", { name: reviewImages.before.alt })).toBeVisible();
  await expect(stage).toBeInViewport();
  await expect(card.getByText("Is the new header spacing acceptable?")).toBeInViewport();
  expect((await stage.boundingBox())!.y).toBeLessThan((await card.boundingBox())!.y);
  for (const control of [
    dialog.getByTestId("question-review-close"),
    dialog.getByTestId("question-review-zoom-in"),
    dialog.getByTestId("question-review-segment").first(),
  ]) {
    const box = (await control.boundingBox())!;
    expect(box.height).toBeGreaterThanOrEqual(44);
  }
  await expectNoHorizontalOverflow(testPage);
  await prCapture.screenshot("phone-question-review", {
    caption: "Phone image review with the answer form below",
  });

  await dialog.getByTestId("question-review-zoom-in").tap();
  await expect(dialog.getByTestId("question-review-zoom-level")).not.toHaveText("");
  await card.getByTestId("clarification-option").first().tap();
  await expect(card).toHaveAttribute("data-question-id", "spacing");
  await card.getByTestId("clarification-option").first().tap();
  await dialog.getByTestId("question-review-submit").tap();
  await expect(dialog).toHaveCount(0);
  await expect(session.chat).toContainText("Review questions complete.", { timeout: 30_000 });
});

test("phone comparison stacks Side by side and keeps touch-sized mode controls", async ({
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
  const modes = dialog.getByTestId("question-review-modes");
  const side = modes.getByText("Side by side", { exact: true });
  expect((await side.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await side.tap();
  const first = (await dialog.getByTestId("question-review-stage").boundingBox())!;
  const second = (await dialog.getByTestId("question-review-stage-b").boundingBox())!;
  expect(second.y).toBeGreaterThanOrEqual(first.y + first.height - 1);
  await modes.getByText("Toggle", { exact: true }).tap();
  await dialog.getByTestId("question-review-flip").tap();
  await expect(dialog.getByRole("img", { name: reviewImages.after.alt })).toBeVisible();
  await expectNoHorizontalOverflow(testPage);
});

test("phone regions can be added and edited with touch-sized controls", async ({
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
  const annotate = dialog.getByTestId("question-review-annotate");
  expect((await annotate.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await annotate.tap();
  await dialog.getByTestId("question-review-stage").focus();
  await testPage.keyboard.press("n");
  const input = testPage.getByTestId("question-review-region-input");
  await expect(input).toBeVisible();
  await input.fill("Too tight");
  const add = testPage.getByTestId("question-review-region-add");
  expect((await add.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await add.tap();
  const item = dialog.getByTestId("question-review-region-item");
  await expect(item).toHaveCount(1);
  await expect(item.getByTestId("question-review-region-text")).toHaveText("Too tight");
  await expect(dialog).toBeVisible();
  const remove = item.getByTestId("question-review-region-delete");
  expect((await remove.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await remove.tap();
  await expect(item).toHaveCount(0);
  await expectNoHorizontalOverflow(testPage);
});

test("phone wipe reaches both edges away from the screen edge and Side by side fits", async ({
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
  const modes = dialog.getByTestId("question-review-modes");
  await modes.getByText("Wipe", { exact: true }).tap();
  const handle = dialog.getByTestId("question-review-wipe-handle");
  const width = testPage.viewportSize()!.width;
  await touchDrag(testPage, handle, 30);
  await expect(handle).toHaveAttribute("aria-valuenow", "0");
  const stage = (await dialog.getByTestId("question-review-stage").boundingBox())!;
  const handleBox = (await handle.boundingBox())!;
  expect(handleBox.x).toBeGreaterThanOrEqual(stage.x - 1);
  await touchDrag(testPage, handle, width - 30);
  await expect(handle).toHaveAttribute("aria-valuenow", "100");

  await modes.getByText("Side by side", { exact: true }).tap();
  for (const id of ["question-review-stage", "question-review-stage-b"]) {
    const pane = dialog.getByTestId(id);
    const paneBox = (await pane.boundingBox())!;
    const image = (await pane.getByTestId("question-review-image").boundingBox())!;
    expect(image.y).toBeGreaterThanOrEqual(paneBox.y - 1);
    expect(image.y + image.height).toBeLessThanOrEqual(paneBox.y + paneBox.height + 1);
    expect(image.x + image.width).toBeLessThanOrEqual(paneBox.x + paneBox.width + 1);
  }
  await expectNoHorizontalOverflow(testPage);
});
