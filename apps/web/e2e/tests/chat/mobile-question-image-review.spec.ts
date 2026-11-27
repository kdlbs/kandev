import { test, expect } from "../../fixtures/test-base";
import {
  choosePhoneMode,
  expectNoHorizontalOverflow,
  expectToastsAvoidControls,
  expectThumbnailsShareRow,
  expectTouchTargets,
  expectWithinViewport,
  openPhoneMore,
  openReviewQuestions,
  removeReviewImages,
  reviewImages,
  reviewScript,
  seedReviewImages,
  showReviewUpdateToasts,
  touchDrag,
} from "./question-image-review-helpers";
import { installRuntimeUpdateFixture } from "../settings/agent-runtime-update-helpers";

test.afterEach(({ seedData }) => removeReviewImages(seedData.repositoryPath));

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
  await dialog.getByTestId("question-review-compare-images").tap();
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

  const flip = bar.getByTestId("question-review-flip");
  expect(
    await flip.evaluate((element) => element.scrollWidth <= element.clientWidth + 1),
    "Flip label fits its button",
  ).toBe(true);
  await flip.tap();
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

  // The zoom chip opens the zoom controls beside it; More no longer holds zoom.
  const chip = dialog.getByTestId("question-review-zoom-chip");
  const fitLevel = await chip.textContent();
  await chip.tap();
  const zoomPopover = testPage.getByTestId("question-review-zoom-popover");
  await expect(zoomPopover).toBeVisible();
  await expectTouchTargets(zoomPopover);
  await expectWithinViewport(testPage, zoomPopover);
  await prCapture.screenshot("phone-question-review-zoom", {
    caption: "The zoom chip opens zoom controls on the image",
  });
  await zoomPopover.getByTestId("question-review-actual").tap();
  await expect(chip).toHaveText("100%");
  await zoomPopover.getByTestId("question-review-zoom-in").tap();
  await expect(zoomPopover.getByTestId("question-review-zoom-level")).not.toHaveText("100%");
  await zoomPopover.getByTestId("question-review-fit").tap();
  await expect(chip).toHaveText(fitLevel!);
  await card.getByText("Is the new header spacing acceptable?").tap();
  await expect(zoomPopover).toHaveCount(0);
  const more = await openPhoneMore(testPage, dialog);
  await expect(more.getByTestId("question-review-zoom-in")).toHaveCount(0);
  await testPage.keyboard.press("Escape");
  await expect(more).toHaveCount(0);

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

// @covers AC-UI-TOAST-INTERACTION-001.1 through AC-UI-TOAST-INTERACTION-001.4
test("update toasts keep review actions clear", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(180_000);
  const runtime = await installRuntimeUpdateFixture(testPage);
  await testPage.setViewportSize({ width: 667, height: 390 });
  seedReviewImages(seedData.repositoryPath);
  const { session } = await openReviewQuestions(testPage, apiClient, seedData, reviewScript());
  const open = session.clarificationOverlay().getByTestId("question-review-open");
  await open.focus();
  await testPage.keyboard.press("Enter");

  const dialog = testPage.getByTestId("question-review-dialog");
  await expect(dialog).toBeVisible();
  await testPage.clock.install();
  await testPage.clock.pauseAt(await testPage.evaluate(() => Date.now() + 250));
  const { releaseToast, runtimeToast } = await showReviewUpdateToasts(testPage, runtime);
  const submit = dialog.getByTestId("question-review-submit");
  const close = dialog.getByTestId("question-review-close");
  const viewport = testPage.viewportSize()!;
  const outlet = dialog.getByTestId("modal-toast-outlet");
  await expect(outlet.getByTestId("toast-message")).toHaveCount(2);
  const scrollRegion = outlet;
  const scrollBox = await scrollRegion.boundingBox();
  expect(scrollBox).not.toBeNull();
  expect(scrollBox!.height).toBeLessThanOrEqual(Math.min(176, viewport.height * 0.22) + 1);
  const scrollExtent = await scrollRegion.evaluate((element) => ({
    clientHeight: element.clientHeight,
    scrollHeight: element.scrollHeight,
  }));
  expect(scrollExtent.scrollHeight).toBeGreaterThan(scrollExtent.clientHeight);
  await expectToastsAvoidControls([releaseToast, runtimeToast], [submit, close]);
  await expectNoHorizontalOverflow(testPage);
  const formBox = await dialog.getByTestId("question-review-form").boundingBox();
  const bottomBarBox = await dialog.getByTestId("question-review-bottom-bar").boundingBox();
  expect(formBox).not.toBeNull();
  expect(bottomBarBox).not.toBeNull();
  expect(formBox!.height).toBeGreaterThan(0);
  expect(bottomBarBox!.y).toBeGreaterThanOrEqual(0);
  await expect(
    dialog
      .getByTestId("clarification-question-card")
      .getByText("Is the new header spacing acceptable?"),
  ).toBeInViewport();
  expect(bottomBarBox!.y + bottomBarBox!.height).toBeLessThanOrEqual(viewport.height);
  const bottomBarButtons = dialog.getByTestId("question-review-bottom-bar").getByRole("button");
  for (let index = 0; index < (await bottomBarButtons.count()); index++) {
    const button = bottomBarButtons.nth(index);
    if (!(await button.isVisible())) continue;
    const box = await button.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThanOrEqual(viewport.width);
    expect(box!.y).toBeGreaterThanOrEqual(0);
    expect(box!.y + box!.height).toBeLessThanOrEqual(viewport.height);
  }
  await prCapture.screenshot("phone-update-toasts-review", {
    caption: "Phone review keeps its form and bottom actions visible below two update notices.",
  });

  expect(await scrollRegion.evaluate((element) => getComputedStyle(element).overflowY)).toBe(
    "auto",
  );
  for (const toast of [releaseToast, runtimeToast]) {
    const dismiss = toast.getByRole("button", { name: "Dismiss notification" });
    await dismiss.scrollIntoViewIfNeeded();
    const box = await dismiss.boundingBox();
    const outletBox = await scrollRegion.boundingBox();
    expect(box).not.toBeNull();
    expect(outletBox).not.toBeNull();
    expect(box!.width).toBeGreaterThanOrEqual(44);
    expect(box!.height).toBeGreaterThanOrEqual(44);
    expect(box!.x).toBeGreaterThanOrEqual(outletBox!.x);
    expect(box!.y).toBeGreaterThanOrEqual(outletBox!.y);
    expect(box!.x + box!.width).toBeLessThanOrEqual(outletBox!.x + outletBox!.width);
    expect(box!.y + box!.height).toBeLessThanOrEqual(outletBox!.y + outletBox!.height);
    expect(
      await dismiss.evaluate((element) => {
        const bounds = element.getBoundingClientRect();
        const target = document.elementFromPoint(
          bounds.left + bounds.width / 2,
          bounds.top + bounds.height / 2,
        );
        return target === element || element.contains(target);
      }),
    ).toBe(true);
  }

  const more = await openPhoneMore(testPage, dialog);
  await expect(more.getByTestId("toast-message")).toHaveCount(2);
  await testPage.keyboard.press("Escape");
  await expect(more).toHaveCount(0);
  await expect(dialog.getByTestId("toast-message")).toHaveCount(2);

  await releaseToast.getByRole("button", { name: "Dismiss notification" }).tap();
  await expect(releaseToast).toHaveCount(0);
  await expect(runtimeToast).toBeVisible();
  await expect(dialog).toBeVisible();

  const firstQuestion = dialog.getByTestId("clarification-question-card");
  await firstQuestion.getByTestId("clarification-option").first().tap();
  await dialog.getByTestId("clarification-next").tap();
  const secondQuestion = dialog.getByTestId("clarification-question-card");
  await secondQuestion.getByTestId("clarification-option").first().tap();
  await dialog.getByTestId("question-review-submit").tap();
  await expect(dialog).toHaveCount(0);
  await expect(session.chat).toContainText("Review questions complete.");
  await expect(runtimeToast).toBeVisible();
  await runtimeToast.getByRole("button", { name: "Dismiss notification" }).tap();
  await expect(runtimeToast).toHaveCount(0);
  await expectNoHorizontalOverflow(testPage);
});

test("phone wipe pins its line and scrubs the images; Side by side fits", async ({
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
    reviewScript(undefined, undefined, "gallery"),
  );
  await session.clarificationOverlay().getByTestId("question-review-open").tap();
  const dialog = testPage.getByTestId("question-review-dialog");
  await dialog.getByTestId("question-review-compare-images").tap();
  await expect(dialog.getByRole("img", { name: reviewImages.before.alt })).toBeVisible();
  await choosePhoneMode(testPage, dialog, "Wipe");
  const handle = dialog.getByTestId("question-review-wipe-handle");
  await expect(handle).toHaveAttribute("data-pinned", "true");
  await expect(handle).toHaveAttribute("aria-valuenow", "50");
  const bar = dialog.getByTestId("question-review-bottom-bar");
  await expect(bar.getByTestId("question-review-picker-button")).toBeInViewport();
  const stage = dialog.getByTestId("question-review-stage");
  const box = (await stage.boundingBox())!;
  const center = box.x + box.width / 2;
  const width = testPage.viewportSize()!.width;
  // Scrub above the handle: touching the handle moves the handle instead.
  const reach = Math.min(box.width / 2, width / 2 - 60);
  await touchDrag(testPage, stage, center + reach, 0.2);
  await touchDrag(testPage, stage, center + reach, 0.2);
  await expect(handle).toHaveAttribute("aria-valuenow", "0");
  for (let attempt = 0; attempt < 3; attempt++) {
    await touchDrag(testPage, stage, center - reach, 0.2);
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

  for (const mode of ["Onion skin", "Difference"]) {
    await choosePhoneMode(testPage, dialog, mode);
    await expect(bar.getByTestId("question-review-picker-button")).toBeInViewport();
    await expectTouchTargets(bar);
    await expectNoHorizontalOverflow(testPage);
  }
  await bar.getByTestId("question-review-picker-button").tap();
  await expect(testPage.getByTestId("question-review-picker-sheet")).toBeVisible();
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
