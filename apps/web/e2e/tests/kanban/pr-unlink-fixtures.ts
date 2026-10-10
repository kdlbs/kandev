import { randomUUID } from "node:crypto";
import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { expect } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { makeGitEnv } from "../../helpers/git-helper";
import type { BackendContext } from "../../fixtures/backend";
import type { SeedData } from "../../fixtures/test-base";

const OWNER = "acme";
const PR_NUMBER = 42;

export async function seedCardWithMultiplePRs(
  apiClient: ApiClient,
  backend: BackendContext,
  seedData: SeedData,
  title: string,
) {
  const secondaryRepoDir = path.join(backend.tmpDir, "repos", `pr-unlink-${randomUUID()}`);
  fs.mkdirSync(secondaryRepoDir, { recursive: true });
  const gitEnv = makeGitEnv(backend.tmpDir);
  execSync("git init -b main", { cwd: secondaryRepoDir, env: gitEnv });
  execSync('git commit --allow-empty -m "init"', { cwd: secondaryRepoDir, env: gitEnv });
  const secondaryRepo = await apiClient.createRepository(
    seedData.workspaceId,
    secondaryRepoDir,
    "main",
    { name: "PR unlink API repository", pull_before_worktree: false },
  );

  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("test-user");
  const task = await apiClient.createTask(seedData.workspaceId, title, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId, secondaryRepo.id],
  });

  for (const [repositoryId, repo] of [
    [seedData.repositoryId, "web"],
    [secondaryRepo.id, "api"],
  ] as const) {
    await apiClient.mockGitHubAssociateTaskPR({
      workspace_id: seedData.workspaceId,
      task_id: task.id,
      repository_id: repositoryId,
      owner: OWNER,
      repo,
      pr_number: PR_NUMBER,
      pr_url: `https://github.com/${OWNER}/${repo}/pull/${PR_NUMBER}`,
      pr_title: `${repo} PR ${PR_NUMBER}`,
      head_branch: `feat/${repo}`,
      base_branch: "main",
      author_login: "test-user",
      state: "open",
    });
  }

  await expect
    .poll(async () =>
      (await apiClient.listTaskPRs(task.id)).map((pr) => `${pr.repo}#${pr.pr_number}`).sort(),
    )
    .toEqual(["api#42", "web#42"]);

  return { task, title, workflowStepId: seedData.startStepId };
}

export const cardPRUnlinkLabels = {
  api: `Remove ${OWNER}/api #${PR_NUMBER} from task`,
  web: `Remove ${OWNER}/web #${PR_NUMBER} from task`,
};
