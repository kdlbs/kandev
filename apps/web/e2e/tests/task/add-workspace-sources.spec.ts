import { expect, test } from "../../fixtures/test-base";
import type { Locator, Page } from "@playwright/test";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

async function chooseDirectory(
  page: Page,
  trigger: Locator,
  pickerRoot: string,
  directory: string,
) {
  const relativeDirectory = path.relative(pickerRoot, directory);
  if (relativeDirectory === ".." || relativeDirectory.startsWith(`..${path.sep}`)) {
    throw new Error(`Directory ${directory} is outside picker root ${pickerRoot}`);
  }
  await trigger.click();
  const picker = page.locator('[data-testid="folder-picker-popover"][data-state="open"]');
  await expect(picker).toBeVisible();
  for (const segment of relativeDirectory.split(path.sep).filter(Boolean)) {
    await picker.getByTestId("folder-picker-entry").filter({ hasText: segment }).click();
  }
  await picker.getByTestId("folder-picker-choose").click();
}

function createSourceDirectories(root: string) {
  const gitEnv = makeGitEnv(root);
  const repositoryPath = path.join(root, "sources", "second-local-repository");
  const folderPath = path.join(root, "sources", "plain-local-folder");
  fs.mkdirSync(repositoryPath, { recursive: true });
  fs.mkdirSync(folderPath, { recursive: true });
  fs.writeFileSync(path.join(repositoryPath, "second-source.txt"), "repository source\n");
  fs.writeFileSync(path.join(folderPath, "folder-source.txt"), "folder source\n");
  execFileSync("git", ["init", "-b", "main"], { cwd: repositoryPath, env: gitEnv });
  execFileSync("git", ["add", "."], { cwd: repositoryPath, env: gitEnv });
  execFileSync("git", ["commit", "-m", "initial source"], { cwd: repositoryPath, env: gitEnv });
  return { repositoryPath, folderPath };
}

test.describe("Attach local workspace sources", () => {
  test("adds a local repository and folder atomically, scopes Changes to Git, and persists", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);
    const { repositoryPath, folderPath } = createSourceDirectories(backend.tmpDir);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Attach mixed local sources",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    await session.clickTab("Files");

    const addSources = testPage.getByTestId("files-add-sources");
    await expect(addSources).toBeEnabled();
    await addSources.click();
    const dialog = testPage.getByTestId("add-workspace-sources-dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button", { name: "Local Git repository" }).click();
    await dialog.getByRole("button", { name: "Local folder" }).click();
    const rows = dialog.getByTestId("workspace-source-row");
    await expect(rows).toHaveCount(2);
    await chooseDirectory(
      testPage,
      rows.nth(0).getByTestId("folder-picker-trigger"),
      backend.tmpDir,
      repositoryPath,
    );
    await rows.nth(0).getByRole("textbox", { name: "Base branch" }).fill("main");
    await chooseDirectory(
      testPage,
      rows.nth(1).getByTestId("folder-picker-trigger"),
      backend.tmpDir,
      folderPath,
    );
    await dialog.getByTestId("add-workspace-sources-submit").click();
    await expect(dialog).not.toBeVisible();

    await expect(
      session.files
        .getByTestId("file-tree-node")
        .filter({ hasText: "second-local-repository-main" }),
    ).toBeVisible({ timeout: 30_000 });
    await expect(
      session.files.getByTestId("file-tree-node").filter({ hasText: "plain-local-folder" }),
    ).toBeVisible();

    const sessionData = (await apiClient.listTaskSessions(task.id)) as {
      sessions: Array<{ worktrees?: Array<{ worktree_path?: string }> }>;
    };
    const repoPaths = (sessionData.sessions[0]?.worktrees ?? []).flatMap((worktree) =>
      worktree.worktree_path ? [worktree.worktree_path] : [],
    );
    expect(repoPaths).toHaveLength(2);
    for (const [index, repoPath] of repoPaths.entries()) {
      new GitHelper(repoPath, makeGitEnv(backend.tmpDir)).createFile(
        `changes/repository-${index}.txt`,
        `repository ${index}\n`,
      );
    }

    await session.clickTab("Changes");
    const changes = session.changes;
    await expect(changes.getByTestId("changes-repo-group")).toHaveCount(2, { timeout: 30_000 });
    await expect(
      changes.getByTestId("changes-repo-header").filter({ hasText: "second-local-repository" }),
    ).toBeVisible();
    await expect(changes.getByText("plain-local-folder", { exact: true })).not.toBeVisible();

    await testPage.reload();
    await session.waitForLoad();
    await session.clickTab("Files");
    await expect(
      session.files
        .getByTestId("file-tree-node")
        .filter({ hasText: "second-local-repository-main" }),
    ).toBeVisible({ timeout: 30_000 });
    await expect(
      session.files.getByTestId("file-tree-node").filter({ hasText: "plain-local-folder" }),
    ).toBeVisible();
  });

  test("explains the disabled action while an active turn is running", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Busy source attachment gate",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    await session.sendMessage("/slow 8s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
    await session.clickTab("Files");

    const action = testPage.getByTestId("files-add-sources");
    await expect(action).toBeDisabled();
    const tooltipTrigger = action.locator("..");
    await tooltipTrigger.focus();
    await expect(testPage.getByRole("tooltip")).toContainText(
      "Wait for the active turn or tool call to finish before adding sources.",
    );
  });
});
