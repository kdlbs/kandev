import { expect, type Locator, type Page, type TestInfo } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import type { PrAssetCapture } from "./pr-asset-capture";
import { DatabaseSync } from "./node-sqlite";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";
import {
  captureSessionRecoveryMessages,
  capturedSessionRecoveryRequest,
  capturedSessionRecoveryResponseType,
} from "./session-resume-recovery";
import { waitForFiniteAnimations } from "./pr-capture";
import { waitForSessionState } from "./session";
import { SessionPage } from "../pages/session-page";
import { readSessionRuntimeIdentity } from "./session-resume-prompt-queue";

const EMPTY_TURN_NO_OUTPUT_NOTICE = "The agent finished without producing any output.";
const EMPTY_TURN_COMMAND_HINT = /without the leading slash/i;

type ACPTraceEvent = {
  event: string;
  session_id: string;
  mode_id?: string;
  config_id?: string;
  value?: string;
  prompt?: string;
};

type SqliteTestDatabase = {
  exec(sql: string): void;
  prepare(sql: string): {
    get(...params: unknown[]): unknown;
    run(...params: unknown[]): unknown;
  };
  close(): void;
};

function captureRecoveryWireFrames(page: Page) {
  const frames: Array<{ direction: "sent" | "received"; payload: string }> = [];
  page.on("websocket", (socket) => {
    if (!socket.url().endsWith("/ws")) return;
    socket.on("framesent", ({ payload }) => {
      frames.push({ direction: "sent", payload: String(payload) });
    });
    socket.on("framereceived", ({ payload }) => {
      frames.push({ direction: "received", payload: String(payload) });
    });
  });
  return frames;
}

type RecoveryWireFrame = {
  id?: string;
  action?: string;
  type?: string;
  payload?: unknown;
};

function parseRecoveryWireFrame(payload: string): RecoveryWireFrame | undefined {
  try {
    const frame = JSON.parse(payload) as unknown;
    return frame && typeof frame === "object" ? (frame as RecoveryWireFrame) : undefined;
  } catch {
    return undefined;
  }
}

function capturedRestoreWorkspaceResponse(
  frames: Array<{ direction: "sent" | "received"; payload: string }>,
  sessionId: string,
) {
  const requests = frames
    .filter((frame) => frame.direction === "sent")
    .map((frame) => parseRecoveryWireFrame(frame.payload))
    .filter(
      (frame): frame is RecoveryWireFrame & { id: string } =>
        frame?.type === "request" &&
        frame.action === "session.launch" &&
        typeof frame.id === "string" &&
        typeof frame.payload === "object" &&
        frame.payload !== null &&
        (frame.payload as { session_id?: unknown; intent?: unknown }).session_id === sessionId &&
        (frame.payload as { intent?: unknown }).intent === "restore_workspace",
    );
  const request = requests.at(-1);
  if (!request) return undefined;
  return frames
    .filter((frame) => frame.direction === "received")
    .map((frame) => parseRecoveryWireFrame(frame.payload))
    .find(
      (frame) => frame?.id === request.id && (frame.type === "response" || frame.type === "error"),
    );
}

async function attachRecoveryDiagnostics({
  testInfo,
  page,
  backend,
  tracePath,
  frames,
  taskId,
  sessionId,
}: {
  testInfo: TestInfo;
  page: Page;
  backend: BackendContext;
  tracePath: string;
  frames: Array<{ direction: "sent" | "received"; payload: string }>;
  taskId: string;
  sessionId: string;
}) {
  const diagnosticsDir = testInfo.outputPath("settings-recovery-diagnostics");
  fs.mkdirSync(diagnosticsDir, { recursive: true });
  const pageText = await page
    .locator("body")
    .innerText()
    .catch(() => "Unable to read page text");
  const appState = await page
    .evaluate(
      ({ taskId, sessionId }) => {
        type RecoveryState = {
          taskSessions?: { items?: Record<string, Record<string, unknown>> };
          kanban?: { tasks?: Array<Record<string, unknown>> };
        };
        const store = (
          window as Window & {
            __KANDEV_E2E_STORE__?: { getState: () => unknown };
          }
        ).__KANDEV_E2E_STORE__;
        const state = store?.getState() as RecoveryState | undefined;
        const session = state?.taskSessions?.items?.[sessionId];
        const task = state?.kanban?.tasks?.find((item) => item.id === taskId);
        const metadata = session?.metadata as Record<string, unknown> | undefined;
        const statusSummary = task?.statusSummary as Record<string, unknown> | undefined;
        return {
          session: session
            ? {
                state: session.state,
                error_message: session.error_message,
                last_agent_error: metadata?.last_agent_error,
              }
            : null,
          task: task
            ? {
                active_session_id: task.activeSessionId,
                active_error: statusSummary?.active_error,
              }
            : null,
        };
      },
      { taskId, sessionId },
    )
    .catch((error: unknown) => ({ error: String(error) }));
  const screenshotPath = testInfo.outputPath("settings-recovery-failure.png");
  await page.screenshot({ path: screenshotPath, fullPage: true }).catch(() => undefined);
  const attachments = [
    {
      name: "mock-acp-trace",
      filename: "mock-acp-trace.jsonl",
      contentType: "application/jsonl",
      contents: fs.existsSync(tracePath)
        ? fs.readFileSync(tracePath)
        : Buffer.from("ACP trace is missing"),
    },
    {
      name: "session-recovery-ws-frames",
      filename: "session-recovery-ws-frames.json",
      contentType: "application/json",
      contents: Buffer.from(JSON.stringify(frames.slice(-500), null, 2)),
    },
    {
      name: "backend-log-tail",
      filename: "backend-log-tail.log",
      contentType: "text/plain",
      contents: fs.existsSync(backend.logPath)
        ? fs.readFileSync(backend.logPath).subarray(-2_000_000)
        : Buffer.from("Backend log is missing"),
    },
    {
      name: "recovery-page-text",
      filename: "recovery-page-text.txt",
      contentType: "text/plain",
      contents: Buffer.from(pageText),
    },
    {
      name: "recovery-store-projection",
      filename: "recovery-store-projection.json",
      contentType: "application/json",
      contents: Buffer.from(JSON.stringify(appState, null, 2)),
    },
  ];
  for (const attachment of attachments) {
    const filepath = path.join(diagnosticsDir, attachment.filename);
    fs.writeFileSync(filepath, attachment.contents);
    await testInfo.attach(attachment.name, { path: filepath, contentType: attachment.contentType });
  }
  await testInfo.attach("recovery-page-screenshot", {
    path: screenshotPath,
    contentType: "image/png",
  });
}

function readACPTrace(tracePath: string): ACPTraceEvent[] {
  if (!fs.existsSync(tracePath)) return [];
  return fs
    .readFileSync(tracePath, "utf8")
    .split("\n")
    .filter(Boolean)
    .map((line) => JSON.parse(line) as ACPTraceEvent);
}

function readTaskSessionMetadata(tmpDir: string, sessionId: string): Record<string, unknown> {
  const db = new DatabaseSync(path.join(tmpDir, "kandev.db")) as unknown as SqliteTestDatabase;
  try {
    const row = db.prepare("SELECT metadata FROM task_sessions WHERE id = ?").get(sessionId) as
      | { metadata?: unknown }
      | undefined;
    if (!row) throw new Error(`E2E task session ${sessionId} was not found`);
    if (typeof row.metadata === "string") {
      const metadata = JSON.parse(row.metadata) as unknown;
      return metadata && typeof metadata === "object" && !Array.isArray(metadata)
        ? (metadata as Record<string, unknown>)
        : {};
    }
    return row.metadata && typeof row.metadata === "object" && !Array.isArray(row.metadata)
      ? (row.metadata as Record<string, unknown>)
      : {};
  } finally {
    db.close();
  }
}

function readLatestProviderRestoredNoticeMetadata(
  tmpDir: string,
  sessionId: string,
): Record<string, unknown> | undefined {
  const db = new DatabaseSync(path.join(tmpDir, "kandev.db")) as unknown as SqliteTestDatabase;
  try {
    const row = db
      .prepare(
        `SELECT metadata FROM task_session_messages
         WHERE task_session_id = ? AND type = 'status'
           AND metadata LIKE '%resume_settings_provider_restored%'
         ORDER BY created_at DESC LIMIT 1`,
      )
      .get(sessionId) as { metadata?: unknown } | undefined;
    if (!row) return undefined;
    const metadata = typeof row.metadata === "string" ? JSON.parse(row.metadata) : row.metadata;
    return metadata && typeof metadata === "object" && !Array.isArray(metadata)
      ? (metadata as Record<string, unknown>)
      : undefined;
  } finally {
    db.close();
  }
}

async function expectProviderRestoredResumeNotice(
  page: Page,
  tmpDir: string,
  sessionId: string,
  expectedCount: number,
) {
  const notice = page.getByTestId("provider-restored-resume-success");
  await expect(notice).toHaveCount(expectedCount);
  const latestNotice = notice.last();
  const metadata = readLatestProviderRestoredNoticeMetadata(tmpDir, sessionId);
  expect(metadata).toBeDefined();
  if (metadata?.effective_model_known === true) {
    const model = metadata.effective_model_name ?? metadata.effective_model_id;
    if (typeof model !== "string" || model === "") {
      throw new Error("confirmed provider model metadata must include a display value");
    }
    await expect(latestNotice).toContainText(`Session resumed with ${model}.`);
  } else {
    await expect(latestNotice).toContainText(
      "Session resumed. Your previous conversation was preserved.",
    );
  }
  await expect(latestNotice).toContainText("Your previous conversation was preserved.");
  await expect(latestNotice).toContainText("Saved mode and model selections remain unchanged");
  await expect(latestNotice).toContainText("restored permission mode may differ");
}

function seedUnadvertisedSessionRuntimeOverrides(tmpDir: string, sessionId: string) {
  const metadata = readTaskSessionMetadata(tmpDir, sessionId);
  const db = new DatabaseSync(path.join(tmpDir, "kandev.db")) as unknown as SqliteTestDatabase;
  try {
    const overrides = {
      model: "unlisted-e2e-model",
      mode: "unlisted-e2e-mode",
    };
    metadata.runtime_config_overrides = overrides;
    db.prepare("UPDATE task_sessions SET metadata = ? WHERE id = ?").run(
      JSON.stringify(metadata),
      sessionId,
    );
    return overrides;
  } finally {
    db.close();
  }
}

async function activate(locator: Locator, mobile: boolean) {
  if (mobile) await locator.tap();
  else await locator.click();
}

async function activateByKeyboard(page: Page, locator: Locator) {
  await expect(locator).toBeEnabled();
  const tabIndex = await locator.evaluate((element) => element.tabIndex);
  expect(tabIndex).toBeGreaterThanOrEqual(0);
  await locator.focus();
  await expect(locator).toBeFocused();
  await page.keyboard.press("Enter");
}

function sendRecoveryPrompt(session: SessionPage, mobile: boolean, prompt: string) {
  return mobile ? session.sendMessageViaButton(prompt) : session.sendMessage(prompt);
}

/** Exercise the explicit settings-omission path through a mock Auggie ACP peer. */
// eslint-disable-next-line complexity
export async function runSessionResumeSettingsRecoveryE2E(options: {
  page: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  backend: BackendContext;
  testInfo: TestInfo;
  mobile: boolean;
  prCapture?: PrAssetCapture;
  onMockAuggieEnabled?: () => void;
  onProfileCreated?: (profileId: string) => void;
  onTaskCreated?: (taskId: string) => void;
}) {
  const { page, apiClient, seedData, backend, mobile } = options;
  const wireFrames = captureRecoveryWireFrames(page);
  const tracePath = path.join(
    backend.tmpDir,
    `settings-recovery-${mobile ? "mobile" : "desktop"}.jsonl`,
  );
  let taskId = "";
  let sessionId = "";
  try {
    options.onMockAuggieEnabled?.();
    await backend.restart({ KANDEV_E2E_MOCK_AUGGIE: "true" });

    const { agents } = await apiClient.listAgents();
    const auggie = agents.find((agent) => agent.name === "auggie");
    if (!auggie) throw new Error("E2E mock Auggie provider was not registered");
    const profile = await apiClient.createAgentProfile(
      auggie.id,
      `Settings recovery ${mobile ? "mobile" : "desktop"} ${Date.now()}`,
      {
        model: "mock-fast",
        mode: "default",
        auto_fallback: false,
        env_vars: [{ key: "E2E_MOCK_AGENT_ACP_TRACE_FILE", value: tracePath }],
        cli_flags: [
          {
            description: "Delay native session load so recovery busy state can be asserted",
            flag: "--delay-resume=1500ms",
            enabled: true,
          },
        ],
      },
    );
    options.onProfileCreated?.(profile.id);

    const recovery = captureSessionRecoveryMessages(page);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      `Explicit settings recovery ${mobile ? "mobile" : "desktop"} ${Date.now()}`,
      profile.id,
      {
        description: "Continue this same provider conversation after recovery.",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    sessionId = task.session_id ?? "";
    taskId = task.id;
    options.onTaskCreated?.(task.id);
    if (!sessionId) throw new Error("settings recovery task has no session_id");

    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId,
      expectedState: "WAITING_FOR_INPUT",
      message: "A valid Auggie session should establish its native conversation first",
      timeout: 60_000,
    });
    const originalIdentity = await readSessionRuntimeIdentity(apiClient, task.id, sessionId);
    expect(originalIdentity.acpSessionId).toBeTruthy();
    const savedOverrides = seedUnadvertisedSessionRuntimeOverrides(backend.tmpDir, sessionId);

    await page.goto(`/t/${task.id}`);
    const session = new SessionPage(page);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 60_000 });
    const stopResponse = await apiClient.stopSession({
      session_id: sessionId,
      reason: "Prepare strict resume after preserving the native conversation",
      force: true,
    });
    expect(stopResponse.success).toBe(true);
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId,
      expectedState: "CANCELLED",
      message: "The native session should stop without replacing its conversation",
      timeout: 60_000,
    });

    const beforeStrictResumeLoads = readACPTrace(tracePath).filter(
      (event) => event.event === "session_load",
    ).length;
    try {
      await apiClient.launchSession({ task_id: task.id, session_id: sessionId }, 60_000);
    } catch {
      // A strict settings failure may be returned synchronously or reported after
      // the launch request has been accepted; the provider trace and FAILED state
      // below prove the ordinary resume reached the existing ACP conversation.
    }
    await expect
      .poll(
        () => readACPTrace(tracePath).filter((event) => event.event === "session_load").length,
        {
          timeout: 60_000,
          message: "the ordinary strict resume should load the existing ACP session",
        },
      )
      .toBeGreaterThan(beforeStrictResumeLoads);
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId,
      expectedState: "FAILED",
      message: "The unadvertised saved model and mode must fail ordinary strict resume",
      timeout: 60_000,
    });

    const startupRecoveryCard = session.activeChat().getByTestId("session-recovery-card");
    await expect(
      startupRecoveryCard.getByRole("heading", { name: "Saved model unavailable" }),
    ).toBeVisible();
    await expect(startupRecoveryCard).toContainText("unlisted-e2e-model");
    await expect(startupRecoveryCard.getByTestId("session-bootstrap-no-prompt")).toBeVisible();
    await expect(
      startupRecoveryCard.getByTestId("session-recovery-fresh-start-warning"),
    ).toContainText("uses your saved selections");
    await expect(session.activeChat()).not.toContainText(EMPTY_TURN_NO_OUTPUT_NOTICE);
    await page.reload();
    await session.waitForLoad();
    await expect(
      session.activeChat().getByRole("heading", { name: "Saved model unavailable" }),
    ).toBeVisible();
    await expect(session.activeChat()).not.toContainText(EMPTY_TURN_NO_OUTPUT_NOTICE);
    await expect(session.recoveryResumeButton()).toBeVisible({ timeout: 30_000 });
    const disclosure = session.activeChat().getByTestId("provider-restored-resume-disclosure");
    await expect(disclosure).toBeVisible();
    await expect(disclosure).toContainText("mode/model");
    await assertNoDocumentHorizontalOverflow(page, "provider-restored resume disclosure");
    if (options.prCapture?.capturing) {
      await waitForFiniteAnimations(session.activeChat().getByTestId("session-recovery-card"));
      await options.prCapture.screenshot("provider-restored-disclosure", {
        caption:
          "Resume keeps the native conversation while skipping saved mode and model overrides",
        fullPage: true,
      });
    }

    const restoreWorkspace = session.recoveryRestoreWorkspaceButton();
    await expect(restoreWorkspace).toBeVisible();
    const restoreResponsesBefore = wireFrames.filter(
      (frame) =>
        frame.direction === "sent" &&
        parseRecoveryWireFrame(frame.payload)?.action === "session.launch" &&
        (parseRecoveryWireFrame(frame.payload)?.payload as { intent?: unknown } | undefined)
          ?.intent === "restore_workspace",
    ).length;
    await activate(restoreWorkspace, mobile);
    const recoveryCard = session.activeChat().getByTestId("session-recovery-card");
    await expect
      .poll(
        () =>
          wireFrames.filter(
            (frame) =>
              frame.direction === "sent" &&
              parseRecoveryWireFrame(frame.payload)?.action === "session.launch" &&
              (parseRecoveryWireFrame(frame.payload)?.payload as { intent?: unknown } | undefined)
                ?.intent === "restore_workspace",
          ).length,
        { timeout: 15_000, message: "read-only recovery should dispatch session.launch" },
      )
      .toBeGreaterThan(restoreResponsesBefore);
    await expect
      .poll(() => capturedRestoreWorkspaceResponse(wireFrames, sessionId), {
        timeout: 60_000,
        message: "read-only recovery should receive a session.launch response",
      })
      .toMatchObject({ type: "response" });
    await expect(
      recoveryCard.getByRole("heading", { name: "Saved model unavailable" }),
    ).toBeVisible();
    await expect(recoveryCard.getByTestId("session-recovery-workspace-status")).toContainText(
      "read-only mode",
      { timeout: 15_000 },
    );
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId,
      expectedState: "FAILED",
      message: "read-only recovery should settle before Resume is requested",
      timeout: 60_000,
    });
    if (options.prCapture?.capturing) {
      await waitForFiniteAnimations(recoveryCard);
      await options.prCapture.screenshot("read-only-recovery-card", {
        caption: "The read-only recovery result appears once while Resume remains available",
        fullPage: true,
      });
    }

    if (mobile) {
      expect((await session.recoveryResumeButton().boundingBox())?.height).toBeGreaterThanOrEqual(
        44,
      );
      expect((await restoreWorkspace.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    }

    await apiClient.updateAgentProfile(profile.id, {
      cli_flags: [
        {
          description: "Reject one provider session load to verify recovery error handling",
          flag: "--fail-on-resume",
          enabled: true,
        },
        {
          description: "Delay native session load so recovery busy state can be asserted",
          flag: "--delay-resume=1500ms",
          enabled: true,
        },
      ],
    });
    await activate(session.recoveryResumeButton(), mobile);
    await expect
      .poll(
        () =>
          capturedSessionRecoveryResponseType(recovery.requestIds, recovery.responses, "resume"),
        {
          timeout: 60_000,
          message: "the provider load rejection should be returned to the recovery UI",
        },
      )
      .toBe("error");
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId,
      expectedState: "FAILED",
      message: "the rejected provider load should settle before Resume is retried",
      timeout: 60_000,
    });
    await expect(recoveryCard.getByRole("heading")).toHaveText(
      /^(Saved model unavailable|Session startup needs attention)$/,
    );
    await expect(session.recoveryResumeButton()).toBeVisible();
    await expect(session.activeChat().getByTestId("provider-restored-resume-success")).toHaveCount(
      0,
    );

    await apiClient.updateAgentProfile(profile.id, {
      cli_flags: [
        {
          description: "Delay native session load so recovery busy state can be asserted",
          flag: "--delay-resume=1500ms",
          enabled: true,
        },
      ],
    });
    if (mobile) await activate(session.recoveryResumeButton(), true);
    else await activateByKeyboard(page, session.recoveryResumeButton());
    const recoveryActions = session.recoveryResumeButton().locator("xpath=..");
    await expect(recoveryActions).toHaveAttribute("aria-busy", "true");
    await expect(session.recoveryResumeButton()).toBeDisabled();
    if (!mobile) {
      await page.keyboard.press("Enter");
    }
    await expect.poll(() => recovery.requestCounts.resume ?? 0).toBe(2);
    await expect
      .poll(
        () => {
          const requestId = recovery.requestIds.resume;
          return requestId ? recovery.responses.get(requestId) : undefined;
        },
        {
          timeout: 60_000,
          message: "the explicit Resume request should settle",
        },
      )
      .toMatchObject({ type: "response" });
    expect(capturedSessionRecoveryRequest(recovery.requests, "resume")).toMatchObject({
      task_id: task.id,
      session_id: sessionId,
      action: "resume",
      settings_policy: "provider_restored",
    });
    await session.waitForChatIdle({ timeout: 60_000 });
    await expectProviderRestoredResumeNotice(page, backend.tmpDir, sessionId, 1);
    const firstSuccessNotice = session.activeChat().getByTestId("provider-restored-resume-success");
    const firstSuccessNoticeText = await firstSuccessNotice.locator("p").allTextContents();
    await expect(session.idleInput()).toBeVisible();
    await expect(recoveryCard).toHaveCount(0);

    const persistedOverrides = readTaskSessionMetadata(backend.tmpDir, sessionId)
      .runtime_config_overrides as typeof savedOverrides;
    expect(persistedOverrides).toEqual(savedOverrides);
    const resumedIdentity = await readSessionRuntimeIdentity(apiClient, task.id, sessionId);
    expect(resumedIdentity.acpSessionId).toBe(originalIdentity.acpSessionId);

    await sendRecoveryPrompt(session, mobile, "/e2e:simple-message");
    await session.expectChatResponseVisible("simple mock response", 0, { timeout: 60_000 });
    const modelSelector = page.getByRole("button", { name: "Session model settings" });
    await expect(modelSelector).toBeVisible();
    if (mobile) {
      expect((await modelSelector.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    }
    await activate(modelSelector, mobile);
    const modelOption = page.getByRole("option", { name: /Mock Smart/ });
    await expect(modelOption).toBeVisible();
    if (mobile) {
      expect((await modelOption.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    }
    await activate(modelOption, mobile);
    await expect(modelSelector).toContainText("Mock Smart", { timeout: 15_000 });

    const modeSelector = page.getByTestId("session-mode-selector");
    await expect(modeSelector).toBeVisible();
    if (mobile) {
      expect((await modeSelector.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    }
    await activate(modeSelector, mobile);
    const planMode = mobile
      ? page.getByTestId("session-mode-option-plan-mock")
      : page.getByRole("menuitem", { name: /Plan Mock/ });
    await expect(planMode).toBeVisible();
    if (mobile) {
      expect((await planMode.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    }
    await activate(planMode, mobile);
    await expect(modeSelector).toContainText("Plan Mock", { timeout: 15_000 });
    await expect(firstSuccessNotice.locator("p")).toHaveText(firstSuccessNoticeText);

    await expect
      .poll(() => readACPTrace(tracePath), {
        timeout: 15_000,
        message: "ACP selector requests should be recorded",
      })
      .toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            event: "set_config_option",
            session_id: expect.any(String),
            config_id: "model",
            value: "mock-smart",
          }),
          expect.objectContaining({
            event: "set_config_option",
            session_id: expect.any(String),
            config_id: "mode",
            value: "plan-mock",
          }),
        ]),
      );
    const trace = readACPTrace(tracePath);
    const sessionCreated = trace.find((event) => event.event === "session_new");
    const loadIndices = trace.flatMap((event, index) =>
      event.event === "session_load" ? [index] : [],
    );
    expect(sessionCreated).toBeDefined();
    expect(loadIndices.length).toBeGreaterThanOrEqual(2);
    expect(trace.filter((event) => event.event === "session_new")).toHaveLength(1);
    expect(
      trace.slice(0, Math.max(...loadIndices)).filter((event) => event.event === "session_new"),
    ).toHaveLength(1);
    const recoveryLoadIndex = loadIndices.at(-1)!;
    expect(trace[recoveryLoadIndex].session_id).toBe(sessionCreated!.session_id);
    const nextPromptIndex = trace.findIndex(
      (event, index) => index > recoveryLoadIndex && event.event === "prompt",
    );
    expect(nextPromptIndex).toBeGreaterThan(recoveryLoadIndex);
    const recoveryStartupSettings = trace.slice(recoveryLoadIndex + 1, nextPromptIndex);
    expect(
      recoveryStartupSettings.some(
        (event) =>
          event.event === "set_mode" ||
          (event.event === "set_config_option" &&
            (event.config_id === "model" || event.config_id === "mode")),
      ),
    ).toBe(false);
    expect(trace[nextPromptIndex].session_id).toBe(sessionCreated!.session_id);
    expect(trace[nextPromptIndex].prompt).toContain("/e2e:simple-message");
    expect(trace[nextPromptIndex].session_id).toBe(originalIdentity.acpSessionId);

    if (options.prCapture?.capturing) {
      const successNotice = session.activeChat().getByTestId("provider-restored-resume-success");
      await successNotice.scrollIntoViewIfNeeded();
      await expect(successNotice).toBeVisible();
      await expect(successNotice).toBeInViewport();
      await expect(modelSelector).toBeInViewport();
      await expect(modeSelector).toBeInViewport();
      await waitForFiniteAnimations(session.activeChat());
      await options.prCapture.screenshot("provider-restored-success", {
        caption:
          "The session continues on the same ACP conversation with working model and mode selectors",
        fullPage: true,
      });
    }

    await page.reload();
    await session.waitForLoad();
    await expectProviderRestoredResumeNotice(page, backend.tmpDir, sessionId, 1);
    expect(seedUnadvertisedSessionRuntimeOverrides(backend.tmpDir, sessionId)).toEqual(
      savedOverrides,
    );

    const beforeLaterStrictResumeLoads = readACPTrace(tracePath).filter(
      (event) => event.event === "session_load",
    ).length;
    const stopAfterRecovery = await apiClient.stopSession({
      session_id: sessionId,
      reason: "Verify a later strict resume failure keeps its own recovery controls",
      force: true,
    });
    expect(stopAfterRecovery.success).toBe(true);
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId,
      expectedState: "CANCELLED",
      message: "The recovered provider conversation should stop without losing its identity",
      timeout: 60_000,
    });
    try {
      await apiClient.launchSession({ task_id: task.id, session_id: sessionId }, 60_000);
    } catch {
      // Strict startup is expected to reject the same saved selections again.
    }
    await expect
      .poll(
        () => readACPTrace(tracePath).filter((event) => event.event === "session_load").length,
        {
          timeout: 60_000,
          message: "the later strict resume should load the preserved ACP conversation",
        },
      )
      .toBeGreaterThan(beforeLaterStrictResumeLoads);
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId,
      expectedState: "FAILED",
      message: "The later strict resume should fail on the unchanged saved selections",
      timeout: 60_000,
    });
    const laterRecoveryCard = session.activeChat().getByTestId("session-recovery-card");
    await expect(
      laterRecoveryCard.getByRole("heading", { name: "Saved model unavailable" }),
    ).toBeVisible();
    await expect(session.recoveryResumeButton()).toBeVisible();
    await expect(
      session.activeChat().getByTestId("session-recovery-resolved").first(),
    ).toBeVisible();
    await expectProviderRestoredResumeNotice(page, backend.tmpDir, sessionId, 1);
    expect(readTaskSessionMetadata(backend.tmpDir, sessionId).runtime_config_overrides).toEqual(
      savedOverrides,
    );
    expect((await readSessionRuntimeIdentity(apiClient, task.id, sessionId)).acpSessionId).toBe(
      originalIdentity.acpSessionId,
    );

    await activate(session.recoveryResumeButton(), mobile);
    await expect.poll(() => recovery.requestCounts.resume ?? 0).toBe(3);
    await expect
      .poll(
        () => {
          const requestId = recovery.requestIds.resume;
          return requestId ? recovery.responses.get(requestId) : undefined;
        },
        {
          timeout: 60_000,
          message: "provider-restored recovery should succeed again after the later strict failure",
        },
      )
      .toMatchObject({ type: "response" });
    await session.waitForChatIdle({ timeout: 60_000 });
    await expectProviderRestoredResumeNotice(page, backend.tmpDir, sessionId, 2);
    await expect(session.recoveryResumeButton()).toHaveCount(0);

    await sendRecoveryPrompt(session, mobile, "/e2e:empty-turn");
    await expect(session.activeChat()).toContainText(EMPTY_TURN_COMMAND_HINT, { timeout: 30_000 });
    await expect(session.activeChat()).not.toContainText(EMPTY_TURN_NO_OUTPUT_NOTICE);
    await assertNoDocumentHorizontalOverflow(page, "reloaded provider-restored recovery notice");
  } catch (error) {
    await attachRecoveryDiagnostics({
      testInfo: options.testInfo,
      page,
      backend,
      tracePath,
      frames: wireFrames,
      taskId,
      sessionId,
    }).catch(() => undefined);
    throw error;
  }
}
