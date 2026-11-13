import { test, expect, type Page } from "@playwright/test";
import path from "node:path";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import { assertDeliveryRecoveryGeometry } from "./delivery-recovery-controls";
import { SessionPage } from "../pages/session-page";
import {
  seedInterruptedPrompt,
  readRecovery,
  expectJournalRetirement,
  withDatabase,
} from "./interrupted-prompt-recovery";

type NativeResumeScenario = boolean | "readiness" | "cancelled" | "unproven";
type NativeResumeOptions = { legacy: NativeResumeScenario; afterResumeClick: () => Promise<void> };

function resumeScenario(options: NativeResumeScenario | NativeResumeOptions) {
  return typeof options === "object"
    ? options
    : { legacy: options, afterResumeClick: async () => {} };
}

// The legacy mutations reuse an older fence shape. "unproven" keeps the real
// journal history and only changes the SQL completion label.
function changesLegacyFence(scenario: NativeResumeScenario) {
  return scenario === true || scenario === "readiness" || scenario === "cancelled";
}

function seedLegacyFence(
  tmpDir: string,
  sessionId: string,
  seeded: ReturnType<typeof seedInterruptedPrompt>,
  recoveryBlockId: string,
  scenario: NativeResumeScenario,
) {
  withDatabase(tmpDir, (db) => {
    db.prepare(
      "UPDATE agent_delivery_submissions SET state = 'dispatching', outcome = 'dispatching' WHERE id = ?",
    ).run(seeded.submissionId);
    if (scenario === "cancelled") {
      db.prepare("UPDATE session_recovery_blocks SET delivery_stream_id = '' WHERE id = ?").run(
        recoveryBlockId,
      );
      db.prepare(
        "UPDATE agent_delivery_submissions SET state = 'interrupted_unknown', outcome = 'prompt_dispatch_failed' WHERE id = ?",
      ).run(seeded.submissionId);
    } else {
      db.prepare(
        "UPDATE session_recovery_blocks SET reason = 'unresolved_durable_work', delivery_submission_id = '', delivery_stream_id = '' WHERE id = ?",
      ).run(recoveryBlockId);
    }
    const row = db.prepare("SELECT metadata FROM task_sessions WHERE id = ?").get(sessionId) as {
      metadata: string;
    };
    const metadata = JSON.parse(row.metadata);
    delete metadata.agent_delivery_recovery;
    delete metadata.last_agent_error;
    db.prepare(
      "UPDATE task_sessions SET metadata = ?, state = 'WAITING_FOR_INPUT', error_message = '' WHERE id = ?",
    ).run(JSON.stringify(metadata), sessionId);
  });
}

function seedUnprovenCompletion(tmpDir: string, submissionId: string) {
  withDatabase(tmpDir, (db) => {
    db.prepare(
      "UPDATE agent_delivery_submissions SET state = 'completed', outcome = 'completed', terminal_stream_id = '', terminal_sequence = 0, terminal_turn_id = '', fully_settled_at = NULL, payload_pruned_at = NULL WHERE id = ?",
    ).run(submissionId);
  });
}

async function expectDeliveryProjectionSettled(tmpDir: string, streamId: string) {
  await expect
    .poll(
      () =>
        withDatabase(tmpDir, (db) => {
          const cursor = db
            .prepare(
              "SELECT received_sequence, projected_sequence, remote_high_water FROM agent_delivery_cursors WHERE stream_id = ?",
            )
            .get(streamId) as
            | {
                received_sequence: number;
                projected_sequence: number;
                remote_high_water: number;
              }
            | undefined;
          return (
            !!cursor &&
            cursor.received_sequence === cursor.projected_sequence &&
            cursor.received_sequence >= cursor.remote_high_water
          );
        }),
      {
        timeout: 15_000,
        message:
          "delivery cursor must be fully projected before simulating an unproven SQL completion",
      },
    )
    .toBe(true);
}

type NativeResumeTrigger = {
  page: Page;
  api: ApiClient;
  taskId: string;
  sessionId: string;
  tmpDir: string;
  recoveryBlockId: string;
  submissionId: string;
  legacyFence: boolean;
  afterResumeClick: () => Promise<void>;
};

async function triggerNativeResume(input: NativeResumeTrigger) {
  try {
    if (input.legacyFence) {
      await expect(input.page.getByTestId("recovery-retry-connection-button")).toBeVisible();
      await expect(input.page.getByTestId("failed-session-banner")).toContainText(
        "Resume checks the saved conversation without sending the previous prompt again.",
      );
      await assertDeliveryRecoveryGeometry(input.page, "recovery-resume-button");
      const resume = input.page.getByTestId("recovery-resume-button");
      if (test.info().project.name === "mobile-chrome") await resume.tap();
      else await resume.click();
      await input.afterResumeClick();
      await expect
        .poll(() => readRecovery(input.tmpDir, input.recoveryBlockId, input.submissionId).block, {
          timeout: 60_000,
        })
        .toMatchObject({ state: "resolved", authorized_action: "resume" });
      return;
    }
    await input.api.wsRequest("session.recover", {
      task_id: input.taskId,
      session_id: input.sessionId,
      action: "resume",
    });
  } catch (error) {
    await test.info().attach("native-resume-backend-log", {
      path: path.join(input.tmpDir, ".kandev/logs/backend-logs.log"),
      contentType: "text/plain",
    });
    await test.info().attach("native-resume-session", {
      body: JSON.stringify(await input.api.getTaskSession(input.sessionId)),
      contentType: "application/json",
    });
    throw error;
  }
}

async function prepareReadinessFence(
  api: ApiClient,
  taskId: string,
  sessionId: string,
  tmpDir: string,
  recoveryBlockId: string,
) {
  withDatabase(tmpDir, (db) => {
    db.prepare("DELETE FROM session_recovery_blocks WHERE id = ?").run(recoveryBlockId);
  });
  await api.launchSession({ task_id: taskId, session_id: sessionId, intent: "resume" });
  await expect
    .poll(() =>
      withDatabase(tmpDir, (db) => {
        const block = db
          .prepare("SELECT id FROM session_recovery_blocks WHERE session_id = ? AND state = 'open'")
          .get(sessionId) as { id: string } | undefined;
        return block?.id;
      }),
    )
    .toBeTruthy();
  return withDatabase(
    tmpDir,
    (db) =>
      (
        db
          .prepare("SELECT id FROM session_recovery_blocks WHERE session_id = ? AND state = 'open'")
          .get(sessionId) as { id: string }
      ).id,
  );
}

async function sendRecoveryInstruction(
  page: Page,
  session: SessionPage,
  useRetainedDraft: boolean,
) {
  if (!useRetainedDraft) {
    await session.sendMessageViaButton("/slow 1s");
    return;
  }
  const draft = session.activeChat().locator(".tiptap.ProseMirror:visible").first();
  await expect(draft).toContainText("/slow 1s");
  if (test.info().project.name === "mobile-chrome") await session.tapSubmitWhenReady();
  else await session.clickSubmitWhenReady();
}

// @covers AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.4
export async function verifyNativeResumeClearsRecoveryNotice(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  tmpDir: string,
  options: NativeResumeScenario | NativeResumeOptions = false,
) {
  const { legacy, afterResumeClick } = resumeScenario(options);
  const unproven = legacy === "unproven";
  const legacyFence = changesLegacyFence(legacy);
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    "Native resume clears interrupted notice",
    seed.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
      executor_profile_id: seed.worktreeExecutorProfileId,
    },
  );
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.expectChatResponseVisible("simple mock response", 0, { timeout: 30_000 });
  const active = (await api.listTaskSessions(task.id)).sessions.find((item) => item.is_primary);
  if (!active) throw new Error("created task has no primary session");
  await expect
    .poll(
      async () =>
        (await api.listTaskSessions(task.id)).sessions.find((item) => item.id === active.id)?.state,
    )
    .toBe("WAITING_FOR_INPUT");
  const native = (await api.getTaskSession(active.id)).session.metadata?.acp;
  expect(native).toMatchObject({ session_id: expect.any(String) });
  if (legacy === "readiness" || unproven) await session.fillMessage("/slow 1s");
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
  expect(active.agent_execution_id).toBeTruthy();
  const originalMessages = (await api.listSessionMessages(active.id)).messages;
  const seeded = seedInterruptedPrompt(tmpDir, active.id, active.agent_execution_id);
  let recoveryBlockId: string = seeded.blockId;
  if (legacyFence) {
    seedLegacyFence(tmpDir, active.id, seeded, recoveryBlockId, legacy);
  }
  if (unproven) {
    await expectDeliveryProjectionSettled(tmpDir, seeded.submission.stream_id);
    seedUnprovenCompletion(tmpDir, seeded.submissionId);
  }
  if (legacy === "readiness") {
    recoveryBlockId = await prepareReadinessFence(api, task.id, active.id, tmpDir, recoveryBlockId);
  }
  await page.reload();
  await session.waitForLoad();
  if (legacy === "readiness") {
    await expect(session.activeChat().locator(".tiptap.ProseMirror:visible")).toHaveCount(0);
    expect(
      (await api.listSessionMessages(active.id)).messages
        .filter((item) => item.author_type === "user")
        .map((item) => item.id),
    ).toEqual(
      originalMessages.filter((item) => item.author_type === "user").map((item) => item.id),
    );
  }
  await expect(page.getByTestId("failed-session-banner")).toContainText(
    "Kandev cannot confirm whether the agent processed your message.",
  );
  if (unproven) {
    // An uncertain notice intentionally exposes Retry connection and Stop only.
    // Resume is available on the legacy and readiness recovery paths below.
    await expect(page.getByTestId("recovery-resume-button")).toHaveCount(0);
    await expect(page.getByTestId("recovery-retry-connection-button")).toBeVisible();
    await expect(page.getByTestId("recovery-stop-button")).toBeVisible();
  }
  await triggerNativeResume({
    page,
    api,
    taskId: task.id,
    sessionId: active.id,
    tmpDir,
    recoveryBlockId,
    submissionId: seeded.submissionId,
    legacyFence,
    afterResumeClick,
  });
  if (!legacyFence)
    await expect
      .poll(async () => {
        const { session: current } = await api.getTaskSession(active.id);
        return (current.metadata?.agent_delivery_recovery as { phase?: string } | undefined)?.phase;
      })
      .toBe("restored");
  await page.reload();
  await session.waitForLoad();
  await expect(page.getByTestId("failed-session-banner")).toHaveCount(0);
  await expect(page.getByTestId("delivery-recovery-result")).toHaveCount(0);
  expect((await api.getTaskSession(active.id)).session.metadata?.acp).toEqual(native);
  expect(readRecovery(tmpDir, recoveryBlockId, seeded.submissionId)).toMatchObject({
    block: { state: "resolved", authorized_action: "resume" },
    submission: { state: unproven ? "completed" : "interrupted_unknown" },
  });
  await expectJournalRetirement(seeded.journalPath, seeded.submission);
  expect(
    (await api.listSessionMessages(active.id)).messages
      .filter((item) => item.author_type === "user")
      .map((item) => item.id),
  ).toEqual(originalMessages.filter((item) => item.author_type === "user").map((item) => item.id));
  await expect(session.chat.getByText("simple mock response", { exact: false })).toHaveCount(1);
  await sendRecoveryInstruction(page, session, legacy === "readiness" || unproven);
  await expect(
    session.chat.getByText("Slow response complete after 1s.", { exact: false }),
  ).toBeVisible({ timeout: 30_000 });
}
