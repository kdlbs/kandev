import { expect, type Locator, type Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import {
  chooseCapture,
  draftSurface,
  dragScreenshotRegion,
  exitAnnotation,
  openBrowserPreview,
  startWithCommentPanelHidden,
  saveDraft,
  selectText,
  startPreviewServer,
  waitForScreenshotCaptureMode,
} from "./preview-feedback-helpers";

async function box(locator: Locator) {
  const value = await locator.boundingBox();
  if (!value) throw new Error("element has no bounding box");
  return value;
}

async function dragBy(page: Page, from: { x: number; y: number }, dx: number, dy: number) {
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move(from.x + dx / 2, from.y + dy / 2);
  await page.mouse.move(from.x + dx, from.y + dy);
  await page.mouse.up();
}

test.describe("Web preview annotation mode", () => {
  test.describe.configure({ retries: 0, timeout: 180_000 });

  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-005.1
  test("comments on selected text without entering annotation mode", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const server = await startPreviewServer();
    try {
      const { frame } = await openBrowserPreview(
        testPage,
        apiClient,
        seedData,
        "Preview selection comment",
        server.url,
      );
      const trigger = testPage.getByTestId("preview-feedback-trigger");
      await expect(trigger).toHaveAttribute("data-annotation-active", "false");

      await selectText(frame, "h1");
      const button = testPage.getByTestId("preview-selection-comment");
      await expect(button).toBeVisible();
      await button.click();
      await expect(draftSurface(testPage)).toContainText("Checkout");
      await saveDraft(testPage, "Use the product name in the heading");
      await expect(trigger).toContainText("1");
      await expect(trigger).toHaveAttribute("data-annotation-active", "false");

      await expect(testPage.getByTestId("preview-feedback-saved-status")).toHaveText(
        "Comment saved. 1 pending.",
      );

      const marker = frame.locator("[data-kandev-inspector-marker]");
      await expect(marker).toHaveText("1");
      await marker.click();
      const editor = testPage.getByTestId("preview-feedback-edit-input");
      await expect(editor).toHaveValue("Use the product name in the heading");
      await expect(trigger).toHaveAttribute("data-annotation-active", "false");
      await frame.locator("#save").dispatchEvent("pointerdown");
      await expect(editor).toBeHidden();
      await expect(marker).toHaveText("1");
    } finally {
      await server.close();
    }
  });

  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-005.3
  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-005.4
  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-005.5
  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-005.6
  test("draws several regions in a row, resumes a dismissed one, and resizes a saved one", async ({
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
        "Preview multiple regions",
        server.url,
      );
      await chooseCapture(testPage, "Select screenshot region");
      await waitForScreenshotCaptureMode(frame);

      await dragScreenshotRegion(testPage, frame, frame.locator("#save"));
      await expect(draftSurface(testPage)).toBeVisible({ timeout: 15_000 });
      await testPage.getByTestId("preview-feedback-draft-input").fill("First region");
      await testPage.keyboard.press("Escape");
      await expect(draftSurface(testPage)).toBeHidden();

      await dragScreenshotRegion(testPage, frame, frame.locator("h1"));
      await expect(draftSurface(testPage)).toBeVisible({ timeout: 15_000 });
      await saveDraft(testPage, "Second region");

      const regions = testPage.getByTestId("preview-region");
      await expect(regions).toHaveCount(2);
      await expect(
        testPage.locator('[data-testid="preview-region"][data-pending="true"]'),
      ).toHaveCount(1);

      await testPage.locator('[data-testid="preview-region"][data-pending="true"]').click();
      await expect(testPage.getByTestId("preview-feedback-draft-input")).toHaveValue(
        "First region",
      );
      await saveDraft(testPage, "First region");
      await expect(testPage.getByTestId("preview-feedback-trigger")).toContainText("2");

      const panel = testPage.getByTestId("preview-comment-panel");
      const panelToggle = testPage.getByTestId("preview-annotation-count");
      await panelToggle.click();
      await expect(panel).toBeVisible();
      const thumbnail = panel.getByRole("img", { name: "Screenshot preview" }).first();
      const before = await thumbnail.getAttribute("src");
      await panelToggle.click();
      await expect(panel).toBeHidden();

      const savedRegion = regions.first();
      const regionBox = await box(savedRegion);
      const corner = savedRegion.locator('[data-region-handle="se"]');
      const cornerBox = await box(corner);
      await dragBy(
        testPage,
        { x: cornerBox.x + cornerBox.width / 2, y: cornerBox.y + cornerBox.height / 2 },
        30,
        20,
      );
      await expect
        .poll(async () => (await box(savedRegion)).width)
        .toBeGreaterThan(regionBox.width + 20);

      await panelToggle.click();
      await expect
        .poll(() =>
          panel.getByRole("img", { name: "Screenshot preview" }).first().getAttribute("src"),
        )
        .not.toBe(before);
      await expect(panel).toContainText("First region");

      await exitAnnotation(testPage);
      await expect(regions).toHaveCount(0);
      await expect(frame.locator("[data-kandev-inspector-marker]")).toHaveCount(0);
    } finally {
      await server.close();
    }
  });

  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-005.7
  test("keeps a region outline on its content while an inner panel scrolls", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const server = await startPreviewServer();
    try {
      const { frame } = await openBrowserPreview(
        testPage,
        apiClient,
        seedData,
        "Preview scroll tracking",
        server.url,
      );
      await frame.locator("#scroller").scrollIntoViewIfNeeded();
      await chooseCapture(testPage, "Select screenshot region");
      await waitForScreenshotCaptureMode(frame);
      await dragScreenshotRegion(testPage, frame, frame.locator("#scroll-target"));
      await saveDraft(testPage, "Scroll panel header");

      const region = testPage.getByTestId("preview-region");
      await expect(region).toHaveCount(1);
      const before = await box(region);
      await frame.locator("#scroller").evaluate((element) => {
        element.scrollTop = 40;
      });
      await expect.poll(async () => (await box(region)).y).toBeLessThan(before.y - 30);
    } finally {
      await server.close();
    }
  });

  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-005.5
  test("steps out with Escape from inside the page and exits with the toolbar", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const server = await startPreviewServer();
    try {
      const { frame } = await openBrowserPreview(
        testPage,
        apiClient,
        seedData,
        "Preview escape layers",
        server.url,
      );
      const trigger = testPage.getByTestId("preview-feedback-trigger");
      await chooseCapture(testPage, "Select element");
      await frame.locator("#save").click();
      await expect(draftSurface(testPage)).toBeVisible({ timeout: 15_000 });
      await testPage.getByTestId("preview-feedback-draft-input").fill("Keep as a draft");

      await frame
        .locator("h1")
        .focus()
        .catch(() => undefined);
      await testPage.keyboard.press("Escape");
      await expect(draftSurface(testPage)).toBeHidden();
      await expect(trigger).toHaveAttribute("data-capture-mode", "element");

      await testPage.keyboard.press("Escape");
      await expect(trigger).toHaveAttribute("data-capture-mode", "none");
      await expect(testPage.getByTestId("preview-tool-interact")).toHaveAttribute(
        "aria-pressed",
        "true",
      );

      await testPage.keyboard.press("Escape");
      await expect(trigger).toHaveAttribute("data-annotation-active", "false");
      await expect(testPage.getByTestId("preview-feedback-unsaved-indicator")).toBeVisible();

      await chooseCapture(testPage, "Select element");
      await testPage.getByTestId("preview-annotation-exit").click();
      await expect(trigger).toHaveAttribute("data-annotation-active", "false");
      await frame.locator("#details-route").click();
      await expect(frame.locator("h1")).toHaveText("Order details");
      await expect(frame.locator("html")).not.toHaveCSS("cursor", "crosshair");
    } finally {
      await server.close();
    }
  });
});
