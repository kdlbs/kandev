import fs from "node:fs";
import path from "node:path";
import { execSync } from "node:child_process";
import { expect, test } from "../../fixtures/test-base";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";
import { MODIFIER } from "./shared";

const SEARCH_TERM = "TaskWorkspaceNeedle";
const EXTRA_REPOSITORY_NAME = "content-search-extra";
const TARGET_LINE = 9;

function fileContent(marker: string, targetLine: number): string {
  const lines = Array.from(
    { length: targetLine - 1 },
    (_, index) => `export const filler${index + 1} = ${index + 1};`,
  );
  lines.push(`export const ${marker} = "${SEARCH_TERM}";`);
  return `${lines.join("\n")}\n`;
}

function initializeRepository(directory: string, gitEnv: NodeJS.ProcessEnv): GitHelper {
  fs.mkdirSync(directory, { recursive: true });
  execSync("git init -b main", { cwd: directory, env: gitEnv });
  return new GitHelper(directory, gitEnv);
}

test("@search keeps workspace modes out of non-task routes", async ({ testPage }) => {
  await testPage.goto("/settings/general");

  const dialog = testPage.getByRole("dialog");
  await testPage.keyboard.press(`${MODIFIER}+Shift+f`);
  await expect(dialog).not.toBeVisible();
  await testPage.keyboard.press(`${MODIFIER}+Shift+k`);
  await expect(dialog).not.toBeVisible();

  await testPage.keyboard.press(`${MODIFIER}+k`);
  await expect(dialog).toBeVisible();
  await expect(dialog.getByRole("tablist", { name: "Command palette mode" })).toHaveCount(0);
  await expect(dialog).not.toContainText("Switch mode");
});

test("@search searches all task repositories and opens the selected match", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(120_000);

  const suffix = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  const sharedPath = `src/content-search-${suffix}.ts`;
  const gitEnv = makeGitEnv(backend.tmpDir);

  const primaryGit = new GitHelper(path.join(backend.tmpDir, "repos", "e2e-repo"), gitEnv);
  primaryGit.createFile(sharedPath, fileContent("PRIMARY_REPOSITORY_MATCH", 3));
  primaryGit.stageAll();
  primaryGit.commit("seed primary workspace content search");

  const extraRepoDir = path.join(backend.tmpDir, "repos", `${EXTRA_REPOSITORY_NAME}-${suffix}`);
  const extraGit = initializeRepository(extraRepoDir, gitEnv);
  extraGit.createFile(sharedPath, fileContent("EXTRA_REPOSITORY_MATCH", TARGET_LINE));
  extraGit.stageAll();
  extraGit.commit("seed extra workspace content search");
  const extraRepository = await apiClient.createRepository(
    seedData.workspaceId,
    extraRepoDir,
    "main",
    { name: EXTRA_REPOSITORY_NAME },
  );

  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Multi-repository workspace content search",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId, extraRepository.id],
      executor_profile_id: seedData.worktreeExecutorProfileId,
    },
  );

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });

  // The global content-search shortcut must win even when an editable chat
  // surface owns focus.
  const composer = session.activeChat().getByTestId("chat-input-editor");
  await composer.click();
  await testPage.keyboard.press(`${MODIFIER}+Shift+f`);

  const dialog = testPage.getByRole("dialog");
  await expect(dialog).toBeVisible({ timeout: 5_000 });
  const input = dialog.getByRole("combobox");
  await expect(input).toBeFocused();
  await input.fill(SEARCH_TERM);

  const groups = dialog.getByTestId("content-search-repo-group");
  await expect(groups).toHaveCount(2, { timeout: 15_000 });
  await expect(
    dialog.locator(
      `[data-testid="content-search-repo-group"][data-repository="${EXTRA_REPOSITORY_NAME}"]`,
    ),
  ).toHaveCount(1);

  const commandsTab = dialog.getByRole("tab", { name: "Commands" });
  const filesTab = dialog.getByRole("tab", { name: "Files" });
  const contentsTab = dialog.getByRole("tab", { name: "Contents" });
  await expect(contentsTab).toHaveAttribute("aria-selected", "true");

  await commandsTab.click();
  await expect(input).toBeFocused();
  await expect(input).toHaveValue(SEARCH_TERM);
  await expect(commandsTab).toHaveAttribute("aria-selected", "true");

  await testPage.keyboard.press("Tab");
  await expect(filesTab).toHaveAttribute("aria-selected", "true");
  await expect(input).toHaveValue(SEARCH_TERM);

  const fileQuery = path.basename(sharedPath);
  await input.fill(fileQuery);
  const fileMatches = dialog.locator("[cmdk-item]").filter({ hasText: fileQuery });
  await expect(fileMatches).toHaveCount(2, { timeout: 10_000 });
  const fileGroups = dialog.getByTestId("file-search-repo-group");
  await expect(fileGroups).toHaveCount(2);
  const extraFileGroup = fileGroups.filter({ hasText: EXTRA_REPOSITORY_NAME });
  await expect(extraFileGroup).toHaveCount(1);
  await expect(extraFileGroup.locator("[cmdk-item]").filter({ hasText: fileQuery })).toHaveCount(1);

  await input.fill(SEARCH_TERM);
  await testPage.keyboard.press("Tab");
  await expect(contentsTab).toHaveAttribute("aria-selected", "true");
  await expect(input).toBeFocused();
  await testPage.keyboard.press("Tab");
  await expect(commandsTab).toHaveAttribute("aria-selected", "true");
  await testPage.keyboard.press("Shift+Tab");
  await expect(contentsTab).toHaveAttribute("aria-selected", "true");
  await expect(input).toBeFocused();
  await testPage.keyboard.press("Shift+Tab");
  await expect(filesTab).toHaveAttribute("aria-selected", "true");
  await expect(dialog).toBeVisible();
  await expect(input).toBeFocused();

  await testPage.keyboard.press("Tab");
  await expect(contentsTab).toHaveAttribute("aria-selected", "true");
  await expect(groups).toHaveCount(2, { timeout: 15_000 });

  const extraResult = dialog
    .locator(`[data-testid="content-search-result"][data-repository="${EXTRA_REPOSITORY_NAME}"]`)
    .filter({ hasText: sharedPath });
  await expect(extraResult).toBeVisible();
  await expect(extraResult).toContainText(`${TARGET_LINE}`);
  await extraResult.click();

  await expect(dialog).not.toBeVisible({ timeout: 5_000 });
  const selectedEditor = testPage
    .locator(".monaco-editor:visible .view-lines")
    .filter({ hasText: "EXTRA_REPOSITORY_MATCH" });
  await expect(selectedEditor).toBeVisible({ timeout: 15_000 });

  await expect
    .poll(
      () =>
        testPage.evaluate((marker) => {
          type Editor = {
            getModel: () => { getValue: () => string } | null;
            getPosition: () => { lineNumber: number } | null;
          };
          type MonacoWindow = Window & {
            monaco?: { editor: { getEditors: () => Editor[] } };
          };
          const editors = (window as MonacoWindow).monaco?.editor.getEditors() ?? [];
          return (
            editors.find((editor) => editor.getModel()?.getValue().includes(marker))?.getPosition()
              ?.lineNumber ?? null
          );
        }, "EXTRA_REPOSITORY_MATCH"),
      { timeout: 10_000 },
    )
    .toBe(TARGET_LINE);
});
