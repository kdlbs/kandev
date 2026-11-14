import { createHash, randomUUID } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import type { Page } from "@playwright/test";
import { expect } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import { setupPage } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { PrAssetCapture } from "./pr-asset-capture";
import { seedInterruptedPrompt, withDatabase } from "./interrupted-prompt-recovery";
import { readMockACPTrace } from "./transient-turn-runtime-continuity";
import { SessionPage } from "../pages/session-page";

const restoredNotice =
  "The session was interrupted. Your conversation is restored. You can continue.";

type RecoveryCase = "ordinary-restart" | "historical-terminal" | "failed-session-upgrade";

export async function verifyAutomaticRestartRecovery({
  initialPage,
  api,
  seed,
  backend,
  recoveryCase,
  captureFile,
}: {
  initialPage: Page;
  api: ApiClient;
  seed: SeedData;
  backend: BackendContext;
  recoveryCase: RecoveryCase;
  captureFile: string;
}) {
  const context = initialPage.context();
  const tracePath = path.join(backend.tmpDir, `automatic-restart-${randomUUID()}.jsonl`);
  let profileId: string | undefined;
  let taskId: string | undefined;
  let replacementPage: Page | undefined;
  let capture: PrAssetCapture | undefined;
  try {
    fs.writeFileSync(tracePath, "");
    const setup = await createRestartRecoveryTask(api, seed, tracePath, recoveryCase);
    const { task, sessionId } = setup;
    profileId = setup.profileId;
    taskId = task.id;
    await initialPage.goto(`/t/${task.id}`);
    const sessionPage = new SessionPage(initialPage);
    await sessionPage.waitForLoad();
    await expect
      .poll(
        async () =>
          (await api.listSessionMessages(sessionId)).messages.some((message) =>
            message.content.includes("simple mock response"),
          ),
        { timeout: 30_000, message: "the initial mock response should be persisted" },
      )
      .toBe(true);
    await sessionPage.expectChatResponseVisible("simple mock response", 0);
    const originalPromptTrace = readMockACPTrace(tracePath)
      .filter((record) => record.event === "prompt")
      .map(({ session_id, prompt }) => ({ session_id, prompt }));
    expect(originalPromptTrace.length).toBeGreaterThan(0);

    const active = await primarySessionForTask(api, task.id);
    await expect
      .poll(async () => (await api.getTaskSession(active.id)).session.state, { timeout: 30_000 })
      .toBe("WAITING_FOR_INPUT");
    const before = (await api.getTaskSession(active.id)).session;
    const nativeSessionId = getNativeSessionId(before);
    let originalMessages = (await api.listSessionMessages(active.id)).messages;
    const independentSibling =
      recoveryCase === "ordinary-restart"
        ? await createIndependentBlockedSibling(api, seed, backend)
        : undefined;

    await sessionPage.fillMessage("/slow 1s");
    const savedDraft = await initialPage.evaluate((sessionId) => {
      const entries: [string, string][] = [];
      const suffix = `.${sessionId}`;
      for (let index = 0; index < window.sessionStorage.length; index++) {
        const key = window.sessionStorage.key(index);
        if (key?.startsWith("kandev.chatDraft.") && key.endsWith(suffix)) {
          const value = window.sessionStorage.getItem(key);
          if (value !== null) entries.push([key, value]);
        }
      }
      return entries;
    }, active.id);
    expect(savedDraft).toEqual(
      expect.arrayContaining([[`kandev.chatDraft.text.${active.id}`, JSON.stringify("/slow 1s")]]),
    );
    await api.stopSession({ session_id: active.id, force: true });
    await expect
      .poll(
        async () =>
          (
            await api.wsRequest<{ is_agent_running: boolean }>("task.session.status", {
              task_id: task.id,
              session_id: active.id,
            })
          ).is_agent_running,
      )
      .toBe(false);
    if (!before.agent_execution_id) throw new Error("active session has no execution identity");

    const seeded = seedInterruptedPrompt(backend.tmpDir, active.id, before.agent_execution_id, {
      createCanonicalMessage: true,
    });
    const sourcePrompt = JSON.parse(
      Buffer.from(seeded.submission.payload, "base64").toString("utf8"),
    ) as { text?: unknown };
    if (typeof sourcePrompt.text !== "string")
      throw new Error("interrupted prompt fixture has no source prompt text");
    const interruptedPromptCount = (records: ReturnType<typeof readMockACPTrace>) =>
      records.filter(
        (record) =>
          record.event === "prompt" && record.prompt?.includes(sourcePrompt.text as string),
      ).length;
    expect(seeded.messageId).toBeTruthy();
    expect(seeded.submissionId).toBe(`prompt:${seeded.messageId}`);
    expect(interruptedPromptCount(readMockACPTrace(tracePath))).toBe(1);
    originalMessages = (await api.listSessionMessages(active.id)).messages;
    if (recoveryCase === "historical-terminal") {
      seedHistoricalTerminalRecovery(
        backend.tmpDir,
        active.id,
        seeded.submissionId,
        before.agent_execution_id,
      );
    }
    withDatabase(backend.tmpDir, (db) => {
      const state = recoveryCase === "failed-session-upgrade" ? "FAILED" : "WAITING_FOR_INPUT";
      const errorMessage = recoveryCase === "failed-session-upgrade" ? "Agent process exited" : "";
      db.prepare("UPDATE task_sessions SET state = ?, error_message = ? WHERE id = ?").run(
        state,
        errorMessage,
        active.id,
      );
    });

    await initialPage.close();
    expect(context.pages()).toHaveLength(0);
    await backend.restart();

    const expectedNoticeId = recoveryNoticeId({
      sessionId: active.id,
      incarnationId: seeded.submission.incarnation_id,
      harnessGeneration: seeded.submission.harness_generation,
      submissionId: seeded.submissionId,
    });
    await expect
      .poll(
        async () => {
          expect(context.pages()).toHaveLength(0);
          const { session } = await api.getTaskSession(active.id);
          const { messages } = await api.listSessionMessages(active.id);
          const recovery = session.metadata?.agent_delivery_recovery as
            | { phase?: string; submission_id?: string }
            | undefined;
          const notices = messages.filter(
            (message) =>
              message.type === "status" &&
              message.metadata?.kind === "session_restored_after_interruption" &&
              message.metadata.source_submission_id === seeded.submissionId,
          );
          return {
            session,
            notices: notices.map((message) => ({
              id: message.id,
              content: message.content,
              metadata: message.metadata,
            })),
            recovery: { phase: recovery?.phase, submissionId: recovery?.submission_id },
          };
        },
        {
          timeout: 60_000,
          message: "startup did not restore the session without browser participation",
        },
      )
      .toMatchObject({
        session: {
          id: active.id,
          task_id: task.id,
          state: "WAITING_FOR_INPUT",
          agent_execution_id: expect.any(String),
          session_recovery_blocks: [],
          metadata: { acp: { session_id: nativeSessionId } },
        },
        recovery: { phase: "restored", submissionId: seeded.submissionId },
        notices: [
          {
            id: expectedNoticeId,
            content: restoredNotice,
            metadata: {
              kind: "session_restored_after_interruption",
              source_session_id: active.id,
              source_submission_id: seeded.submissionId,
            },
          },
        ],
      });

    expect(context.pages()).toHaveLength(0);
    expect(
      readMockACPTrace(tracePath)
        .filter((record) => record.event === "prompt")
        .map(({ session_id, prompt }) => ({ session_id, prompt })),
    ).toEqual(originalPromptTrace);
    expect(interruptedPromptCount(readMockACPTrace(tracePath))).toBe(1);
    const recoveryRows = withDatabase(backend.tmpDir, (db) => ({
      block: db
        .prepare("SELECT state, authorized_action FROM session_recovery_blocks WHERE id = ?")
        .get(seeded.blockId) as { state: string; authorized_action: string },
      submission: db
        .prepare("SELECT state FROM agent_delivery_submissions WHERE id = ?")
        .get(seeded.submissionId) as { state: string },
    }));
    expect(recoveryRows).toEqual({
      block: { state: "resolved", authorized_action: "silent_restart_restore" },
      submission: { state: "interrupted_unknown" },
    });
    expect(
      (await api.listSessionMessages(active.id)).messages
        .filter((message) => message.author_type === "user")
        .map(({ id, content }) => ({ id, content })),
    ).toEqual(
      originalMessages
        .filter((message) => message.author_type === "user")
        .map(({ id, content }) => ({ id, content })),
    );

    if (independentSibling) {
      const { session: siblingSession } = await api.getTaskSession(independentSibling.sessionId);
      expect(siblingSession).toMatchObject({
        state: "FAILED",
        session_recovery_blocks: [
          expect.objectContaining({
            id: independentSibling.blockId,
            reason: "native_state_missing",
            consumer_reference: "native_resume",
          }),
        ],
      });
      expect(
        withDatabase(backend.tmpDir, (db) =>
          db
            .prepare("SELECT state, consumer_reference FROM session_recovery_blocks WHERE id = ?")
            .get(independentSibling.blockId),
        ),
      ).toEqual({ state: "open", consumer_reference: "native_resume" });
    }

    replacementPage = await context.newPage();
    // Closing the page is required to prove backend-owned recovery. Restore
    // the same tab's session-scoped draft once in the replacement document.
    await replacementPage.addInitScript(
      ({ entries, marker }: { entries: [string, string][]; marker: string }) => {
        if (window.sessionStorage.getItem(marker) === "seeded") return;
        window.sessionStorage.setItem(marker, "seeded");
        for (const [key, value] of entries) window.sessionStorage.setItem(key, value);
      },
      { entries: savedDraft, marker: `kandev.e2e.recoveryDraftSeeded.${active.id}` },
    );
    await setupPage(replacementPage, backend);
    await replacementPage.goto(`/t/${task.id}`);
    capture = new PrAssetCapture(replacementPage, captureFile, { captureKey: recoveryCase });
    const restoredChat = new SessionPage(replacementPage);
    await restoredChat.waitForLoad();
    await expect(replacementPage.getByTestId("session-recovery-notice")).toHaveCount(1);
    await expect(replacementPage.getByTestId("failed-session-banner")).toHaveCount(0);
    await expect(replacementPage.getByTestId("recovery-resume-button")).toHaveCount(0);
    await expect(replacementPage.getByTestId("recovery-retry-connection-button")).toHaveCount(0);

    const editor = restoredChat.activeChat().locator(".tiptap.ProseMirror:visible").first();
    await expect(editor).toBeEditable();
    await expect(editor).toContainText("/slow 1s");
    await expect(restoredChat.submitButton()).toBeEnabled();
    await capture.screenshot("restored-chat");

    await replacementPage.reload();
    await restoredChat.waitForLoad();
    await expect(replacementPage.getByTestId("session-recovery-notice")).toHaveCount(1);
    await expect(
      restoredChat.activeChat().locator(".tiptap.ProseMirror:visible").first(),
    ).toContainText("/slow 1s");
    if (recoveryCase === "failed-session-upgrade") {
      const nativeAfterReload = getNativeSessionId((await api.getTaskSession(active.id)).session);
      expect(nativeAfterReload).toBe(nativeSessionId);
    }
    if (testIsMobile(replacementPage)) {
      const sendBox = await restoredChat.submitButton().boundingBox();
      expect(sendBox).not.toBeNull();
      if (sendBox) {
        const viewportHeight = await replacementPage.evaluate(() => window.innerHeight);
        expect(sendBox.y + sendBox.height).toBeLessThanOrEqual(viewportHeight);
      }
      await expectNoHorizontalOverflow(replacementPage);
      await restoredChat.tapSubmitWhenReady();
    } else {
      await restoredChat.clickSubmitWhenReady();
    }
    await expect
      .poll(
        async () =>
          (await api.listSessionMessages(active.id)).messages.some((message) =>
            message.content.includes("Slow response complete after 1s."),
          ),
        { timeout: 30_000, message: "the follow-up response should be persisted before rendering" },
      )
      .toBe(true);
    await expect
      .poll(() => readMockACPTrace(tracePath).filter((record) => record.event === "prompt"), {
        timeout: 30_000,
      })
      .toHaveLength(originalPromptTrace.length + 1);
    const promptsAfterFollowUp = readMockACPTrace(tracePath).filter(
      (record) => record.event === "prompt",
    );
    expect(
      promptsAfterFollowUp
        .slice(0, originalPromptTrace.length)
        .map(({ session_id, prompt }) => ({ session_id, prompt })),
    ).toEqual(originalPromptTrace);
    expect(interruptedPromptCount(readMockACPTrace(tracePath))).toBe(1);
    expect(promptsAfterFollowUp.at(-1)?.prompt).toContain("/slow 1s");
    await restoredChat.expectChatResponseVisible("Slow response complete after 1s.", 0);
    const followUp = (await api.listSessionMessages(active.id)).messages.filter(
      (message) => message.author_type === "user" && message.content.trim() === "/slow 1s",
    );
    expect(followUp).toHaveLength(1);
    expect(followUp[0].turn_id).toBeTruthy();
    await expect(replacementPage.getByTestId("session-recovery-notice")).toHaveCount(1);
    if (independentSibling) {
      await replacementPage.goto(`/t/${independentSibling.taskId}`);
      const blockedSibling = new SessionPage(replacementPage);
      await blockedSibling.waitForLoad();
      await expect(replacementPage.getByTestId("session-recovery-card")).toBeVisible();
      await expect(replacementPage.getByTestId("recovery-resume-button")).toBeVisible();
    }
  } finally {
    capture?.flush();
    await replacementPage?.close().catch(() => undefined);
    if (taskId) await api.deleteTask(taskId).catch(() => undefined);
    if (profileId) await api.deleteAgentProfile(profileId, true).catch(() => undefined);
    fs.rmSync(tracePath, { force: true });
  }
}

async function createRestartRecoveryTask(
  api: ApiClient,
  seed: SeedData,
  tracePath: string,
  recoveryCase: RecoveryCase,
) {
  const { agents } = await api.listAgents();
  const profileOwner = agents.find((agent) =>
    agent.profiles?.some((profile) => profile.id === seed.agentProfileId),
  );
  if (!profileOwner) throw new Error("mock-agent profile owner is missing");
  const profile = await api.createAgentProfile(
    profileOwner.id,
    `Restart recovery ${randomUUID()}`,
    {
      model: "mock-fast",
      auto_fallback: false,
      env_vars: [{ key: "E2E_MOCK_AGENT_ACP_TRACE_FILE", value: tracePath }],
    },
  );
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    `Restart recovery ${recoveryCase}`,
    profile.id,
    {
      description: "/e2e:simple-message",
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
      executor_profile_id: seed.worktreeExecutorProfileId,
    },
  );
  if (!task.session_id) throw new Error("created task has no session ID");
  return { profileId: profile.id, task, sessionId: task.session_id };
}

async function primarySessionForTask(api: ApiClient, taskId: string) {
  const { sessions } = await api.listTaskSessions(taskId);
  const session = sessions.find((item) => item.is_primary);
  if (!session) throw new Error("created task has no primary session");
  return session;
}

async function createIndependentBlockedSibling(
  api: ApiClient,
  seed: SeedData,
  backend: BackendContext,
) {
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    "Independent blocked recovery sibling",
    seed.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
      executor_profile_id: seed.worktreeExecutorProfileId,
    },
  );
  const session = (await api.listTaskSessions(task.id)).sessions.find((item) => item.is_primary);
  if (!session) throw new Error("independent sibling task has no primary session");
  await expect
    .poll(async () => (await api.getTaskSession(session.id)).session.state, { timeout: 30_000 })
    .toBe("WAITING_FOR_INPUT");
  await api.stopSession({ session_id: session.id, force: true });
  await expect
    .poll(
      async () =>
        (
          await api.wsRequest<{ is_agent_running: boolean }>("task.session.status", {
            task_id: task.id,
            session_id: session.id,
          })
        ).is_agent_running,
    )
    .toBe(false);
  const blockId = seedIndependentRecoveryBlock(backend.tmpDir, session.id);
  return { taskId: task.id, sessionId: session.id, blockId };
}

function seedIndependentRecoveryBlock(tmpDir: string, sessionId: string): string {
  return withDatabase(tmpDir, (db) => {
    const session = db
      .prepare("SELECT task_id, queue_incarnation_id, metadata FROM task_sessions WHERE id = ?")
      .get(sessionId) as
      | { task_id: string; queue_incarnation_id: string; metadata: string }
      | undefined;
    if (!session) throw new Error("independent sibling session is missing from SQLite");
    const generation = db
      .prepare(
        `SELECT generation FROM harness_session_generations
         WHERE session_id = ? AND incarnation_id = ? ORDER BY generation DESC LIMIT 1`,
      )
      .get(sessionId, session.queue_incarnation_id) as { generation: number } | undefined;
    if (!generation) throw new Error("independent sibling has no native session generation");
    const blockId = randomUUID();
    const metadata = JSON.parse(session.metadata || "{}");
    metadata.last_agent_error = {
      code: "NATIVE_STATE_MISSING",
      message: "Saved native session state needs attention.",
      occurred_at: new Date().toISOString(),
    };
    db.prepare(
      "UPDATE task_sessions SET state = 'FAILED', error_message = ?, metadata = ? WHERE id = ?",
    ).run("Saved native session state needs attention.", JSON.stringify(metadata), sessionId);
    const now = new Date().toISOString();
    db.prepare(
      `INSERT INTO session_recovery_blocks
       (id, session_id, incarnation_id, expected_generation, reason, state,
        consumer_reference, created_at, updated_at)
       VALUES (?, ?, ?, ?, 'native_state_missing', 'open', 'native_resume', ?, ?)`,
    ).run(blockId, sessionId, session.queue_incarnation_id, generation.generation, now, now);
    return blockId;
  });
}

function getNativeSessionId(session: Awaited<ReturnType<ApiClient["getTaskSession"]>>["session"]) {
  const acp = session.metadata?.acp;
  const id =
    typeof acp === "object" && acp !== null ? (acp as Record<string, unknown>).session_id : null;
  if (typeof id !== "string" || id === "")
    throw new Error(`session ${session.id} has no native ACP identity`);
  return id;
}

function seedHistoricalTerminalRecovery(
  tmpDir: string,
  sessionId: string,
  currentSubmissionId: string,
  executionId: string,
) {
  withDatabase(tmpDir, (db) => {
    const source = db
      .prepare(
        `SELECT id, incarnation_id, harness_generation, state, outcome, terminal_stream_id,
          terminal_sequence, terminal_turn_id, payload_hash
         FROM agent_delivery_submissions
         WHERE session_id = ? AND id <> ? AND state = 'completed'
           AND terminal_stream_id <> '' AND terminal_sequence > 0 AND terminal_turn_id <> ''
         ORDER BY created_at LIMIT 1`,
      )
      .get(sessionId, currentSubmissionId) as
      | {
          id: string;
          incarnation_id: string;
          harness_generation: number;
          state: string;
          outcome: string;
          terminal_stream_id: string;
          terminal_sequence: number;
          terminal_turn_id: string;
          payload_hash: string;
        }
      | undefined;
    if (!source)
      throw new Error("historical terminal recovery case needs a completed source submission");
    const blockId = randomUUID();
    const now = new Date().toISOString();
    db.prepare(
      `INSERT INTO session_recovery_blocks
       (id, session_id, incarnation_id, expected_generation, reason, state, consumer_reference,
        delivery_submission_id, delivery_stream_id, delivery_sequence, delivery_turn_id,
        delivery_outcome, authorized_action, created_at, updated_at, resolved_at)
       VALUES (?, ?, ?, ?, 'unknown_prompt_outcome', 'resolved', 'agent_delivery', ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
    ).run(
      blockId,
      sessionId,
      source.incarnation_id,
      source.harness_generation,
      source.id,
      source.terminal_stream_id,
      source.terminal_sequence,
      source.terminal_turn_id,
      source.outcome,
      `durable_delivery_${source.outcome}`,
      now,
      now,
      now,
    );
    const row = db.prepare("SELECT metadata FROM task_sessions WHERE id = ?").get(sessionId) as {
      metadata: string;
    };
    const metadata = JSON.parse(row.metadata || "{}");
    metadata.agent_delivery_recovery = {
      phase: "settled",
      revision: 1,
      session_id: sessionId,
      agent_execution_id: executionId,
      submission_id: source.id,
      stream_id: source.terminal_stream_id,
      incarnation_id: source.incarnation_id,
      harness_generation: source.harness_generation,
      prompt_generation: 1,
      updated_at: now,
    };
    db.prepare("UPDATE task_sessions SET metadata = ? WHERE id = ?").run(
      JSON.stringify(metadata),
      sessionId,
    );
  });
}

function recoveryNoticeId(source: {
  sessionId: string;
  incarnationId: string;
  harnessGeneration: number;
  submissionId: string;
}) {
  const name = `session-restored-after-interruption:{"session_id":"${source.sessionId}","incarnation_id":"${source.incarnationId}","harness_generation":${source.harnessGeneration},"submission_id":"${source.submissionId}"}`;
  const namespace = Buffer.from("6ba7b8129dad11d180b400c04fd430c8", "hex");
  const digest = createHash("sha1").update(namespace).update(name).digest().subarray(0, 16);
  digest[6] = (digest[6] & 0x0f) | 0x50;
  digest[8] = (digest[8] & 0x3f) | 0x80;
  const hex = digest.toString("hex");
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

function testIsMobile(page: Page) {
  return page.viewportSize()?.width !== undefined && page.viewportSize()!.width < 768;
}

async function expectNoHorizontalOverflow(page: Page) {
  const geometry = await page.evaluate(() => ({
    clientWidth: document.documentElement.clientWidth,
    scrollWidth: document.documentElement.scrollWidth,
  }));
  expect(geometry.scrollWidth).toBeLessThanOrEqual(geometry.clientWidth);
}
