// Routing: /t/{taskId} (task-keyed). File starts with "mobile-" so it runs on
// the mobile-chrome Playwright project (Pixel 5 emulation).
//
// Regression guard: tapping a file in the Files → All files tab must replace
// the Files panel with the file viewer, NOT navigate to the Changes panel.
import { type Page } from "@playwright/test";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { BackendContext } from "../../fixtures/backend";
import { GitHelper, makeGitEnv, createStandardProfile } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

function createLongFileContent(lines = 500): string {
  return Array.from(
    { length: lines },
    (_, index) => `export const line_${index} = "line ${index}";`,
  ).join("\n");
}

async function setupMobileFileViewerTest({
  testPage,
  apiClient,
  seedData,
  backend,
  taskTitle,
  options,
}: {
  testPage: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  backend: BackendContext;
  taskTitle: string;
  options?: {
    extension?: string;
    content?: string | Buffer;
    directory?: string;
  };
}): Promise<{ session: SessionPage; filePath: string }> {
  const fileExtension = options?.extension ?? "ts";
  const fileContent = options?.content ?? 'export const greeting = "hello";';
  const fileName = `viewer-test-${Date.now()}-${Math.random().toString(36).slice(2, 8)}.${fileExtension}`;
  const filePath = options?.directory ? `${options.directory}/${fileName}` : fileName;
  const git = new GitHelper(
    path.join(backend.tmpDir, "repos", "e2e-repo"),
    makeGitEnv(backend.tmpDir),
  );
  git.createFile(filePath, fileContent);
  git.stageAll();
  git.commit(`add ${filePath}`);

  const profile = await createStandardProfile(apiClient, `mobile-fv-${Date.now()}`);
  const task = await apiClient.createTaskWithAgent(seedData.workspaceId, taskTitle, profile.id, {
    description: "/e2e:simple-message",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.idleInput()).toBeVisible({ timeout: 45_000 });
  return { session, filePath };
}

test.describe("Mobile file viewer panel", () => {
  test.describe.configure({ retries: 1 });

  test("tapping file in All-files tab opens viewer panel instead of Changes panel", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);

    const { filePath } = await setupMobileFileViewerTest({
      testPage,
      apiClient,
      seedData,
      backend,
      taskTitle: "Mobile FV Open",
    });

    // Navigate to the Files panel via the bottom nav
    await testPage.getByRole("button", { name: "Files" }).tap();

    // Wait for the file to appear in the browser tree
    const fileNode = testPage.locator(`[data-testid="file-tree-node"][data-path="${filePath}"]`);
    await expect(fileNode).toBeVisible({ timeout: 15_000 });

    // Tap the file — must open the inline viewer panel, NOT switch to the Changes panel
    await fileNode.tap();

    const viewer = testPage.getByTestId("mobile-file-viewer-panel");
    await expect(viewer).toBeVisible({ timeout: 5_000 });
    await expect(viewer.getByText(filePath)).toBeVisible();
  });

  test("closing the viewer panel returns to the files browser", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);

    const { filePath } = await setupMobileFileViewerTest({
      testPage,
      apiClient,
      seedData,
      backend,
      taskTitle: "Mobile FV Close",
    });

    await testPage.getByRole("button", { name: "Files" }).tap();

    const fileNode = testPage.locator(`[data-testid="file-tree-node"][data-path="${filePath}"]`);
    await expect(fileNode).toBeVisible({ timeout: 15_000 });
    await fileNode.tap();

    const viewer = testPage.getByTestId("mobile-file-viewer-panel");
    await expect(viewer).toBeVisible({ timeout: 5_000 });

    await viewer.getByRole("button", { name: "Close" }).tap();
    await expect(viewer).not.toBeVisible({ timeout: 5_000 });
    await expect(fileNode).toBeVisible();
  });

  test("binary files show preview-unavailable message in viewer panel", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);

    const { filePath } = await setupMobileFileViewerTest({
      testPage,
      apiClient,
      seedData,
      backend,
      taskTitle: "Mobile FV Binary",
      options: { extension: "bin", content: Buffer.from([0x00, 0x01, 0x02, 0x03, 0xff, 0x10]) },
    });

    await testPage.getByRole("button", { name: "Files" }).tap();

    const fileNode = testPage.locator(`[data-testid="file-tree-node"][data-path="${filePath}"]`);
    await expect(fileNode).toBeVisible({ timeout: 15_000 });
    await fileNode.tap();

    const viewer = testPage.getByTestId("mobile-file-viewer-panel");
    await expect(viewer).toBeVisible({ timeout: 5_000 });
    await expect(viewer.getByText("Cannot preview this file")).toBeVisible();
    await expect(viewer.getByText("Binary file")).toBeVisible();
  });

  test("viewer panel keeps a visible close action and supports scrolling large files", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);

    const { filePath } = await setupMobileFileViewerTest({
      testPage,
      apiClient,
      seedData,
      backend,
      taskTitle: "Mobile FV Scroll",
      options: { extension: "ts", content: createLongFileContent(1_000) },
    });

    await testPage.getByRole("button", { name: "Files" }).tap();

    const fileNode = testPage.locator(`[data-testid="file-tree-node"][data-path="${filePath}"]`);
    await expect(fileNode).toBeVisible({ timeout: 15_000 });
    await fileNode.tap();

    const viewer = testPage.getByTestId("mobile-file-viewer-panel");
    await expect(viewer).toBeVisible({ timeout: 5_000 });

    const closeButton = viewer.getByRole("button", { name: "Close" });
    await expect(closeButton).toBeVisible();
    await expect(closeButton).toBeInViewport();

    // CodeMirror's `.cm-scroller` owns the scroll for text files (matches the
    // desktop editor tab). Verify it's the element that actually scrolls.
    const cmScrollerLocator = viewer.locator(".cm-scroller");
    await expect(cmScrollerLocator).toHaveCount(1);
    const cmScroller = cmScrollerLocator.first();
    await expect(cmScroller).toBeVisible();

    const metrics = await cmScroller.evaluate((element) => ({
      clientHeight: element.clientHeight,
      scrollHeight: element.scrollHeight,
      touchAction: getComputedStyle(element).touchAction,
      overflowY: getComputedStyle(element).overflowY,
    }));
    expect(metrics.scrollHeight).toBeGreaterThan(metrics.clientHeight);
    // touch-action must allow vertical pan or mobile touch scroll won't work.
    expect(metrics.touchAction).toMatch(/pan-y|auto/);
    expect(metrics.overflowY).toMatch(/auto|scroll/);

    const scrollTopAfterMove = await cmScroller.evaluate((element) => {
      element.scrollTop = 0;
      element.scrollBy(0, 1_400);
      return element.scrollTop;
    });
    expect(scrollTopAfterMove).toBeGreaterThan(0);

    // Real touch swipe: dispatch CDP touch events to exercise the gesture path
    // that the programmatic scrollBy above bypasses. This is the path the user
    // actually triggers when finger-scrolling a long file.
    await cmScroller.evaluate((element) => {
      element.scrollTop = 0;
    });
    const box = await cmScroller.boundingBox();
    if (!box) throw new Error("cm-scroller has no bounding box");
    const cdp = await testPage.context().newCDPSession(testPage);
    const centerX = box.x + box.width / 2;
    const startY = box.y + box.height - 20;
    const endY = box.y + 20;
    await cdp.send("Input.dispatchTouchEvent", {
      type: "touchStart",
      touchPoints: [{ x: centerX, y: startY }],
    });
    for (let i = 1; i <= 8; i++) {
      const y = startY + ((endY - startY) * i) / 8;
      await cdp.send("Input.dispatchTouchEvent", {
        type: "touchMove",
        touchPoints: [{ x: centerX, y }],
      });
    }
    await cdp.send("Input.dispatchTouchEvent", {
      type: "touchEnd",
      touchPoints: [],
    });

    await expect
      .poll(async () => cmScroller.evaluate((element) => element.scrollTop), {
        timeout: 5_000,
      })
      .toBeGreaterThan(0);
  });

  test("markdown files can toggle preview in the viewer panel", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);

    const { filePath } = await setupMobileFileViewerTest({
      testPage,
      apiClient,
      seedData,
      backend,
      taskTitle: "Mobile FV Markdown",
      options: { extension: "md", content: "# Heading\n\nBody text" },
    });

    await testPage.getByRole("button", { name: "Files" }).tap();

    const fileNode = testPage.locator(`[data-testid="file-tree-node"][data-path="${filePath}"]`);
    await expect(fileNode).toBeVisible({ timeout: 15_000 });
    await fileNode.tap();

    const viewer = testPage.getByTestId("mobile-file-viewer-panel");
    await expect(viewer).toBeVisible({ timeout: 5_000 });

    await viewer.getByTestId("markdown-preview-toggle").tap();
    await expect(viewer.getByTestId("markdown-preview")).toBeVisible();
  });

  test("viewer header shows full path for files inside subdirectories", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);

    const directory = `nested-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`;
    const { filePath } = await setupMobileFileViewerTest({
      testPage,
      apiClient,
      seedData,
      backend,
      taskTitle: "Mobile FV Subdir",
      options: { directory },
    });

    await testPage.getByRole("button", { name: "Files" }).tap();

    const dirNode = testPage.locator(`[data-testid="file-tree-node"][data-path="${directory}"]`);
    await expect(dirNode).toBeVisible({ timeout: 15_000 });
    await dirNode.tap();

    const fileNode = testPage.locator(`[data-testid="file-tree-node"][data-path="${filePath}"]`);
    await expect(fileNode).toBeVisible({ timeout: 15_000 });
    await fileNode.tap();

    const viewer = testPage.getByTestId("mobile-file-viewer-panel");
    await expect(viewer).toBeVisible({ timeout: 5_000 });

    // Header must show full relative path, not just the basename.
    await expect(viewer.getByText(filePath)).toBeVisible();
  });
});
