import { type Page } from "@playwright/test";
import path from "node:path";
import { test, expect, resetSeedRepositoryCheckout } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { BackendContext } from "../../fixtures/backend";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { GitHelper, makeGitEnv, createStandardProfile } from "../../helpers/git-helper";
import { routeMainWebSocketWithPreviewFeedbackCreateFailure } from "../../helpers/ws-drop";
import { SessionPage } from "../../pages/session-page";
import { waitForScreenshotCaptureMode } from "../preview/preview-feedback-helpers";

const SVG_ASSET =
  '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24"><rect width="24" height="24" fill="#60a5fa"/></svg>';

function mobilePreviewHtml(assetDirectory: string): string {
  return `<!doctype html>
<html>
  <head>
    <meta charset="utf-8">
    <link rel="stylesheet" href="./${assetDirectory}/preview.css">
  </head>
  <body>
    <h1>Mobile native preview</h1>
    <p id="css-status">CSS asset pending</p>
    <img id="asset-image" src="./${assetDirectory}/logo.svg" alt="asset">
    <button id="capture-button">Review order</button>
    <section id="capture-card">Generated order summary</section>
    <p id="native-status">Browser script pending</p>
    <script src="./${assetDirectory}/preview.js"></script>
  </body>
</html>`;
}

function mobilePreviewScript(entryName: string): string {
  return `(() => {
  const image = document.querySelector("#asset-image");
  const status = document.querySelector("#native-status");
  const render = () => {
    status.textContent = [
      typeof ResizeObserver === "function" ? "api:available" : "api:missing",
      image.complete && image.naturalWidth > 0 ? "image:loaded" : "image:pending",
      location.pathname.endsWith("/${entryName}") ? "path:entry" : "path:wrong",
    ].join(" | ");
  };
  image.addEventListener("load", render);
  render();
})();`;
}

async function setupMobileHtmlPreviewTest({
  testPage,
  apiClient,
  seedData,
  backend,
}: {
  testPage: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  backend: BackendContext;
}): Promise<{ session: SessionPage; filePath: string; taskId: string; sessionId: string }> {
  const suffix = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  const filePath = `mobile-preview-${suffix}.html`;
  const assetDirectory = `mobile-preview-assets-${suffix}`;
  const git = new GitHelper(
    path.join(backend.tmpDir, "repos", "e2e-repo"),
    makeGitEnv(backend.tmpDir),
  );
  git.createFile(filePath, mobilePreviewHtml(assetDirectory));
  git.createFile(`${assetDirectory}/preview.css`, "#css-status { font-weight: 700; }");
  git.createFile(`${assetDirectory}/preview.js`, mobilePreviewScript(filePath));
  git.createFile(`${assetDirectory}/logo.svg`, SVG_ASSET);
  git.stageAll();
  git.commit(`add ${filePath}`);

  const profile = await createStandardProfile(apiClient, `mobile-html-${Date.now()}`);
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile HTML Preview",
    profile.id,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  await expect
    .poll(async () => (await apiClient.getTaskEnvironment(task.id))?.status ?? null, {
      timeout: 60_000,
      message: "the mobile HTML preview workspace did not become ready",
    })
    .toBe("ready");
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 45_000 });
  if (!task.session_id) throw new Error("Mobile HTML preview task has no session");
  return { session, filePath, taskId: task.id, sessionId: task.session_id };
}

test.describe("Mobile HTML preview", () => {
  test.describe.configure({ retries: 0, timeout: 120_000 });

  test.beforeEach(({ backend, seedData }) => {
    resetSeedRepositoryCheckout(seedData, backend.tmpDir);
  });

  test.afterEach(({ backend, seedData }) => {
    resetSeedRepositoryCheckout(seedData, backend.tmpDir);
  });

  test("renders native scripts and relative assets in the focused viewer", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const { filePath } = await setupMobileHtmlPreviewTest({
      testPage,
      apiClient,
      seedData,
      backend,
    });

    await testPage.getByRole("button", { name: "Files", exact: true }).tap();
    const fileNode = testPage.locator(`[data-testid="file-tree-node"][data-path="${filePath}"]`);
    await expect(fileNode).toBeVisible({ timeout: 15_000 });
    await fileNode.tap();

    const viewer = testPage.getByTestId("mobile-file-viewer-panel");
    await expect(viewer).toBeVisible({ timeout: 5_000 });
    const previewToggle = viewer.getByTestId("html-preview-toggle");
    await expect(previewToggle).toBeVisible();
    const toggleBox = await previewToggle.boundingBox();
    expect(toggleBox?.width).toBeGreaterThanOrEqual(44);
    expect(toggleBox?.height).toBeGreaterThanOrEqual(44);
    await expect(previewToggle).toHaveAttribute("title", /Previewing runs workspace code/);

    await previewToggle.tap();
    const preview = viewer.getByTestId("html-preview");
    await expect(preview).toBeVisible();
    await expect(preview.getByTestId("html-preview-trust-warning")).toBeVisible();
    const frame = preview.frameLocator("iframe");
    await expect(preview.locator("iframe")).toHaveAttribute("src", /\/port-proxy\/.*\/\d+\//);
    await expect(frame.locator("h1")).toHaveText("Mobile native preview", { timeout: 15_000 });
    await expect(frame.locator("#native-status")).toHaveText(
      "api:available | image:loaded | path:entry",
    );
    await expect(frame.locator("#css-status")).toHaveCSS("font-weight", "700");
    await expect(frame.locator("#asset-image")).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "mobile HTML preview");

    await prCapture.screenshot("html-preview-mobile", {
      caption: "Native HTML preview in the focused mobile viewer",
    });

    await preview.getByRole("button", { name: "Show code" }).tap();
    await expect(viewer.getByRole("textbox", { name: "Editor content" })).toBeVisible();
    await expect(viewer.getByText("Mobile native preview", { exact: true })).toBeVisible();
    await expect(preview).toBeHidden();
  });

  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-005.1
  // @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-005.2
  test("captures touch feedback from the native HTML preview toolbar", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const createFailure = await routeMainWebSocketWithPreviewFeedbackCreateFailure(testPage);
    const { filePath, session, taskId, sessionId } = await setupMobileHtmlPreviewTest({
      testPage,
      apiClient,
      seedData,
      backend,
    });

    await testPage.getByRole("button", { name: "Files", exact: true }).tap();
    const fileNode = testPage.locator(`[data-testid="file-tree-node"][data-path="${filePath}"]`);
    await expect(fileNode).toBeVisible({ timeout: 15_000 });
    await fileNode.tap();

    const viewer = testPage.getByTestId("mobile-file-viewer-panel");
    await expect(viewer).toBeVisible({ timeout: 5_000 });
    await viewer.getByTestId("html-preview-toggle").tap();
    const preview = viewer.getByTestId("html-preview");
    const frame = preview.frameLocator("iframe");
    await expect(frame.locator("#capture-button")).toBeVisible({ timeout: 15_000 });

    const trigger = preview.getByTestId("preview-feedback-trigger");
    const triggerBox = await trigger.boundingBox();
    expect(triggerBox?.width).toBeGreaterThanOrEqual(44);
    expect(triggerBox?.height).toBeGreaterThanOrEqual(44);
    await trigger.tap();

    const toolbar = preview.getByTestId("preview-annotation-toolbar");
    await expect(toolbar).toBeVisible();
    const viewport = testPage.viewportSize();
    expect(viewport).not.toBeNull();
    const toolbarBox = await toolbar.boundingBox();
    expect(toolbarBox).not.toBeNull();
    expect(toolbarBox!.x).toBeGreaterThanOrEqual(0);
    expect(toolbarBox!.x + toolbarBox!.width).toBeLessThanOrEqual(viewport!.width);
    expect(toolbarBox!.y + toolbarBox!.height).toBeLessThanOrEqual(viewport!.height);

    const toolBoxes = [];
    for (const tool of ["interact", "text", "element", "region"]) {
      const box = await toolbar.getByTestId(`preview-tool-${tool}`).boundingBox();
      expect(box?.height).toBeGreaterThanOrEqual(44);
      expect(box?.width).toBeGreaterThanOrEqual(44);
      toolBoxes.push(box);
    }
    expect(new Set(toolBoxes.map((box) => Math.round(box?.y ?? -1))).size).toBe(1);
    expect(
      (await toolbar.getByTestId("preview-annotation-exit").boundingBox())?.height,
    ).toBeGreaterThanOrEqual(44);

    // Coarse pointers keep the comment list in a bottom sheet that starts hidden.
    await expect(testPage.getByTestId("preview-comment-panel")).toHaveCount(0);
    const commentsToggle = toolbar.getByTestId("preview-annotation-count");
    await expect(commentsToggle).toHaveAttribute("aria-pressed", "false");
    await commentsToggle.tap();
    const commentsSheet = testPage.getByTestId("preview-feedback-collection-body");
    await expect(commentsSheet).toBeVisible();
    await testPage
      .getByRole("dialog")
      .filter({ has: commentsSheet })
      .getByRole("button", {
        name: "Close",
      })
      .tap();
    await expect(commentsSheet).toBeHidden();

    await toolbar.getByTestId("preview-tool-element").tap();
    await expect(trigger).toHaveAttribute("data-capture-mode", "element");
    await frame.locator("#capture-button").tap();

    const elementDraft = testPage.getByTestId("preview-feedback-draft");
    await expect(elementDraft).toContainText("<button#capture-button>");
    await testPage
      .getByTestId("preview-feedback-draft-input")
      .fill("Increase this touch target contrast");
    const elementSave = testPage.getByTestId("preview-feedback-draft-save");
    expect((await elementSave.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await elementSave.tap();
    await expect(elementDraft).toBeHidden({ timeout: 15_000 });

    await toolbar.getByTestId("preview-tool-region").tap();
    await expect(trigger).toHaveAttribute("data-capture-mode", "screenshot");
    await waitForScreenshotCaptureMode(frame);
    const screenshotTarget = frame.locator("#capture-card");
    const region = await screenshotTarget.evaluate((element) => {
      const rect = element.getBoundingClientRect();
      return { left: rect.left, top: rect.top, right: rect.right, bottom: rect.bottom };
    });
    const pointer = { pointerId: 7, pointerType: "touch", isPrimary: true, buttons: 1 };
    await screenshotTarget.dispatchEvent("pointerdown", {
      ...pointer,
      clientX: region.left + 2,
      clientY: region.top + 2,
    });
    await screenshotTarget.dispatchEvent("pointermove", {
      ...pointer,
      clientX: region.right - 2,
      clientY: region.bottom - 2,
    });
    await screenshotTarget.dispatchEvent("pointerup", {
      ...pointer,
      buttons: 0,
      clientX: region.right - 2,
      clientY: region.bottom - 2,
    });

    const screenshotDraft = testPage.getByTestId("preview-feedback-draft");
    await expect(screenshotDraft.getByRole("img", { name: "Screenshot preview" })).toBeVisible({
      timeout: 15_000,
    });
    const draftClose = screenshotDraft.getByRole("button", { name: "Close" });
    expect((await draftClose.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await testPage
      .getByTestId("preview-feedback-draft-input")
      .fill("Reduce the vertical space in this card");
    createFailure.failNextCreate();
    await testPage.getByTestId("preview-feedback-draft-save").tap();
    await expect.poll(() => createFailure.failedCount()).toBe(1);
    await expect(screenshotDraft).toBeVisible();
    await expect(testPage.getByTestId("preview-feedback-draft-input")).toHaveValue(
      "Reduce the vertical space in this card",
    );
    await expect(screenshotDraft.getByRole("img", { name: "Screenshot preview" })).toBeVisible();
    await expect(screenshotDraft.getByRole("alert")).toContainText("feedback could not be saved");
    await testPage.getByTestId("preview-feedback-draft-save").tap();
    await expect(screenshotDraft).toBeHidden({ timeout: 15_000 });
    await expect(preview.getByTestId("preview-feedback-trigger")).toContainText("2");
    await expect(preview.getByTestId("preview-region")).toHaveCount(1);

    await frame.locator("#capture-button").evaluate((element) => {
      window.scrollTo(0, document.body.scrollHeight);
      return element.getBoundingClientRect().top;
    });
    const annotationToggle = toolbar.getByTestId("preview-annotation-count");
    await annotationToggle.tap();
    const annotationSheet = testPage.getByTestId("preview-feedback-collection-body");
    const annotationItems = annotationSheet.getByTestId("preview-feedback-item");
    await expect(annotationItems).toHaveCount(2);
    await expect(annotationItems.nth(0).getByTestId("preview-capture-number")).toHaveText("1");
    await expect(annotationItems.nth(1).getByTestId("preview-capture-number")).toHaveText("2");
    const buttonFeedback = annotationItems.filter({ hasText: "#capture-button" });
    await expect(buttonFeedback).toHaveCount(1);
    await buttonFeedback.tap();
    await expect(buttonFeedback).toHaveAttribute("data-selected", "true");
    await expect
      .poll(() =>
        frame.locator("#capture-button").evaluate((element) => element.getBoundingClientRect().top),
      )
      .toBeGreaterThanOrEqual(0);
    await buttonFeedback.getByRole("button", { name: "Edit feedback" }).tap();
    const annotationEditor = testPage.getByTestId("preview-feedback-edit");
    await expect(annotationEditor).toBeVisible();
    await expect(annotationSheet).toBeHidden();
    await testPage.getByTestId("preview-feedback-edit-input").fill("Keep the mobile edit focused");
    await testPage.getByTestId("preview-feedback-edit-save").tap();
    await expect(annotationEditor).toBeHidden();
    await assertNoDocumentHorizontalOverflow(testPage, "mobile preview annotation toolbar");

    await toolbar.getByTestId("preview-annotation-exit").tap();
    await expect(trigger).toHaveAttribute("data-annotation-active", "false");
    await expect(toolbar).toBeHidden();

    await frame.locator("#capture-card").evaluate((element) => {
      const range = document.createRange();
      range.selectNodeContents(element);
      const selection = window.getSelection();
      selection?.removeAllRanges();
      selection?.addRange(range);
    });
    const selectionComment = testPage.getByTestId("preview-selection-comment");
    await expect(selectionComment).toBeVisible();
    expect((await selectionComment.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await selectionComment.tap();
    const selectionDraft = testPage.getByTestId("preview-feedback-draft");
    await expect(selectionDraft).toContainText("Generated order summary");
    await testPage.getByTestId("preview-feedback-draft-input").fill("Explain this summary");
    await testPage.getByTestId("preview-feedback-draft-save").tap();
    await expect(selectionDraft).toBeHidden({ timeout: 15_000 });
    await expect(trigger).toContainText("3");
    await preview.getByRole("button", { name: "Show code" }).tap();
    await expect(viewer.locator(".cm-content, .monaco-editor .view-lines")).toContainText(
      "Generated order summary",
    );
    await expect(preview).toBeHidden();

    await testPage.getByRole("navigation").getByRole("button", { name: "Chat", exact: true }).tap();
    await apiClient.seedTaskSession(taskId, {
      state: "COMPLETED",
      sessionId,
      completedAt: new Date().toISOString(),
    });
    await testPage.reload();
    await session.waitForLoad();
    await testPage.getByRole("navigation").getByRole("button", { name: "Chat", exact: true }).tap();
    await expect(testPage.getByTestId("completed-session-banner")).toBeVisible({ timeout: 15_000 });

    const fallbackTrigger = testPage.getByTestId("preview-feedback-collection-trigger");
    await expect(fallbackTrigger).toBeVisible();
    const fallbackTriggerBox = await fallbackTrigger.boundingBox();
    expect(fallbackTriggerBox?.width).toBeGreaterThanOrEqual(44);
    expect(fallbackTriggerBox?.height).toBeGreaterThanOrEqual(44);
    const userMessageCount = await session.activeChat().getByTestId("user-message-bubble").count();
    await fallbackTrigger.tap();

    const collectionBody = testPage.getByTestId("preview-feedback-collection-body");
    const collectionDrawer = testPage.getByRole("dialog").filter({ has: collectionBody });
    await expect(collectionDrawer).toBeVisible();
    const collectionViewport = testPage.viewportSize();
    expect(collectionViewport).not.toBeNull();
    await expect
      .poll(async () => {
        const box = await collectionDrawer.boundingBox();
        return box ? box.y + box.height : Number.POSITIVE_INFINITY;
      })
      .toBeLessThanOrEqual(collectionViewport!.height);
    const collectionBox = await collectionDrawer.boundingBox();
    expect(collectionBox).not.toBeNull();
    expect(collectionBox!.y + collectionBox!.height).toBeLessThanOrEqual(
      collectionViewport!.height,
    );
    expect(
      parseFloat(
        await collectionBody.evaluate((element) => getComputedStyle(element).paddingBottom),
      ),
    ).toBeGreaterThanOrEqual(16);
    const collectionItems = collectionBody.getByTestId("preview-feedback-item");
    await expect(collectionItems).toHaveCount(3);
    await expect(
      collectionItems.nth(1).getByRole("img", { name: "Screenshot preview" }),
    ).toBeVisible();

    const firstItem = collectionItems.nth(0);
    await firstItem.getByRole("button", { name: "Edit feedback" }).tap();
    const editDrawer = testPage.getByTestId("preview-feedback-edit");
    await expect(editDrawer).toBeVisible();
    await testPage.getByTestId("preview-feedback-edit-input").fill("Keep this touch action clear");
    const editSave = testPage.getByTestId("preview-feedback-edit-save");
    expect((await editSave.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await editSave.tap();
    await expect(editDrawer).toBeHidden();
    await expect(collectionDrawer).toBeVisible();
    await expect(firstItem).toContainText("Keep this touch action clear");

    await collectionItems.nth(1).getByRole("button", { name: "Delete feedback" }).tap();
    await expect(collectionItems).toHaveCount(2);
    expect(await session.activeChat().getByTestId("user-message-bubble").count()).toBe(
      userMessageCount,
    );
    await assertNoDocumentHorizontalOverflow(testPage, "mobile task feedback collection");

    await testPage.keyboard.press("Escape");
    await expect(collectionDrawer).toBeHidden();
    await expect
      .poll(() =>
        testPage.evaluate(() => document.activeElement?.getAttribute("data-testid") ?? ""),
      )
      .toBe("preview-feedback-collection-trigger");

    await testPage.reload();
    await expect(
      testPage.getByRole("navigation").getByRole("button", { name: "Chat", exact: true }),
    ).toBeVisible();
    await testPage.getByRole("navigation").getByRole("button", { name: "Chat", exact: true }).tap();
    const restoredTrigger = testPage.getByTestId("preview-feedback-collection-trigger");
    await expect(restoredTrigger).toContainText("2");
    await restoredTrigger.tap();
    await expect(testPage.getByTestId("preview-feedback-collection-body")).toContainText(
      "Keep this touch action clear",
    );
  });

  test("recovers from a failed publish without losing the source viewer", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const { filePath } = await setupMobileHtmlPreviewTest({
      testPage,
      apiClient,
      seedData,
      backend,
    });

    await testPage.getByRole("button", { name: "Files", exact: true }).tap();
    const fileNode = testPage.locator(`[data-testid="file-tree-node"][data-path="${filePath}"]`);
    await expect(fileNode).toBeVisible({ timeout: 15_000 });
    await fileNode.tap();
    const viewer = testPage.getByTestId("mobile-file-viewer-panel");
    await expect(viewer).toBeVisible({ timeout: 5_000 });

    await testPage.route("**/api/v1/task-sessions/*/html-previews", async (route) => {
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: "agentctl unavailable" }),
      });
    });
    await viewer.getByTestId("html-preview-toggle").tap();
    const preview = viewer.getByTestId("html-preview");
    await expect(preview.getByTestId("html-preview-error")).toBeVisible({ timeout: 10_000 });
    await expect(preview.getByTestId("html-preview-error")).toContainText(
      "HTML preview session is not available",
    );
    await testPage.unroute("**/api/v1/task-sessions/*/html-previews");

    await preview.getByRole("button", { name: "Retry HTML preview" }).tap();
    await expect(preview.frameLocator("iframe").locator("h1")).toHaveText("Mobile native preview", {
      timeout: 15_000,
    });
    await preview.getByRole("button", { name: "Show code" }).tap();
    await expect(viewer.getByText("Mobile native preview", { exact: true })).toBeVisible();
  });
});
