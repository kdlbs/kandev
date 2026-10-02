import { randomUUID } from "node:crypto";
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../fixtures/test-base";
import type { CreateTaskResponse } from "../../lib/types/http";
import type { ApiClient } from "./api-client";
import { GitHelper, makeGitEnv } from "./git-helper";
import { SessionPage } from "../pages/session-page";
import { waitForSessionState } from "./session";

type SqliteTestDatabase = {
  prepare(sql: string): {
    get(...parameters: unknown[]): unknown;
    run(...parameters: unknown[]): unknown;
  };
  close(): void;
};

const nodeRequire = createRequire(path.join(process.cwd(), "package.json"));

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
  slots: [MultiRepoRelocationSlot, MultiRepoRelocationSlot];
};

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
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 60_000 });
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

  return { task, session, environment, slots };
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
  try {
    const row = db.prepare("SELECT metadata FROM task_sessions WHERE id = ?").get(sessionId) as
      | { metadata?: string | null }
      | undefined;
    if (!row) throw new Error(`legacy recovery session ${sessionId} does not exist`);
    const metadata = row.metadata ? (JSON.parse(row.metadata) as Record<string, unknown>) : {};
    metadata.last_agent_error = {
      message: "The previous agent launch failed.",
      occurred_at: new Date().toISOString(),
      scope: "session",
      phase: "bootstrap",
      stamp: `legacy-${randomUUID()}`,
    };
    db.prepare(
      "UPDATE task_sessions SET state = ?, error_message = ?, metadata = ?, updated_at = ? WHERE id = ?",
    ).run(
      "FAILED",
      "The previous agent launch failed.",
      JSON.stringify(metadata),
      new Date().toISOString(),
      sessionId,
    );
  } finally {
    db.close();
  }
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
  seedLegacyGenericSessionError(tmpDir, sessionId);
}

export async function cleanupMultiRepoManagedCloneRelocationFixture(
  apiClient: ApiClient,
  seedData: SeedData,
  fixture: MultiRepoRelocationFixture,
) {
  await apiClient.e2eReset(seedData.workspaceId, [seedData.workflowId]);
  await apiClient.mockGitHubReset();
  await apiClient.deleteRepository(fixture.slots[1].repositoryId);
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

export function assertSnapshotContent(slot: MultiRepoRelocationSlot) {
  const record = JSON.parse(
    fs.readFileSync(`${slot.originalPath}.kandev-clone-relocation.json`, "utf8"),
  ) as {
    original: string;
  };
  expect(record.original).toContain(`${path.sep}.kandev-recovery${path.sep}`);
  expect(fs.readFileSync(path.join(record.original, slot.dirtyFileName), "utf8")).toBe(
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
