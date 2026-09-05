import { type Page } from "@playwright/test";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { BackendContext } from "../../fixtures/backend";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { GitHelper, makeGitEnv, createStandardProfile } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

const HTML_CONTENT = `<!doctype html>
<html>
  <body>
    <h1>Mobile HTML preview</h1>
    <button id="increment">Increment</button>
    <output id="value">0</output>
    <script>
      let count = 0;
      document.getElementById("increment").addEventListener("click", () => {
        count += 1;
        document.getElementById("value").textContent = String(count);
      });
    </script>
  </body>
</html>`;

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
  const filePath = `mobile-preview-${Date.now()}-${Math.random().toString(36).slice(2, 8)}.html`;
  const git = new GitHelper(
    path.join(backend.tmpDir, "repos", "e2e-repo"),
    makeGitEnv(backend.tmpDir),
  );
  git.createFile(filePath, HTML_CONTENT);
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

  test("previews runtime output with a native touch target and no page overflow", async ({
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

    await previewToggle.tap();
    const preview = viewer.getByTestId("html-preview");
    await expect(preview).toBeVisible();
    await expect(preview.getByTestId("html-preview-surface")).toBeVisible();
    await expect(preview.locator("h1")).toHaveText("Mobile HTML preview");
    await expect(preview.locator("#value")).toHaveText("0");
    await preview.locator("#increment").tap();
    await expect(preview.locator("#value")).toHaveText("1");
    await assertNoDocumentHorizontalOverflow(testPage, "mobile HTML preview");

    await prCapture.screenshot("html-preview-mobile", {
      caption: "HTML preview on the native mobile file viewer",
    });

    await preview.getByRole("button", { name: "Show code" }).tap();
    await expect(viewer.locator(".cm-editor:visible")).toBeVisible();
  });
});
