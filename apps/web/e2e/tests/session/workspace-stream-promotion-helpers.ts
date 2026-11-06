import fs from "node:fs";
import path from "node:path";
import { expect, type Page, type TestInfo } from "@playwright/test";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
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

type PromotionTask = Awaited<ReturnType<ApiClient["createTask"]>>;
type PromotionEnvironment = NonNullable<Awaited<ReturnType<ApiClient["getTaskEnvironment"]>>>;
type PromotionScope = {
  taskId: string;
  sessionId: string;
  environmentId: string;
  executionId: string;
};

async function createPreparedPromotionFixture(
  apiClient: ApiClient,
  seedData: SeedData,
  mobile: boolean,
  onTaskCreated: (taskId: string) => void,
): Promise<{
  task: PromotionTask;
  sessionId: string;
  environment: PromotionEnvironment;
  targetFile: string;
  originalContent: Buffer;
}> {
  const task = await apiClient.createTask(
    seedData.workspaceId,
    `Workspace stream promotion ${mobile ? "mobile" : "desktop"} ${Date.now()}`,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
      repository_ids: [seedData.repositoryId],
      prepare_session: true,
    },
  );
  onTaskCreated(task.id);
  if (!task.session_id) throw new Error("prepared task did not return a session_id");
  const sessionId = task.session_id;

  await expect
    .poll(async () => (await apiClient.getTaskEnvironment(task.id))?.status ?? null, {
      timeout: 90_000,
      message: "prepared task workspace did not become ready",
    })
    .toBe("ready");

  const environment = await apiClient.getTaskEnvironment(task.id);
  if (!environment) throw new Error("prepared task has no workspace environment");
  const repository = environment.repos?.find(
    (repo) => repo.repository_id === seedData.repositoryId,
  );
  const worktreePath = repository?.worktree_path ?? environment.worktree_path;
  if (!worktreePath) throw new Error("prepared task has no canonical repository path");
  const targetFile = path.join(worktreePath, RETAINED_WORKSPACE_FILE);
  return { task, sessionId, environment, targetFile, originalContent: fs.readFileSync(targetFile) };
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

async function navigateToPanel(
  page: Page,
  session: SessionPage,
  mobile: boolean,
  panel: "files" | "changes" | "chat",
): Promise<void> {
  if (panel === "chat") {
    if (mobile) await page.getByRole("button", { name: "Chat", exact: true }).tap();
    else await session.clickSessionChatTab();
    return;
  }
  if (panel === "files") {
    if (mobile) await page.getByRole("button", { name: "Files", exact: true }).tap();
    else await session.clickTab("Files");
    return;
  }
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

async function openWorkspacePanels(
  page: Page,
  session: SessionPage,
  mobile: boolean,
): Promise<void> {
  await navigateToPanel(page, session, mobile, "files");
  const fileNode = await session.fileTree.waitForFileTreeNode(RETAINED_WORKSPACE_FILE, 60_000);
  if (mobile) {
    await fileNode.tap();
    const viewer = page.getByTestId("mobile-file-viewer-panel");
    await expect(viewer).toBeVisible({ timeout: 15_000 });
    await viewer.getByRole("button", { name: "Close" }).tap();
    await expect(viewer).toHaveCount(0);
  }
  await navigateToPanel(page, session, mobile, "changes");
}

async function startPreparedAgent(
  page: Page,
  session: SessionPage,
  mobile: boolean,
  trace: PromotionTrace,
  workspaceExecutionId: string,
): Promise<string> {
  await navigateToPanel(page, session, mobile, "chat");
  const responsePromise = trace.waitForActionResponse("start_created", "session.launch");
  const startButton = session.activeChat().getByTestId("task-description-start-button");
  if (mobile) await startButton.tap();
  else await startButton.click();
  const response = await responsePromise;
  expect(response.response.success).not.toBe(false);
  const executionId = response.response.agent_execution_id;
  expect(executionId, "start-created promotion returns its current execution identity").toBe(
    workspaceExecutionId,
  );
  return String(executionId);
}

function hasPromotionScope(payload: Record<string, unknown>, scope: PromotionScope): boolean {
  return (
    payload.task_id === scope.taskId &&
    payload.session_id === scope.sessionId &&
    payload.task_environment_id === scope.environmentId &&
    payload.agent_id === scope.executionId
  );
}

function armMutationEvents(
  trace: PromotionTrace,
  scope: PromotionScope,
  marker: string,
  mutationStartedAtMs: number,
): { membership: Promise<WorkspaceStatusEvent>; detail: Promise<WorkspaceStatusEvent> } {
  return {
    membership: trace.waitForStatusEvent(
      "post-promotion file membership",
      (payload) => {
        const file = recordValue(
          statusFiles(statusFromPayload(payload))?.[RETAINED_WORKSPACE_FILE],
        );
        return (
          hasPromotionScope(payload, scope) &&
          typeof file?.diff === "string" &&
          file.diff.includes(marker)
        );
      },
      mutationStartedAtMs,
    ),
    detail: trace.waitForStatusEvent(
      "settled post-promotion file detail",
      (payload) => {
        const status = statusFromPayload(payload);
        const file = recordValue(statusFiles(status)?.[RETAINED_WORKSPACE_FILE]);
        return (
          hasPromotionScope(payload, scope) &&
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

function assertPromotionEvents(
  membershipEvent: WorkspaceStatusEvent,
  settledEvent: WorkspaceStatusEvent,
  scope: PromotionScope,
  mutationStartedAtMs: number,
  marker: string,
): void {
  expect(hasPromotionScope(membershipEvent.payload, scope)).toBe(true);
  expect(membershipEvent.receivedAtMs).toBeGreaterThanOrEqual(mutationStartedAtMs);
  const membershipFile = recordValue(
    statusFiles(statusFromPayload(membershipEvent.payload))?.[RETAINED_WORKSPACE_FILE],
  );
  expect(membershipFile?.diff).toContain(marker);
  const status = statusFromPayload(settledEvent.payload);
  const file = recordValue(statusFiles(status)?.[RETAINED_WORKSPACE_FILE]);
  expect(hasPromotionScope(settledEvent.payload, scope)).toBe(true);
  expect(status?.detail_state).toBe("ready");
  expect(file?.diff).toContain(marker);
}

async function attachPromotionEvidence(options: {
  testInfo: TestInfo;
  taskId: string;
  sessionId: string;
  environment: PromotionEnvironment;
  seedData: SeedData;
  workspaceExecutionId: string;
  promotedExecutionId: string;
  mutationStartedAt: string;
  mutationStartedAtMs: number;
  membershipEvent: WorkspaceStatusEvent;
  settledEvent: WorkspaceStatusEvent;
  trace: PromotionTrace;
}): Promise<void> {
  const details = options.trace.diagnostics();
  await options.testInfo.attach("workspace-stream-promotion-timing.json", {
    body: JSON.stringify(
      {
        taskId: options.taskId,
        sessionId: options.sessionId,
        environmentId: options.environment.id,
        repositoryId: options.seedData.repositoryId,
        repositoryName: path.basename(options.seedData.repositoryPath),
        filePath: RETAINED_WORKSPACE_FILE,
        workspaceOnlyExecutionId: options.workspaceExecutionId,
        promotedExecutionId: options.promotedExecutionId,
        mutationStartedAt: options.mutationStartedAt,
        membershipEventReceivedAt: options.membershipEvent.receivedAt,
        settledEventReceivedAt: options.settledEvent.receivedAt,
        mutationToMembershipMs: options.membershipEvent.receivedAtMs - options.mutationStartedAtMs,
        mutationToSettledDetailMs: options.settledEvent.receivedAtMs - options.mutationStartedAtMs,
        gitReads: details,
      },
      null,
      2,
    ),
    contentType: "application/json",
  });
}

/** Run the desktop or phone promotion path and return source-correlated evidence. */
export async function runWorkspaceStreamPromotion(options: {
  page: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  prCapture: PrAssetCapture;
  testInfo: TestInfo;
  mobile: boolean;
}): Promise<void> {
  const { page, apiClient, seedData, prCapture, testInfo, mobile } = options;
  let cleanupTaskId: string | null = null;
  let cleanupFile: { path: string; originalContent: Buffer } | null = null;
  let cleanupTrace: PromotionTrace | null = null;
  let fileChanged = false;

  try {
    const fixture = await createPreparedPromotionFixture(apiClient, seedData, mobile, (taskId) => {
      cleanupTaskId = taskId;
    });
    const { task, sessionId, environment, targetFile, originalContent } = fixture;
    cleanupFile = { path: targetFile, originalContent };
    const trace = await routeWorkspaceStreamPromotion(page, task.id, sessionId);
    cleanupTrace = trace;
    await page.goto(`/t/${task.id}`);
    const session = new SessionPage(page);
    await session.waitForLoad();
    // A visible chat shell can precede selection of the prepared session.
    await expect(session.activeChat().getByTestId("task-description-start-button")).toBeVisible();
    const workspaceExecutionId = await getWorkspaceOnlyExecutionId(apiClient, task.id, sessionId);
    await openWorkspacePanels(page, session, mobile);
    const promotedExecutionId = await startPreparedAgent(
      page,
      session,
      mobile,
      trace,
      workspaceExecutionId,
    );
    await navigateToPanel(page, session, mobile, "changes");
    await trace.holdGitReadsAfterDrain();
    const marker = `WORKSPACE_STREAM_PROMOTION_${Date.now()}`;
    const mutationStartedAt = new Date().toISOString();
    const mutationStartedAtMs = Date.now();
    const scope = {
      taskId: task.id,
      sessionId,
      environmentId: environment.id,
      executionId: workspaceExecutionId,
    };
    const expected = armMutationEvents(trace, scope, marker, mutationStartedAtMs);
    fs.writeFileSync(targetFile, `${marker}\n`);
    fileChanged = true;

    const [membershipEvent, settledEvent] = await Promise.all([
      expected.membership,
      expected.detail,
    ]);
    assertPromotionEvents(membershipEvent, settledEvent, scope, mutationStartedAtMs, marker);
    await showChangedFile(page, session, mobile);
    await page.waitForFunction(
      (searchText: string) =>
        Array.from(document.querySelectorAll("diffs-container")).some((container) =>
          container.shadowRoot?.textContent?.includes(searchText),
        ),
      marker,
      { timeout: 30_000 },
    );

    expect(trace.diagnostics().gitReadResponsesAfterHold).toBe(0);
    await prCapture.screenshot(
      mobile ? "workspace-stream-promotion-mobile" : "workspace-stream-promotion-desktop",
      {
        caption: `${mobile ? "Phone" : "Desktop"} Changes shows a post-promotion file and its settled diff from the workspace stream.`,
      },
    );

    await attachPromotionEvidence({
      testInfo,
      taskId: task.id,
      sessionId,
      environment,
      seedData,
      workspaceExecutionId,
      promotedExecutionId,
      mutationStartedAt,
      mutationStartedAtMs,
      membershipEvent,
      settledEvent,
      trace,
    });
  } finally {
    if (fileChanged && cleanupFile) {
      fs.writeFileSync(cleanupFile.path, cleanupFile.originalContent);
    }
    cleanupTrace?.releaseHeldGitReads();
    if (cleanupTaskId) {
      await apiClient
        .deleteTask(cleanupTaskId, { discardWorktreeChanges: true })
        .catch(() => undefined);
    }
  }
}
