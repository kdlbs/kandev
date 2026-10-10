import { createRequire } from "node:module";
import path from "node:path";
import { randomUUID } from "node:crypto";
import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { waitForSessionState } from "../../helpers/session";
import { watchWs } from "../../helpers/causal-waits";
import { captureSessionRecoveryMessages } from "../../helpers/session-resume-recovery";
import {
  openQuickChatSetup,
  sendQuickChatMessage,
  startQuickChatFromSetup,
} from "./quick-chat-helpers";

const nodeRequire = createRequire(path.join(process.cwd(), "package.json"));
type TestDatabase = {
  prepare(sql: string): { get(...args: unknown[]): unknown; run(...args: unknown[]): unknown };
  close(): void;
};

function withDatabase<T>(tmpDir: string, read: (db: TestDatabase) => T): T {
  const { DatabaseSync } = nodeRequire("node:sqlite") as {
    DatabaseSync: new (databasePath: string) => TestDatabase;
  };
  const db = new DatabaseSync(path.join(tmpDir, "kandev.db"));
  try {
    return read(db);
  } finally {
    db.close();
  }
}

function seedInterruptedPrompt(tmpDir: string, sessionId: string) {
  return withDatabase(tmpDir, (db) => {
    const submission = db
      .prepare(
        `SELECT id, incarnation_id, harness_generation
      FROM agent_delivery_submissions WHERE session_id = ? ORDER BY created_at DESC LIMIT 1`,
      )
      .get(sessionId) as { id: string; incarnation_id: string; harness_generation: number };
    expect(submission).toBeTruthy();
    const blockId = randomUUID();
    const submissionId = `prompt:${randomUUID()}`;
    db.prepare(
      `INSERT INTO agent_delivery_submissions
      (id, session_id, incarnation_id, harness_generation, owner_generation,
       payload_hash, payload, state, outcome, created_at, updated_at)
      SELECT ?, session_id, incarnation_id, harness_generation, owner_generation,
       payload_hash, payload, 'interrupted_unknown', 'prompt_dispatch_failed', created_at, updated_at
      FROM agent_delivery_submissions WHERE id = ?`,
    ).run(submissionId, submission.id);
    db.prepare(
      `INSERT INTO session_recovery_blocks
      (id, session_id, incarnation_id, expected_generation, reason, state,
       consumer_reference, delivery_submission_id, created_at, updated_at)
      VALUES (?, ?, ?, ?, 'unknown_prompt_outcome', 'open', 'agent_delivery', ?, ?, ?)`,
    ).run(
      blockId,
      sessionId,
      submission.incarnation_id,
      submission.harness_generation,
      submissionId,
      new Date().toISOString(),
      new Date().toISOString(),
    );
    return { blockId, submissionId };
  });
}

function readRecovery(tmpDir: string, blockId: string, submissionId: string) {
  return withDatabase(tmpDir, (db) => ({
    block: db
      .prepare("SELECT state, authorized_action FROM session_recovery_blocks WHERE id = ?")
      .get(blockId),
    submission: db
      .prepare("SELECT state FROM agent_delivery_submissions WHERE id = ?")
      .get(submissionId),
  }));
}

async function openExistingQuickChat(page: Page, mobile: boolean) {
  if (mobile) {
    await page.getByTestId("app-nav-trigger").tap();
    await page.getByTestId("mobile-quick-chat-button").tap();
  } else {
    await page.keyboard.press(`${process.platform === "darwin" ? "Meta" : "Control"}+Shift+q`);
  }
  const dialog = page.getByRole("dialog", { name: "Quick Chat" });
  await expect(dialog).toBeVisible();
  return dialog;
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.7
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.8
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.9
export async function verifyQuickChatResumeRecovery(
  page: Page,
  apiClient: ApiClient,
  tmpDir: string,
  mobile: boolean,
) {
  const recoveryMessages = captureSessionRecoveryMessages(page);
  const ws = watchWs(page);
  let dialog;
  if (mobile) {
    await page.goto("/");
    dialog = await openExistingQuickChat(page, true);
  } else {
    dialog = await openQuickChatSetup(page);
  }
  const startResponse = page.waitForResponse(
    (response) => response.url().includes("/quick-chat") && response.request().method() === "POST",
  );
  await startQuickChatFromSetup(dialog, page, "/e2e:simple-message");
  const started = (await (await startResponse).json()) as { task_id: string; session_id: string };
  await waitForSessionState(apiClient, {
    taskId: started.task_id,
    sessionId: started.session_id,
    expectedState: "WAITING_FOR_INPUT",
    message: "Quick Chat initial instruction did not settle",
    timeout: 30_000,
  });
  await expect(dialog.getByText("simple mock response", { exact: false })).toBeVisible();
  const { sessions } = await apiClient.listTaskSessions(started.task_id);
  const original = sessions.find((session) => session.id === started.session_id)!;
  const nativeIdentity = original.metadata?.acp;
  expect(nativeIdentity).toBeTruthy();
  await apiClient.stopSession({ session_id: started.session_id, force: true });
  await expect
    .poll(
      async () =>
        (
          await apiClient.wsRequest<{ is_agent_running: boolean }>("task.session.status", {
            task_id: started.task_id,
            session_id: started.session_id,
          })
        ).is_agent_running,
      { timeout: 30_000 },
    )
    .toBe(false);
  const { blockId, submissionId } = seedInterruptedPrompt(tmpDir, started.session_id);
  const restored = await apiClient.wsRequest<{ success: boolean }>("session.launch", {
    task_id: started.task_id,
    session_id: started.session_id,
    intent: "restore_workspace",
  });
  expect(restored.success).toBe(true);
  expect(readRecovery(tmpDir, blockId, submissionId)).toMatchObject({
    block: { state: "open" },
    submission: { state: "interrupted_unknown" },
  });
  await page.reload();
  await page.waitForLoadState("networkidle");
  dialog = await openExistingQuickChat(page, mobile);
  await dialog.locator(`[data-tab-reference="conversation:${started.session_id}"]`).click();
  const recovered = ws.waitForResponse("session.recover", { timeout: 60_000 });
  await dialog.getByTestId("recovery-resume-button").click();
  await recovered;
  await expect.poll(() => recoveryMessages.requestCounts.resume ?? 0).toBe(1);
  await waitForSessionState(apiClient, {
    taskId: started.task_id,
    sessionId: started.session_id,
    expectedState: "WAITING_FOR_INPUT",
    message: "Explicit Resume did not recover Quick Chat",
    timeout: 30_000,
  });
  expect(readRecovery(tmpDir, blockId, submissionId)).toMatchObject({
    block: { state: "resolved", authorized_action: "resume" },
    submission: { state: "interrupted_unknown" },
  });
  const resumed = (await apiClient.listTaskSessions(started.task_id)).sessions.find(
    (session) => session.id === started.session_id,
  )!;
  expect(resumed.metadata?.acp).toEqual(nativeIdentity);
  await expect(dialog.getByText("simple mock response", { exact: false })).toHaveCount(1);
  await sendQuickChatMessage(dialog, page, "/e2e:bulk:3");
  await expect(dialog.getByText("Done. Emitted 3 messages", { exact: false })).toBeVisible({
    timeout: 30_000,
  });
  await expect(dialog.getByText("simple mock response", { exact: false })).toHaveCount(1);
}
