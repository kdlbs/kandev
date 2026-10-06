import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { expect, type Locator, type Page, type Route, type TestInfo } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { DatabaseSync } from "./node-sqlite";
import { waitForSessionState } from "./session";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";
import {
  captureSessionRecoveryMessages,
  capturedSessionRecoveryResponse,
} from "./session-resume-recovery";
import { KanbanPage } from "../pages/kanban-page";
import { MobileKanbanPage } from "../pages/mobile-kanban-page";
import { SessionPage } from "../pages/session-page";

type TraceBlock = {
  type: string;
  mime_type?: string;
  byte_length?: number;
  sha256?: string;
};

type ACPTraceEvent = {
  event: string;
  session_id: string;
  prompt?: string;
  prompt_blocks?: string;
};

type RecoveryFixture = {
  taskId: string;
  sessionId: string;
  profileId: string;
  imageAttachmentId: string;
  resourceAttachmentId: string;
  errorStamp: string;
  content: string;
  imageBytes: Buffer;
  resourceBytes: Buffer;
};

type ScenarioContext = {
  page: Page;
  apiClient: ApiClient;
  backend: BackendContext;
  fixture: RecoveryFixture;
  mobile: boolean;
  tracePath: string;
  session: SessionPage;
  recovery: ReturnType<typeof captureSessionRecoveryMessages>;
};
type ListedSession = Awaited<ReturnType<ApiClient["listTaskSessions"]>>["sessions"][number];
type UserSettings = Awaited<ReturnType<ApiClient["getUserSettings"]>>["settings"];

type AttachmentStorageRow = { storage_key: string };
type SQLiteFixtureDatabase = {
  prepare(sql: string): { get(...params: unknown[]): unknown };
  close(): void;
};

function readTrace(tracePath: string): ACPTraceEvent[] {
  if (!fs.existsSync(tracePath)) return [];
  return fs
    .readFileSync(tracePath, "utf8")
    .split("\n")
    .filter(Boolean)
    .map((line) => JSON.parse(line) as ACPTraceEvent);
}

function removeStoredAttachment(backend: BackendContext, attachmentId: string) {
  const database = new DatabaseSync(path.join(backend.tmpDir, "kandev.db"), {
    readOnly: true,
  }) as unknown as SQLiteFixtureDatabase;
  let row: AttachmentStorageRow | undefined;
  try {
    row = database
      .prepare("SELECT storage_key FROM task_message_attachments WHERE id = ?")
      .get(attachmentId) as AttachmentStorageRow | undefined;
  } finally {
    database.close();
  }
  if (!row?.storage_key) throw new Error(`attachment storage key is missing for ${attachmentId}`);
  const storedPath = path.join(backend.tmpDir, ".kandev", "attachments", row.storage_key);
  const bytes = fs.readFileSync(storedPath);
  fs.unlinkSync(storedPath);
  return { storedPath, bytes };
}

function requireValue<T>(value: T | undefined | null, message: string): T {
  if (value === undefined || value === null || value === "") throw new Error(message);
  return value;
}

async function activate(locator: Locator, mobile: boolean) {
  if (mobile) await locator.tap();
  else await locator.click();
}

async function openTaskCreateDialog(page: Page, mobile: boolean) {
  if (mobile) {
    const kanban = new MobileKanbanPage(page);
    await kanban.goto();
    await kanban.mobileFab.tap();
  } else {
    const kanban = new KanbanPage(page);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
  }
  const dialog = page.getByTestId("create-task-dialog");
  await expect(dialog).toBeVisible();
  return dialog;
}

async function createTaskFromDialog(options: {
  page: Page;
  dialog: Locator;
  mobile: boolean;
  taskTitle: string;
  content: string;
}): Promise<{ id: string; session_id: string }> {
  const { page, dialog, mobile, taskTitle, content } = options;
  const imageBytes = fs.readFileSync(
    path.join(process.cwd(), "public/web-app-manifest-192x192.png"),
  );
  const resourceBytes = Buffer.from("fresh-start attachment bytes\n", "utf8");
  await dialog.getByTestId("task-title-input").fill(taskTitle);
  await dialog
    .getByTestId("task-description-input")
    .fill(content || "Attachment-only fixture placeholder");
  await dialog.locator('input[type="file"]').setInputFiles([
    { name: "screen.png", mimeType: "image/png", buffer: imageBytes },
    { name: "notes.txt", mimeType: "text/plain", buffer: resourceBytes },
  ]);
  const responsePromise = page.waitForResponse(
    (response) =>
      response.url().endsWith("/api/v1/tasks") && response.request().method() === "POST",
  );
  const clearPlaceholder = async (route: Route) => {
    if (route.request().method() !== "POST" || !route.request().url().endsWith("/api/v1/tasks")) {
      await route.continue();
      return;
    }
    const request = route.request().postDataJSON() as Record<string, unknown>;
    await route.continue({ postData: JSON.stringify({ ...request, description: "" }) });
  };
  if (!content) await page.route("**/api/v1/tasks", clearPlaceholder);
  const submit = dialog.getByTestId("submit-start-agent");
  await expect(submit).toBeEnabled();
  await activate(submit, mobile);
  let response;
  try {
    response = await responsePromise;
  } finally {
    if (!content) await page.unroute("**/api/v1/tasks", clearPlaceholder);
  }
  expect(response.ok()).toBeTruthy();
  const created = (await response.json()) as { id: string; session_id: string };
  expect(created.session_id).toBeTruthy();
  if (mobile) await page.goto(`/t/${created.id}`);
  else await expect(page).toHaveURL(new RegExp(`/t/${created.id}(?:$|[/?])`));
  return created;
}

async function selectProfileAndWorkflow(options: {
  apiClient: ApiClient;
  seedData: SeedData;
  profileId: string;
}) {
  const { apiClient, seedData, profileId } = options;
  await apiClient.saveUserSettings({
    task_create_last_used: {
      repository_id: seedData.repositoryId,
      branch: "main",
      agent_profile_id: profileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
      workflow_ids_by_workspace: { [seedData.workspaceId]: seedData.workflowId },
    },
  });
}

async function createMockAgentProfile(apiClient: ApiClient, mobile: boolean, tracePath: string) {
  const { agents } = await apiClient.listAgents();
  const mockAgent = agents.find((agent) => agent.name === "mock-agent");
  if (!mockAgent) throw new Error("mock-agent is unavailable in the E2E profile");
  return apiClient.createAgentProfile(
    mockAgent.id,
    `Fresh start ${mobile ? "phone" : "desktop"} ${Date.now()}`,
    {
      model: "mock-fast",
      mode: "default",
      auto_fallback: false,
      require_exact_model: true,
      env_vars: [
        { key: "E2E_MOCK_AGENT_ACP_TRACE_FILE", value: tracePath },
        { key: "E2E_MOCK_AGENT_FAIL_INITIALIZE", value: "true" },
      ],
    },
  );
}

function createSubmissionContent(attachmentOnly: boolean) {
  return attachmentOnly ? "" : "Inspect screen.png and notes.txt, then describe what you received.";
}

async function inspectFailedStartup(options: {
  page: Page;
  apiClient: ApiClient;
  taskId: string;
  sessionId: string;
  mobile: boolean;
  content: string;
}) {
  const { page, apiClient, taskId, sessionId, mobile, content } = options;
  await waitForSessionState(apiClient, {
    taskId,
    sessionId,
    expectedState: "FAILED",
    message: "An unadvertised saved model must fail bootstrap before prompt dispatch",
    timeout: 60_000,
  });
  await page.goto(`/t/${taskId}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await expect(session.activeChat().getByTestId("session-recovery-card")).toBeVisible();
  await expect(session.recoveryFreshButton()).toBeVisible();
  const { sessions } = await apiClient.listTaskSessions(taskId);
  const captured = sessions.find((entry) => entry.id === sessionId);
  if (!captured) throw new Error("initial recovery session was not persisted");
  const submissionValue = captured.metadata?.initial_prompt_submission;
  expect(submissionValue).toMatchObject({ content, state: "pending" });
  const submission = submissionValue as {
    content?: string;
    attachments?: Array<{ attachment_id?: string; name?: string; delivery_mode?: string }>;
  };
  expect(submission.attachments).toEqual(
    expect.arrayContaining([
      expect.objectContaining({ name: "screen.png" }),
      expect.objectContaining({ name: "notes.txt", delivery_mode: "path" }),
    ]),
  );
  const imageAttachmentId = submission.attachments?.find(
    (attachment) => attachment.name === "screen.png",
  )?.attachment_id;
  const resourceAttachmentId = submission.attachments?.find(
    (attachment) => attachment.name === "notes.txt",
  )?.attachment_id;
  const error = captured.metadata?.last_agent_error as { stamp?: string } | undefined;
  if (!imageAttachmentId || !resourceAttachmentId || !error?.stamp) {
    throw new Error("failed startup is missing its original attachment or warning identity");
  }
  if (mobile) {
    const buttonBox = await session.recoveryFreshButton().boundingBox();
    expect(buttonBox?.height).toBeGreaterThanOrEqual(44);
  }
  return { imageAttachmentId, resourceAttachmentId, errorStamp: error.stamp };
}

async function createFreshStartFixture(options: {
  page: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  mobile: boolean;
  attachmentOnly: boolean;
  tracePath: string;
  onProfileCreated: (profileId: string) => void;
  onTaskCreated: (taskId: string) => void;
}): Promise<RecoveryFixture> {
  const {
    page,
    apiClient,
    seedData,
    mobile,
    attachmentOnly,
    tracePath,
    onProfileCreated,
    onTaskCreated,
  } = options;
  const profile = await createMockAgentProfile(apiClient, mobile, tracePath);
  onProfileCreated(profile.id);
  await selectProfileAndWorkflow({ apiClient, seedData, profileId: profile.id });
  const taskTitle = `Fresh start attachment replay ${Date.now()}`;
  const content = createSubmissionContent(attachmentOnly);
  const imageBytes = fs.readFileSync(
    path.join(process.cwd(), "public/web-app-manifest-192x192.png"),
  );
  const resourceBytes = Buffer.from("fresh-start attachment bytes\n", "utf8");
  const dialog = await openTaskCreateDialog(page, mobile);
  const created = await createTaskFromDialog({ page, dialog, mobile, taskTitle, content });
  onTaskCreated(created.id);
  const identities = await inspectFailedStartup({
    page,
    apiClient,
    taskId: created.id,
    sessionId: created.session_id,
    mobile,
    content,
  });

  return {
    taskId: created.id,
    sessionId: created.session_id,
    profileId: profile.id,
    ...identities,
    content,
    imageBytes,
    resourceBytes,
  };
}

async function assertUnavailableAttachment(
  context: ScenarioContext,
  storedAttachment: ReturnType<typeof removeStoredAttachment>,
) {
  const { page, apiClient, fixture, mobile, tracePath, session, recovery } = context;
  await page.goto(`/t/${fixture.taskId}`);
  await session.waitForLoad();
  await expect(session.recoveryFreshButton()).toBeVisible();
  await activate(session.recoveryFreshButton(), mobile);
  await expect
    .poll(() => recovery.requestCounts.fresh_start ?? 0, {
      message: "the attachment preflight refusal should come from the fresh-start request",
    })
    .toBe(1);
  await expect
    .poll(
      () => capturedSessionRecoveryResponse(recovery.requestIds, recovery.responses, "fresh_start"),
      { message: "the unavailable attachment recovery request should settle" },
    )
    .toBeDefined();
  const refusal = capturedSessionRecoveryResponse(
    recovery.requestIds,
    recovery.responses,
    "fresh_start",
  );
  expect(
    JSON.stringify(refusal?.payload),
    `unavailable attachment refusal response: ${JSON.stringify(refusal)}`,
  ).toContain("original task attachments are unavailable");
  await expect(session.recoveryError()).toBeVisible();
  if (mobile) {
    const buttonBox = await session.recoveryFreshButton().boundingBox();
    expect(buttonBox?.height).toBeGreaterThanOrEqual(44);
  }
  await waitForSessionState(apiClient, {
    taskId: fixture.taskId,
    sessionId: fixture.sessionId,
    expectedState: "FAILED",
    message: "unavailable attachment preflight must leave the failed session unchanged",
  });
  expect(readTrace(tracePath).filter((event) => event.event === "prompt")).toHaveLength(0);
  fs.writeFileSync(storedAttachment.storedPath, storedAttachment.bytes);
}

async function restoreWorkspaceReadOnly(context: ScenarioContext) {
  const { apiClient, fixture, mobile, session } = context;
  await activate(session.recoveryRestoreWorkspaceButton(), mobile);
  await expect(session.activeChat().getByTestId("session-recovery-workspace-status")).toContainText(
    "read-only mode",
  );
  await waitForSessionState(apiClient, {
    taskId: fixture.taskId,
    sessionId: fixture.sessionId,
    expectedState: "FAILED",
    message: "read-only restoration must retain the agent startup error",
  });
  await expect(session.activeChat().getByTestId("session-recovery-card")).toBeVisible();
}

async function startFreshAfterRestore(context: ScenarioContext) {
  const { apiClient, fixture, mobile, recovery, session, tracePath } = context;
  await apiClient.updateAgentProfile(fixture.profileId, {
    model: "mock-fast",
    env_vars: [{ key: "E2E_MOCK_AGENT_ACP_TRACE_FILE", value: tracePath }],
  });
  await activate(session.recoveryFreshButton(), mobile);
  await expect.poll(() => recovery.requestCounts.fresh_start ?? 0).toBe(2);
  await expect
    .poll(
      () => capturedSessionRecoveryResponse(recovery.requestIds, recovery.responses, "fresh_start"),
      { timeout: 60_000, message: "the successful fresh-start request should settle" },
    )
    .toBeDefined();
  await waitForSessionState(apiClient, {
    taskId: fixture.taskId,
    sessionId: fixture.sessionId,
    expectedState: "WAITING_FOR_INPUT",
    message: "fresh start must replay the captured submission into the same session",
    timeout: 60_000,
  });
}

async function assertReplayDelivery(context: ScenarioContext) {
  const { apiClient, fixture, tracePath } = context;
  await expect
    .poll(() => readTrace(tracePath).filter((event) => event.event === "prompt"), {
      timeout: 60_000,
      message: "the mock ACP agent must receive the replayed request",
    })
    .toHaveLength(1);
  const trace = readTrace(tracePath).find((event) => event.event === "prompt");
  expect(trace?.session_id).toBeTruthy();
  if (fixture.content) expect(trace?.prompt).toContain(fixture.content);
  else expect(trace?.prompt).not.toContain("Inspect screen.png and notes.txt");
  expect(trace?.prompt).toContain("notes.txt");
  const blocks = JSON.parse(trace?.prompt_blocks ?? "[]") as TraceBlock[];
  expect(blocks.find((block) => block.type === "image")).toMatchObject({
    mime_type: "image/png",
    byte_length: fixture.imageBytes.byteLength,
    sha256: createHash("sha256").update(fixture.imageBytes).digest("hex"),
  });
  const relativePath = requireValue(
    trace?.prompt?.match(/saved to ([^\s)]+) in the workspace/)?.[1],
    "mock agent did not report the materialized resource path",
  );
  expect(relativePath).toMatch(/\.kandev\/attachments\//);
  const { sessions } = await apiClient.listTaskSessions(fixture.taskId);
  const recovered = sessions.find((entry) => entry.id === fixture.sessionId);
  const workspacePath = requireValue(
    recovered?.workspace_path,
    "recovered workspace path is missing",
  );
  expect(fs.readFileSync(path.resolve(workspacePath, relativePath))).toEqual(fixture.resourceBytes);
  return recovered;
}

async function assertOneOriginalMessage(
  context: ScenarioContext,
  recovered: ListedSession | undefined,
) {
  const { apiClient, fixture } = context;
  const { messages } = await apiClient.listSessionMessages(fixture.sessionId);
  const userMessages = messages.filter((message) => message.author_type === "user");
  expect(userMessages).toHaveLength(1);
  expect(userMessages[0].content).toBe(fixture.content);
  expect(userMessages[0].metadata?.plan_mode).not.toBe(true);
  expect(userMessages[0].metadata?.attachments).toEqual(
    expect.arrayContaining([
      expect.objectContaining({ name: "screen.png" }),
      expect.objectContaining({ name: "notes.txt", delivery_mode: "path" }),
    ]),
  );
  expect(recovered?.metadata?.initial_prompt_submission).toMatchObject({ state: "accepted" });
  const lastError = recovered?.metadata?.last_agent_error as
    | { stamp?: string; message?: string; dismissed_at?: string }
    | undefined;
  expect(lastError?.stamp).toBe(fixture.errorStamp);
  expect(lastError?.message).toBeTruthy();
  expect(lastError?.dismissed_at).toBeTruthy();
}

async function assertHistoryAndDraft(context: ScenarioContext) {
  const { page, mobile, session } = context;
  await page.reload();
  await session.waitForLoad();
  await expect(session.recoveryFreshButton()).toHaveCount(0);
  await expect(
    page.getByTestId("session-recovery-history").first().getByTestId("session-recovery-resolved"),
  ).toBeVisible();
  await expect(page.getByTestId("user-message-bubble")).toHaveCount(1);
  if (mobile) await assertNoDocumentHorizontalOverflow(page, "fresh-start recovery");
  const editor = session
    .activeChat()
    .locator('.tiptap.ProseMirror[contenteditable="true"]:visible')
    .first();
  await expect(editor).toBeEditable();
  if (mobile) expect((await editor.boundingBox())?.height).toBeGreaterThanOrEqual(44);
  await editor.fill("Keep this draft after fresh recovery");
  const uploadResponse = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" && response.url().includes("/api/v1/attachments"),
    { timeout: 15_000 },
  );
  await page
    .getByTestId("session-chat")
    .locator('input[type="file"]')
    .setInputFiles({
      name: "recovery-draft.txt",
      mimeType: "text/plain",
      buffer: Buffer.from("preserve selected attachment", "utf8"),
    });
  expect((await uploadResponse).ok()).toBe(true);
  await expect
    .poll(
      () =>
        page.evaluate((sessionId) => {
          const raw = sessionStorage.getItem(`kandev.chatDraft.attachments.${sessionId}`);
          if (!raw) return false;
          try {
            const attachments = JSON.parse(raw) as Array<{
              attachmentId?: string;
              fileName?: string;
            }>;
            return attachments.some(
              (attachment) =>
                attachment.fileName === "recovery-draft.txt" && Boolean(attachment.attachmentId),
            );
          } catch {
            return false;
          }
        }, context.fixture.sessionId),
      { timeout: 15_000, message: "Wait for the composer attachment upload to be saved" },
    )
    .toBe(true);
  await expect(page.getByText("recovery-draft.txt", { exact: false })).toBeVisible();
  await page.reload();
  await session.waitForLoad();
  const restoredEditor = session.activeChat().locator(".tiptap.ProseMirror:visible").first();
  await expect(restoredEditor).toBeEditable();
  await expect(restoredEditor).toContainText("Keep this draft after fresh recovery");
  await expect(page.getByText("recovery-draft.txt", { exact: false })).toBeVisible();
}

async function cleanupScenario(options: {
  apiClient: ApiClient;
  settings: UserSettings;
  taskId?: string;
  profileId?: string;
  tracePath: string;
  storedAttachment?: ReturnType<typeof removeStoredAttachment>;
}) {
  const { apiClient, settings, taskId, profileId, tracePath, storedAttachment } = options;
  const errors: unknown[] = [];
  const clean = async (operation: () => Promise<void> | void) => {
    try {
      await operation();
    } catch (error) {
      errors.push(error);
    }
  };
  if (storedAttachment) {
    await clean(() => fs.writeFileSync(storedAttachment.storedPath, storedAttachment.bytes));
  }
  if (taskId)
    await clean(async () => apiClient.deleteTask(taskId, { discardWorktreeChanges: true }));
  if (profileId) await clean(async () => apiClient.deleteAgentProfile(profileId, true));
  await clean(async () =>
    apiClient.saveUserSettings({
      task_create_last_used:
        (settings.task_create_last_used as Parameters<
          ApiClient["saveUserSettings"]
        >[0]["task_create_last_used"]) ?? {},
    }),
  );
  await clean(() => fs.rmSync(tracePath, { force: true }));
  if (errors.length > 0) throw errors[0];
}

export function freshStartSubmissionRecoveryScenario(mobile: boolean, attachmentOnly = false) {
  return async (
    {
      testPage,
      apiClient,
      seedData,
      backend,
    }: {
      testPage: Page;
      apiClient: ApiClient;
      seedData: SeedData;
      backend: BackendContext;
    },
    testInfo: TestInfo,
  ) => {
    testInfo.setTimeout(150_000);
    const tracePath = path.join(backend.tmpDir, `fresh-start-replay-${Date.now()}.jsonl`);
    const recovery = captureSessionRecoveryMessages(testPage);
    const { settings } = await apiClient.getUserSettings();
    let fixture: RecoveryFixture | undefined;
    let profileId: string | undefined;
    let taskId: string | undefined;
    let storedAttachment: ReturnType<typeof removeStoredAttachment> | undefined;
    try {
      fixture = await createFreshStartFixture({
        page: testPage,
        apiClient,
        seedData,
        mobile,
        attachmentOnly,
        tracePath,
        onProfileCreated: (id) => (profileId = id),
        onTaskCreated: (id) => (taskId = id),
      });
      profileId = fixture.profileId;
      taskId = fixture.taskId;
      storedAttachment = removeStoredAttachment(backend, fixture.imageAttachmentId);
      const context: ScenarioContext = {
        page: testPage,
        apiClient,
        backend,
        fixture,
        mobile,
        tracePath,
        session: new SessionPage(testPage),
        recovery,
      };
      await assertUnavailableAttachment(context, storedAttachment);
      storedAttachment = undefined;
      await restoreWorkspaceReadOnly(context);
      await startFreshAfterRestore(context);
      const recovered = await assertReplayDelivery(context);
      await assertOneOriginalMessage(context, recovered);
      await assertHistoryAndDraft(context);
    } finally {
      await cleanupScenario({
        apiClient,
        settings,
        taskId,
        profileId,
        tracePath,
        storedAttachment,
      });
    }
  };
}
