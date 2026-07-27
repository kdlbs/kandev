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
  test("adds a local repository and folder successively, scopes Changes to Git, and persists", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
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

    const workspaceActions = testPage.getByTestId("files-workspace-actions");
    await expect(workspaceActions).toBeEnabled();
    await expect(workspaceActions).toHaveAccessibleName("Workspace actions");
    const workspaceActionsBox = await workspaceActions.boundingBox();
    expect(workspaceActionsBox).not.toBeNull();
    expect(workspaceActionsBox!.width).toBeLessThanOrEqual(44);
    await workspaceActions.click();
    const addSources = testPage.getByRole("menuitem", {
      name: "Add Repositories to workspace",
    });
    const openFolder = testPage.getByRole("menuitem", { name: "Open workspace folder" });
    await expect(addSources).toBeEnabled();
    await expect(openFolder).toBeEnabled();
    if (!task.session_id) throw new Error("task creation did not return a session id");
    await Promise.all([
      testPage.waitForRequest(
        (request) =>
          request.method() === "POST" &&
          request.url().endsWith(`/api/v1/task-sessions/${task.session_id}/open-folder`),
      ),
      openFolder.click(),
    ]);
    await expect(openFolder).not.toBeVisible();
    await workspaceActions.click();
    await testPage.getByRole("menuitem", { name: "Add Repositories to workspace" }).click();
    const dialog = testPage.getByTestId("add-workspace-sources-dialog");
    await expect(dialog).toBeVisible();
    const [dialogBox, viewport] = await Promise.all([
      dialog.boundingBox(),
      testPage.evaluate(() => ({ width: innerWidth, height: innerHeight })),
    ]);
    expect(dialogBox).not.toBeNull();
    expect(dialogBox!.y).toBeGreaterThanOrEqual(0);
    expect(dialogBox!.y + dialogBox!.height).toBeLessThanOrEqual(viewport.height);
    await expect(dialog.getByTestId("add-workspace-sources-dialog-scroll")).toHaveCSS(
      "overflow-y",
      "auto",
    );
    const consequences = dialog.getByTestId("workspace-change-consequences");
    await expect(consequences).toBeVisible();
    await expect(consequences).toContainText("This restarts the task workspace");
    await expect(consequences).toContainText("Cancel leaves the workspace unchanged");
    await expect(consequences).toContainText(
      "terminals, dev servers, and other workspace processes stop",
    );
    const fullImpactDetails = consequences.getByText("Full impact details");
    await fullImpactDetails.click();
    await expect(
      consequences.getByText(/The task root becomes the agent's working directory/),
    ).toBeVisible();
    await fullImpactDetails.click();
    await expect(dialog.getByTestId("source-mode-local")).toHaveCount(0);
    const addRepository = dialog.getByRole("button", { name: "Add repository" });
    const submit = dialog.getByTestId("add-workspace-sources-submit");
    await expect(submit).toBeDisabled();
    await addRepository.click();
    await expect(testPage.getByRole("menuitem", { name: "Local Git repository" })).toBeVisible();
    await expect(testPage.getByRole("menuitem", { name: "Remote repository" })).toBeVisible();
    await testPage.getByRole("menuitem", { name: "Workspace repository" }).click();
    const savedRepositoryRow = dialog.getByTestId("workspace-source-row").first();
    await expect(savedRepositoryRow.getByRole("alert")).toHaveCount(0);
    await expect(savedRepositoryRow.getByRole("textbox", { name: "Checkout branch" })).toHaveCount(
      0,
    );
    await savedRepositoryRow.getByTestId("repo-chip-trigger").click();
    await expect(testPage.getByTestId("repo-refresh-button")).toBeVisible();
    await expect(testPage.getByTestId("create-local-repository-button")).toBeVisible();
    await testPage.keyboard.press("Escape");
    await submit.click();
    const savedRepositoryError = savedRepositoryRow.getByRole("alert");
    await expect(savedRepositoryError).toHaveText("Choose a repository and base branch.");
    await expect(savedRepositoryError).toHaveCSS("font-size", "12px");
    await savedRepositoryRow.getByRole("button", { name: "Remove source" }).click();
    await addRepository.click();
    await testPage.getByRole("menuitem", { name: "Local Git repository" }).click();
    const repositoryRow = dialog.getByTestId("workspace-source-row");
    await chooseDirectory(
      testPage,
      repositoryRow.getByTestId("folder-picker-trigger"),
      backend.tmpDir,
      repositoryPath,
    );
    await repositoryRow.getByRole("textbox", { name: "Base branch" }).fill("main");
    await submit.click();
    await expect(dialog).not.toBeVisible();
    await expect(
      session.files
        .getByTestId("file-tree-node")
        .filter({ hasText: "second-local-repository-main" }),
    ).toBeVisible({ timeout: 30_000 });
    if (!task.session_id) throw new Error("task creation did not return a session id");
    const turnsAfterFirstAttachment = await apiClient.listSessionTurns(task.session_id);
    expect(turnsAfterFirstAttachment.turns.filter((turn) => !turn.completed_at)).toEqual([]);

    await workspaceActions.click();
    await testPage.getByRole("menuitem", { name: "Add Repositories to workspace" }).click();
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button", { name: "Add folder" }).click();
    const folderRow = dialog.getByTestId("workspace-source-row");
    await chooseDirectory(
      testPage,
      folderRow.getByTestId("folder-picker-trigger"),
      backend.tmpDir,
      folderPath,
    );
    await prCapture.screenshot("workspace-actions-mixed-sources", {
      caption: "Desktop Add to workspace dialog with a local folder configured",
    });
    await submit.click();
    await expect(dialog).not.toBeVisible();

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

    const workspaceActions = testPage.getByTestId("files-workspace-actions");
    await expect(workspaceActions).toBeEnabled();
    await workspaceActions.click();
    const action = testPage.getByRole("menuitem", {
      name: "Add Repositories to workspace",
    });
    await expect(action).toBeDisabled();
    await expect(action).toContainText(
      "Wait for the active turn or tool call to finish before adding sources.",
    );
    await expect(testPage.getByRole("menuitem", { name: "Open workspace folder" })).toBeEnabled();
  });
});
