import fs from "node:fs";
import path from "node:path";
import { expect, type Locator, type Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

const MOBILE_MARKDOWN_CONTENT = `# Mobile Markdown

Edit this file from the phone Files surface.

#### Mobile source markers

| Area | State | Notes |
| --- | --- | --- |
| Preview | Ready | The table remains contained |

${Array.from(
  { length: 56 },
  (_, index) => `Long mobile paragraph ${index + 1} keeps the editor content below the fold.`,
).join("\n\n")}

<div data-unsupported="true">Unsupported mobile source</div>
`;
const UNSUPPORTED_MOBILE_MARKDOWN_SOURCE =
  '<div data-unsupported="true">Unsupported mobile source</div>';

async function seedMobileMarkdownSession({
  testPage,
  apiClient,
  seedData,
  backend,
  fileName,
}: {
  testPage: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  backend: { tmpDir: string };
  fileName: string;
}): Promise<{ session: SessionPage; filePath: string }> {
  const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
  const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
  git.createFile(fileName, MOBILE_MARKDOWN_CONTENT);
  git.stageAll();
  git.commit(`seed ${fileName}`);

  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile Markdown editing",
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
  return { session, filePath: path.join(repoDir, fileName) };
}

async function appendToHybrid(testPage: Page, viewer: Locator, marker: string): Promise<void> {
  const editor = viewer.getByTestId("hybrid-markdown-editor");
  await expect(editor).toBeVisible({ timeout: 15_000 });
  const paragraph = editor.locator(".md-paragraph").last();
  await expect(paragraph).toBeVisible({ timeout: 15_000 });
  await paragraph.tap();
  await testPage.keyboard.press("Control+End");
  await testPage.keyboard.insertText(`\n\n${marker}`);
  await expect(editor).toContainText(marker);
}

test.describe("Mobile Markdown file editing", () => {
  test.describe.configure({ retries: 1, timeout: 120_000 });

  test("edits and saves the hybrid document below the fold and keeps phone controls reachable", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const fileName = `mobile-markdown-${Date.now()}.md`;
    const marker = `mobile saved marker ${Date.now()}`;
    const { session, filePath } = await seedMobileMarkdownSession({
      testPage,
      apiClient,
      seedData,
      backend,
      fileName,
    });

    await testPage.getByRole("button", { name: "Files" }).tap();
    const fileNode = session.fileTreeNode(fileName);
    await expect(fileNode).toBeVisible({ timeout: 15_000 });
    await fileNode.tap();

    const viewer = testPage.getByTestId("mobile-file-viewer-panel");
    await expect(viewer).toBeVisible({ timeout: 15_000 });
    const controls = viewer.getByTestId("mobile-markdown-mode-controls");
    const editButton = viewer.getByTestId("mobile-markdown-mode-edit");
    const previewButton = viewer.getByTestId("mobile-markdown-mode-preview");
    await expect(controls).toBeVisible();
    await expect(previewButton).toHaveAttribute("aria-pressed", "true");

    for (const button of [
      viewer.getByTestId("mobile-markdown-mode-source"),
      editButton,
      previewButton,
    ]) {
      const box = await button.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.width).toBeGreaterThanOrEqual(44);
      expect(box!.height).toBeGreaterThanOrEqual(44);
    }

    await editButton.tap();
    const hybrid = viewer.getByTestId("hybrid-markdown-editor");
    await expect(hybrid).toBeVisible({ timeout: 15_000 });
    const subheader = hybrid.locator("h4.md-heading", { hasText: "Mobile source markers" });
    await subheader.tap();
    const [hybridBox, markerBox] = await Promise.all([
      hybrid.boundingBox(),
      subheader.locator(".md-marker-headingMarker").boundingBox(),
    ]);
    expect(hybridBox).not.toBeNull();
    expect(markerBox).not.toBeNull();
    expect(markerBox!.x).toBeGreaterThanOrEqual(hybridBox!.x);
    const scrollMetrics = await hybrid.evaluate((element) => {
      const scroller = element as HTMLElement;
      const before = { scrollHeight: scroller.scrollHeight, clientHeight: scroller.clientHeight };
      scroller.scrollTop = scroller.scrollHeight;
      return { ...before, scrollTop: scroller.scrollTop };
    });
    expect(scrollMetrics.scrollHeight).toBeGreaterThan(scrollMetrics.clientHeight);
    expect(scrollMetrics.scrollTop).toBeGreaterThan(0);
    await appendToHybrid(testPage, viewer, marker);

    const saveButton = viewer.getByTestId("mobile-file-save");
    await expect(saveButton).toBeEnabled();
    await expect(saveButton).toBeInViewport();
    await testPage.keyboard.press(process.platform === "darwin" ? "Meta+S" : "Control+S");
    await expect
      .poll(() => fs.readFileSync(filePath, "utf8"), { timeout: 15_000 })
      .toContain(marker);
    expect(fs.readFileSync(filePath, "utf8")).toBe(`${MOBILE_MARKDOWN_CONTENT}\n\n${marker}`);
    expect(fs.readFileSync(filePath, "utf8")).toContain(UNSUPPORTED_MOBILE_MARKDOWN_SOURCE);
    await expect(saveButton).toBeDisabled();

    await previewButton.tap();
    const preview = viewer.getByTestId("markdown-preview");
    await expect(preview).toBeVisible();
    await expect(preview).toContainText(marker);
    await expect(preview.locator("table")).toBeVisible();
    await expect(preview).toContainText("Long mobile paragraph 56");
    const previewScroll = viewer.getByTestId("markdown-preview-scroll-container");
    const previewScrollTop = await previewScroll.evaluate((element) => {
      const scroller = element as HTMLElement;
      scroller.scrollTop = scroller.scrollHeight;
      return scroller.scrollTop;
    });
    expect(previewScrollTop).toBeGreaterThan(0);
    await prCapture.screenshot("mobile-markdown-preview", {
      caption: "Mobile Markdown Preview with contained table content",
    });
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1,
      ),
    ).toBe(true);
    expect(
      await testPage.evaluate(() => {
        const root = document.documentElement;
        return root.scrollHeight <= root.clientHeight + 1;
      }),
    ).toBe(true);

    await editButton.tap();
    await expect(viewer.getByTestId("mobile-markdown-hybrid-editor-host")).toBeVisible({
      timeout: 15_000,
    });
    await expect(viewer.getByTestId("hybrid-markdown-editor")).toBeVisible({ timeout: 15_000 });
    await previewButton.tap();
    await expect
      .poll(() => previewScroll.evaluate((element) => (element as HTMLElement).scrollTop))
      .toBe(previewScrollTop);

    await editButton.tap();
    const discardedMarker = `mobile discarded marker ${Date.now()}`;
    await appendToHybrid(testPage, viewer, discardedMarker);
    await viewer.getByRole("button", { name: "Back" }).tap();
    const discardDialog = testPage.getByTestId("discard-local-changes-dialog");
    await expect(discardDialog).toBeVisible();
    await discardDialog.getByTestId("discard-local-changes-cancel").tap();
    await expect(viewer).toContainText(discardedMarker);

    await testPage.getByRole("button", { name: "Plan" }).tap();
    await expect(discardDialog).toBeVisible();
    await discardDialog.getByTestId("discard-local-changes-cancel").tap();
    await expect(viewer).toContainText(discardedMarker);

    await viewer.getByRole("button", { name: "Back" }).tap();
    await expect(discardDialog).toBeVisible();
    await discardDialog.getByTestId("discard-local-changes-confirm").tap();
    await expect(session.fileTreeNode(fileName)).toBeVisible({ timeout: 15_000 });
  });
});
