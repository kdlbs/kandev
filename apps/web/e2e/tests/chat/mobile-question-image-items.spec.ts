import { test, expect } from "../../fixtures/test-base";
import {
  choosePhoneMode,
  expectNoHorizontalOverflow,
  expectNoOverlap,
  expectTouchTargets,
  expectWithinViewport,
  itemsScript,
  openReviewQuestions,
  removeReviewImages,
  reviewImages,
  seedReviewImages,
  touchDrag,
  touchPress,
} from "./question-image-review-helpers";

test.afterEach(({ seedData }) => removeReviewImages(seedData.repositoryPath));

test("phone moves between a pair and singles with the bar, not by dragging the image", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  seedReviewImages(seedData.repositoryPath);
  const { session } = await openReviewQuestions(testPage, apiClient, seedData, itemsScript());
  await session.clarificationOverlay().getByTestId("question-review-open").tap();
  const dialog = testPage.getByTestId("question-review-dialog");
  const bar = dialog.getByTestId("question-review-bottom-bar");
  const position = bar.getByTestId("question-review-item-position");
  await expect(position).toHaveText("1/3 · Pair");
  await expect(dialog.getByRole("img", { name: reviewImages.before.alt })).toBeVisible();
  await expectTouchTargets(bar);
  await expectNoHorizontalOverflow(testPage);

  // A drag on the image never moves to another item.
  const stage = dialog.getByTestId("question-review-stage");
  const box = (await stage.boundingBox())!;
  await touchDrag(testPage, stage, box.x + box.width * 0.1);
  await expect(position).toHaveText("1/3 · Pair");

  await bar.getByTestId("question-review-item-next").tap();
  await expect(position).toHaveText("2/3 · Single");
  await expect(dialog.getByRole("img", { name: reviewImages.spacing.alt })).toBeVisible();
  await expectTouchTargets(bar);
  await expectNoHorizontalOverflow(testPage);
  await touchDrag(testPage, stage, box.x + box.width * 0.1);
  await expect(position).toHaveText("2/3 · Single");

  await bar.getByTestId("question-review-item-next").tap();
  await expect(position).toHaveText("3/3 · Single");
  await bar.getByTestId("question-review-item-prev").tap();
  await expect(position).toHaveText("2/3 · Single");
});

test("phone identity names the shown image without covering controls", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(180_000);
  seedReviewImages(seedData.repositoryPath);
  const { session } = await openReviewQuestions(testPage, apiClient, seedData, itemsScript());
  await session.clarificationOverlay().getByTestId("question-review-open").tap();
  const dialog = testPage.getByTestId("question-review-dialog");
  const bar = dialog.getByTestId("question-review-bottom-bar");
  const chip = dialog.getByTestId("question-review-zoom-chip");
  const badge = dialog.getByTestId("question-review-identity-badge");
  await expect(badge).toHaveText(["A · before"]);
  // A pair's order is fixed: Flip, but no Swap.
  await expect(bar.getByTestId("question-review-swap")).toHaveCount(0);
  await bar.getByTestId("question-review-flip").tap();
  await expect(badge).toHaveText(["B · after"]);
  const badgeBox = (await badge.boundingBox())!;
  const chipBox = (await chip.boundingBox())!;
  expect(Math.abs(badgeBox.y + badgeBox.height - (chipBox.y + chipBox.height))).toBeLessThan(4);
  await expectNoOverlap(badge, chip, "identity clears the zoom chip");
  expect(badgeBox.y + badgeBox.height).toBeLessThanOrEqual((await bar.boundingBox())!.y);

  await choosePhoneMode(testPage, dialog, "Wipe");
  const handle = dialog.getByTestId("question-review-wipe-handle");
  const labels = handle.getByTestId("question-review-wipe-label");
  await expect(labels).toHaveText(["before", "after"]);
  await expect(labels.first()).toHaveCSS("opacity", "0");
  // Touching the handle shows the names; dragging it moves it up the fixed line.
  const before = (await handle.boundingBox())!;
  const release = await touchPress(testPage, handle, -100);
  await expect(labels.first()).toHaveCSS("opacity", "1");
  await prCapture.screenshot("phone-question-review-wipe-names", {
    caption: "Phone Wipe names each image on the handle while it is held",
  });
  for (const label of await labels.all()) {
    await expectNoOverlap(label, chip, "name clears the zoom chip");
  }
  await release();
  const lifted = (await handle.boundingBox())!;
  expect(before.y - lifted.y).toBeGreaterThan(80);
  expect(Math.abs(lifted.x - before.x)).toBeLessThan(2);
  await expect(handle).toHaveAttribute("aria-valuenow", "50");
  await expect(labels.first()).toHaveCSS("opacity", "0");
  await expectNoHorizontalOverflow(testPage);
});

test("phone gallery keeps its bottom bar within touch targets and compares on demand", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  seedReviewImages(seedData.repositoryPath);
  await testPage.setViewportSize({ width: 320, height: 700 });
  const { session } = await openReviewQuestions(testPage, apiClient, seedData, itemsScript(false));
  await session.clarificationOverlay().getByTestId("question-review-open").tap();
  const dialog = testPage.getByTestId("question-review-dialog");
  const bar = dialog.getByTestId("question-review-bottom-bar");
  // Gallery items have no More: zoom is on the image and Mark area in the bar.
  await expect(dialog.getByTestId("question-review-more")).toHaveCount(0);
  await expect(bar.getByTestId("question-review-item-position")).toHaveText("1/4 · Image");
  await expectTouchTargets(bar);
  await expectWithinViewport(testPage, bar);
  await expectTouchTargets(dialog.getByTestId("question-review-stage").locator(".."));
  await expectNoHorizontalOverflow(testPage);
  const buttons = await bar.getByRole("button").all();
  const boxes = await Promise.all(buttons.map((button) => button.boundingBox()));
  const visible = boxes.filter((box): box is NonNullable<typeof box> => Boolean(box));
  for (let index = 1; index < visible.length; index++) {
    const [previous, current] = [visible[index - 1], visible[index]];
    // The bar stacks rows; only buttons on the same row can overlap.
    if (Math.abs(current.y - previous.y) > previous.height / 2) continue;
    expect(current.x, "bar buttons do not overlap").toBeGreaterThanOrEqual(
      previous.x + previous.width - 1,
    );
  }
  const compare = dialog.getByTestId("question-review-compare-images");
  await expect(compare).toHaveText("Compare");
  await expect(compare).toHaveAccessibleName("Compare images");
  await expectWithinViewport(testPage, compare.locator(".."));
  await compare.tap();
  const back = bar.getByTestId("question-review-back-to-gallery");
  await expect(back).toHaveText("Gallery");
  // B's position among the candidates shows next to the stepper.
  const stepper = bar.getByTestId("question-review-stepper");
  await expect(stepper).toContainText("1/3");
  await stepper.getByTestId("question-review-step-next").tap();
  await expect(stepper).toContainText("2/3");
  await expect(back).toHaveAccessibleName("Back to gallery");
  await expectTouchTargets(bar);
  await expectWithinViewport(testPage, bar);
  await expectWithinViewport(
    testPage,
    dialog.getByTestId("question-review-mode-button").locator(".."),
  );
  await expectNoHorizontalOverflow(testPage);
  await back.tap();
  await expect(bar.getByTestId("question-review-item-position")).toHaveText("1/4 · Image");
});

for (const width of [393, 320]) {
  test(`phone pair slider modes keep every bar control on screen at ${width}px`, async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    seedReviewImages(seedData.repositoryPath);
    await testPage.setViewportSize({ width, height: 760 });
    const { session } = await openReviewQuestions(testPage, apiClient, seedData, itemsScript());
    await session.clarificationOverlay().getByTestId("question-review-open").tap();
    const dialog = testPage.getByTestId("question-review-dialog");
    const bar = dialog.getByTestId("question-review-bottom-bar");
    await expect(dialog.getByRole("img", { name: reviewImages.before.alt })).toBeVisible();
    for (const mode of ["Toggle", "Wipe", "Onion skin", "Difference"]) {
      if (mode !== "Toggle") await choosePhoneMode(testPage, dialog, mode);
      await expectTouchTargets(bar);
      await expectWithinViewport(testPage, bar);
      const identity = dialog.locator(
        "[data-testid='question-review-identity-badge'], [data-testid='question-review-wipe-label']",
      );
      for (const label of await identity.all()) {
        const box = (await label.boundingBox())!;
        expect(box.x, "identity stays on screen").toBeGreaterThanOrEqual(0);
        expect(box.x + box.width, "identity stays on screen").toBeLessThanOrEqual(width);
      }
    }
    await expectNoHorizontalOverflow(testPage);
  });
}

test("phone Wipe keeps the scrubbed split when returning to a pair", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  seedReviewImages(seedData.repositoryPath);
  const { session } = await openReviewQuestions(testPage, apiClient, seedData, itemsScript());
  await session.clarificationOverlay().getByTestId("question-review-open").tap();
  const dialog = testPage.getByTestId("question-review-dialog");
  const bar = dialog.getByTestId("question-review-bottom-bar");
  await expect(dialog.getByRole("img", { name: reviewImages.before.alt })).toBeVisible();
  await choosePhoneMode(testPage, dialog, "Wipe");
  const handle = dialog.getByTestId("question-review-wipe-handle");
  await expect(handle).toHaveAttribute("data-pinned", "true");
  const stage = dialog.getByTestId("question-review-stage");
  const box = (await stage.boundingBox())!;
  await touchDrag(testPage, stage, box.x + box.width * 0.75, 0.2);
  await expect(handle).not.toHaveAttribute("aria-valuenow", "50");
  const scrubbed = await handle.getAttribute("aria-valuenow");

  await bar.getByTestId("question-review-item-next").tap();
  await expect(bar.getByTestId("question-review-item-position")).toHaveText("2/3 · Single");
  await bar.getByTestId("question-review-item-prev").tap();
  await expect(dialog.getByRole("img", { name: reviewImages.before.alt })).toBeVisible();
  await expect(handle).toHaveAttribute("aria-valuenow", scrubbed!);
});

test("phone shows a choice's image when the choice is selected", async ({
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
  await session.clarificationOverlay().getByTestId("question-review-open").tap();
  const dialog = testPage.getByTestId("question-review-dialog");
  const position = dialog
    .getByTestId("question-review-bottom-bar")
    .getByTestId("question-review-item-position");
  await expect(position).toHaveText("1/2 · Image");
  const choice = dialog.getByTestId("clarification-option").nth(1);
  const hint = choice.getByTestId("question-review-choice-image");
  await expect(hint).toContainText("after");
  const [hintBox, choiceBox] = [(await hint.boundingBox())!, (await choice.boundingBox())!];
  expect(hintBox.x + hintBox.width).toBeLessThanOrEqual(choiceBox.x + choiceBox.width);
  expect(choiceBox.height).toBeGreaterThanOrEqual(44);
  await choice.tap();
  await expect(position).toHaveText("2/2 · Image");
  await expect(dialog.getByRole("img", { name: reviewImages.after.alt })).toBeVisible();
  await expectNoHorizontalOverflow(testPage);
});
