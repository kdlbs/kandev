import fs from "node:fs";
import path from "node:path";
import { execSync } from "node:child_process";
import type { Page } from "@playwright/test";
import { expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { BackendContext } from "../../fixtures/backend";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { watchWs } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";

export async function setupPathCopyTask(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  backend: BackendContext,
) {
  const suffix = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  const repositoryName = `path-copy-${suffix}`;
  const repoDir = path.join(backend.tmpDir, "repos", repositoryName);
  const gitEnv = makeGitEnv(backend.tmpDir);
  fs.mkdirSync(repoDir, { recursive: true });
  execSync("git init -b main", { cwd: repoDir, env: gitEnv });
  const git = new GitHelper(repoDir, gitEnv);
  const fileName = `café copy ${suffix}.txt`;
  const folderName = `資料 folder ${suffix}`;
  const contents = "Path copy fixture contents\n";
  git.createFile(fileName, contents);
  git.createFile(`${folderName}/nested.txt`, "nested content\n");
  git.stageAll();
  git.commit("add path copy fixture");
  const repository = await apiClient.createRepository(seedData.workspaceId, repoDir, "main", {
    name: repositoryName,
    pull_before_worktree: false,
  });
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    `Files path copy ${suffix}`,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId, repository.id],
      executor_profile_id: seedData.worktreeExecutorProfileId,
    },
  );
  let workspacePath = "";
  let repoPath = "";
  await expect
    .poll(
      async () => {
        const environment = await apiClient.getTaskEnvironment(task.id);
        workspacePath = environment?.workspace_path ?? "";
        repoPath =
          environment?.repos?.find((repo) => repo.repository_id === repository.id)?.worktree_path ??
          "";
        return (
          environment?.status === "ready" &&
          !!workspacePath &&
          !!repoPath &&
          fs.existsSync(path.join(repoPath, fileName)) &&
          fs.existsSync(path.join(repoPath, folderName, "nested.txt"))
        );
      },
      { timeout: 60_000, message: "Waiting for the multi-repository task workspace" },
    )
    .toBe(true);
  const root = path.relative(workspacePath, repoPath).split(path.sep).join("/");
  expect(root).not.toBe("");
  expect(root.startsWith("..")).toBe(false);
  const filePath = `${root}/${fileName}`;
  const folderPath = `${root}/${folderName}`;
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  if (!task.session_id) throw new Error("Path copy task did not return a session ID");
  const gateway = watchWs(page);
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 45_000 });
  return {
    session,
    sessionId: task.session_id,
    gateway,
    root,
    filePath,
    folderPath,
    fileName,
    repoPath,
    contents,
  };
}

export async function expectClipboard(page: Page, expected: string) {
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(expected);
}
