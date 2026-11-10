import { type Locator, type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { waitForHttp } from "../../helpers/causal-waits";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";
import {
  armHtmlPreviewPublication,
  createHtmlDiffTask,
  nativeHtmlDocument,
  SOURCE_COPY,
  TRUST_COPY,
} from "./html-diff-preview-helpers";

async function expectTouchTarget(locator: Locator) {
  const box = await locator.boundingBox();
  expect(box?.width ?? 0).toBeGreaterThanOrEqual(44);
  expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
}

async function openSingleFileDiff(testPage: Page, session: SessionPage, filePath: string) {
  await session.clickTab("Changes");
  const row = testPage.getByTestId(`file-row-${filePath.replace(/[/\\]/g, "-")}`).first();
  await expect(row).toBeVisible({ timeout: 20_000 });
  await row.click();
  const header = testPage
    .locator(`[data-testid="review-file-header"][data-file-path="${filePath}"]`)
    .first();
  await expect(header).toBeVisible({ timeout: 20_000 });
  return header;
}

test.describe("HTML diff preview", () => {
  test.describe.configure({ retries: 0, timeout: 120_000 });

  test("previews the complete workspace file from a single-file diff", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const { session, filePath } = await createHtmlDiffTask({
      testPage,
      apiClient,
      seedData,
      backend,
      title: "HTML Diff Preview Desktop",
      fileName: "index.html",
      entryContent: nativeHtmlDocument("Diff preview workspace"),
    });

    const header = await openSingleFileDiff(testPage, session, filePath);
    await expect(header.getByRole("button", { name: "Preview HTML" })).toBeVisible();
    await expect(header.getByRole("checkbox")).not.toBeChecked();
    const openTabs = await testPage.locator(".dv-default-tab:visible").count();
    const publication = armHtmlPreviewPublication(testPage);
    await header.getByRole("button", { name: "Preview HTML" }).click();
    await publication.published;
    await publication.resources;

    const preview = testPage.getByTestId("review-html-diff-preview");
    await expect(preview).toBeVisible();
    await expect(preview.getByTestId("review-html-preview-source")).toContainText(SOURCE_COPY);
    await expect(preview.getByTestId("html-preview-trust-warning")).toContainText(TRUST_COPY);
    const frame = preview.frameLocator("iframe");
    await expect(frame.locator("h1")).toHaveText("Diff preview workspace");
    await expect(frame.locator("#script-status")).toHaveText("script:ready");
    await expect(frame.locator("#css-status")).toHaveCSS("font-weight", "700");

    await expect(session.browserPanel).toHaveCount(0);
    await expect(testPage.locator(".monaco-editor:visible")).toHaveCount(0);
    await expect(testPage.locator(".cm-editor:visible")).toHaveCount(0);
    expect(await testPage.locator(".dv-default-tab:visible").count()).toBe(openTabs);
    await expect(header.getByRole("checkbox")).not.toBeChecked();
    await prCapture.screenshot("html-diff-preview-desktop", {
      caption: "Native HTML preview opened from a desktop Changes diff",
    });

    await header.getByRole("button", { name: "Show diff" }).click();
    await expect(testPage.getByTestId("review-html-diff-preview")).toHaveCount(0);
    await expect(header.getByRole("button", { name: "Preview HTML" })).toBeVisible();
  });

  test("previews the workspace revision when the staged patch differs", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const stagedDocument = nativeHtmlDocument("Staged revision");
    const workspaceDocument = nativeHtmlDocument("Workspace revision");
    const { session, git, filePath } = await createHtmlDiffTask({
      testPage,
      apiClient,
      seedData,
      backend,
      title: "HTML Diff Preview Staged Source",
      fileName: "staged.html",
      entryContent: stagedDocument,
    });
    git.stageFile(filePath);
    git.createFile(filePath, workspaceDocument);
    expect(git.exec(`git show ":${filePath}"`)).toContain("Staged revision");

    const header = await openSingleFileDiff(testPage, session, filePath);
    await expect(header.getByRole("button", { name: "Preview HTML" })).toBeVisible();
    const publication = armHtmlPreviewPublication(testPage);
    await header.getByRole("button", { name: "Preview HTML" }).click();
    await publication.published;
    await publication.resources;

    const preview = testPage.getByTestId("review-html-diff-preview");
    await expect(preview).toBeVisible();
    await expect(preview.getByTestId("review-html-preview-source")).toContainText(SOURCE_COPY);
    const frame = preview.frameLocator("iframe");
    await expect(frame.locator("h1")).toHaveText("Workspace revision");
    await expect(frame.locator("h1")).not.toHaveText("Staged revision");
  });

  test("shows a retryable publication error without losing the diff", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const { session, filePath } = await createHtmlDiffTask({
      testPage,
      apiClient,
      seedData,
      backend,
      title: "HTML Diff Preview Recovery",
      fileName: "recovery.html",
      entryContent: nativeHtmlDocument("Recovered workspace preview"),
    });

    const header = await openSingleFileDiff(testPage, session, filePath);
    await testPage.route("**/api/v1/task-sessions/*/html-previews", async (route) => {
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: "agentctl unavailable" }),
      });
    });

    const failedPublication = waitForHttp(testPage, "POST", /\/html-previews$/);
    await header.getByRole("button", { name: "Preview HTML" }).click();
    await failedPublication;
    const preview = testPage.getByTestId("review-html-diff-preview");
    await expect(preview.getByTestId("html-preview-error")).toContainText(
      "HTML preview session is not available",
    );
    await expect(header.getByRole("button", { name: "Show diff" })).toBeVisible();
    await expect(session.browserPanel).toHaveCount(0);

    await testPage.unroute("**/api/v1/task-sessions/*/html-previews");
    const retryPublication = armHtmlPreviewPublication(testPage);
    await preview.getByRole("button", { name: "Retry HTML preview" }).click();
    await retryPublication.published;
    await retryPublication.resources;
    await expect(preview.frameLocator("iframe").locator("h1")).toHaveText(
      "Recovered workspace preview",
    );

    await header.getByRole("button", { name: "Show diff" }).click();
    await expect(testPage.getByTestId("review-html-diff-preview")).toHaveCount(0);
  });

  test("replaces the body with the preview from the expanded Review dialog", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const { session, filePath } = await createHtmlDiffTask({
      testPage,
      apiClient,
      seedData,
      backend,
      title: "HTML Diff Preview Review Dialog",
      fileName: "review.html",
      entryContent: nativeHtmlDocument("Expanded review preview"),
    });

    await session.clickTab("Changes");
    await expect(
      testPage.getByTestId(`file-row-${filePath.replace(/[/\\]/g, "-")}`).first(),
    ).toBeVisible({ timeout: 20_000 });
    await testPage
      .getByTestId(`file-row-${filePath.replace(/[/\\]/g, "-")}`)
      .first()
      .click();
    await testPage.getByRole("button", { name: "Expand review" }).click();

    const dialog = testPage.getByRole("dialog", { name: "Review Changes" });
    await expect(dialog).toBeVisible();
    const header = dialog.locator(
      `[data-testid="review-file-header"][data-file-path="${filePath}"]`,
    );
    const publication = armHtmlPreviewPublication(testPage);
    await header.getByRole("button", { name: "Preview HTML" }).click();
    await publication.published;
    await publication.resources;

    const preview = dialog.getByTestId("review-html-diff-preview");
    await expect(preview).toBeVisible();
    await expect(preview.frameLocator("iframe").locator("h1")).toHaveText(
      "Expanded review preview",
    );
    await expect(dialog).toBeVisible();

    await header.getByRole("button", { name: "Show diff" }).click();
    await expect(dialog.getByTestId("review-html-diff-preview")).toHaveCount(0);
    await expect(dialog).toBeVisible();
  });

  test("discloses source and trust copy before activation on a coarse-pointer tablet", async ({
    tabletTestPage,
    apiClient,
    seedData,
    backend,
  }) => {
    await tabletTestPage.setViewportSize({ width: 1024, height: 900 });
    await expect
      .poll(() => tabletTestPage.evaluate(() => matchMedia("(pointer: coarse)").matches))
      .toBe(true);
    const { session, filePath } = await createHtmlDiffTask({
      testPage: tabletTestPage,
      apiClient,
      seedData,
      backend,
      title: "HTML Diff Preview Tablet Disclosure",
      fileName: "tablet.html",
      entryContent: nativeHtmlDocument("Tablet disclosure preview"),
    });

    const header = await openSingleFileDiff(tabletTestPage, session, filePath);
    const trigger = header.getByRole("button", { name: "Preview HTML" });
    await expectTouchTarget(trigger);
    await trigger.click();

    const drawer = tabletTestPage.getByTestId("review-html-preview-help-drawer");
    await expect(drawer).toBeVisible();
    await expect(drawer).toContainText(SOURCE_COPY);
    await expect(drawer).toContainText(TRUST_COPY);
    await expect(tabletTestPage.getByTestId("review-html-diff-preview")).toHaveCount(0);

    const confirm = drawer.getByTestId("review-html-preview-confirm");
    await expectTouchTarget(confirm);
    const publication = armHtmlPreviewPublication(tabletTestPage);
    await confirm.click();
    await publication.published;
    await publication.resources;

    const preview = tabletTestPage.getByTestId("review-html-diff-preview");
    await expect(preview).toBeVisible();
    await expect(preview.frameLocator("iframe").locator("h1")).toHaveText(
      "Tablet disclosure preview",
    );
    await expect(tabletTestPage.getByTestId("review-html-preview-help-drawer")).toHaveCount(0);

    await preview.getByRole("button", { name: "Show diff" }).click();
    await expect(tabletTestPage.getByTestId("review-html-diff-preview")).toHaveCount(0);
    await expect(header.getByRole("button", { name: "Preview HTML" })).toBeVisible();
  });

  test("keeps the action and 44-pixel targets at a narrow fine-pointer viewport", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    await testPage.setViewportSize({ width: 760, height: 900 });
    const { filePath } = await createHtmlDiffTask({
      testPage,
      apiClient,
      seedData,
      backend,
      title: "HTML Diff Preview Narrow",
      fileName: "narrow.html",
      entryContent: nativeHtmlDocument("Narrow viewport preview"),
    });

    await testPage
      .getByRole("navigation")
      .getByRole("button", { name: /Changes$/ })
      .click();
    const changesPanel = testPage.getByTestId("mobile-changes-panel");
    await expect(changesPanel).toBeVisible();
    const row = testPage.getByTestId(`file-row-${filePath.replace(/[/\\]/g, "-")}`).first();
    await expect(row).toBeVisible({ timeout: 20_000 });
    await row.click();

    const sheet = testPage.getByTestId("mobile-diff-sheet");
    await expect(sheet).toBeVisible();
    const header = sheet
      .locator(`[data-testid="review-file-header"][data-file-path="${filePath}"]`)
      .first();
    await expect(header).toBeVisible();
    const menuTrigger = header.getByRole("button", {
      name: `More actions for ${filePath}`,
    });
    await expect(menuTrigger).toBeVisible();
    await menuTrigger.click();

    const menu = testPage.getByTestId("review-file-actions-menu");
    await waitForFiniteAnimations(menu);
    const menuItem = menu.getByRole("menuitem", { name: "Preview HTML" });
    await expect(menuItem).toBeVisible();
    const menuItemBox = await menuItem.boundingBox();
    expect(menuItemBox?.height ?? 0).toBeGreaterThanOrEqual(44);

    await testPage.route("**/api/v1/task-sessions/*/html-previews", async (route) => {
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: "agentctl unavailable" }),
      });
    });
    const failedPublication = waitForHttp(testPage, "POST", /\/html-previews$/);
    await menuItem.click();
    await failedPublication;

    const preview = testPage.getByTestId("review-html-diff-preview");
    await expect(preview).toBeVisible();
    await expect(preview.getByTestId("html-preview-error")).toContainText(
      "HTML preview session is not available",
    );
    const retry = preview.getByRole("button", { name: "Retry HTML preview" });
    await expectTouchTarget(retry);
    await expectTouchTarget(preview.getByRole("button", { name: "Show diff" }));
    await testPage.unroute("**/api/v1/task-sessions/*/html-previews");
    const retryPublication = armHtmlPreviewPublication(testPage);
    await retry.click();
    await retryPublication.published;
    await retryPublication.resources;

    await expect(preview.frameLocator("iframe").locator("h1")).toHaveText(
      "Narrow viewport preview",
    );
    const source = preview.getByTestId("review-html-preview-source");
    await expect(source).toContainText(SOURCE_COPY);
    await expectTouchTarget(preview.getByRole("button", { name: "Refresh HTML preview" }));
    await expectTouchTarget(preview.getByRole("button", { name: "Show diff" }));
    await assertNoDocumentHorizontalOverflow(testPage, "narrow HTML diff preview");
  });
});
