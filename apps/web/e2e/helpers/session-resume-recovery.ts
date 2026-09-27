import { expect, type Page } from "@playwright/test";
import { randomUUID } from "node:crypto";
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import type { SeedData } from "../fixtures/test-base";
import type { CreateTaskResponse } from "../../lib/types/http";
import type { ApiClient } from "./api-client";
import { GitHelper, makeGitEnv } from "./git-helper";
import { SessionPage } from "../pages/session-page";
import { waitForSessionState } from "./session";

type SqliteTestDatabase = {
  exec(sql: string): void;
  prepare(sql: string): {
    get(...parameters: unknown[]): unknown;
    run(...parameters: unknown[]): unknown;
  };
  close(): void;
};

const nodeRequire = createRequire(path.join(process.cwd(), "package.json"));

/** Seed the typed recovery projection after the test has stopped its session. */
export function seedManagedCloneRelocationFailure(tmpDir: string, sessionId: string): string {
  const { DatabaseSync } = nodeRequire("node:sqlite") as {
    DatabaseSync: new (databasePath: string) => SqliteTestDatabase;
  };
  const db = new DatabaseSync(path.join(tmpDir, "kandev.db"));
  try {
    db.exec("PRAGMA busy_timeout = 5000");
    const row = db.prepare("SELECT metadata FROM task_sessions WHERE id = ?").get(sessionId) as
      | { metadata?: string | null }
      | undefined;
    if (!row) throw new Error(`Session ${sessionId} was not found in the E2E database`);
    const metadata = row.metadata ? (JSON.parse(row.metadata) as Record<string, unknown>) : {};
    const stamp = `managed-clone-e2e-${Date.now()}`;
    metadata.last_agent_error = {
      message: "The task workspace contains local changes and needs explicit relocation.",
      occurred_at: new Date().toISOString(),
      scope: "session",
      phase: "bootstrap",
      code: "managed_clone_relocation_required",
      details: "Move the workspace files and resume to continue this task session.",
      recovery_actions: ["relocate_and_resume"],
      stamp,
    };
    db.prepare("UPDATE task_sessions SET metadata = ?, updated_at = ? WHERE id = ?").run(
      JSON.stringify(metadata),
      new Date().toISOString(),
      sessionId,
    );
    return stamp;
  } finally {
    db.close();
  }
}

/** Remove relocation test tasks and return the worker's shared repository to its local fixture. */
export async function cleanupManagedCloneRelocationFixture(
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<void> {
  await apiClient.e2eReset(seedData.workspaceId, [seedData.workflowId]);
  await apiClient.mockGitHubReset();
  await apiClient.updateRepository(seedData.repositoryId, {
    source_type: "local",
    local_path: seedData.repositoryPath,
    provider: "",
    provider_repo_id: "",
    provider_host: "",
    provider_scope: "",
    provider_owner: "",
    provider_name: "",
    remote_url: seedData.repositoryRemoteURL,
  });
}

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

export type ManagedCloneRelocationFixture = WorktreeRecoveryFixture & {
  sourceClonePath: string;
  destinationClonePath: string;
  dirtyFileName: string;
  dirtyFileContent: string;
  originalHead: string;
  originalBranch: string;
};

export async function countSimpleMockResponses(
  apiClient: ApiClient,
  sessionId: string,
): Promise<number> {
  const { messages } = await apiClient.listSessionMessages(sessionId);
  return messages.filter(
    (message) =>
      message.author_type === "agent" && message.content.includes("simple mock response"),
  ).length;
}

/** Create the old shared clone, its workspace clone, and a dirty linked worktree. */
export async function seedManagedCloneRelocationFixture(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  backend: { tmpDir: string },
  title: string,
): Promise<ManagedCloneRelocationFixture> {
  await apiClient.mockGitHubReset();
  const repoName = `relocation-${randomUUID().replaceAll("-", "").slice(0, 12)}`;
  const cloneRoot = path.join(backend.tmpDir, "managed-repos");
  const sourceClonePath = path.join(cloneRoot, "e2e", repoName);
  const destinationClonePath = path.join(
    cloneRoot,
    "workspaces",
    seedData.workspaceId,
    "github",
    "e2e",
    repoName,
  );
  const origin = `https://github.com/e2e/${repoName}.git`;
  const gitEnv = makeGitEnv(backend.tmpDir);

  fs.mkdirSync(path.dirname(sourceClonePath), { recursive: true });
  execFileSync("git", ["clone", "--local", seedData.repositoryPath, sourceClonePath], {
    env: gitEnv,
    stdio: "pipe",
  });
  execFileSync("git", ["-C", sourceClonePath, "remote", "set-url", "origin", origin], {
    env: gitEnv,
    stdio: "pipe",
  });
  await apiClient.updateRepository(seedData.repositoryId, {
    // Model a pre-isolation task: create its linked worktree while this clone
    // is still registered as a local source, then switch repository metadata
    // to the new workspace-scoped provider clone after the task is idle.
    source_type: "local",
    local_path: sourceClonePath,
    remote_url: origin,
    default_branch: "main",
    pull_before_worktree: false,
  });

  const fixture = await seedWorktreeRecoveryFixture(page, apiClient, seedData, title);
  fs.mkdirSync(path.dirname(destinationClonePath), { recursive: true });
  execFileSync("git", ["clone", "--local", sourceClonePath, destinationClonePath], {
    env: gitEnv,
    stdio: "pipe",
  });
  execFileSync("git", ["-C", destinationClonePath, "remote", "set-url", "origin", origin], {
    env: gitEnv,
    stdio: "pipe",
  });
  await apiClient.updateRepository(seedData.repositoryId, {
    source_type: "provider",
    local_path: destinationClonePath,
    provider: "github",
    provider_repo_id: `e2e-${repoName}`,
    provider_host: "https://github.com",
    provider_owner: "e2e",
    provider_name: repoName,
    remote_url: origin,
  });
  await apiClient.mockGitHubSetUser("relocation-e2e");
  await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
    source: "legacy_shared",
    status: "active",
  });

  const originalPath = fixture.repository.worktree_path;
  if (!originalPath) throw new Error("managed clone fixture has no original worktree path");
  const dirtyFileName = `relocation-note-${randomUUID().slice(0, 8)}.txt`;
  const dirtyFileContent = `preserved local worktree content ${randomUUID()}`;
  fs.writeFileSync(path.join(originalPath, dirtyFileName), dirtyFileContent, { mode: 0o755 });
  const git = new GitHelper(originalPath, gitEnv);

  return {
    ...fixture,
    sourceClonePath,
    destinationClonePath,
    dirtyFileName,
    dirtyFileContent,
    originalHead: git.getCurrentSha(),
    originalBranch: git.exec("git branch --show-current").trim(),
  };
}

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
  await session.expectChatResponseVisible("simple mock response", 0, { timeout: 30_000 });

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

/** Use the fixture's active primary session as the archive recovery session. */
export async function prepareArchiveRecoverySession(
  apiClient: ApiClient,
  fixture: WorktreeRecoveryFixture,
): Promise<string> {
  const sessionId = fixture.task.session_id;
  if (!sessionId) throw new Error("worktree recovery fixture has no primary session");
  await waitForSessionState(apiClient, {
    taskId: fixture.task.id,
    sessionId,
    expectedState: "WAITING_FOR_INPUT",
    message: "Waiting for the archive recovery session to become active",
    timeout: 60_000,
  });
  return sessionId;
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
