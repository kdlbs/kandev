import { expect, type Page, type TestInfo } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { watchWs } from "../../helpers/causal-waits";
import { withDatabase } from "../../helpers/interrupted-prompt-recovery";
import { waitForSessionDone } from "../../helpers/session";
import {
  createFailOnResumeProfile,
  readSessionRuntimeIdentity,
} from "../../helpers/session-resume-prompt-queue";
import { SessionPage } from "../../pages/session-page";

type GatewayFrame = {
  id?: string;
  type?: string;
  action?: string;
  payload?: unknown;
};

type LaunchOutcome = {
  intent: string;
  responseType: "response" | "error";
  payload?: unknown;
};

type SessionLaunchObserver = {
  waitForIntent: (intent: string, timeoutMs?: number) => Promise<LaunchOutcome>;
};

type ReadOnlySessionStatus = {
  state: string;
  is_agent_running: boolean;
  is_resumable: boolean;
  needs_resume: boolean;
  needs_workspace_restore: boolean;
  acp_session_id?: string;
};

function parseGatewayFrame(payload: unknown): GatewayFrame | undefined {
  try {
    const value = JSON.parse(String(payload)) as unknown;
    return value && typeof value === "object" ? (value as GatewayFrame) : undefined;
  } catch {
    return undefined;
  }
}

function observeSessionLaunches(page: Page, sessionId: string): SessionLaunchObserver {
  const requests = new Map<string, string>();
  const outcomes: LaunchOutcome[] = [];
  const waiters = new Map<string, Set<(outcome: LaunchOutcome) => void>>();

  const observe = (direction: "sent" | "received", payload: unknown) => {
    const frame = parseGatewayFrame(payload);
    if (!frame || !frame.id) return;

    if (direction === "sent" && frame.type === "request") {
      if (frame.action !== "session.launch") return;
      const body = frame.payload as { session_id?: unknown; intent?: unknown } | undefined;
      if (body?.session_id === sessionId && typeof body.intent === "string") {
        requests.set(frame.id, body.intent);
      }
      return;
    }

    if (direction !== "received" || (frame.type !== "response" && frame.type !== "error")) {
      return;
    }
    const intent = requests.get(frame.id);
    if (!intent) return;
    requests.delete(frame.id);
    const outcome: LaunchOutcome = {
      intent,
      responseType: frame.type,
      payload: frame.payload,
    };
    outcomes.push(outcome);
    for (const resolve of waiters.get(intent) ?? []) resolve(outcome);
    waiters.delete(intent);
  };

  page.on("websocket", (socket) => {
    if (!socket.url().endsWith("/ws")) return;
    socket.on("framesent", ({ payload }) => observe("sent", payload));
    socket.on("framereceived", ({ payload }) => observe("received", payload));
  });

  return {
    waitForIntent: (intent, timeoutMs = 60_000) => {
      const existing = outcomes.find((outcome) => outcome.intent === intent);
      if (existing) return Promise.resolve(existing);
      return new Promise((resolve, reject) => {
        const timeout = setTimeout(() => {
          const listeners = waiters.get(intent);
          listeners?.delete(onOutcome);
          if (!listeners?.size) waiters.delete(intent);
          reject(new Error(`Timed out waiting for session.launch intent ${intent}`));
        }, timeoutMs);
        const onOutcome = (outcome: LaunchOutcome) => {
          clearTimeout(timeout);
          resolve(outcome);
        };
        const listeners = waiters.get(intent) ?? new Set<(outcome: LaunchOutcome) => void>();
        listeners.add(onOutcome);
        waiters.set(intent, listeners);
      });
    },
  };
}

function readPromptEvidence(tmpDir: string, sessionId: string) {
  return withDatabase(tmpDir, (db) => {
    const activeSubmissions = db
      .prepare(
        `SELECT COUNT(*) AS count FROM agent_delivery_submissions
         WHERE session_id = ? AND state IN ('prepared', 'accepted', 'dispatching', 'interrupted_unknown')`,
      )
      .get(sessionId) as { count: number };
    const unknownPromptBlocks = db
      .prepare(
        `SELECT COUNT(*) AS count FROM session_recovery_blocks
         WHERE session_id = ? AND reason = 'unknown_prompt_outcome'`,
      )
      .get(sessionId) as { count: number };
    const submissionCount = db
      .prepare("SELECT COUNT(*) AS count FROM agent_delivery_submissions WHERE session_id = ?")
      .get(sessionId) as { count: number };
    const execution = db
      .prepare(
        "SELECT agent_execution_id, resume_token, resumable FROM executors_running WHERE session_id = ?",
      )
      .get(sessionId) as
      | { agent_execution_id: string; resume_token: string; resumable: number }
      | undefined;
    return {
      activeSubmissions: activeSubmissions.count,
      unknownPromptBlocks: unknownPromptBlocks.count,
      submissionCount: submissionCount.count,
      execution,
    };
  });
}

async function attachReadOnlyRecoveryEvidence({
  api,
  page,
  testInfo,
  taskId,
  sessionId,
  originalAcpSessionId,
  resumeOutcome,
  restoreOutcome,
  tmpDir,
  expectedSubmissionCount,
}: {
  api: ApiClient;
  page: Page;
  testInfo: TestInfo;
  taskId: string;
  sessionId: string;
  originalAcpSessionId: string;
  resumeOutcome: LaunchOutcome;
  restoreOutcome: LaunchOutcome;
  tmpDir: string;
  expectedSubmissionCount: number;
}) {
  const status = await api.wsRequest<ReadOnlySessionStatus>("task.session.status", {
    task_id: taskId,
    session_id: sessionId,
  });
  const promptEvidence = readPromptEvidence(tmpDir, sessionId);
  const card = await page.evaluate(() => {
    const element = document.querySelector<HTMLElement>('[data-testid="session-recovery-card"]');
    if (!element) return null;
    const heading = element.querySelector("h3");
    return {
      visible: element.getClientRects().length > 0,
      rootRole: element.getAttribute("role"),
      heading: {
        text: heading?.textContent?.trim() ?? "",
        ariaLive: heading?.getAttribute("aria-live"),
        ariaAtomic: heading?.getAttribute("aria-atomic"),
      },
      statusCount: element.querySelectorAll('[role="status"]').length,
      readOnlyStatus: element
        .querySelector('[data-testid="session-recovery-workspace-status"]')
        ?.textContent?.trim(),
      details: Array.from(element.querySelectorAll("details"), (details) => ({
        open: details.open,
        summary: details.querySelector("summary")?.textContent?.trim() ?? "",
      })),
      actions: Array.from(element.querySelectorAll("button"), (button) => ({
        testId: button.dataset.testid ?? "",
        label: button.textContent?.trim().slice(0, 100) ?? "",
        disabled: button.disabled,
      })),
    };
  });
  await testInfo.attach("read-only-recovery-before-cleanup.json", {
    contentType: "application/json",
    body: JSON.stringify(
      {
        wire: {
          resume: {
            responseType: resumeOutcome.responseType,
            succeeded:
              resumeOutcome.responseType === "response" &&
              (resumeOutcome.payload as { success?: unknown } | undefined)?.success === true,
          },
          restoreWorkspace: {
            responseType: restoreOutcome.responseType,
            succeeded:
              restoreOutcome.responseType === "response" &&
              (restoreOutcome.payload as { success?: unknown } | undefined)?.success === true,
          },
        },
        status: {
          state: status.state,
          is_agent_running: status.is_agent_running,
          is_resumable: status.is_resumable,
          needs_resume: status.needs_resume,
          needs_workspace_restore: status.needs_workspace_restore,
          acp_session_id_matches: status.acp_session_id === originalAcpSessionId,
        },
        promptEvidence: {
          activeSubmissions: promptEvidence.activeSubmissions,
          unknownPromptBlocks: promptEvidence.unknownPromptBlocks,
          submissionCount: promptEvidence.submissionCount,
          expectedSubmissionCount,
          executionPresent: Boolean(promptEvidence.execution),
          executionMatches: promptEvidence.execution?.resume_token === originalAcpSessionId,
        },
        card,
      },
      null,
      2,
    ),
  });
  return { status, promptEvidence };
}

async function createSettledHistoryTask(
  api: ApiClient,
  seed: SeedData,
  title: string,
  agentProfileId: string,
) {
  const task = await api.createTaskWithAgent(seed.workspaceId, title, agentProfileId, {
    description: "/e2e:simple-message",
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
    repository_ids: [seed.repositoryId],
  });
  if (!task.session_id) throw new Error("history recovery task has no session_id");
  await waitForSessionDone(
    api,
    task.id,
    task.session_id,
    "Waiting for the history recovery task's initial prompt to finish",
    60_000,
  );
  return task;
}

async function runReadOnlyResumeScenario(input: {
  testInfo: TestInfo;
  page: Page;
  api: ApiClient;
  seed: SeedData;
  options: {
    tmpDir: string;
    capture: PrAssetCapture;
    mobile: boolean;
    restart: () => Promise<void>;
  };
  failOnResumeProfileId: string;
  setSessionId: (sessionId: string) => void;
}) {
  const { testInfo, page, api, seed, options, failOnResumeProfileId, setSessionId } = input;
  const { tmpDir, capture, mobile } = options;
  const task = await createSettledHistoryTask(
    api,
    seed,
    "Resume restored workspace",
    failOnResumeProfileId,
  );
  const sessionId = task.session_id!;
  setSessionId(sessionId);

  await api.addUserMessage(task.id, sessionId, "/e2e:bulk:1");
  await expect
    .poll(
      async () => {
        const { messages } = await api.listSessionMessages(sessionId);
        return messages.some((message) => message.content.includes("Done. Emitted 1 messages"));
      },
      { timeout: 30_000 },
    )
    .toBe(true);
  await waitForSessionDone(api, task.id, sessionId, "Seeded user turn did not settle", 30_000);

  const original = (await api.listTaskSessions(task.id)).sessions.find((s) => s.id === sessionId)!;
  expect(original.metadata?.acp).toBeTruthy();
  const originalIdentity = await readSessionRuntimeIdentity(api, task.id, sessionId);
  const historyUserMessageIds = (await api.listSessionMessages(sessionId)).messages
    .filter((message) => message.author_type === "user")
    .map((message) => message.id);

  await api.stopSession({ session_id: sessionId, force: true });
  await expect
    .poll(
      async () =>
        (
          await api.wsRequest<{ is_agent_running: boolean }>("task.session.status", {
            task_id: task.id,
            session_id: sessionId,
          })
        ).is_agent_running,
      { timeout: 30_000 },
    )
    .toBe(false);

  withDatabase(tmpDir, (db) => {
    db.prepare(
      `UPDATE task_sessions SET state = 'WAITING_FOR_INPUT', error_message = '',
       metadata = json_remove(metadata, '$.last_agent_error', '$.agent_delivery_recovery')
       WHERE id = ?`,
    ).run(sessionId);
  });

  const promptEvidence = readPromptEvidence(tmpDir, sessionId);
  expect(promptEvidence).toMatchObject({
    activeSubmissions: 0,
    unknownPromptBlocks: 0,
    execution: {
      agent_execution_id: originalIdentity.executionId,
      resume_token: originalIdentity.acpSessionId,
      resumable: 1,
    },
  });
  const beforeNavigation = await api.wsRequest<{
    state: string;
    is_agent_running: boolean;
    is_resumable: boolean;
    needs_resume: boolean;
    needs_workspace_restore: boolean;
    acp_session_id?: string;
  }>("task.session.status", {
    task_id: task.id,
    session_id: sessionId,
  });
  expect(beforeNavigation).toMatchObject({
    state: "WAITING_FOR_INPUT",
    is_agent_running: false,
    is_resumable: true,
    needs_resume: true,
    needs_workspace_restore: false,
    acp_session_id: originalIdentity.acpSessionId,
  });

  const launches = observeSessionLaunches(page, sessionId);
  const failedResume = launches.waitForIntent("resume");
  const restoredWorkspace = launches.waitForIntent("restore_workspace");
  const ws = watchWs(page);
  const [, resumeOutcome, restoreOutcome] = await Promise.all([
    page.goto(`/t/${task.id}`),
    failedResume,
    restoredWorkspace,
  ]);
  expect(
    resumeOutcome.responseType === "error" ||
      (resumeOutcome.payload as { success?: unknown } | undefined)?.success === false,
  ).toBe(true);
  expect(restoreOutcome.responseType).toBe("response");
  expect((restoreOutcome.payload as { success?: unknown } | undefined)?.success).toBe(true);

  const recoverySnapshot = await attachReadOnlyRecoveryEvidence({
    api,
    page,
    testInfo,
    taskId: task.id,
    sessionId,
    originalAcpSessionId: originalIdentity.acpSessionId,
    resumeOutcome,
    restoreOutcome,
    tmpDir,
    expectedSubmissionCount: promptEvidence.submissionCount,
  });
  const restoredStatus = recoverySnapshot.status;
  const restoredPromptEvidence = recoverySnapshot.promptEvidence;

  const recoveryCard = page.getByTestId("session-recovery-card");
  await expect(recoveryCard).toBeVisible();
  const recoveryTitle = recoveryCard.getByRole("heading", {
    name: "Workspace restored in read-only mode",
  });
  await expect(recoveryTitle).toBeVisible();
  const details = recoveryCard.locator('details:has(> summary:text-is("Technical details"))');
  await expect(details).toHaveCount(1);
  await expect(details.locator("summary")).toContainText("Technical details");
  await expect(details).not.toHaveAttribute("open", "");
  const resume = recoveryCard.getByTestId("recovery-resume-button");
  await expect(resume).toBeEnabled();
  expect(restoredPromptEvidence).toMatchObject({
    activeSubmissions: 0,
    unknownPromptBlocks: 0,
    submissionCount: promptEvidence.submissionCount,
  });
  expect(restoredStatus.is_agent_running).toBe(false);
  expect(
    (await api.listSessionMessages(sessionId)).messages
      .filter((message) => message.author_type === "user")
      .map((message) => message.id),
  ).toEqual(historyUserMessageIds);

  if (mobile) {
    const bounds = await resume.boundingBox();
    expect(bounds?.height).toBeGreaterThanOrEqual(44);
    const centerIsReachable = await resume.evaluate((button) => {
      const bounds = button.getBoundingClientRect();
      const target = document.elementFromPoint(
        bounds.x + bounds.width / 2,
        bounds.y + bounds.height / 2,
      );
      return target === button || button.contains(target);
    });
    expect(centerIsReachable).toBe(true);
  }
  await capture.screenshot("resume-action", {
    caption: "Resume remains available after workspace-only restoration.",
  });

  await api.updateAgentProfile(failOnResumeProfileId, { cli_flags: [] });
  const recovered = ws.waitForResponse("session.recover", { timeout: 60_000 });
  if (mobile) await resume.tap();
  else await resume.click();
  await recovered;
  await expect(recoveryCard).toHaveCount(0);
  const resumed = (await api.listTaskSessions(task.id)).sessions.find((s) => s.id === sessionId)!;
  expect(resumed.metadata?.acp).toEqual(original.metadata?.acp);
  expect(await readSessionRuntimeIdentity(api, task.id, sessionId)).toMatchObject({
    acpSessionId: originalIdentity.acpSessionId,
  });
  expect(
    (await api.listSessionMessages(sessionId)).messages
      .filter((message) => message.author_type === "user")
      .map((message) => message.id),
  ).toEqual(historyUserMessageIds);
  expect(readPromptEvidence(tmpDir, sessionId)).toMatchObject({
    activeSubmissions: 0,
    unknownPromptBlocks: 0,
    submissionCount: promptEvidence.submissionCount,
  });

  await options.restart();
  await page.reload();
  await expect(recoveryCard).toHaveCount(0);
  const session = new SessionPage(page);
  await expect(
    session.activeChat().getByText("simple mock response", { exact: false }),
  ).toHaveCount(1);
  if (mobile) await session.sendMessageViaButton("/e2e:bulk:3");
  else await session.sendMessage("/e2e:bulk:3");
  await expect(
    session.activeChat().getByText("Done. Emitted 3 messages", { exact: false }),
  ).toBeVisible({ timeout: 30_000 });
  await expect(
    session.activeChat().getByText("simple mock response", { exact: false }),
  ).toHaveCount(1);
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.11
export async function verifyReadOnlyResume(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  testInfo: TestInfo,
  options: {
    tmpDir: string;
    capture: PrAssetCapture;
    mobile: boolean;
    restart: () => Promise<void>;
  },
) {
  const profile = await createFailOnResumeProfile(api, `Read-only fallback ${Date.now()}`);
  let sessionId: string | undefined;
  try {
    await runReadOnlyResumeScenario({
      testInfo,
      page,
      api,
      seed,
      options,
      failOnResumeProfileId: profile.id,
      setSessionId: (id) => {
        sessionId = id;
      },
    });
  } finally {
    if (sessionId) {
      await api.stopSession({ session_id: sessionId, force: true }).catch(() => undefined);
    }
    await api.deleteAgentProfile(profile.id, true).catch(() => undefined);
  }
}
