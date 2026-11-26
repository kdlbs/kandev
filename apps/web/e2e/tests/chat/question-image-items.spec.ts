import { test, expect } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
import {
  chooseZoom,
  expectNoOverlap,
  itemsScript,
  openReviewQuestions,
  recordImageRequests,
  removeReviewImages,
  reviewImages,
  seedReviewImages,
} from "./question-image-review-helpers";

test.afterEach(({ seedData }) => removeReviewImages(seedData.repositoryPath));

test("reviews a pair with comparison tools and singles alone, item by item", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(180_000);
  seedReviewImages(seedData.repositoryPath);
  const { session } = await openReviewQuestions(testPage, apiClient, seedData, itemsScript());
  const overlay = session.clarificationOverlay();
  for (const image of [reviewImages.before, reviewImages.after, reviewImages.spacing]) {
    await expect(overlay.getByRole("img", { name: image.alt })).toBeVisible();
  }
  const requested = recordImageRequests(testPage);
  await overlay.getByTestId("question-review-open").click();
  const dialog = testPage.getByTestId("question-review-dialog");
  const position = dialog.getByTestId("question-review-item-position");
  await expect(position).toHaveText("Item 1 of 3 · Pair");
  await expect(dialog.getByTestId("question-review-modes")).toBeVisible();
  // A pair's order is fixed: no pickers and no Swap.
  await expect(dialog.getByTestId("question-review-pick-a")).toHaveCount(0);
  await expect(dialog.getByTestId("question-review-swap")).toHaveCount(0);
  await expect(dialog.getByRole("img", { name: reviewImages.before.alt })).toBeVisible();
  await prCapture.screenshot("desktop-question-review-pair", {
    caption: "A declared pair with comparison tools and item navigation",
  });

  await dialog.getByTestId("question-review-item-next").click();
  await expect(position).toHaveText("Item 2 of 3 · Single");
  await expect(dialog.getByTestId("question-review-modes")).toHaveCount(0);
  await expect(dialog.getByRole("img", { name: reviewImages.spacing.alt })).toBeVisible();

  await dialog.getByTestId("question-review-stage").focus();
  await testPage.keyboard.press("PageDown");
  await expect(position).toHaveText("Item 3 of 3 · Single");
  await expect(dialog.getByTestId("question-review-item-next")).toBeDisabled();
  await testPage.keyboard.press("PageUp");
  await testPage.keyboard.press("PageUp");
  await expect(position).toHaveText("Item 1 of 3 · Pair");
  await expect(dialog.getByRole("img", { name: reviewImages.before.alt })).toBeVisible();

  // The view reads each segment image once; moving between items reads none again.
  await dwell(testPage, 500, "negative-assertion", "item navigation must not reread images");
  const counts = new Map<string, number>();
  for (const id of requested) counts.set(id, (counts.get(id) ?? 0) + 1);
  for (const [id, count] of counts) expect(count, `${id} read once`).toBe(1);
});

test("returns to an item at the zoom it was left at", async ({ testPage, apiClient, seedData }) => {
  test.setTimeout(180_000);
  seedReviewImages(seedData.repositoryPath);
  const { session } = await openReviewQuestions(testPage, apiClient, seedData, itemsScript());
  await session.clarificationOverlay().getByTestId("question-review-open").click();
  const dialog = testPage.getByTestId("question-review-dialog");
  const position = dialog.getByTestId("question-review-item-position");
  const level = dialog.getByTestId("question-review-zoom-level");
  await expect(dialog.getByRole("img", { name: reviewImages.before.alt })).toBeVisible();
  await chooseZoom(testPage, dialog, "question-review-actual");
  await dialog.getByTestId("question-review-zoom-in").click();
  await expect(level).toHaveText("200%");

  // The single has a different size, so it starts at Fit.
  await dialog.getByTestId("question-review-item-next").click();
  await expect(position).toHaveText("Item 2 of 3 · Single");
  await expect(dialog.getByRole("img", { name: reviewImages.spacing.alt })).toBeVisible();
  await expect(level).not.toHaveText("200%");

  await dialog.getByTestId("question-review-item-prev").click();
  await expect(position).toHaveText("Item 1 of 3 · Pair");
  await expect(level).toHaveText("200%");
});

test("shows ungrouped images as a gallery and compares two on demand", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  seedReviewImages(seedData.repositoryPath);
  const { session } = await openReviewQuestions(testPage, apiClient, seedData, itemsScript(false));
  await session.clarificationOverlay().getByTestId("question-review-open").click();
  const dialog = testPage.getByTestId("question-review-dialog");
  await expect(dialog.getByTestId("question-review-item-position")).toHaveText(
    "Item 1 of 4 · Image",
  );
  await expect(dialog.getByTestId("question-review-modes")).toHaveCount(0);
  await dialog.getByTestId("question-review-compare-images").click();
  await expect(dialog.getByTestId("question-review-modes")).toBeVisible();
  await expect(dialog.getByTestId("question-review-pick-a")).toContainText("before");
  await expect(dialog.getByTestId("question-review-pick-b")).toContainText("after");
  await expect(dialog.getByTestId("question-review-swap")).toBeVisible();
  await dialog.getByTestId("question-review-back-to-gallery").click();
  await expect(dialog.getByTestId("question-review-item-position")).toHaveText(
    "Item 1 of 4 · Image",
  );
});

test("names the visible image at the bottom left and on the Wipe handle", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(180_000);
  seedReviewImages(seedData.repositoryPath);
  const { session } = await openReviewQuestions(testPage, apiClient, seedData, itemsScript());
  await session.clarificationOverlay().getByTestId("question-review-open").click();
  const dialog = testPage.getByTestId("question-review-dialog");
  const stage = dialog.getByTestId("question-review-stage");
  const zoom = stage.getByTestId("question-review-zoom");
  const badges = dialog.getByTestId("question-review-identity-badge");
  await expect(dialog.getByRole("img", { name: reviewImages.before.alt })).toBeVisible();
  await expect(badges).toHaveText(["A · before"]);
  await dialog.getByTestId("question-review-flip").click();
  await expect(badges).toHaveText(["B · after"]);
  // Bottom left, on the zoom controls' row, without touching them.
  const stageBox = (await stage.boundingBox())!;
  const badge = (await badges.first().boundingBox())!;
  expect(badge.x - stageBox.x).toBeLessThan(16);
  expect(stageBox.y + stageBox.height - (badge.y + badge.height)).toBeLessThan(16);
  await expectNoOverlap(badges.first(), zoom, "identity clears the zoom controls");

  const modes = dialog.getByTestId("question-review-modes");
  await modes.getByText("Onion skin", { exact: true }).click();
  await expect(badges.first()).toContainText("B · after");
  await expect(badges.first()).toContainText("A · before");
  await modes.getByText("Side by side", { exact: true }).click();
  // Zoom stays at the stage's far corner, under the second pane.
  await expect(
    dialog.getByTestId("question-review-stage-b").getByTestId("question-review-zoom"),
  ).toBeVisible();

  await modes.getByText("Wipe", { exact: true }).click();
  await expect(badges).toHaveCount(0);
  const handle = dialog.getByTestId("question-review-wipe-handle");
  const labels = handle.getByTestId("question-review-wipe-label");
  await expect(labels).toHaveText(["before", "after"]);
  // The names stay hidden until the handle or the divider is in use.
  await expect(labels.first()).toHaveCSS("opacity", "0");
  await handle.hover();
  await expect(labels.first()).toHaveCSS("opacity", "1");
  const handleBox = (await handle.boundingBox())!;
  const line = handleBox.x + handleBox.width / 2;
  const [left, right] = [
    (await labels.nth(0).boundingBox())!,
    (await labels.nth(1).boundingBox())!,
  ];
  expect(left.x + left.width, "A's name sits left of the line").toBeLessThanOrEqual(line);
  expect(right.x, "B's name sits right of the line").toBeGreaterThanOrEqual(line);
  expect(left.height, "letters stack vertically").toBeGreaterThan(left.width);
  for (const label of await labels.all()) {
    await expectNoOverlap(label, zoom, "name clears the zoom controls");
  }
  await prCapture.screenshot("desktop-question-review-identity", {
    caption: "Wipe names each image on the handle, on its side of the line",
  });
  await testPage.mouse.move(stageBox.x + 4, stageBox.y + 4);
  await expect(labels.first()).toHaveCSS("opacity", "0");
  await dialog.getByTestId("question-review-wipe-slider").hover();
  await expect(labels.first()).toHaveCSS("opacity", "1");

  // Dragging the handle moves the divider sideways and the handle up the line.
  await testPage.mouse.move(line, handleBox.y + handleBox.height / 2);
  await testPage.mouse.down();
  await testPage.mouse.move(line - 80, handleBox.y + handleBox.height / 2 - 120, { steps: 8 });
  await testPage.mouse.up();
  const lifted = (await handle.boundingBox())!;
  expect(handleBox.y - lifted.y).toBeGreaterThan(100);
  expect(Number(await handle.getAttribute("aria-valuenow"))).toBeLessThan(50);
  // A drag on the image does not move to another item.
  const position = dialog.getByTestId("question-review-item-position");
  const positionText = await position.innerText();
  await testPage.mouse.move(stageBox.x + stageBox.width * 0.8, stageBox.y + 40);
  await testPage.mouse.down();
  await testPage.mouse.move(stageBox.x + stageBox.width * 0.2, stageBox.y + 40, { steps: 8 });
  await testPage.mouse.up();
  await expect(position).toHaveText(positionText);
  // The handle keeps its height for the question round.
  await dialog.getByRole("button", { name: "Close", exact: true }).click();
  await session.clarificationOverlay().getByTestId("question-review-open").click();
  await modes.getByText("Wipe", { exact: true }).click();
  await expect.poll(async () => (await handle.boundingBox())!.y).toBeCloseTo(lifted.y, -1);

  // At the left edge, A's side is too narrow for its name.
  await handle.focus();
  await testPage.keyboard.press("Home");
  await expect(labels).toHaveText(["after"]);
  // At either end the handle stays fully inside the stage.
  const stageNow = (await stage.boundingBox())!;
  expect((await handle.boundingBox())!.x).toBeGreaterThanOrEqual(stageNow.x);
  await testPage.keyboard.press("End");
  await expect(labels).toHaveText(["before"]);
  const atEnd = (await handle.boundingBox())!;
  expect(atEnd.x + atEnd.width).toBeLessThanOrEqual(stageNow.x + stageNow.width + 0.5);
});

test("shows a choice's image when the choice is selected", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  seedReviewImages(seedData.repositoryPath);
  const question = {
    id: "header",
    title: "Header",
    prompt: "Which header do you prefer?",
    options: [
      { label: "Old", description: "Keep the old header", images: [reviewImages.before] },
      { label: "New", description: "Use the new header", images: [reviewImages.after] },
    ],
  };
  const script = `e2e:mcp:kandev:ask_user_question_kandev(${JSON.stringify({ questions: [question] })})\ne2e:message("Done.")`;
  const { session } = await openReviewQuestions(testPage, apiClient, seedData, script);
  await session.clarificationOverlay().getByTestId("question-review-open").click();
  const dialog = testPage.getByTestId("question-review-dialog");
  const position = dialog.getByTestId("question-review-item-position");
  await expect(position).toHaveText("Item 1 of 2 · Image");
  const choices = dialog.getByTestId("clarification-option");
  await expect(choices.nth(1).getByTestId("question-review-choice-image")).toContainText("after");
  await choices.nth(1).click();
  await expect(position).toHaveText("Item 2 of 2 · Image");
  await expect(dialog.getByRole("img", { name: reviewImages.after.alt })).toBeVisible();
  await dialog.getByTestId("question-review-item-prev").click();
  await expect(position).toHaveText("Item 1 of 2 · Image");
  await choices.nth(1).click();
  await expect(position).toHaveText("Item 2 of 2 · Image");
});
