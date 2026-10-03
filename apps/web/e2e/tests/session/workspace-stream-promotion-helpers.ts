import fs from "node:fs";
import path from "node:path";
import { expect, type Page, type TestInfo } from "@playwright/test";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { RETAINED_WORKSPACE_FILE } from "./completed-workspace-restoration-helpers";

type WireFrame = {
  id?: unknown;
  type?: unknown;
  action?: unknown;
  payload?: unknown;
};

type LaunchTrace = {
  action: string;
  intent: string;
  requestAt: string;
  responseAt: string;
  response: Record<string, unknown>;
};

type WorkspaceStatusEvent = {
  receivedAt: string;
  receivedAtMs: number;
  payload: Record<string, unknown>;
};

type HeldRequest = {
  frame: string;
  server: BridgeSocket;
};

type BridgeSocket = {
  send(message: string | Buffer): void;
  onMessage(handler: (message: string | Buffer) => void): void;
};

type PromotionTraceState = {
  taskId: string;
  sessionId: string;
  correlatedRequests: Map<string, { action: string; intent: string; requestAt: string }>;
  launches: LaunchTrace[];
  statusEvents: WorkspaceStatusEvent[];
  pendingGitReads: Map<string, string>;
  heldGitReads: HeldRequest[];
  holdGitReads: boolean;
  holdStartedAt: string | null;
  holdStartedAtMs: number | null;
  gitReadRequestsBeforeHold: number;
  gitReadResponsesBeforeHold: number;
  gitReadRequestsAfterHold: number;
  gitReadResponsesAfterHold: number;
};

function recordValue(value: unknown): Record<string, unknown> | null {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

function parseFrame(value: string): WireFrame | null {
  try {
    return recordValue(JSON.parse(value)) as WireFrame | null;
  } catch {
    return null;
  }
}

function isGitReadAction(action: unknown): action is string {
  return (
    typeof action === "string" &&
    (/^session\.git\./.test(action) ||
      action === "session.cumulative_diff" ||
      action === "session.commit_diff")
  );
}

function trackLaunchRequest(
  frame: WireFrame,
  payload: Record<string, unknown> | null,
  state: PromotionTraceState,
) {
  if (
    typeof frame.id !== "string" ||
    (frame.action !== "session.launch" && frame.action !== "session.recover") ||
    payload?.task_id !== state.taskId ||
    payload.session_id !== state.sessionId
  ) {
    return;
  }
  const intent = frame.action === "session.recover" ? payload.action : payload.intent;
  state.correlatedRequests.set(frame.id, {
    action: frame.action,
    intent: typeof intent === "string" ? intent : "",
    requestAt: new Date().toISOString(),
  });
}

function trackGitReadRequest(
  frame: WireFrame,
  rawFrame: string,
  server: BridgeSocket,
  state: PromotionTraceState,
): boolean {
  if (typeof frame.id !== "string" || !isGitReadAction(frame.action)) return true;
  state.pendingGitReads.set(frame.id, frame.action);
  if (!state.holdGitReads) {
    state.gitReadRequestsBeforeHold += 1;
    return true;
  }
  state.gitReadRequestsAfterHold += 1;
  state.heldGitReads.push({ frame: rawFrame, server });
  return false;
}

function handleOutgoingFrame(
  rawFrame: string,
  frame: WireFrame | null,
  server: BridgeSocket,
  state: PromotionTraceState,
): boolean {
  if (frame?.type !== "request") return true;
  trackLaunchRequest(frame, recordValue(frame.payload), state);
  return trackGitReadRequest(frame, rawFrame, server, state);
}

function recordLaunchResponse(
  frame: WireFrame,
  payload: Record<string, unknown>,
  state: PromotionTraceState,
) {
  if (typeof frame.id !== "string") return;
  const request = state.correlatedRequests.get(frame.id);
  if (!request) return;
  state.launches.push({ ...request, responseAt: new Date().toISOString(), response: payload });
  state.correlatedRequests.delete(frame.id);
}

function recordGitReadResponse(frame: WireFrame, state: PromotionTraceState) {
  if (typeof frame.id !== "string" || !isGitReadAction(state.pendingGitReads.get(frame.id))) return;
  state.pendingGitReads.delete(frame.id);
  if (state.holdStartedAtMs === null) state.gitReadResponsesBeforeHold += 1;
  else state.gitReadResponsesAfterHold += 1;
}

function recordStatusEvent(payload: Record<string, unknown>, state: PromotionTraceState) {
  if (payload.session_id !== state.sessionId || payload.type !== "status_update") return;
  const receivedAtMs = Date.now();
  state.statusEvents.push({
    receivedAt: new Date(receivedAtMs).toISOString(),
    receivedAtMs,
    payload,
  });
}

function handleIncomingFrame(frame: WireFrame | null, state: PromotionTraceState) {
  if (!frame) return;
  const payload = recordValue(frame.payload);
  if (payload && (frame.type === "response" || frame.type === "error")) {
    recordLaunchResponse(frame, payload, state);
    recordGitReadResponse(frame, state);
  }
  if (frame.type === "notification" && frame.action === "session.git.event" && payload) {
    recordStatusEvent(payload, state);
  }
}

function forwardBridgeMessage(
  message: string | Buffer,
  target: BridgeSocket,
  onFrame: (rawFrame: string, frame: WireFrame | null) => boolean,
) {
  if (typeof message !== "string") {
    target.send(message);
    return;
  }
  const forwarded = message
    .split("\n")
    .filter((part) => part.trim())
    .filter((part) => onFrame(part.trim(), parseFrame(part.trim())));
  if (forwarded.length > 0) target.send(forwarded.join("\n"));
}

function createPromotionTrace(state: PromotionTraceState) {
  return {
    waitForActionResponse(intent: string, action: string): Promise<LaunchTrace> {
      return expect
        .poll(
          () =>
            state.launches.find((trace) => trace.intent === intent && trace.action === action) ??
            null,
          {
            timeout: 30_000,
            message: `Waiting for the correlated ${action} ${intent} response`,
          },
        )
        .not.toBeNull()
        .then(
          () => state.launches.find((trace) => trace.intent === intent && trace.action === action)!,
        );
    },
    waitForStatusEvent(
      description: string,
      predicate: (payload: Record<string, unknown>) => boolean,
      afterMs?: number,
    ): Promise<WorkspaceStatusEvent> {
      return expect
        .poll(
          () =>
            state.statusEvents.find(
              (event) =>
                (afterMs === undefined || event.receivedAtMs >= afterMs) &&
                predicate(event.payload),
            ) ?? null,
          {
            timeout: 30_000,
            message: `Waiting for the ${description} workspace stream event`,
          },
        )
        .not.toBeNull()
        .then(
          () =>
            state.statusEvents.find(
              (event) =>
                (afterMs === undefined || event.receivedAtMs >= afterMs) &&
                predicate(event.payload),
            )!,
        );
    },
    async holdGitReadsAfterDrain(): Promise<void> {
      await expect
        .poll(() => state.pendingGitReads.size, {
          timeout: 30_000,
          message: "Initial Git reads should settle before the post-mutation stream proof",
        })
        .toBe(0);
      state.holdStartedAtMs = Date.now();
      state.holdStartedAt = new Date(state.holdStartedAtMs).toISOString();
      state.holdGitReads = true;
    },
    releaseHeldGitReads(): void {
      state.holdGitReads = false;
      for (const request of state.heldGitReads.splice(0)) request.server.send(request.frame);
    },
    diagnostics() {
      return {
        holdStartedAt: state.holdStartedAt,
        gitReadRequestsBeforeHold: state.gitReadRequestsBeforeHold,
        gitReadResponsesBeforeHold: state.gitReadResponsesBeforeHold,
        gitReadRequestsAfterHold: state.gitReadRequestsAfterHold,
        gitReadResponsesAfterHold: state.gitReadResponsesAfterHold,
        heldGitReadActions: state.heldGitReads
          .map((request) => parseFrame(request.frame)?.action)
          .filter((action): action is string => typeof action === "string"),
        launches: state.launches.map((trace) => ({
          action: trace.action,
          intent: trace.intent,
          requestAt: trace.requestAt,
          responseAt: trace.responseAt,
          success: trace.response.success === true,
          agentExecutionId: trace.response.agent_execution_id,
          state: trace.response.state,
        })),
        statusEvents: state.statusEvents.map(({ receivedAt, payload }) => {
          const status = statusFromPayload(payload);
          return {
            receivedAt,
            taskId: payload.task_id,
            sessionId: payload.session_id,
            environmentId: payload.task_environment_id,
            agentExecutionId: payload.agent_id,
            statusState: status?.status_state,
            detailState: status?.detail_state,
            fileCount: Object.keys(statusFiles(status) ?? {}).length,
          };
        }),
      };
    },
  };
}

/** Observe the real browser gateway and keep later Git reads from masking a missing push. */
export async function routeWorkspaceStreamPromotion(page: Page, taskId: string, sessionId: string) {
  const state: PromotionTraceState = {
    taskId,
    sessionId,
    correlatedRequests: new Map(),
    launches: [],
    statusEvents: [],
    pendingGitReads: new Map(),
    heldGitReads: [],
    holdGitReads: false,
    holdStartedAt: null,
    holdStartedAtMs: null,
    gitReadRequestsBeforeHold: 0,
    gitReadResponsesBeforeHold: 0,
    gitReadRequestsAfterHold: 0,
    gitReadResponsesAfterHold: 0,
  };
  await page.routeWebSocket(/\/ws$/, (socket) => {
    const server = socket.connectToServer();
    socket.onMessage((message) =>
      forwardBridgeMessage(message, server, (rawFrame, frame) =>
        handleOutgoingFrame(rawFrame, frame, server, state),
      ),
    );
    server.onMessage((message) =>
      forwardBridgeMessage(message, socket, (_rawFrame, frame) => {
        handleIncomingFrame(frame, state);
        return true;
      }),
    );
  });
  return createPromotionTrace(state);
}

function statusFromPayload(payload: Record<string, unknown>): Record<string, unknown> | null {
  return recordValue(payload.status);
}

function statusFiles(status: Record<string, unknown> | null): Record<string, unknown> | null {
  return recordValue(status?.files);
}

type PromotionTask = Awaited<ReturnType<ApiClient["createTask"]>>;
type PromotionEnvironment = NonNullable<Awaited<ReturnType<ApiClient["getTaskEnvironment"]>>>;
type PromotionTrace = Awaited<ReturnType<typeof routeWorkspaceStreamPromotion>>;
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
      (payload) =>
        hasPromotionScope(payload, scope) &&
        Object.prototype.hasOwnProperty.call(
          statusFiles(statusFromPayload(payload)) ?? {},
          RETAINED_WORKSPACE_FILE,
        ),
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
  const fixture = await createPreparedPromotionFixture(apiClient, seedData, mobile);
  const { task, sessionId, environment, targetFile, originalContent } = fixture;
  const trace = await routeWorkspaceStreamPromotion(page, task.id, sessionId);
  let fileChanged = false;

  try {
    await page.goto(`/t/${task.id}`);
    const session = new SessionPage(page);
    await session.waitForLoad();
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
    if (fileChanged) fs.writeFileSync(targetFile, originalContent);
    trace.releaseHeldGitReads();
  }
}
