import { expect, type Page } from "@playwright/test";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { pathToFileURL } from "node:url";
import type { ApiClient } from "./api-client";
import { GitHelper, makeGitEnv } from "./git-helper";
import { SessionPage } from "../pages/session-page";

export type EmptyRemoteRepository = {
  localPath: string;
  remotePath: string;
  remoteURL: string;
  gitEnv: NodeJS.ProcessEnv;
  cleanup: () => void;
};

export function createEmptyRemoteRepository(tmpDir: string, label: string): EmptyRemoteRepository {
  const root = path.join(tmpDir, "repos", `empty-remote-${label}-${Date.now()}-${process.pid}`);
  const remotePath = path.join(root, "origin.git");
  const localPath = path.join(root, "checkout");
  const gitEnv = makeGitEnv(tmpDir);

  fs.mkdirSync(path.dirname(root), { recursive: true });
  execFileSync("git", ["init", "--bare", "-b", "main", remotePath], {
    env: gitEnv,
    stdio: "ignore",
  });
  execFileSync("git", ["init", "-b", "main", localPath], {
    env: gitEnv,
    stdio: "ignore",
  });
  const remoteURL = pathToFileURL(remotePath).href;
  execFileSync("git", ["remote", "add", "origin", remoteURL], {
    cwd: localPath,
    env: gitEnv,
    stdio: "ignore",
  });

  return {
    localPath,
    remotePath,
    remoteURL,
    gitEnv,
    cleanup: () => fs.rmSync(root, { recursive: true, force: true }),
  };
}

export async function openTaskByID(page: Page, taskID: string): Promise<SessionPage> {
  await page.goto(`/t/${taskID}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  return session;
}

export async function waitForTaskWorktree(
  apiClient: ApiClient,
  taskID: string,
  repositoryID: string,
): Promise<string> {
  await expect
    .poll(
      async () => {
        const environment = await apiClient.getTaskEnvironment(taskID);
        const repositoryWorktree = environment?.repos?.find(
          (repo) => repo.repository_id === repositoryID,
        )?.worktree_path;
        return (
          repositoryWorktree || environment?.worktree_path || environment?.workspace_path || ""
        );
      },
      { timeout: 60_000, message: "Waiting for the empty-remote task worktree" },
    )
    .not.toBe("");

  const environment = await apiClient.getTaskEnvironment(taskID);
  const repositoryWorktree = environment?.repos?.find(
    (repo) => repo.repository_id === repositoryID,
  )?.worktree_path;
  const worktreePath =
    repositoryWorktree || environment?.worktree_path || environment?.workspace_path;
  if (!worktreePath) throw new Error("Empty-remote task environment has no worktree path");
  return worktreePath;
}

export function remoteRef(git: GitHelper, branch: string): string {
  const output = git.exec(`git ls-remote --refs origin refs/heads/${branch}`).trim();
  return output.split(/\s+/u)[0] || "";
}

export async function removeTestRepository(apiClient: ApiClient, repositoryID: string) {
  const response = await apiClient.rawRequest("DELETE", `/api/v1/repositories/${repositoryID}`);
  if (!response.ok && response.status !== 404 && response.status !== 409) {
    throw new Error(
      `Failed to remove empty-remote test repository (${response.status}): ${await response.text()}`,
    );
  }
}

export function taskWorktreeGit(pathname: string, gitEnv: NodeJS.ProcessEnv): GitHelper {
  return new GitHelper(pathname, gitEnv);
}
