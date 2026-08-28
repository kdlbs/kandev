import fs from "node:fs";
import path from "node:path";
import { expect, type Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

const MOBILE_MARKDOWN_CONTENT = `# Mobile Markdown

Edit this file from the phone Files surface.

| Area | State | Notes |
| --- | --- | --- |
| Preview | Ready | The table remains contained |
`;

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

test.describe("Mobile Markdown file editing", () => {
  test.describe.configure({ retries: 1, timeout: 120_000 });

  test("edits and saves Source, switches to Preview, and keeps phone controls reachable", async ({
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
    const sourceButton = viewer.getByTestId("mobile-markdown-mode-source");
    const editButton = viewer.getByTestId("mobile-markdown-mode-edit");
    const previewButton = viewer.getByTestId("mobile-markdown-mode-preview");
    await expect(controls).toBeVisible();
    await expect(sourceButton).toHaveAttribute("aria-pressed", "true");

    for (const button of [sourceButton, editButton, previewButton]) {
      const box = await button.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.width).toBeGreaterThanOrEqual(44);
      expect(box!.height).toBeGreaterThanOrEqual(44);
    }

    const source = viewer.locator(".cm-content").first();
    await expect(source).toHaveAttribute("contenteditable", "true");
    await source.tap();
    await testPage.keyboard.press("Control+End");
    await testPage.keyboard.type(`\n\n${marker}`);
    await expect(viewer.locator(".cm-line").filter({ hasText: marker })).toBeVisible();

    const saveButton = viewer.getByTestId("mobile-file-save");
    await expect(saveButton).toBeEnabled();
    await expect(saveButton).toBeInViewport();
    await saveButton.tap();
    await expect
      .poll(() => fs.readFileSync(filePath, "utf8"), { timeout: 15_000 })
      .toContain(marker);
    await expect(saveButton).toBeDisabled();

    await previewButton.tap();
    const preview = viewer.getByTestId("markdown-preview");
    await expect(preview).toBeVisible();
    await expect(preview).toContainText(marker);
    await expect(preview.locator("table")).toBeVisible();
    await prCapture.screenshot("mobile-markdown-preview", {
      caption: "Mobile Markdown Preview with contained table content",
    });
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1,
      ),
    ).toBe(true);

    await editButton.tap();
    await expect(viewer.getByTestId("mobile-markdown-hybrid-editor-host")).toBeVisible({
      timeout: 15_000,
    });
    await expect(viewer.getByTestId("hybrid-markdown-editor")).toBeVisible({ timeout: 15_000 });
  });
});
