import { expect, type FrameLocator, type Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import {
  draftSurface,
  openBrowserPreview,
  saveDraft,
  selectText,
  startPreviewServer,
} from "./preview-feedback-helpers";

const EDIT_SURFACE = '[data-testid="preview-feedback-edit"]';
const SAMPLED_FRAMES = 30;

type Sample = { left: number; top: number; opacity: string };

async function commentOnHeading(page: Page, frame: FrameLocator) {
  await selectText(frame, "h1");
  await page.getByTestId("preview-selection-comment").click();
  await expect(draftSurface(page)).toContainText("Checkout");
  await saveDraft(page, "Use the product name in the heading");
  const marker = frame.locator("[data-kandev-inspector-marker]");
  await expect(marker).toHaveText("1");
  return marker;
}

/** Records the edit box's position and opacity on every frame from its first one. */
async function recordEditSurface(page: Page) {
  await page.evaluate(
    ({ selector, frames }) => {
      const samples: Sample[] = [];
      (window as unknown as { __editSurface: Sample[] }).__editSurface = samples;
      const tick = () => {
        const surface = document.querySelector<HTMLElement>(selector);
        if (surface) {
          const rect = surface.getBoundingClientRect();
          const opacity = getComputedStyle(surface).opacity;
          samples.push({ left: rect.left, top: rect.top, opacity });
        }
        if (samples.length < frames) requestAnimationFrame(tick);
      };
      requestAnimationFrame(tick);
    },
    { selector: EDIT_SURFACE, frames: SAMPLED_FRAMES },
  );
  return {
    samples: () =>
      page.evaluate(() => (window as unknown as { __editSurface: Sample[] }).__editSurface),
  };
}

function paintedText(frame: FrameLocator) {
  return frame.locator("body").evaluate(() => {
    const highlight = CSS.highlights.get("kandev-comment-target");
    return highlight ? Array.from(highlight, (range) => (range as Range).toString()) : [];
  });
}

test.describe("Web preview marker targets", () => {
  test.describe.configure({ retries: 0, timeout: 180_000 });

  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-005.8
  test("opens a marker's comment beside it in place, without animating or moving", async ({
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
        "Preview marker opens in place",
        server.url,
      );
      const marker = await commentOnHeading(testPage, frame);
      const recorder = await recordEditSurface(testPage);
      await marker.click();

      await expect.poll(async () => (await recorder.samples()).length).toBe(SAMPLED_FRAMES);
      const samples = await recorder.samples();
      const [first] = samples;
      expect(samples.every((sample) => sample.opacity === "1")).toBe(true);
      expect(samples.every((s) => s.left === first.left && s.top === first.top)).toBe(true);

      const markerBox = await marker.boundingBox();
      if (!markerBox) throw new Error("marker has no bounding box");
      expect(Math.abs(first.left - (markerBox.x + markerBox.width / 2))).toBeLessThan(40);
      expect(Math.abs(first.top - (markerBox.y + markerBox.height / 2))).toBeLessThan(40);
    } finally {
      await server.close();
    }
  });

  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-005.9
  test("highlights a marker's text while hovered and while its comment is open", async ({
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
        "Preview marker highlights its text",
        server.url,
      );
      const marker = await commentOnHeading(testPage, frame);
      await expect.poll(() => paintedText(frame)).toEqual([]);

      await marker.hover();
      await expect.poll(() => paintedText(frame)).toEqual(["Checkout"]);
      await frame.locator("#save").hover();
      await expect.poll(() => paintedText(frame)).toEqual([]);

      await marker.click();
      await expect(testPage.locator(EDIT_SURFACE)).toBeVisible();
      await frame.locator("#save").hover();
      await expect.poll(() => paintedText(frame)).toEqual(["Checkout"]);

      await frame.locator("#save").dispatchEvent("pointerdown");
      await expect(testPage.locator(EDIT_SURFACE)).toBeHidden();
      await expect.poll(() => paintedText(frame)).toEqual([]);
    } finally {
      await server.close();
    }
  });
});
