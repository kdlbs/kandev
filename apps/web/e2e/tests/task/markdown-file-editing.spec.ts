import fs from "node:fs";
import path from "node:path";
import { expect, type Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

const MARKDOWN_CONTENT = `# Markdown lifecycle

This paragraph stays in the canonical source.

| Area | State |
| --- | --- |
| Preview | Ready |
`;

async function seedMarkdownSession({
  testPage,
  apiClient,
  seedData,
  backend,
  fileName,
  content,
  taskTitle,
}: {
  testPage: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  backend: { tmpDir: string };
  fileName: string;
  content: string;
  taskTitle: string;
}): Promise<{ session: SessionPage; filePath: string; sessionId: string }> {
  const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
  const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
  git.createFile(fileName, content);
  git.stageAll();
  git.commit(`seed ${fileName}`);

  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    taskTitle,
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
  return {
    session,
    filePath: path.join(repoDir, fileName),
    sessionId: task.session_id,
  };
}

async function openFile(session: SessionPage, testPage: Page, fileName: string): Promise<void> {
  await session.clickTab("Files");
  await expect(session.files).toBeVisible({ timeout: 10_000 });
  const fileNode = session.fileTreeNode(fileName);
  await expect(fileNode).toBeVisible({ timeout: 15_000 });
  await fileNode.click();
  await expect(testPage.getByTestId("markdown-file-editor")).toBeVisible({ timeout: 15_000 });
}

async function appendToMonaco(testPage: Page, marker: string): Promise<void> {
  const editor = testPage.locator(".monaco-editor:visible").first();
  await expect(editor).toBeVisible({ timeout: 15_000 });
  const nativeEditContext = editor.locator(".native-edit-context");
  const input =
    (await nativeEditContext.count()) > 0
      ? nativeEditContext.first()
      : editor.locator("textarea.inputarea").first();
  await input.focus();
  await testPage.keyboard.press(process.platform === "darwin" ? "Meta+End" : "Control+End");
  await testPage.keyboard.type(`\n\n${marker}`);
  await expect(editor.locator(".view-lines")).toContainText(marker);
}

test.describe("Markdown file editing", () => {
  test.describe.configure({ retries: 1, timeout: 120_000 });

  test("opens in Preview, edits the exact Source buffer, saves, and restores Edit mode", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const fileName = `markdown-lifecycle-${Date.now()}.md`;
    const marker = `saved markdown marker ${Date.now()}`;
    const { session, filePath, sessionId } = await seedMarkdownSession({
      testPage,
      apiClient,
      seedData,
      backend,
      fileName,
      content: MARKDOWN_CONTENT,
      taskTitle: "Markdown lifecycle",
    });

    await openFile(session, testPage, fileName);
    const editor = testPage.getByTestId("markdown-file-editor");
    const preview = testPage.getByTestId("markdown-preview");
    await expect(editor.getByTestId("markdown-mode-preview")).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    await expect(preview.locator("h1")).toHaveText("Markdown lifecycle");

    await editor.getByTestId("markdown-mode-edit").click();
    await expect(testPage.getByTestId("hybrid-markdown-editor")).toBeVisible({ timeout: 15_000 });
    await editor.getByTestId("markdown-mode-source").click();
    await appendToMonaco(testPage, marker);

    const saveButton = editor.getByTestId("markdown-file-save");
    await expect(saveButton).toBeEnabled();
    await saveButton.click();
    await expect
      .poll(() => fs.readFileSync(filePath, "utf8"), { timeout: 15_000 })
      .toContain(marker);
    await expect(saveButton).toBeDisabled();

    await editor.getByTestId("markdown-mode-preview").click();
    await expect(preview).toContainText(marker);
    await prCapture.screenshot("desktop-markdown-preview", {
      caption: "Desktop Markdown Preview with the saved source change",
    });
    await editor.getByTestId("markdown-mode-edit").click();
    await expect(testPage.getByTestId("hybrid-markdown-editor")).toBeVisible({ timeout: 15_000 });

    const storedTabs = await testPage.evaluate((sid) => {
      const raw = window.sessionStorage.getItem(`kandev.openFiles.${sid}`);
      return raw ? (JSON.parse(raw) as Array<{ path: string; markdownMode?: string }>) : [];
    }, sessionId);
    const storedTab = storedTabs.find((tab) => tab.path.endsWith(fileName));
    expect(storedTab?.markdownMode).toBe("edit");

    await testPage.reload();
    await expect(testPage.getByTestId("markdown-file-editor")).toBeVisible({ timeout: 30_000 });
    await expect(testPage.getByTestId("markdown-mode-edit")).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    await expect(testPage.getByTestId("hybrid-markdown-editor")).toBeVisible({ timeout: 15_000 });
  });

  test("keeps MDX on the safe Preview and exact Source paths", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const fileName = `markdown-components-${Date.now()}.mdx`;
    const { session } = await seedMarkdownSession({
      testPage,
      apiClient,
      seedData,
      backend,
      fileName,
      content: "# MDX content\n\n<Component />",
      taskTitle: "Markdown MDX modes",
    });

    await openFile(session, testPage, fileName);
    const editor = testPage.getByTestId("markdown-file-editor");
    await expect(editor.getByTestId("markdown-mode-preview")).toBeVisible();
    await expect(editor.getByTestId("markdown-mode-source")).toBeVisible();
    await expect(editor.getByTestId("markdown-mode-edit")).toHaveCount(0);
    await editor.getByTestId("markdown-mode-source").click();
    await expect(testPage.locator(".monaco-editor:visible").first()).toBeVisible({
      timeout: 15_000,
    });
    await expect(testPage.getByTestId("hybrid-markdown-editor")).toHaveCount(0);
  });
});
