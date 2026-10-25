import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import type { ApiClient } from "../../helpers/api-client";
import { makeGitEnv } from "../../helpers/git-helper";

export const SIDEBAR_COMBINATION_REPOSITORY_NAME = "Sidebar combination repo";

export async function seedSidebarCombinationRepository(
  apiClient: Pick<ApiClient, "createRepository">,
  workspaceId: string,
  tmpDir: string,
): Promise<{ id: string }> {
  const repositoryPath = path.join(tmpDir, "repos", "sidebar-combination-repo");
  fs.mkdirSync(repositoryPath, { recursive: true });
  const gitEnv = makeGitEnv(tmpDir);
  execSync("git init -b main", { cwd: repositoryPath, env: gitEnv });
  execSync('git commit --allow-empty -m "init"', { cwd: repositoryPath, env: gitEnv });
  return apiClient.createRepository(workspaceId, repositoryPath, "main", {
    name: SIDEBAR_COMBINATION_REPOSITORY_NAME,
  });
}

export async function repositoryName(
  apiClient: Pick<ApiClient, "listRepositories">,
  workspaceId: string,
  repositoryId: string,
): Promise<string> {
  const { repositories } = await apiClient.listRepositories(workspaceId);
  const repository = repositories.find((candidate) => candidate.id === repositoryId);
  if (!repository) throw new Error(`Repository ${repositoryId} was not returned by the workspace`);
  return repository.name;
}
