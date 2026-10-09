import type { Page } from "@playwright/test";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import type { ApiClient } from "./api-client";
import { makeGitEnv } from "./git-helper";

export async function createFolderTestRepository(
  apiClient: ApiClient,
  workspaceId: string,
  root: string,
) {
  const directory = path.join(root, "repos", "folder-second-repo");
  fs.mkdirSync(directory, { recursive: true });
  const env = makeGitEnv(root);
  execFileSync("git", ["init", "-b", "main"], { cwd: directory, env });
  execFileSync("git", ["commit", "--allow-empty", "-m", "init"], { cwd: directory, env });
  return apiClient.createRepository(workspaceId, directory, "main", {
    name: "Folder second repo",
    pull_before_worktree: false,
  });
}

export async function findFolderTestWorktree(
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
  repositoryId: string,
) {
  const { sessions } = await apiClient.listTaskSessions(taskId);
  const session = sessions.find((item) => item.id === sessionId);
  const worktree = session?.worktrees?.find((item) => item.repository_id === repositoryId);
  if (!worktree) throw new Error("Task session has no worktree for folder test repository");
  const id = worktree.worktree_id || worktree.id;
  if (!id) throw new Error("Folder test worktree has no identity");
  return id;
}

export async function mockFolderAvailability(page: Page, available: boolean) {
  await page.route("**/api/v1/editors", async (route) => {
    const response = await route.fetch();
    await route.fulfill({
      response,
      json: { ...(await response.json()), folder_opening_available: available },
    });
  });
}
