import type { Locator } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  armHtmlPreviewPublication,
  createHtmlDiffTask,
  nativeHtmlDocument,
  SOURCE_COPY,
  TRUST_COPY,
} from "./html-diff-preview-helpers";

function htmlPreviewFrame(filePath: string) {
  return `[data-testid="review-file-header"][data-file-path="${filePath}"]`;
}

/** Checks the 44px touch target with 0.001 CSS pixels of coordinate tolerance. */
async function expectTouchTarget(locator: Locator) {
  const box = await locator.boundingBox();
  // Transformed browser coordinates can differ by less than 0.001 CSS pixels.
  const precision = 0.001;
  expect((box?.width ?? 0) + precision).toBeGreaterThanOrEqual(44);
  expect((box?.height ?? 0) + precision).toBeGreaterThanOrEqual(44);
}

test.describe("HTML diff preview on mobile", () => {
  test.describe.configure({ retries: 0, timeout: 120_000 });

  test("opens the preview from the focused diff sheet and returns to the diff", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const { filePath } = await createHtmlDiffTask({
      testPage,
      apiClient,
      seedData,
      backend,
      title: "Mobile HTML Diff Preview",
      fileName: "index.html",
      entryContent: nativeHtmlDocument("Mobile diff preview"),
    });

    await testPage
      .getByRole("navigation")
      .getByRole("button", { name: /Changes$/ })
      .tap();
    await expect(testPage.getByTestId("mobile-changes-panel")).toBeVisible();
    const row = testPage.getByTestId(`file-row-${filePath.replace(/[/\\]/g, "-")}`);
    await expect(row).toBeVisible({ timeout: 20_000 });
    await row.tap();

    const sheet = testPage.getByTestId("mobile-diff-sheet");
    await expect(sheet).toBeVisible();
    await expect(sheet.getByTestId("mobile-diff-sheet-close")).toBeVisible();
    const header = sheet.locator(htmlPreviewFrame(filePath));
    const menuTrigger = header.getByRole("button", {
      name: `More actions for ${filePath}`,
    });
    await expectTouchTarget(menuTrigger);
    await menuTrigger.tap();

    const menu = testPage.getByTestId("review-file-actions-menu");
    await waitForFiniteAnimations(menu);
    const help = menu.getByTestId("review-html-preview-help");
    await expect(help).toContainText(SOURCE_COPY);
    await expect(help).toContainText(TRUST_COPY);
    const menuItem = menu.getByRole("menuitem", { name: "Preview HTML" });
    await expectTouchTarget(menuItem);
    const publication = armHtmlPreviewPublication(testPage);
    await menuItem.tap();
    await publication.published;
    await publication.resources;

    const preview = sheet.getByTestId("review-html-diff-preview");
    await expect(preview).toBeVisible();
    await expect(preview.getByTestId("review-html-preview-source")).toContainText(SOURCE_COPY);
    await expect(preview.getByTestId("html-preview-trust-warning")).toContainText(TRUST_COPY);
    const frame = preview.frameLocator("iframe");
    await expect(frame.locator("h1")).toHaveText("Mobile diff preview");
    await expect(frame.locator("#script-status")).toHaveText("script:ready");
    await expect(frame.locator("#css-status")).toHaveCSS("font-weight", "700");

    const showDiff = preview.getByRole("button", { name: "Show diff" });
    await expectTouchTarget(showDiff);
    await expectTouchTarget(preview.getByRole("button", { name: "Refresh HTML preview" }));
    const viewport = testPage.viewportSize();
    expect(viewport).not.toBeNull();
    const previewBox = await preview.boundingBox();
    expect(previewBox).not.toBeNull();
    expect(previewBox!.x).toBeGreaterThanOrEqual(0);
    expect(previewBox!.x + previewBox!.width).toBeLessThanOrEqual(viewport!.width + 1);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile HTML diff preview");
    await prCapture.screenshot("html-diff-preview-mobile", {
      caption: "Native HTML preview opened from the focused mobile diff sheet",
    });

    await showDiff.tap();
    await expect(sheet.getByTestId("review-html-diff-preview")).toHaveCount(0);
    await expect(
      header.getByRole("button", { name: `More actions for ${filePath}` }),
    ).toBeVisible();
    await expect(sheet).toBeVisible();
  });

  test("opens and closes the preview inside the expanded Review dialog", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const { filePath } = await createHtmlDiffTask({
      testPage,
      apiClient,
      seedData,
      backend,
      title: "Mobile HTML Diff Preview Review",
      fileName: "review.html",
      entryContent: nativeHtmlDocument("Mobile review dialog preview"),
    });

    await testPage
      .getByRole("navigation")
      .getByRole("button", { name: /Changes$/ })
      .tap();
    const changesPanel = testPage.getByTestId("mobile-changes-panel");
    await expect(changesPanel).toBeVisible();
    await expect(testPage.getByTestId(`file-row-${filePath.replace(/[/\\]/g, "-")}`)).toBeVisible({
      timeout: 20_000,
    });
    await changesPanel.getByRole("button", { name: "Review", exact: true }).tap();

    const dialog = testPage.getByRole("dialog", { name: "Review Changes" });
    await expect(dialog).toBeVisible();
    const header = dialog.locator(htmlPreviewFrame(filePath));
    await header.getByRole("button", { name: `More actions for ${filePath}` }).tap();
    const menuItem = testPage
      .getByTestId("review-file-actions-menu")
      .getByRole("menuitem", { name: "Preview HTML" });
    const publication = armHtmlPreviewPublication(testPage);
    await menuItem.tap();
    await publication.published;
    await publication.resources;

    const preview = dialog.getByTestId("review-html-diff-preview");
    await expect(preview).toBeVisible();
    await expect(preview.frameLocator("iframe").locator("h1")).toHaveText(
      "Mobile review dialog preview",
    );
    await expect(dialog).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "mobile review HTML diff preview");

    await preview.getByRole("button", { name: "Show diff" }).tap();
    await expect(dialog.getByTestId("review-html-diff-preview")).toHaveCount(0);
    await expect(dialog).toBeVisible();
    await expect(
      header.getByRole("button", { name: `More actions for ${filePath}` }),
    ).toBeVisible();
  });
});
