import { randomUUID } from "node:crypto";
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { expect, type ConsoleMessage, type Page } from "@playwright/test";
import type { SeedData } from "../fixtures/test-base";
import type { CreateTaskResponse } from "../../lib/types/http";
import type { Repository } from "../../lib/types/http";
import type { ApiClient } from "./api-client";
import { GitHelper, makeGitEnv } from "./git-helper";
import { SessionPage } from "../pages/session-page";
import { readManagedCloneRecoveryConsumers } from "./session-resume-recovery";
import { waitForSessionState } from "./session";

type SqliteTestDatabase = {
  prepare(sql: string): {
    get(...parameters: unknown[]): unknown;
    run(...parameters: unknown[]): { changes?: number | bigint };
  };
  exec(sql: string): void;
  close(): void;
};

const nodeRequire = createRequire(path.join(process.cwd(), "package.json"));
const ACTIVE_EXECUTOR_STATUSES = new Set(["starting", "prepared", "ready", "running"]);

async function openRecoverySession(page: Page, taskId: string): Promise<SessionPage> {
  const browserErrors: string[] = [];
  const captureConsoleError = (message: ConsoleMessage) => {
    if (message.type() === "error") browserErrors.push(message.text());
  };
  const capturePageError = (error: Error) => browserErrors.push(error.message);
  page.on("console", captureConsoleError);
  page.on("pageerror", capturePageError);
  try {
    await page.goto(`/t/${taskId}`);
    const session = new SessionPage(page);
    await session.waitForLoad();
    return session;
  } catch (cause) {
    const details = browserErrors.length ? ` Browser errors: ${browserErrors.join(" | ")}` : "";
    throw new Error(`Failed to load managed-clone recovery task.${details}`, { cause });
  } finally {
    page.off("console", captureConsoleError);
    page.off("pageerror", capturePageError);
  }
}

export type MultiRepoRelocationSlot = {
  repositoryId: string;
  sourceClonePath: string;
  destinationClonePath: string;
  originalPath: string;
  originalWorktreeId: string;
  originalBranch: string;
  originalHead: string;
  dirtyFileName: string;
  dirtyFileContent: string;
  ignored: boolean;
};

export type MultiRepoRelocationFixture = {
  task: CreateTaskResponse;
  session: SessionPage;
  environment: Awaited<ReturnType<ApiClient["getTaskEnvironment"]>> & {};
  originalSeedRepository: Repository;
  slots: [MultiRepoRelocationSlot, MultiRepoRelocationSlot];
};

export type ManagedCloneRecoveryGitOperationGate = {
  configPath: string;
  directory: string;
  startedFile: string;
  releaseFile: string;
};

/** Read the hydrated session projection exposed by the E2E store bridge. */
export async function readWorkspaceRecoveryProjectionFromStore(page: Page, sessionId: string) {
  return page.evaluate((id) => {
    type RecoveryProjection = {
      state: string;
      phase: string;
      workspace_complete: boolean;
      agent_ready: boolean;
      runner_live: boolean;
    } | null;
    type RecoverySession = {
      id: string;
      task_id: string;
      task_environment_id?: string;
      workspace_recovery?: RecoveryProjection;
    };
    type RecoveryStoreWindow = Window & {
      __KANDEV_E2E_STORE__?: {
        getState(): { taskSessions: { items: Record<string, RecoverySession> } };
      };
    };
    const session = (window as RecoveryStoreWindow).__KANDEV_E2E_STORE__?.getState().taskSessions
      .items[id];
    return session ?? null;
  }, sessionId);
}

/** Holds original-checkout retention until the test releases real recovery. */
export function installManagedCloneRecoveryGitOperationGate(
  tmpDir: string,
): ManagedCloneRecoveryGitOperationGate {
  const directory = path.join(tmpDir, `managed-clone-recovery-gate-${randomUUID()}`);
  fs.mkdirSync(directory, { recursive: true });
  const gate = {
    configPath: path.join(tmpDir, "git-delay-ms"),
    directory,
    startedFile: path.join(directory, "started"),
    releaseFile: path.join(directory, "release"),
  };
  fs.writeFileSync(
    gate.configPath,
    JSON.stringify({
      subcommand: "worktree",
      requiredArgs: ["move"],
      startedFile: gate.startedFile,
      releaseFile: gate.releaseFile,
    }),
  );
  return gate;
}

export function releaseManagedCloneRecoveryGitOperationGate(
  gate: ManagedCloneRecoveryGitOperationGate,
) {
  fs.writeFileSync(gate.releaseFile, "released");
  try {
    const config = JSON.parse(fs.readFileSync(gate.configPath, "utf8")) as {
      startedFile?: string;
    };
    if (config.startedFile === gate.startedFile) fs.rmSync(gate.configPath, { force: true });
  } catch {
    // The test may release the gate after the runner already passed it.
  }
}

export function cleanupManagedCloneRecoveryGitOperationGate(
  gate: ManagedCloneRecoveryGitOperationGate,
) {
  releaseManagedCloneRecoveryGitOperationGate(gate);
  fs.rmSync(gate.directory, { recursive: true, force: true });
}

export function readWorkspaceRecoveryOperation(
  tmpDir: string,
  environmentId: string,
): {
  state: string;
  phase: string;
  repository_position: number;
  repository_total: number;
  completed_slots: number;
  workspace_complete: boolean;
  agent_ready: boolean;
  reason_code: string;
} | null {
  const { DatabaseSync } = nodeRequire("node:sqlite") as {
    DatabaseSync: new (databasePath: string) => SqliteTestDatabase;
  };
  const db = new DatabaseSync(path.join(tmpDir, "kandev.db"));
  try {
    const row = db
      .prepare(
        `SELECT state, phase, repository_position, repository_total, completed_slots,
                workspace_complete, agent_ready, reason_code
         FROM task_environment_recovery_operations WHERE task_environment_id = ?`,
      )
      .get(environmentId) as
      | {
          state: string;
          phase: string;
          repository_position: number;
          repository_total: number;
          completed_slots: number;
          workspace_complete: number;
          agent_ready: number;
          reason_code: string;
        }
      | undefined;
    if (!row) return null;
    return {
      ...row,
      workspace_complete: Boolean(row.workspace_complete),
      agent_ready: Boolean(row.agent_ready),
    };
  } finally {
    db.close();
  }
}

export async function waitForMultiRepoRecoveryReady(
  apiClient: ApiClient,
  tmpDir: string,
  taskId: string,
  sessionId: string,
  environmentId: string,
) {
  try {
    await waitForSessionState(apiClient, {
      taskId,
      sessionId,
      expectedState: "WAITING_FOR_INPUT",
      message: "Waiting for the recovered multi-repository session to become ready",
      timeout: 120_000,
    });
  } catch (cause) {
    const { sessions } = await apiClient.listTaskSessions(taskId);
    const session = sessions.find((candidate) => candidate.id === sessionId);
    const recovery = readWorkspaceRecoveryOperation(tmpDir, environmentId);
    const consumers = readManagedCloneRecoveryConsumers(tmpDir, environmentId);
    throw new Error(
      `Recovered session did not become ready: session=${JSON.stringify({
        state: session?.state,
        error_message: session?.error_message,
      })}; recovery=${JSON.stringify(recovery)}; consumers=${JSON.stringify(consumers)}`,
      { cause },
    );
  }
  await expect
    .poll(() => readWorkspaceRecoveryOperation(tmpDir, environmentId))
    .toMatchObject({ state: "completed", workspace_complete: true, agent_ready: true });
}

/** Seed two linked worktrees on legacy clones with tracked and ignored local changes. */
export async function seedMultiRepoManagedCloneRelocationFixture(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  backend: { tmpDir: string },
  title: string,
): Promise<MultiRepoRelocationFixture> {
  await apiClient.mockGitHubReset();
  const suffix = randomUUID().replaceAll("-", "").slice(0, 12);
  const repoNames = [`relocation-${suffix}`, `relocation-extra-${suffix}`] as const;
  const cloneRoot = path.join(backend.tmpDir, "managed-repos");
  const gitEnv = makeGitEnv(backend.tmpDir);
  const repoIds = [seedData.repositoryId, ""] as [string, string];
  const sourceClonePaths = repoNames.map((name) =>
    path.join(cloneRoot, "e2e", name),
  ) as unknown as [string, string];
  const destinationClonePaths = repoNames.map((name) =>
    path.join(cloneRoot, "workspaces", seedData.workspaceId, "github", "e2e", name),
  ) as unknown as [string, string];
  const remoteUrls = repoNames.map((name) => `https://github.com/e2e/${name}.git`) as unknown as [
    string,
    string,
  ];

  const extraSeedPath = path.join(backend.tmpDir, "seed-repositories", repoNames[1]);
  createIgnoredFixtureRepository(extraSeedPath, `relocation-ignored-${suffix}.txt`, gitEnv);

  for (let index = 0; index < repoNames.length; index += 1) {
    const source = index === 0 ? seedData.repositoryPath : extraSeedPath;
    fs.mkdirSync(path.dirname(sourceClonePaths[index]), { recursive: true });
    execFileSync("git", ["clone", "--local", source, sourceClonePaths[index]], {
      env: gitEnv,
      stdio: "pipe",
    });
    execFileSync(
      "git",
      ["-C", sourceClonePaths[index], "remote", "set-url", "origin", remoteUrls[index]],
      {
        env: gitEnv,
        stdio: "pipe",
      },
    );
  }

  const originalSeedRepository = await apiClient.getRepository(seedData.repositoryId);
  let extraRepositoryId = "";
  try {
    await apiClient.updateRepository(seedData.repositoryId, {
      source_type: "local",
      local_path: sourceClonePaths[0],
      remote_url: remoteUrls[0],
      default_branch: "main",
      pull_before_worktree: false,
    });
    const extraRepository = await apiClient.createRepository(
      seedData.workspaceId,
      extraSeedPath,
      "main",
      {
        name: repoNames[1],
      },
    );
    extraRepositoryId = extraRepository.id;
    repoIds[1] = extraRepository.id;
    await apiClient.updateRepository(extraRepository.id, {
      source_type: "local",
      local_path: sourceClonePaths[1],
      remote_url: remoteUrls[1],
      default_branch: "main",
      pull_before_worktree: false,
    });

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      title,
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: repoIds,
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    if (!task.session_id) throw new Error("multi-repository relocation task has no session_id");
    const session = await openRecoverySession(page, task.id);
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId: task.session_id,
      expectedState: "WAITING_FOR_INPUT",
      message: "Waiting for the initial multi-repository session turn to finish",
      timeout: 120_000,
    });
    const environment = await apiClient.getTaskEnvironment(task.id);
    if (!environment || environment.repos?.length !== 2) {
      throw new Error("multi-repository relocation task did not create two selected worktrees");
    }

    for (let index = 0; index < repoIds.length; index += 1) {
      fs.mkdirSync(path.dirname(destinationClonePaths[index]), { recursive: true });
      execFileSync(
        "git",
        ["clone", "--local", sourceClonePaths[index], destinationClonePaths[index]],
        {
          env: gitEnv,
          stdio: "pipe",
        },
      );
      execFileSync(
        "git",
        ["-C", destinationClonePaths[index], "remote", "set-url", "origin", remoteUrls[index]],
        {
          env: gitEnv,
          stdio: "pipe",
        },
      );
      await apiClient.updateRepository(repoIds[index], {
        source_type: "provider",
        local_path: destinationClonePaths[index],
        provider: "github",
        provider_repo_id: `e2e-${repoNames[index]}`,
        provider_host: "https://github.com",
        provider_owner: "e2e",
        provider_name: repoNames[index],
        remote_url: remoteUrls[index],
      });
    }
    await apiClient.mockGitHubSetUser("relocation-e2e");
    await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
      source: "legacy_shared",
      status: "active",
    });

    const slots = repoIds.map((repositoryId, index) => {
      const repository = environment.repos?.find(
        (candidate) => candidate.repository_id === repositoryId,
      );
      if (!repository?.worktree_path || !repository.worktree_id) {
        throw new Error(`selected repository ${repositoryId} has no persisted worktree`);
      }
      const originalPath = repository.worktree_path;
      const ignored = index === 1;
      let dirtyFileName = ignored ? `relocation-ignored-${suffix}.txt` : "README.md";
      let dirtyFileContent: string;
      if (ignored) {
        dirtyFileContent = `ignored local content ${suffix}`;
        fs.writeFileSync(path.join(originalPath, dirtyFileName), dirtyFileContent, { mode: 0o644 });
        execFileSync("git", ["-C", originalPath, "check-ignore", "--quiet", dirtyFileName], {
          env: gitEnv,
          stdio: "pipe",
        });
      } else {
        const trackedFiles = execFileSync("git", ["-C", originalPath, "ls-files", "-z"], {
          env: gitEnv,
          stdio: "pipe",
        })
          .toString("utf8")
          .split("\0")
          .filter((file) => file && fs.existsSync(path.join(originalPath, file)));
        dirtyFileName = trackedFiles[0] ?? "";
        if (!dirtyFileName) throw new Error(`worktree ${originalPath} has no tracked file to edit`);
        const previous = fs.readFileSync(path.join(originalPath, dirtyFileName), "utf8");
        dirtyFileContent = `${previous}\ntracked local content ${suffix}\n`;
        fs.writeFileSync(path.join(originalPath, dirtyFileName), dirtyFileContent, { mode: 0o644 });
      }
      const git = new GitHelper(originalPath, gitEnv);
      return {
        repositoryId,
        sourceClonePath: sourceClonePaths[index],
        destinationClonePath: destinationClonePaths[index],
        originalPath,
        originalWorktreeId: repository.worktree_id,
        originalBranch: repository.worktree_branch ?? "",
        originalHead: git.getCurrentSha(),
        dirtyFileName,
        dirtyFileContent,
        ignored,
      };
    }) as [MultiRepoRelocationSlot, MultiRepoRelocationSlot];

    return { task, session, environment, originalSeedRepository, slots };
  } catch (setupError) {
    const cleanup = [
      () => apiClient.e2eReset(seedData.workspaceId, [seedData.workflowId]),
      () => apiClient.mockGitHubReset(),
      ...(extraRepositoryId ? [() => apiClient.deleteRepository(extraRepositoryId)] : []),
      () => restoreSeedRepository(apiClient, seedData.repositoryId, originalSeedRepository),
    ];
    try {
      await attemptEveryCleanup(cleanup);
    } catch {
      // Keep the setup failure as the cause; cleanup still attempts every restoration step.
    }
    throw setupError;
  }
}

async function restoreSeedRepository(
  apiClient: ApiClient,
  repositoryId: string,
  repository: Repository,
) {
  await apiClient.updateRepository(repositoryId, {
    source_type: repository.source_type,
    local_path: repository.local_path,
    provider: repository.provider,
    provider_repo_id: repository.provider_repo_id,
    provider_host: repository.provider_host ?? "",
    provider_scope: repository.provider_scope ?? "",
    provider_owner: repository.provider_owner,
    provider_name: repository.provider_name,
    remote_url: repository.remote_url ?? "",
    default_branch: repository.default_branch,
    pull_before_worktree: repository.pull_before_worktree,
    setup_script: repository.setup_script,
    cleanup_script: repository.cleanup_script,
    dev_script: repository.dev_script,
    copy_files: repository.copy_files,
    secret_bindings: repository.secret_bindings,
  });
}

async function attemptEveryCleanup(steps: Array<() => Promise<void>>) {
  let firstError: unknown;
  for (const step of steps) {
    try {
      await step();
    } catch (error) {
      firstError ??= error;
    }
  }
  if (firstError !== undefined) throw firstError;
}

function createIgnoredFixtureRepository(
  repositoryPath: string,
  ignoredFileName: string,
  gitEnv: NodeJS.ProcessEnv,
) {
  fs.mkdirSync(repositoryPath, { recursive: true });
  execFileSync("git", ["init", "-b", "main"], { cwd: repositoryPath, env: gitEnv, stdio: "pipe" });
  execFileSync("git", ["-C", repositoryPath, "config", "user.email", "e2e@test.local"], {
    env: gitEnv,
    stdio: "pipe",
  });
  execFileSync("git", ["-C", repositoryPath, "config", "user.name", "E2E Test"], {
    env: gitEnv,
    stdio: "pipe",
  });
  fs.writeFileSync(path.join(repositoryPath, "README.md"), "extra repository baseline\n");
  fs.writeFileSync(path.join(repositoryPath, ".gitignore"), `${ignoredFileName}\n`);
  execFileSync("git", ["-C", repositoryPath, "add", "README.md", ".gitignore"], {
    env: gitEnv,
    stdio: "pipe",
  });
  execFileSync("git", ["-C", repositoryPath, "commit", "-m", "initial"], {
    env: gitEnv,
    stdio: "pipe",
  });
}

/** Seed a legacy generic failure without the new relocation category. */
export function seedLegacyGenericSessionError(tmpDir: string, sessionId: string) {
  const { DatabaseSync } = nodeRequire("node:sqlite") as {
    DatabaseSync: new (databasePath: string) => SqliteTestDatabase;
  };
  const db = new DatabaseSync(path.join(tmpDir, "kandev.db"));
  db.exec("PRAGMA busy_timeout = 5000; BEGIN IMMEDIATE");
  let committed = false;
  try {
    const row = db
      .prepare("SELECT metadata, state FROM task_sessions WHERE id = ?")
      .get(sessionId) as { metadata?: string | null; state?: string } | undefined;
    if (!row) throw new Error(`legacy recovery session ${sessionId} does not exist`);
    if (row.state !== "CANCELLED") {
      throw new Error(
        `legacy recovery session ${sessionId} is not stopped (${row.state ?? "unknown"})`,
      );
    }
    const execution = db
      .prepare(
        "SELECT status, COALESCE(local_pid, 0) AS local_pid FROM executors_running WHERE session_id = ? LIMIT 1",
      )
      .get(sessionId) as { status?: string; local_pid?: number } | undefined;
    if (execution && hasLiveExecutor(execution)) {
      throw new Error(`legacy recovery session ${sessionId} still has a live execution`);
    }
    const metadata = row.metadata ? (JSON.parse(row.metadata) as Record<string, unknown>) : {};
    metadata.last_agent_error = {
      message: "The previous agent launch failed.",
      occurred_at: new Date().toISOString(),
      scope: "session",
      phase: "bootstrap",
      stamp: `legacy-${randomUUID()}`,
    };
    const updated = db
      .prepare(
        `UPDATE task_sessions
       SET state = ?, error_message = ?, metadata = ?, updated_at = ?
       WHERE id = ? AND state = 'CANCELLED'
         AND NOT EXISTS (
           SELECT 1 FROM executors_running er
           WHERE er.session_id = task_sessions.id
             AND (er.status IN ('starting', 'prepared', 'ready', 'running')
               OR COALESCE(er.local_pid, 0) > 0)
         )`,
      )
      .run(
        "FAILED",
        "The previous agent launch failed.",
        JSON.stringify(metadata),
        new Date().toISOString(),
        sessionId,
      );
    if (updated.changes !== 1 && updated.changes !== 1n) {
      throw new Error(
        `legacy recovery session ${sessionId} changed before its error could be seeded`,
      );
    }
    db.exec("COMMIT");
    committed = true;
  } finally {
    if (!committed) db.exec("ROLLBACK");
    db.close();
  }
}

function hasLiveExecution(tmpDir: string, sessionId: string) {
  const { DatabaseSync } = nodeRequire("node:sqlite") as {
    DatabaseSync: new (databasePath: string) => SqliteTestDatabase;
  };
  const db = new DatabaseSync(path.join(tmpDir, "kandev.db"));
  try {
    db.exec("PRAGMA busy_timeout = 5000");
    const execution = db
      .prepare(
        "SELECT status, COALESCE(local_pid, 0) AS local_pid FROM executors_running WHERE session_id = ? LIMIT 1",
      )
      .get(sessionId) as { status?: string; local_pid?: number } | undefined;
    return execution ? hasLiveExecutor(execution) : false;
  } finally {
    db.close();
  }
}

function hasLiveExecutor(execution: { status?: string; local_pid?: number }) {
  return ACTIVE_EXECUTOR_STATUSES.has(execution.status ?? "") || (execution.local_pid ?? 0) > 0;
}

export function readSessionErrorStamp(tmpDir: string, sessionId: string): string | null {
  const { DatabaseSync } = nodeRequire("node:sqlite") as {
    DatabaseSync: new (databasePath: string) => SqliteTestDatabase;
  };
  const db = new DatabaseSync(path.join(tmpDir, "kandev.db"));
  try {
    const row = db.prepare("SELECT metadata FROM task_sessions WHERE id = ?").get(sessionId) as
      | { metadata?: string | null }
      | undefined;
    if (!row?.metadata) return null;
    const metadata = JSON.parse(row.metadata) as {
      last_agent_error?: { stamp?: unknown };
    };
    const stamp = metadata.last_agent_error?.stamp;
    return typeof stamp === "string" ? stamp : null;
  } finally {
    db.close();
  }
}

/** Capture session launch requests and responses, grouped by launch intent. */
export function captureSessionLaunchMessages(page: Page) {
  const requestIds: Record<string, string> = {};
  const requestIdsByIntent: Record<string, string[]> = {};
  const requestCounts: Record<string, number> = {};
  const responses = new Map<string, { type?: string; payload?: unknown }>();
  page.on("websocket", (socket) => {
    if (!socket.url().endsWith("/ws")) return;
    socket.on("framesent", ({ payload }) => {
      if (typeof payload !== "string") return;
      for (const part of payload.split("\n").filter(Boolean)) {
        let frame: { id?: string; action?: string; type?: string; payload?: unknown };
        try {
          frame = JSON.parse(part) as typeof frame;
        } catch {
          continue;
        }
        if (frame.action !== "session.launch" || frame.type !== "request" || !frame.id) continue;
        const intent = (frame.payload as { intent?: unknown } | null)?.intent;
        const key = typeof intent === "string" ? intent : "unknown";
        requestIds[key] = frame.id;
        requestCounts[key] = (requestCounts[key] ?? 0) + 1;
        requestIdsByIntent[key] ??= [];
        requestIdsByIntent[key].push(frame.id);
      }
    });
    socket.on("framereceived", ({ payload }) => {
      if (typeof payload !== "string") return;
      for (const part of payload.split("\n").filter(Boolean)) {
        let frame: { id?: string; action?: string; type?: string; payload?: unknown };
        try {
          frame = JSON.parse(part) as typeof frame;
        } catch {
          continue;
        }
        if (
          frame.action === "session.launch" &&
          frame.id &&
          (frame.type === "response" || frame.type === "error")
        ) {
          responses.set(frame.id, { type: frame.type, payload: frame.payload });
        }
      }
    });
  });
  return { requestIds, requestIdsByIntent, requestCounts, responses };
}

export function capturedSessionLaunchResponse(
  capture: ReturnType<typeof captureSessionLaunchMessages>,
  intent: string,
) {
  const id = capture.requestIds[intent];
  return id ? capture.responses.get(id) : undefined;
}

export function capturedSessionLaunchResponses(
  capture: ReturnType<typeof captureSessionLaunchMessages>,
  intent: string,
  fromRequestIndex = 0,
) {
  return (capture.requestIdsByIntent[intent] ?? []).slice(fromRequestIndex).flatMap((id) => {
    const response = capture.responses.get(id);
    return response ? [response] : [];
  });
}

export async function stopAndSeedLegacySessionFailure(
  apiClient: ApiClient,
  tmpDir: string,
  fixture: MultiRepoRelocationFixture,
  reason: string,
) {
  const sessionId = fixture.task.session_id!;
  const response = await apiClient.stopSession({ session_id: sessionId, reason, force: true });
  expect(response.success).toBe(true);
  await waitForSessionState(apiClient, {
    taskId: fixture.task.id,
    sessionId,
    expectedState: "CANCELLED",
    message: "Waiting for the multi-repository recovery session to stop",
    timeout: 30_000,
  });
  await expect
    .poll(() => hasLiveExecution(tmpDir, sessionId), {
      timeout: 30_000,
      message: "Waiting for the stopped multi-repository recovery execution to be removed",
    })
    .toBe(false);
  seedLegacyGenericSessionError(tmpDir, sessionId);
}

export async function cleanupMultiRepoManagedCloneRelocationFixture(
  apiClient: ApiClient,
  seedData: SeedData,
  fixture: MultiRepoRelocationFixture,
) {
  await attemptEveryCleanup([
    () => apiClient.e2eReset(seedData.workspaceId, [seedData.workflowId]),
    () => apiClient.mockGitHubReset(),
    () => apiClient.deleteRepository(fixture.slots[1].repositoryId),
    () => restoreSeedRepository(apiClient, seedData.repositoryId, fixture.originalSeedRepository),
  ]);
}

export function assertPrivateRecoveryArtifactContent(
  slot: MultiRepoRelocationSlot,
  tmpDir: string,
  environmentId: string,
) {
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
      .get(environmentId, slot.repositoryId) as { artifactPaths?: unknown } | undefined;
    if (typeof row?.artifactPaths !== "string") {
      throw new Error(`Recovery artifacts are not registered for repository ${slot.repositoryId}`);
    }
    const parsed = JSON.parse(row.artifactPaths) as unknown;
    if (!Array.isArray(parsed) || parsed.some((value) => typeof value !== "string")) {
      throw new Error(`Recovery artifact paths are invalid for repository ${slot.repositoryId}`);
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
    throw new Error(`Private relocation records are missing for repository ${slot.repositoryId}`);
  }
  expect(relocationPath).toContain(`${path.sep}.kandev-recovery${path.sep}`);
  const record = JSON.parse(fs.readFileSync(relocationPath, "utf8")) as { original?: unknown };
  const recovery = JSON.parse(fs.readFileSync(recoveryPath, "utf8")) as { snapshot?: unknown };
  if (typeof record.original !== "string" || typeof recovery.snapshot !== "string") {
    throw new Error(`Private recovery record is incomplete for repository ${slot.repositoryId}`);
  }
  expect(record.original).toContain(`${path.sep}.kandev-recovery${path.sep}`);
  expect(recovery.snapshot).toContain(`${path.sep}.kandev-recovery${path.sep}`);
  expect(fs.existsSync(`${slot.originalPath}.kandev-clone-relocation.json`)).toBe(false);
  expect(fs.readFileSync(path.join(record.original, slot.dirtyFileName), "utf8")).toBe(
    slot.dirtyFileContent,
  );
  expect(fs.readFileSync(path.join(recovery.snapshot, slot.dirtyFileName), "utf8")).toBe(
    slot.dirtyFileContent,
  );
  return record.original;
}

export function assertRelocatedSlot(
  slot: MultiRepoRelocationSlot,
  worktreePath: string,
  backendTmpDir: string,
) {
  expect(fs.existsSync(worktreePath)).toBe(true);
  const git = new GitHelper(worktreePath, makeGitEnv(backendTmpDir));
  expect(git.getCurrentSha()).toBe(slot.originalHead);
  expect(git.exec("git rev-parse --git-common-dir").trim()).toBe(
    path.join(slot.destinationClonePath, ".git"),
  );
  expect(fs.readFileSync(path.join(worktreePath, slot.dirtyFileName), "utf8")).toBe(
    slot.dirtyFileContent,
  );
}
