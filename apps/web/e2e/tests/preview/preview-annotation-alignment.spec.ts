import { expect, type FrameLocator, type Locator, type Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import {
  chooseCapture,
  draftSurface,
  dragScreenshotRegion,
  openBrowserPreview,
  regionOutlines,
  saveDraft,
  startPreviewServer,
  startWithCommentPanelHidden,
  waitForScreenshotCaptureMode,
} from "./preview-feedback-helpers";

async function box(locator: Locator) {
  const value = await locator.boundingBox();
  if (!value) throw new Error("element has no bounding box");
  return value;
}

/** Outline position relative to the content it was drawn over, in page pixels. */
async function offsetFrom(outline: Locator, content: Locator) {
  const [a, b] = await Promise.all([box(outline), box(content)]);
  return { x: Math.round(a.x - b.x), y: Math.round(a.y - b.y) };
}

async function drawSavedRegion(page: Page, frame: FrameLocator, target: Locator, note: string) {
  await chooseCapture(page, "Select screenshot region");
  await waitForScreenshotCaptureMode(frame);
  await target.scrollIntoViewIfNeeded();
  await dragScreenshotRegion(page, frame, target);
  await saveDraft(page, note);
  await expect(regionOutlines(page)).toHaveCount(1);
}

async function narrowWindow(page: Page) {
  const size = page.viewportSize();
  if (!size) throw new Error("viewport size is unknown");
  await page.setViewportSize({ width: size.width - 240, height: size.height });
}

test.describe("Web preview region alignment", () => {
  test.describe.configure({ retries: 0, timeout: 180_000 });

  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-007.1
  test("keeps a region on centered content when the preview narrows", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const server = await startPreviewServer();
    try {
      await startWithCommentPanelHidden(testPage);
      const { frame } = await openBrowserPreview(
        testPage,
        apiClient,
        seedData,
        "Preview region follows content",
        server.url,
      );
      const card = frame.locator("#centered");
      await drawSavedRegion(testPage, frame, card, "Centered card spacing");
      const outline = regionOutlines(testPage).first();
      const before = await offsetFrom(outline, card);
      const cardBefore = await box(card);

      await narrowWindow(testPage);
      await expect.poll(async () => (await box(card)).x).toBeLessThan(cardBefore.x - 40);
      await expect.poll(() => offsetFrom(outline, card)).toEqual(before);
      await expect(outline).not.toHaveAttribute("data-drifted", "true");
    } finally {
      await server.close();
    }
  });

  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-007.3
  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-007.4
  test("flags a region whose content reflowed, clears it on move, and shows the saved capture", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const server = await startPreviewServer();
    try {
      await startWithCommentPanelHidden(testPage);
      const { frame } = await openBrowserPreview(
        testPage,
        apiClient,
        seedData,
        "Preview region drift",
        server.url,
      );
      await drawSavedRegion(testPage, frame, frame.locator("#fluid"), "Fluid card copy");
      const outline = regionOutlines(testPage).first();

      await outline.hover();
      await outline.getByRole("button", { name: "Show capture" }).click();
      await expect(outline.getByTestId("region-capture-image")).toBeVisible();
      await outline.getByRole("button", { name: "Hide capture" }).click();
      await expect(outline.getByTestId("region-capture-image")).toHaveCount(0);

      await narrowWindow(testPage);
      await expect(outline).toHaveAttribute("data-drifted", "true");
      const warning = outline.getByTestId("region-drift-warning");
      await expect(warning).toHaveAccessibleName(/page layout changed since this capture/);

      const start = await box(outline);
      await testPage.mouse.move(start.x + start.width / 2, start.y + start.height / 2);
      await testPage.mouse.down();
      await testPage.mouse.move(start.x + start.width / 2 + 12, start.y + start.height / 2 + 8, {
        steps: 4,
      });
      await testPage.mouse.up();
      await expect(outline).not.toHaveAttribute("data-drifted", "true", { timeout: 15_000 });
    } finally {
      await server.close();
    }
  });

  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-005.3
  test("closes an open comment when another tool is chosen, so the new tool works at once", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const server = await startPreviewServer();
    try {
      await startWithCommentPanelHidden(testPage);
      const { frame } = await openBrowserPreview(
        testPage,
        apiClient,
        seedData,
        "Preview tool switch",
        server.url,
      );
      await chooseCapture(testPage, "Select element");
      await frame.locator("#save").click();
      const draft = draftSurface(testPage);
      await expect(draft).toBeVisible();

      await testPage.getByTestId("preview-tool-region").click();
      await expect(draft).toBeHidden();
      await waitForScreenshotCaptureMode(frame);
      await dragScreenshotRegion(testPage, frame, frame.locator("h1"));
      await expect(draft).toBeVisible({ timeout: 15_000 });
      await expect(regionOutlines(testPage)).toHaveCount(1);
    } finally {
      await server.close();
    }
  });

  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-005.2
  test("moves the toolbar by its handle and keeps it there when the mode reopens", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const server = await startPreviewServer();
    try {
      await startWithCommentPanelHidden(testPage);
      await openBrowserPreview(testPage, apiClient, seedData, "Preview toolbar move", server.url);
      await chooseCapture(testPage, "Select element");
      const toolbar = testPage.getByTestId("preview-annotation-toolbar");
      const handle = testPage.getByTestId("preview-annotation-toolbar-handle");
      const before = await box(toolbar);
      const grip = await box(handle);

      await testPage.mouse.move(grip.x + grip.width / 2, grip.y + grip.height / 2);
      await testPage.mouse.down();
      await testPage.mouse.move(grip.x + grip.width / 2 - 40, grip.y + grip.height / 2 + 120, {
        steps: 5,
      });
      await testPage.mouse.up();
      const moved = await box(toolbar);
      expect(moved.y).toBeGreaterThan(before.y + 100);

      await testPage.getByTestId("preview-annotation-exit").click();
      await testPage.getByTestId("preview-feedback-trigger").click();
      await expect(toolbar).toBeVisible();
      const reopened = await box(toolbar);
      expect(Math.round(reopened.x)).toBe(Math.round(moved.x));
      expect(Math.round(reopened.y)).toBe(Math.round(moved.y));

      await handle.dblclick();
      await expect.poll(async () => Math.round((await box(toolbar)).y)).toBe(Math.round(before.y));
    } finally {
      await server.close();
    }
  });
});
