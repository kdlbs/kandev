import { expect, type Locator, type Page } from "@playwright/test";
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
import { waitForAgentMessage, waitForSessionDone } from "./session";

type SqliteTestDatabase = {
  exec(sql: string): void;
  prepare(sql: string): {
    get(...parameters: unknown[]): unknown;
    all(...parameters: unknown[]): unknown[];
    run(...parameters: unknown[]): unknown;
  };
  close(): void;
};

const nodeRequire = createRequire(path.join(process.cwd(), "package.json"));

type SessionRecoveryFrame = {
  id?: string;
  action?: string;
  type?: string;
  payload?: unknown;
};

function forEachSessionRecoveryFrame(
  payload: unknown,
  consume: (frame: SessionRecoveryFrame) => void,
) {
  if (typeof payload !== "string") return;
  for (const part of payload.split("\n").filter(Boolean)) {
    let frame: SessionRecoveryFrame;
    try {
      frame = JSON.parse(part) as SessionRecoveryFrame;
    } catch {
      continue;
    }
    if (frame.action === "session.recover" && typeof frame.id === "string") consume(frame);
  }
}

/** Capture requests and responses for explicit session.recover requests. */
export function captureSessionRecoveryMessages(page: Page) {
  const requestIds: Record<string, string> = {};
  const requestCounts: Record<string, number> = {};
  const requests = new Map<string, unknown>();
  const responses = new Map<string, { type?: string; payload?: unknown }>();
  page.on("websocket", (socket) => {
    if (!socket.url().endsWith("/ws")) return;
    socket.on("framesent", ({ payload }) => {
      forEachSessionRecoveryFrame(payload, (frame) => {
        const action = (frame.payload as { action?: unknown } | null)?.action;
        if (frame.type === "request" && typeof action === "string") {
          requestIds[action] = frame.id!;
          requestCounts[action] = (requestCounts[action] ?? 0) + 1;
          requests.set(action, frame.payload);
        }
      });
    });
    socket.on("framereceived", ({ payload }) => {
      forEachSessionRecoveryFrame(payload, (frame) => {
        if (frame.type === "response" || frame.type === "error") {
          responses.set(frame.id!, { type: frame.type, payload: frame.payload });
        }
      });
    });
  });
  return { requestIds, requestCounts, requests, responses };
}

export function capturedSessionRecoveryRequest(requests: Map<string, unknown>, action: string) {
  return requests.get(action);
}

export function capturedSessionRecoveryResponse(
  requestIds: Record<string, string>,
  responses: Map<string, { type?: string; payload?: unknown }>,
  action: string,
) {
  const id = requestIds[action];
  return id ? responses.get(id) : undefined;
}

export function capturedSessionRecoveryResponseType(
  requestIds: Record<string, string>,
  responses: Map<string, { type?: string; payload?: unknown }>,
  action: string,
) {
  return capturedSessionRecoveryResponse(requestIds, responses, action)?.type ?? null;
}

export function assertSuccessfulRelocationResponse(
  response: { type?: string; payload?: unknown } | undefined,
  runtimeConsumers: unknown,
) {
  if (response?.type === "error") {
    throw new Error(
      `relocate_and_resume was rejected: ${JSON.stringify(response.payload)}; ` +
        `durable consumers: ${JSON.stringify(runtimeConsumers)}`,
    );
  }
  expect(response?.type).toBe("response");
}

export async function expectMinimumElementHeight(locator: Locator, height: number) {
  expect((await locator.boundingBox())?.height).toBeGreaterThan(height);
}

export function taskEnvironmentRepository(
  environment: Awaited<ReturnType<ApiClient["getTaskEnvironment"]>>,
  repositoryId: string,
) {
  return environment?.repos?.find((repository) => repository.repository_id === repositoryId);
}

export function taskEnvironmentRepositoryWorktreePath(
  environment: Awaited<ReturnType<ApiClient["getTaskEnvironment"]>>,
  repositoryId: string,
) {
  return taskEnvironmentRepository(environment, repositoryId)?.worktree_path ?? null;
}

/** Read the durable session/runtime inventory considered by relocation admission. */
export function readManagedCloneRecoveryConsumers(
  tmpDir: string,
  environmentId: string,
): Array<{ sessionId: string; state: string; runtimeStatus?: string }> {
  const { DatabaseSync } = nodeRequire("node:sqlite") as {
    DatabaseSync: new (databasePath: string) => SqliteTestDatabase;
  };
  const db = new DatabaseSync(path.join(tmpDir, "kandev.db"));
  try {
    return db
      .prepare(
        `
        SELECT ts.id AS sessionId, ts.state, er.status AS runtimeStatus
        FROM task_sessions ts
        LEFT JOIN executors_running er ON er.session_id = ts.id
        WHERE ts.task_environment_id = ?
        ORDER BY ts.id
      `,
      )
      .all(environmentId) as Array<{ sessionId: string; state: string; runtimeStatus?: string }>;
  } finally {
    db.close();
  }
}

export function readPrivateManagedCloneRecoveryArtifacts(
  tmpDir: string,
  environmentId: string,
  repositoryId: string,
): { relocationPath: string; recoveryPath: string; original: string; snapshot: string } {
  const { DatabaseSync } = nodeRequire("node:sqlite") as {
    DatabaseSync: new (databasePath: string) => SqliteTestDatabase;
  };
  const db = new DatabaseSync(path.join(tmpDir, "kandev.db"));
  let artifactPaths: string[];
  try {
    const row = db
      .prepare(
        `SELECT artifact_paths_json AS artifactPaths
         FROM task_environment_recovery_artifacts
         WHERE task_environment_id = ? AND repository_id = ?
         ORDER BY updated_at DESC LIMIT 1`,
      )
      .get(environmentId, repositoryId) as { artifactPaths?: unknown } | undefined;
    if (typeof row?.artifactPaths !== "string") {
      throw new Error(`Recovery artifacts are not registered for repository ${repositoryId}`);
    }
    const parsed = JSON.parse(row.artifactPaths) as unknown;
    if (!Array.isArray(parsed) || parsed.some((value) => typeof value !== "string")) {
      throw new Error(`Recovery artifact paths are invalid for repository ${repositoryId}`);
    }
    artifactPaths = parsed;
  } finally {
    db.close();
  }

  const relocationPath = artifactPaths.find(
    (artifactPath) => path.basename(artifactPath) === "relocation.json",
  );
  const recoveryPath = artifactPaths.find(
    (artifactPath) => path.basename(artifactPath) === "recovery.json",
  );
  if (!relocationPath || !recoveryPath) {
    throw new Error(`Private relocation records are missing for repository ${repositoryId}`);
  }
  const relocation = JSON.parse(fs.readFileSync(relocationPath, "utf8")) as {
    original?: unknown;
  };
  const recovery = JSON.parse(fs.readFileSync(recoveryPath, "utf8")) as {
    snapshot?: unknown;
  };
  if (typeof relocation.original !== "string" || typeof recovery.snapshot !== "string") {
    throw new Error(`Private recovery record is incomplete for repository ${repositoryId}`);
  }
  return {
    relocationPath,
    recoveryPath,
    original: relocation.original,
    snapshot: recovery.snapshot,
  };
}

/** Read the durable state of the task's cascade archive cleanup job. */
export function readCascadeArchiveCleanupState(tmpDir: string, taskId: string): string | null {
  const { DatabaseSync } = nodeRequire("node:sqlite") as {
    DatabaseSync: new (databasePath: string) => SqliteTestDatabase;
  };
  const db = new DatabaseSync(path.join(tmpDir, "kandev.db"));
  try {
    const row = db
      .prepare(
        `
        SELECT state
        FROM task_resource_cleanup_jobs
        WHERE task_id = ? AND trigger = 'cascade_archive'
        ORDER BY created_at DESC
        LIMIT 1
      `,
      )
      .get(taskId) as { state?: unknown } | undefined;
    return typeof row?.state === "string" ? row.state : null;
  } finally {
    db.close();
  }
}

export async function waitForCascadeArchiveCleanup(tmpDir: string, taskId: string) {
  await expect
    .poll(() => readCascadeArchiveCleanupState(tmpDir, taskId), {
      timeout: 30_000,
      intervals: [250, 500, 1_000],
      message: `Waiting for cascade archive cleanup job row to be created for task ${taskId}`,
    })
    .not.toBeNull();
  await expect
    .poll(() => readCascadeArchiveCleanupState(tmpDir, taskId), {
      timeout: 60_000,
      intervals: [250, 500, 1_000],
      message: `Waiting for cascade archive cleanup to finish for task ${taskId}`,
    })
    .toBe("succeeded");
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
  branch_slug?: string;
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
  await waitForSessionDone(
    apiClient,
    fixture.task.id,
    sessionId,
    "Waiting for the archive recovery session to settle",
    60_000,
  );
  await waitForAgentMessage(apiClient, sessionId, "simple mock response", 60_000);
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
