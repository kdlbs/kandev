import fs from "node:fs";
import path from "node:path";
import { randomUUID } from "node:crypto";
import { expect, test } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { DatabaseSync } from "../../helpers/node-sqlite";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { waitForSessionState } from "../../helpers/session";
import { seedWorktreeRecoveryFixture } from "../../helpers/session-resume-recovery";
import type { Page } from "@playwright/test";
import type { SessionPage } from "../../pages/session-page";
import type { routeGitStatusRefresh } from "./git-status-refresh-helpers";

type RawWorkspaceBinding = {
  task_environment_id: string;
  workspace_path: string;
  state: string;
};

export type ResumeWorkspaceBindingFixture = {
  databasePath: string;
  taskId: string;
  sessionId: string;
  environmentId: string;
  repositoryId: string;
  worktreeId: string;
  worktreePath: string;
  workspacePath: string;
  branch: string;
  head: string;
  dirtyFileName: string;
  dirtyFilePath: string;
  dirtyFileContent: string;
};

type WorkspaceBindingReloadOptions = {
  bridge: GitRefreshBridge;
  touch: boolean;
};

type GitRefreshBridge = Awaited<ReturnType<typeof routeGitStatusRefresh>>;

export function readRawWorkspaceBinding(
  databasePath: string,
  sessionId: string,
): RawWorkspaceBinding {
  const database = new DatabaseSync(databasePath);
  try {
    database.exec("PRAGMA busy_timeout = 10000");
    const row = database
      .prepare("SELECT task_environment_id, workspace_path, state FROM task_sessions WHERE id = ?")
      .get(sessionId) as RawWorkspaceBinding | undefined;
    if (!row) throw new Error(`No raw task session row exists for ${sessionId}`);
    return row;
  } finally {
    database.close();
  }
}

export async function seedResumeWorkspaceBindingFixture(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  backend: { tmpDir: string },
  title: string,
): Promise<ResumeWorkspaceBindingFixture> {
  const fixture = await seedWorktreeRecoveryFixture(page, apiClient, seedData, title);
  let dirtyFilePath: string | undefined;
  try {
    const sessionId = fixture.task.session_id;
    if (!sessionId) throw new Error("resume binding fixture has no session ID");
    const environment = await apiClient.getTaskEnvironment(fixture.task.id);
    const { workspacePath, worktreePath, worktreeId, branch } = resolveResumeWorkspaceIdentity(
      fixture,
      environment,
    );

    const databasePath = path.join(backend.tmpDir, "kandev.db");
    const initialBinding = readRawWorkspaceBinding(databasePath, sessionId);
    if (
      initialBinding.task_environment_id !== environment.id ||
      initialBinding.workspace_path !== workspacePath
    ) {
      throw new Error("initial launch did not persist the canonical session workspace binding");
    }
    const dirtyFileName = `resume-binding-${randomUUID().slice(0, 8)}.txt`;
    dirtyFilePath = path.join(worktreePath, dirtyFileName);
    const dirtyFileContent = `preserved resume workspace content ${randomUUID()}`;

    await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: true });
    fs.writeFileSync(dirtyFilePath, `${dirtyFileContent}\n`);
    const git = new GitHelper(worktreePath, makeGitEnv(backend.tmpDir));
    const head = git.getCurrentSha();

    const stop = await apiClient.stopSession({
      session_id: sessionId,
      reason: "prepare missing raw workspace binding fixture",
      force: true,
    });
    if (!stop.success) throw new Error("failed to stop the resume binding fixture session");
    await waitForSessionState(apiClient, {
      taskId: fixture.task.id,
      sessionId,
      expectedState: "CANCELLED",
      message: "Waiting for the disposable resume binding session to stop",
      timeout: 30_000,
    });

    const database = new DatabaseSync(databasePath);
    try {
      database.exec("PRAGMA busy_timeout = 10000");
      const result = database
        .prepare(
          `UPDATE task_sessions
           SET state = 'FAILED', completed_at = NULL, error_message = ?, workspace_path = ''
           WHERE id = ? AND task_id = ? AND task_environment_id = ? AND state = 'CANCELLED'`,
        )
        .run(
          "Simulated launch failure before session workspace binding",
          sessionId,
          fixture.task.id,
          environment.id,
        );
      if (Number(result.changes) !== 1) {
        throw new Error("could not seed the failed session with its missing raw workspace binding");
      }
    } finally {
      database.close();
    }

    const failedBinding = readRawWorkspaceBinding(databasePath, sessionId);
    if (
      failedBinding.state !== "FAILED" ||
      failedBinding.task_environment_id !== environment.id ||
      failedBinding.workspace_path !== ""
    ) {
      throw new Error("failed-session fixture did not preserve the expected raw workspace state");
    }

    return {
      databasePath,
      taskId: fixture.task.id,
      sessionId,
      environmentId: environment.id,
      repositoryId: seedData.repositoryId,
      worktreeId,
      worktreePath,
      workspacePath,
      branch,
      head,
      dirtyFileName,
      dirtyFilePath,
      dirtyFileContent,
    };
  } catch (error) {
    await cleanupFailedResumeWorkspaceBindingSeed(apiClient, seedData, dirtyFilePath);
    throw error;
  }
}

async function cleanupFailedResumeWorkspaceBindingSeed(
  apiClient: ApiClient,
  seedData: SeedData,
  dirtyFilePath: string | undefined,
): Promise<void> {
  const cleanupErrors: unknown[] = [];
  if (dirtyFilePath) {
    try {
      fs.rmSync(dirtyFilePath, { force: true });
    } catch (error) {
      cleanupErrors.push(error);
    }
  }
  try {
    await apiClient.e2eReset(seedData.workspaceId, [seedData.workflowId]);
  } catch (error) {
    cleanupErrors.push(error);
  }
  try {
    await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: false });
  } catch (error) {
    cleanupErrors.push(error);
  }
  if (cleanupErrors.length > 0) {
    console.warn("Failed to fully clean up the resume workspace binding fixture", cleanupErrors);
  }
}

function resolveResumeWorkspaceIdentity(
  fixture: Awaited<ReturnType<typeof seedWorktreeRecoveryFixture>>,
  environment: Awaited<ReturnType<ApiClient["getTaskEnvironment"]>>,
) {
  if (!environment || environment.id !== fixture.environment.id) {
    throw new Error("resume binding fixture lost its original task environment");
  }
  const workspacePath = environment.workspace_path || fixture.repository.worktree_path;
  const worktreePath = fixture.repository.worktree_path;
  const worktreeId = fixture.repository.worktree_id;
  const branch = fixture.repository.worktree_branch;
  if (!workspacePath || !worktreePath || !worktreeId || !branch) {
    throw new Error("resume binding fixture has incomplete worktree identity");
  }
  return { workspacePath, worktreePath, worktreeId, branch };
}

export async function cleanupResumeWorkspaceBindingFixture(
  apiClient: ApiClient,
  seedData: SeedData,
  fixture: ResumeWorkspaceBindingFixture,
): Promise<void> {
  fs.rmSync(fixture.dirtyFilePath, { force: true });
  try {
    await apiClient.e2eReset(seedData.workspaceId, [seedData.workflowId]);
  } finally {
    await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: false });
  }
}

export async function resumeFailedSessionAndReload(
  page: Page,
  session: SessionPage,
  apiClient: ApiClient,
  fixture: ResumeWorkspaceBindingFixture,
  options: WorkspaceBindingReloadOptions,
): Promise<number> {
  await test.step("reload the failed session and resume from the UI", async () => {
    await waitForSessionState(apiClient, {
      taskId: fixture.taskId,
      sessionId: fixture.sessionId,
      expectedState: "FAILED",
      message: "Waiting for the authoritative failed session state before reload",
      timeout: 30_000,
    });
    await page.reload();
    await session.waitForLoad();
    const resume = session.recoveryResumeButton();
    await expect(resume).toBeVisible();
    await expect(resume).toBeEnabled();
    if (options.touch) {
      await resume.tap();
    } else {
      await resume.click();
    }
  });

  await test.step("wait for the resume and raw binding to persist", async () => {
    await waitForSessionState(apiClient, {
      taskId: fixture.taskId,
      sessionId: fixture.sessionId,
      expectedState: "WAITING_FOR_INPUT",
      message: "Waiting for the manually resumed workspace session to become idle",
      timeout: 90_000,
    });
    await expect
      .poll(() => readRawWorkspaceBinding(fixture.databasePath, fixture.sessionId), {
        message: "Waiting for the resume binding to persist before reload",
      })
      .toEqual({
        task_environment_id: fixture.environmentId,
        workspace_path: fixture.workspacePath,
        state: "WAITING_FOR_INPUT",
      });
  });

  const responsesBeforeReload = options.bridge.responseCount("fresh");
  await test.step("reload the recovered session", async () => {
    await page.reload();
    await session.waitForLoad();
  });
  return responsesBeforeReload;
}
