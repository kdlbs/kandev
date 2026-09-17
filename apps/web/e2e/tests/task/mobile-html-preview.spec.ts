import { type Page } from "@playwright/test";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { BackendContext } from "../../fixtures/backend";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { GitHelper, makeGitEnv, createStandardProfile } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

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
}): Promise<{ session: SessionPage; filePath: string }> {
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

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 45_000 });
  return { session, filePath };
}

test.describe("Mobile HTML preview", () => {
  test.describe.configure({ retries: 1, timeout: 120_000 });

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

    await testPage.getByRole("button", { name: "Files" }).tap();
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
    await expect(viewer.locator(".cm-editor:visible")).toBeVisible();
    await expect(viewer.locator(".cm-content")).toContainText("Mobile native preview");
    await expect(preview).toBeHidden();
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

    await testPage.getByRole("button", { name: "Files" }).tap();
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
    await expect(viewer.locator(".cm-content")).toContainText("Mobile native preview");
  });
});
