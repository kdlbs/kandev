import { expect, type Page, type TestInfo } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import type { BackendContext } from "../../fixtures/backend";
import type { SeedData } from "../../fixtures/test-base";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";
import type { ApiClient } from "../../helpers/api-client";
import {
  managedRuntimeStartupAttemptFile,
  prepareManagedRuntimeProfile,
  restoreE2EAgentRegistry,
} from "../../helpers/managed-runtime-recovery";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import { RETAINED_WORKSPACE_FILE } from "./completed-workspace-restoration-helpers";
import {
  recordValue,
  routeWorkspaceStreamPromotion,
  statusFiles,
  statusFromPayload,
  type PromotionTrace,
  type WorkspaceStatusEvent,
} from "./workspace-stream-promotion-trace";

type GitRestartScope = {
  taskId: string;
  sessionId: string;
  environmentId: string;
  executionId: string;
};

type GitRestartFixture = {
  taskId: string;
  sessionId: string;
  environmentId: string;
  executionId: string;
  worktreePath: string;
  targetFile: string;
  originalContent: Buffer;
};

type GitRefreshResponse = {
  success: boolean;
  session_id: string;
  status_state: "ready" | "unavailable";
  error_code?: string;
  snapshots: Array<{ payload?: Record<string, unknown> }>;
};

async function createPreparedGitRestartFixture(
  apiClient: ApiClient,
  seedData: SeedData,
  profileId: string,
  onTaskCreated: (taskId: string) => void,
): Promise<GitRestartFixture> {
  const task = await apiClient.createTask(
    seedData.workspaceId,
    `Managed runtime git restart ${Date.now()}`,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      agent_profile_id: profileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
      repository_ids: [seedData.repositoryId],
      prepare_session: true,
    },
  );
  onTaskCreated(task.id);
  if (!task.session_id) throw new Error("prepared task did not return a session_id");

  await expect
    .poll(async () => (await apiClient.getTaskEnvironment(task.id))?.status ?? null, {
      timeout: 90_000,
      message: "prepared task workspace did not become ready",
    })
    .toBe("ready");
  const environment = await apiClient.getTaskEnvironment(task.id);
  if (!environment) throw new Error("prepared task has no workspace environment");
  const repository = environment.repos?.find(
    (candidate) => candidate.repository_id === seedData.repositoryId,
  );
  const worktreePath = repository?.worktree_path ?? environment.worktree_path;
  if (!worktreePath) throw new Error("prepared task has no canonical repository path");
  const executionId = await getWorkspaceOnlyExecutionId(apiClient, task.id, task.session_id);
  const targetFile = path.join(worktreePath, RETAINED_WORKSPACE_FILE);
  return {
    taskId: task.id,
    sessionId: task.session_id,
    environmentId: environment.id,
    executionId,
    worktreePath,
    targetFile,
    originalContent: fs.readFileSync(targetFile),
  };
}

async function getWorkspaceOnlyExecutionId(
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
): Promise<string> {
  const status = await apiClient.wsRequest<{ state: string; is_agent_running: boolean }>(
    "task.session.status",
    { task_id: taskId, session_id: sessionId },
  );
  expect(status.state).toBe("CREATED");
  expect(status.is_agent_running).toBe(false);

  await expect
    .poll(async () => {
      const result = await apiClient.listTaskSessions(taskId);
      const session = result.sessions.find((candidate) => candidate.id === sessionId);
      return session?.state === "CREATED" && Boolean(session.agent_execution_id);
    })
    .toBe(true);
  const result = await apiClient.listTaskSessions(taskId);
  const executionId = result.sessions.find(
    (candidate) => candidate.id === sessionId,
  )?.agent_execution_id;
  if (!executionId) throw new Error("prepared workspace-only session has no execution identity");
  return executionId;
}

async function currentExecutionId(apiClient: ApiClient, taskId: string, sessionId: string) {
  const { sessions } = await apiClient.listTaskSessions(taskId);
  const executionId = sessions.find((candidate) => candidate.id === sessionId)?.agent_execution_id;
  if (!executionId) throw new Error("recovered session has no execution identity");
  return executionId;
}

async function openChat(page: Page, session: SessionPage, mobile: boolean): Promise<void> {
  if (mobile) {
    await page.getByRole("button", { name: "Chat", exact: true }).tap();
    return;
  }
  await session.clickSessionChatTab();
}

async function openChanges(page: Page, session: SessionPage, mobile: boolean): Promise<void> {
  if (mobile) {
    await page
      .getByRole("navigation")
      .getByRole("button", { name: /Changes$/ })
      .tap();
    await expect(page.getByTestId("mobile-changes-panel")).toBeVisible();
    return;
  }
  await session.clickTab("Changes");
  await expect(session.changes).toBeVisible();
}

async function startPreparedAgent(
  page: Page,
  session: SessionPage,
  mobile: boolean,
): Promise<void> {
  await openChat(page, session, mobile);
  const startButton = session.activeChat().getByTestId("task-description-start-button");
  if (mobile) await startButton.tap();
  else await startButton.click();
}

async function waitForSecondAttempt(
  page: Page,
  session: SessionPage,
  retryReleaseFile: string,
  launchId: string,
): Promise<void> {
  await expect
    .poll(() => fs.existsSync(`${retryReleaseFile}.waiting`), {
      timeout: 60_000,
      message: "the managed runtime retry fixture should pause before its second attempt",
    })
    .toBe(true);
  expect(fs.readFileSync(managedRuntimeStartupAttemptFile(launchId), "utf8").trim()).toBe("2");
  await expect(
    session.activeChat().getByText("Retrying agent startup (attempt 2 of 2)", { exact: true }),
  ).toBeVisible({ timeout: 30_000 });
}

function hasGitRestartScope(payload: Record<string, unknown>, scope: GitRestartScope): boolean {
  return (
    payload.task_id === scope.taskId &&
    payload.session_id === scope.sessionId &&
    payload.task_environment_id === scope.environmentId &&
    payload.agent_id === scope.executionId
  );
}

function armGitRestartEvents(
  trace: PromotionTrace,
  scope: GitRestartScope,
  marker: string,
  mutationStartedAtMs: number,
): { membership: Promise<WorkspaceStatusEvent>; detail: Promise<WorkspaceStatusEvent> } {
  return {
    membership: trace.waitForStatusEvent(
      "restart file membership",
      (payload) => {
        const file = recordValue(
          statusFiles(statusFromPayload(payload))?.[RETAINED_WORKSPACE_FILE],
        );
        return (
          hasGitRestartScope(payload, scope) &&
          typeof file?.diff === "string" &&
          file.diff.includes(marker)
        );
      },
      mutationStartedAtMs,
    ),
    detail: trace.waitForStatusEvent(
      "restart settled file detail",
      (payload) => {
        const status = statusFromPayload(payload);
        const file = recordValue(statusFiles(status)?.[RETAINED_WORKSPACE_FILE]);
        return (
          hasGitRestartScope(payload, scope) &&
          status?.detail_state === "ready" &&
          typeof file?.diff === "string" &&
          file.diff.includes(marker)
        );
      },
      mutationStartedAtMs,
    ),
  };
}

async function showChangedFile(page: Page, session: SessionPage, mobile: boolean): Promise<void> {
  const fileRow = mobile
    ? page.getByTestId("mobile-changes-panel").getByTestId(`file-row-${RETAINED_WORKSPACE_FILE}`)
    : session.changes.getByTestId(`file-row-${RETAINED_WORKSPACE_FILE}`);
  await expect(fileRow).toBeVisible({ timeout: 15_000 });
  if (mobile) await fileRow.tap();
  else await fileRow.click();
}

async function assertFreshRefreshAfterRelease(
  apiClient: ApiClient,
  sessionId: string,
  marker: string,
): Promise<GitRefreshResponse> {
  const refresh = await apiClient.wsRequest<GitRefreshResponse>(
    "session.git.refresh",
    { session_id: sessionId, mode: "fresh" },
    65_000,
  );
  expect(refresh.success).toBe(true);
  expect(refresh.status_state).toBe("ready");
  const snapshots = refresh.snapshots.map((snapshot) => snapshot.payload).filter(Boolean);
  const seesMarker = snapshots.some((payload) => {
    const file = recordValue(statusFiles(statusFromPayload(payload!))?.[RETAINED_WORKSPACE_FILE]);
    return typeof file?.diff === "string" && file.diff.includes(marker);
  });
  expect(seesMarker, "the refreshed snapshot should still carry the mutated file diff").toBe(true);
  return refresh;
}

async function attachGitRestartEvidence(options: {
  testInfo: TestInfo;
  fixture: GitRestartFixture;
  recoveredExecutionId: string;
  attemptFile: string;
  mutationStartedAt: string;
  membershipEvent: WorkspaceStatusEvent;
  settledEvent: WorkspaceStatusEvent;
  trace: PromotionTrace;
  refresh: GitRefreshResponse;
}): Promise<void> {
  await options.testInfo.attach("managed-runtime-git-restart.json", {
    body: JSON.stringify(
      {
        taskId: options.fixture.taskId,
        sessionId: options.fixture.sessionId,
        environmentId: options.fixture.environmentId,
        worktreePath: options.fixture.worktreePath,
        workspaceOnlyExecutionId: options.fixture.executionId,
        recoveredExecutionId: options.recoveredExecutionId,
        startupAttempt: fs.readFileSync(options.attemptFile, "utf8").trim(),
        mutationStartedAt: options.mutationStartedAt,
        membershipEventReceivedAt: options.membershipEvent.receivedAt,
        settledEventReceivedAt: options.settledEvent.receivedAt,
        streamDiagnostics: options.trace.diagnostics(),
        freshRefresh: {
          success: options.refresh.success,
          statusState: options.refresh.status_state,
          snapshotCount: options.refresh.snapshots.length,
        },
      },
      null,
      2,
    ),
    contentType: "application/json",
  });
}

/** Run the desktop or phone startup-retry Changes recovery path with real Git evidence. */
export async function runManagedRuntimeGitRestart(options: {
  page: Page;
  apiClient: ApiClient;
  backend: BackendContext;
  seedData: SeedData;
  prCapture: PrAssetCapture;
  testInfo: TestInfo;
  mobile: boolean;
}): Promise<void> {
  const { page, apiClient, backend, seedData, prCapture, testInfo, mobile } = options;
  const launchId = `git-restart-${Date.now()}`;
  const retryReleaseFile = path.join(backend.tmpDir, `git-restart-retry-${launchId}`);
  const attemptFile = managedRuntimeStartupAttemptFile(launchId);
  let profileId = "";
  let hostFixturePath = "";
  let cleanupTaskId: string | null = null;
  let fixture: GitRestartFixture | null = null;
  let trace: PromotionTrace | null = null;
  let fileChanged = false;

  try {
    const prepared = await prepareManagedRuntimeProfile(apiClient, backend, {
      startupMode: "silent-exit",
      launchId,
      hostSubprocess: true,
      retryReleaseFile,
    });
    profileId = prepared.profile.id;
    hostFixturePath = prepared.hostFixturePath ?? "";

    fixture = await createPreparedGitRestartFixture(apiClient, seedData, profileId, (taskId) => {
      cleanupTaskId = taskId;
    });
    const scope: GitRestartScope = {
      taskId: fixture.taskId,
      sessionId: fixture.sessionId,
      environmentId: fixture.environmentId,
      executionId: fixture.executionId,
    };
    trace = await routeWorkspaceStreamPromotion(page, fixture.taskId, fixture.sessionId);
    await page.goto(`/t/${fixture.taskId}`);
    const session = new SessionPage(page);
    await session.waitForLoad();
    await openChanges(page, session, mobile);
    await startPreparedAgent(page, session, mobile);
    await waitForSecondAttempt(page, session, retryReleaseFile, launchId);
    await openChanges(page, session, mobile);

    fs.writeFileSync(retryReleaseFile, "released");
    await waitForSessionDone(
      apiClient,
      fixture.taskId,
      fixture.sessionId,
      "Wait for managed runtime startup recovery before proving Changes delivery",
    );
    const recoveredExecutionId = await currentExecutionId(
      apiClient,
      fixture.taskId,
      fixture.sessionId,
    );
    expect(recoveredExecutionId).toBe(fixture.executionId);
    expect(fs.readFileSync(attemptFile, "utf8").trim()).toBe("2");
    const { messages } = await apiClient.listSessionMessages(fixture.sessionId);
    expect(
      messages.filter(
        (message) => message.author_type === "user" && message.content === "/e2e:simple-message",
      ),
    ).toHaveLength(1);
    expect(
      messages.some(
        (message) =>
          message.author_type === "agent" && message.content.includes("simple mock response"),
      ),
    ).toBe(true);

    await trace.holdGitReadsAfterDrain();
    const marker = `MANAGED_RUNTIME_GIT_RESTART_${Date.now()}`;
    const mutationStartedAt = new Date().toISOString();
    const mutationStartedAtMs = Date.now();
    const expected = armGitRestartEvents(trace, scope, marker, mutationStartedAtMs);
    fs.writeFileSync(fixture.targetFile, `${marker}\n`);
    fileChanged = true;

    const [membershipEvent, settledEvent] = await Promise.all([
      expected.membership,
      expected.detail,
    ]);
    expect(trace.diagnostics().gitReadResponsesAfterHold).toBe(0);
    await showChangedFile(page, session, mobile);
    await page.waitForFunction(
      (searchText: string) =>
        Array.from(document.querySelectorAll("diffs-container")).some((container) =>
          container.shadowRoot?.textContent?.includes(searchText),
        ),
      marker,
      { timeout: 30_000 },
    );
    await prCapture.screenshot(
      mobile ? "managed-runtime-git-restart-mobile" : "managed-runtime-git-restart-desktop",
      {
        caption: `${mobile ? "Phone" : "Desktop"} Changes recovers live membership and its settled diff after a managed-runtime startup retry.`,
      },
    );
    await page.screenshot({
      path: testInfo.outputPath(
        mobile
          ? "managed-runtime-git-restart-mobile.png"
          : "managed-runtime-git-restart-desktop.png",
      ),
      fullPage: true,
    });

    await trace.releaseHeldGitReadsAndWaitForResponses();
    const refresh = await assertFreshRefreshAfterRelease(apiClient, fixture.sessionId, marker);
    await attachGitRestartEvidence({
      testInfo,
      fixture,
      recoveredExecutionId,
      attemptFile,
      mutationStartedAt,
      membershipEvent,
      settledEvent,
      trace,
      refresh,
    });
  } finally {
    if (fileChanged && fixture) fs.writeFileSync(fixture.targetFile, fixture.originalContent);
    trace?.releaseHeldGitReads();
    fs.writeFileSync(retryReleaseFile, "released");
    if (cleanupTaskId) {
      await apiClient
        .deleteTask(cleanupTaskId, { discardWorktreeChanges: true })
        .catch(() => undefined);
    }
    if (profileId) await apiClient.deleteAgentProfile(profileId, true).catch(() => undefined);
    if (hostFixturePath) fs.rmSync(hostFixturePath, { force: true });
    await restoreE2EAgentRegistry(backend);
    fs.rmSync(retryReleaseFile, { force: true });
    fs.rmSync(`${retryReleaseFile}.waiting`, { force: true });
  }
}
