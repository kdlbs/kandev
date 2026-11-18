import { expect, type FrameLocator, type Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import {
  chooseCapture,
  draftSurface,
  dragScreenshotRegion,
  fillDraftComment,
  openBrowserPreview,
  saveDraft,
  startPreviewServer,
  waitForScreenshotCaptureMode,
} from "./preview-feedback-helpers";

/** Counts distinct colours in the draft thumbnail, drawn into a canvas in the Kandev page. */
async function draftImageColourCount(page: Page): Promise<number> {
  const image = draftSurface(page).getByRole("img", { name: "Screenshot preview" });
  await expect(image).toBeVisible({ timeout: 15_000 });
  return image.evaluate(async (element) => {
    const img = element as HTMLImageElement;
    if (!img.complete) await new Promise((resolve) => img.addEventListener("load", resolve));
    const canvas = document.createElement("canvas");
    canvas.width = img.naturalWidth;
    canvas.height = img.naturalHeight;
    const context = canvas.getContext("2d");
    if (!context) return 0;
    context.drawImage(img, 0, 0);
    const data = context.getImageData(0, 0, canvas.width, canvas.height).data;
    const colours = new Set<string>();
    for (let index = 0; index < data.length; index += 4) {
      colours.add(`${data[index]},${data[index + 1]},${data[index + 2]},${data[index + 3]}`);
      if (colours.size > 1) break;
    }
    return colours.size;
  });
}

async function fixtureHasWebGL(frame: FrameLocator): Promise<boolean> {
  return frame
    .locator("html")
    .evaluate(() =>
      Boolean((window as unknown as { __kandevFixtureWebGL?: boolean }).__kandevFixtureWebGL),
    );
}

test.describe("Web preview feedback comment surface", () => {
  test.describe.configure({ retries: 0, timeout: 120_000 });

  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-004.6
  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-005.4
  test("opens the shared comment surface at the capture and resumes a dismissed draft", async ({
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
        "Preview comment surface",
        server.url,
      );

      await chooseCapture(testPage, "Select element");
      // The floating surface is used while the comment panel is hidden.
      await testPage.getByTestId("preview-annotation-count").click();
      await expect(testPage.getByTestId("preview-comment-panel")).toBeHidden();
      const target = frame.locator("#save");
      await target.click();
      const surface = draftSurface(testPage);
      await expect(surface).toBeVisible({ timeout: 15_000 });
      await expect(testPage.getByTestId("preview-annotation-toolbar")).toBeVisible();
      await expect(surface.getByTestId("selection-comment-context-label")).toHaveText("/");
      await expect(surface.getByRole("button", { name: /Run/ })).toHaveCount(0);

      const targetBox = await target.boundingBox();
      const surfaceBox = await surface.boundingBox();
      expect(targetBox && surfaceBox).toBeTruthy();
      expect(Math.abs(surfaceBox!.x - targetBox!.x)).toBeLessThanOrEqual(24);
      const below = surfaceBox!.y >= targetBox!.y + targetBox!.height - 2;
      const above = surfaceBox!.y + surfaceBox!.height <= targetBox!.y + 2;
      expect(below || above).toBe(true);

      await fillDraftComment(testPage, "Half-written note");
      await testPage.keyboard.press("Escape");
      await expect(surface).toBeHidden();

      const trigger = testPage.getByTestId("preview-feedback-trigger");
      await expect(testPage.getByTestId("preview-feedback-unsaved-indicator")).toBeVisible();
      await testPage.getByTestId("preview-annotation-count").click();
      await expect(testPage.getByTestId("preview-comment-panel")).toBeVisible();
      const unsaved = testPage.getByTestId("preview-unsaved-draft");
      await expect(unsaved).toContainText("/");
      await unsaved.getByTestId("preview-unsaved-resume").click();
      await expect(testPage.getByTestId("preview-feedback-draft-input")).toHaveValue(
        "Half-written note",
      );
      await saveDraft(testPage, "Make the primary action more prominent");
      await expect(trigger).toContainText("1");
    } finally {
      await server.close();
    }
  });

  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-002.8
  test("captures WebGL pixels that the page does not preserve", async ({
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
        "Preview WebGL capture",
        server.url,
      );
      test.skip(!(await fixtureHasWebGL(frame)), "this browser build has no WebGL");

      await chooseCapture(testPage, "Select screenshot region");
      await waitForScreenshotCaptureMode(frame);
      await dragScreenshotRegion(testPage, frame, frame.locator("#gl-canvas"));

      await expect(draftSurface(testPage)).toBeVisible({ timeout: 15_000 });
      expect(await draftImageColourCount(testPage)).toBeGreaterThan(1);
      await expect(
        draftSurface(testPage).getByText(/Canvas content may not have been captured/),
      ).toHaveCount(0);
      await saveDraft(testPage, "The red banner should be blue");
    } finally {
      await server.close();
    }
  });
});
