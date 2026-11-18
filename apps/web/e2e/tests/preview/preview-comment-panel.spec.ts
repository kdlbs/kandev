import { expect, type FrameLocator, type Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import {
  chooseCapture,
  draftSurface,
  dragScreenshotRegion,
  openBrowserPreview,
  saveDraft,
  startPreviewServer,
  waitForScreenshotCaptureMode,
} from "./preview-feedback-helpers";

const PANEL = "preview-comment-panel";
const TOGGLE = "preview-annotation-count";

async function frameWidth(page: Page) {
  const box = await page.getByTestId("preview-annotation-frame").boundingBox();
  if (!box) throw new Error("preview frame has no bounding box");
  return box.width;
}

async function selectText(frame: FrameLocator, selector: string) {
  await frame.locator(selector).evaluate((element) => {
    const range = document.createRange();
    range.selectNodeContents(element);
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
    element.dispatchEvent(new MouseEvent("mouseup", { bubbles: true }));
  });
}

test.describe("Web preview comment panel", () => {
  test.describe.configure({ retries: 0, timeout: 180_000 });

  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-006.1
  test("docks beside the preview, hides for full width, and remembers the choice", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const server = await startPreviewServer();
    try {
      await openBrowserPreview(testPage, apiClient, seedData, "Preview panel toggle", server.url);
      const before = await frameWidth(testPage);
      await chooseCapture(testPage, "Select screenshot region");

      const panel = testPage.getByTestId(PANEL);
      await expect(panel).toBeVisible();
      await expect(testPage.getByTestId(TOGGLE)).toHaveAttribute("aria-pressed", "true");
      if ((await panel.getAttribute("data-overlay")) === "true") {
        // A narrow preview keeps its width; the panel overlays it below the toolbar.
        expect(await frameWidth(testPage)).toBe(before);
        await expect(testPage.getByTestId("preview-tool-region")).toBeVisible();
      } else {
        expect(await frameWidth(testPage)).toBeLessThan(before);
      }

      await testPage.getByTestId(TOGGLE).click();
      await expect(panel).toBeHidden();
      await expect(testPage.getByTestId(TOGGLE)).toHaveAttribute("aria-pressed", "false");
      await expect.poll(() => frameWidth(testPage)).toBe(before);

      await testPage.getByTestId("preview-annotation-exit").click();
      await testPage.getByTestId("preview-feedback-trigger").click();
      await expect(testPage.getByTestId("preview-annotation-toolbar")).toBeVisible();
      await expect(panel).toBeHidden();

      await testPage.getByTestId(TOGGLE).click();
      await expect(panel).toBeVisible();
    } finally {
      await server.close();
    }
  });

  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-006.3
  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-006.4
  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-006.5
  test("writes region comments in the floating box and removes an empty capture on Close", async ({
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
        "Preview panel regions",
        server.url,
      );
      await chooseCapture(testPage, "Select screenshot region");
      await waitForScreenshotCaptureMode(frame);
      const panel = testPage.getByTestId(PANEL);
      await expect(panel).toBeVisible();
      const regions = testPage.getByTestId("preview-region");
      const draft = draftSurface(testPage);

      await dragScreenshotRegion(testPage, frame, frame.locator("#save"));
      await expect(regions).toHaveCount(1);
      await expect(draft.getByTestId("preview-feedback-draft-input")).toBeFocused();
      await expect(panel.getByTestId("preview-feedback-draft-input")).toHaveCount(0);
      await expect(draft.getByRole("img", { name: "Screenshot preview" })).toBeVisible({
        timeout: 15_000,
      });
      await expect(regions.first()).not.toHaveAttribute("data-busy", "true");
      await draft.getByRole("button", { name: "Close" }).click();
      await expect(regions).toHaveCount(0);
      await expect(panel.getByTestId("preview-unsaved-draft")).toHaveCount(0);

      await dragScreenshotRegion(testPage, frame, frame.locator("h1"));
      await draft.getByTestId("preview-feedback-draft-input").fill("Tighter heading");
      await draft.getByRole("button", { name: "Close" }).click();
      await expect(regions).toHaveCount(1);
      await expect(regions.first()).toHaveAttribute("data-pending", "true");
      await expect(panel.getByTestId("preview-unsaved-draft")).toContainText("Tighter heading");

      await panel.getByTestId("preview-unsaved-resume").click();
      await expect(panel.getByTestId("preview-unsaved-draft")).toHaveAttribute(
        "data-active",
        "true",
      );
      await saveDraft(testPage, "Tighter heading");
      await expect(panel.getByTestId("preview-feedback-item")).toHaveCount(1);
      await expect(
        panel.getByTestId("preview-feedback-item").getByTestId("preview-capture-number"),
      ).toHaveText("1");
    } finally {
      await server.close();
    }
  });

  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-006.2
  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-006.6
  test("pauses element picking while commenting and opens a comment from its marker", async ({
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
        "Preview panel markers",
        server.url,
      );
      const trigger = testPage.getByTestId("preview-feedback-trigger");
      await chooseCapture(testPage, "Select element");
      await frame.locator("#save").click();
      await expect(draftSurface(testPage)).toBeVisible();
      await expect(trigger).toHaveAttribute("data-capture-mode", "none");
      await frame.locator("h1").hover();
      await expect(trigger).toHaveAttribute("data-capture-mode", "none");

      await saveDraft(testPage, "Make the button primary");
      await expect(trigger).toHaveAttribute("data-capture-mode", "element");

      const marker = frame.locator("[data-kandev-inspector-marker]");
      await expect(marker).toHaveText("1");
      await marker.click();
      const panel = testPage.getByTestId(PANEL);
      await expect(panel.getByTestId("preview-feedback-edit-input")).toHaveCount(0);
      await expect(testPage.getByTestId("preview-feedback-edit-input")).toHaveValue(
        "Make the button primary",
      );
    } finally {
      await server.close();
    }
  });

  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-006.4
  test("with the panel hidden, a press in the page closes an empty floating comment", async ({
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
        "Preview panel hidden",
        server.url,
      );
      await chooseCapture(testPage, "Select text");
      await testPage.getByTestId(TOGGLE).click();
      await expect(testPage.getByTestId(PANEL)).toBeHidden();

      await selectText(frame, "h1");
      const draft = draftSurface(testPage);
      await expect(draft).toBeVisible();
      await expect(draft.getByRole("button", { name: "Close" })).toBeVisible();
      await frame.locator("#save").dispatchEvent("pointerdown");
      await expect(draft).toBeHidden();
      await expect(frame.locator("[data-kandev-inspector-marker]")).toHaveCount(0);
    } finally {
      await server.close();
    }
  });
});
