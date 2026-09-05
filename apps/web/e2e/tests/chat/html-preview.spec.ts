import path from "node:path";
import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { BackendContext } from "../../fixtures/backend";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

const SAVED_HTML = "<!doctype html><html><body><p>Saved source</p></body></html>";
const SVG_ASSET =
  '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24"><rect width="24" height="24" fill="#4ade80"/></svg>';

function nativePreviewHtml(assetDirectory: string, entryName: string, heading: string): string {
  return `<!doctype html>
<html>
  <head>
    <meta charset="utf-8">
    <link rel="stylesheet" href="./${assetDirectory}/preview.css">
  </head>
  <body>
    <h1>${heading}</h1>
    <p id="css-status">CSS asset pending</p>
    <img id="asset-image" src="./${assetDirectory}/logo.svg" alt="asset">
    <p id="native-status">Browser script pending</p>
    <button id="increment">Increment</button>
    <output id="value">0</output>
    <script src="./${assetDirectory}/preview.js"></script>
  </body>
</html>`;
}

function nativePreviewScript(entryName: string): string {
  return `(() => {
  const image = document.querySelector("#asset-image");
  const status = document.querySelector("#native-status");
  const value = document.querySelector("#value");
  const button = document.querySelector("#increment");
  const render = () => {
    status.textContent = [
      typeof ResizeObserver === "function" ? "api:available" : "api:missing",
      image.complete && image.naturalWidth > 0 ? "image:loaded" : "image:pending",
      location.pathname.endsWith("/${entryName}") ? "path:entry" : "path:wrong",
    ].join(" | ");
  };
  let count = 0;
  image.addEventListener("load", render);
  button.addEventListener("click", () => {
    count += 1;
    value.textContent = String(count);
  });
  render();
})();`;
}

async function setupDesktopHtmlPreviewTest({
  testPage,
  apiClient,
  seedData,
  backend,
  title,
}: {
  testPage: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  backend: BackendContext;
  title: string;
}): Promise<{ session: SessionPage; fileName: string; assetDirectory: string }> {
  const suffix = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  const fileName = `preview-${suffix}.html`;
  const assetDirectory = `preview-assets-${suffix}`;
  const currentHtml = nativePreviewHtml(assetDirectory, fileName, "Unsaved native preview");
  const git = new GitHelper(
    path.join(backend.tmpDir, "repos", "e2e-repo"),
    makeGitEnv(backend.tmpDir),
  );
  git.createFile(fileName, SAVED_HTML);
  git.createFile(`${assetDirectory}/preview.css`, "#css-status { font-weight: 700; }");
  git.createFile(`${assetDirectory}/preview.js`, nativePreviewScript(fileName));
  git.createFile(`${assetDirectory}/logo.svg`, SVG_ASSET);
  git.stageAll();
  git.commit(`add ${fileName}`);

  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
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

  await session.clickTab("Files");
  await expect(session.files.getByText(fileName)).toBeVisible({ timeout: 15_000 });
  await session.files.getByText(fileName).click();
  await expect(testPage.locator(`.dv-default-tab:has-text('${fileName}')`)).toBeVisible({
    timeout: 10_000,
  });

  const editor = testPage.locator(".monaco-editor:visible").first();
  await expect(editor).toBeVisible({ timeout: 15_000 });
  await editor.locator(".view-lines").click();
  await testPage.keyboard.press("ControlOrMeta+A");
  await testPage.keyboard.insertText(currentHtml);

  return { session, fileName, assetDirectory };
}

function browserFrame(session: SessionPage) {
  return session.browserPanel.frameLocator("iframe");
}

test.describe("HTML preview", () => {
  test.describe.configure({ retries: 1, timeout: 120_000 });

  test("runs the current buffer in the native Browser panel with relative assets", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const assetRequests: string[] = [];
    testPage.on("request", (request) => {
      const requestPath = new URL(request.url()).pathname;
      if (requestPath.includes("/port-proxy/") && /\.(css|js|svg)$/.test(requestPath)) {
        assetRequests.push(requestPath);
      }
    });

    const { session, fileName, assetDirectory } = await setupDesktopHtmlPreviewTest({
      testPage,
      apiClient,
      seedData,
      backend,
      title: "HTML Native Browser Preview",
    });

    const sourceTab = testPage.locator(".dv-default-tab:visible").filter({ hasText: fileName });

    await testPage.getByTestId("html-preview-toggle").first().click();

    const browser = session.browserPanel;
    await expect(browser).toBeVisible({ timeout: 10_000 });
    const browserIframe = browser.locator("iframe");
    await expect(browserIframe).toHaveAttribute("src", /\/port-proxy\/.*\/\d+\//);
    const frame = browserFrame(session);
    await expect(frame.locator("h1")).toHaveText("Unsaved native preview", { timeout: 15_000 });
    await expect(frame.locator("#native-status")).toHaveText(
      "api:available | image:loaded | path:entry",
    );
    await expect(frame.locator("#css-status")).toHaveCSS("font-weight", "700");
    await expect(frame.locator("#asset-image")).toBeVisible();
    await expect
      .poll(() => assetRequests.some((requestPath) => requestPath.endsWith("/preview.js")))
      .toBe(true);
    await expect
      .poll(() => assetRequests.some((requestPath) => requestPath.endsWith("/preview.css")))
      .toBe(true);
    await expect
      .poll(() => assetRequests.some((requestPath) => requestPath.endsWith("/logo.svg")))
      .toBe(true);

    await expect(frame.locator("#value")).toHaveText("0");
    await frame.locator("#increment").click();
    await expect(frame.locator("#value")).toHaveText("1");

    await browser.getByTitle("Refresh").click();
    await expect(frame.locator("#native-status")).toHaveText(
      "api:available | image:loaded | path:entry",
      { timeout: 15_000 },
    );

    const secondHtml = nativePreviewHtml(assetDirectory, fileName, "Republished native preview");
    await sourceTab.click();
    const editor = testPage.locator(".monaco-editor:visible").first();
    await expect(editor).toBeVisible();
    await editor.locator(".view-lines").click();
    await testPage.keyboard.press("ControlOrMeta+A");
    await testPage.keyboard.insertText(secondHtml);
    await testPage.getByTestId("html-preview-toggle").first().click();

    await expect(browser).toHaveCount(1);
    await expect(frame.locator("h1")).toHaveText("Republished native preview", { timeout: 15_000 });
    const refreshedSrc = await browserIframe.getAttribute("src");
    expect(refreshedSrc).toContain("v=2");

    await prCapture.screenshot("html-preview-desktop", {
      caption: "Unsaved HTML rendered by the native Browser panel",
    });

    await sourceTab.click();
    await expect(
      testPage.locator(".monaco-editor:visible").first().locator(".view-lines"),
    ).toContainText("Republished native preview");
  });

  test("shows a retryable error when the session publish endpoint fails", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const { session } = await setupDesktopHtmlPreviewTest({
      testPage,
      apiClient,
      seedData,
      backend,
      title: "HTML Native Browser Preview Error",
    });

    await testPage.route("**/api/v1/task-sessions/*/html-previews", async (route) => {
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: "agentctl unavailable" }),
      });
    });
    await testPage.getByTestId("html-preview-toggle").first().click();
    await expect(testPage.getByTestId("toast-message")).toContainText(
      "HTML preview session is not available",
    );
    await expect(session.browserPanel).toHaveCount(0);
    await testPage.unroute("**/api/v1/task-sessions/*/html-previews");
  });
});
