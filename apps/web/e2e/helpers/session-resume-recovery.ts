import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../fixtures/test-base";
import type { CreateTaskResponse } from "../../lib/types/http";
import type { ApiClient } from "./api-client";
import { GitHelper, makeGitEnv } from "./git-helper";
import { SessionPage } from "../pages/session-page";

type TaskEnvironmentRepository = {
  repository_id?: string;
  worktree_id?: string;
  worktree_path?: string;
  worktree_branch?: string;
  status?: string;
};

type TaskEnvironment = {
  id: string;
  status: string;
  repos?: TaskEnvironmentRepository[];
};

export type WorktreeRecoveryFixture = {
  task: CreateTaskResponse;
  session: SessionPage;
  environment: TaskEnvironment;
  repository: TaskEnvironmentRepository;
};

/** Create a real worktree-backed session and wait until its first turn is idle. */
export async function seedWorktreeRecoveryFixture(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<WorktreeRecoveryFixture> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repositories: [{ repository_id: seedData.repositoryId, base_branch: "main" }],
      executor_profile_id: seedData.worktreeExecutorProfileId,
    },
  );
  if (!task.session_id) throw new Error("worktree recovery task has no session_id");

  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 60_000 });

  let environment: TaskEnvironment | null = null;
  await expect
    .poll(
      async () => {
        environment = (await apiClient.getTaskEnvironment(task.id)) as TaskEnvironment | null;
        return environment?.repos?.some(
          (repository) =>
            repository.repository_id === seedData.repositoryId &&
            repository.worktree_path &&
            repository.worktree_branch,
        );
      },
      { timeout: 60_000, message: "Waiting for the worktree recovery fixture" },
    )
    .toBe(true);

  const repository = environment?.repos?.find(
    (candidate) => candidate.repository_id === seedData.repositoryId,
  );
  if (!environment || !repository?.worktree_path || !repository.worktree_branch) {
    throw new Error("worktree recovery fixture did not expose a repository worktree");
  }
  return { task, session, environment, repository };
}

/** Remove the branch from both the local repository and its disposable origin. */
export function removeRecoveryBranch(
  repositoryPath: string,
  tmpDir: string,
  repository: TaskEnvironmentRepository,
): { originalPath: string; originalBranch: string } {
  if (!repository.worktree_path || !repository.worktree_branch) {
    throw new Error("cannot remove a recovery branch without a worktree path and branch");
  }
  const git = new GitHelper(repositoryPath, makeGitEnv(tmpDir));
  const branch = repository.worktree_branch;
  const worktreePath = repository.worktree_path;

  // Publish the branch first so the test proves the normal remote-ref lookup
  // also observes the branch deletion, rather than relying on a never-pushed
  // local-only branch.
  git.exec(`git push origin "${branch}"`);
  git.exec(`git worktree remove --force "${worktreePath}"`);
  git.exec(`git branch -D "${branch}"`);
  git.exec(`git push origin --delete "${branch}"`);

  return { originalPath: worktreePath, originalBranch: branch };
}
