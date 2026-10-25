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

type NativeResumeScenario = boolean | "readiness" | "cancelled";
type NativeResumeOptions = { legacy: NativeResumeScenario; afterResumeClick: () => Promise<void> };

function resumeScenario(options: NativeResumeScenario | NativeResumeOptions) {
  return typeof options === "object"
    ? options
    : { legacy: options, afterResumeClick: async () => {} };
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
  if (legacy === "readiness") await session.fillMessage("/slow 1s");
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
  if (legacy) {
    withDatabase(tmpDir, (db) => {
      db.prepare(
        "UPDATE agent_delivery_submissions SET state = 'dispatching', outcome = 'dispatching' WHERE id = ?",
      ).run(seeded.submissionId);
      if (legacy === "cancelled") {
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
      const row = db.prepare("SELECT metadata FROM task_sessions WHERE id = ?").get(active.id) as {
        metadata: string;
      };
      const metadata = JSON.parse(row.metadata);
      delete metadata.agent_delivery_recovery;
      delete metadata.last_agent_error;
      db.prepare(
        "UPDATE task_sessions SET metadata = ?, state = 'WAITING_FOR_INPUT', error_message = '' WHERE id = ?",
      ).run(JSON.stringify(metadata), active.id);
    });
  }
  if (legacy === "readiness") {
    withDatabase(tmpDir, (db) => {
      db.prepare("DELETE FROM session_recovery_blocks WHERE id = ?").run(recoveryBlockId);
    });
    await api.launchSession({ task_id: task.id, session_id: active.id, intent: "resume" });
    await expect
      .poll(() =>
        withDatabase(tmpDir, (db) => {
          const block = db
            .prepare(
              "SELECT id FROM session_recovery_blocks WHERE session_id = ? AND state = 'open'",
            )
            .get(active.id) as { id: string } | undefined;
          return block?.id;
        }),
      )
      .toBeTruthy();
    recoveryBlockId = withDatabase(
      tmpDir,
      (db) =>
        (
          db
            .prepare(
              "SELECT id FROM session_recovery_blocks WHERE session_id = ? AND state = 'open'",
            )
            .get(active.id) as { id: string }
        ).id,
    );
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
  try {
    if (legacy) {
      await expect(page.getByTestId("recovery-retry-connection-button")).toBeVisible();
      await expect(page.getByTestId("failed-session-banner")).toContainText(
        "Resume checks the saved conversation without sending the previous prompt again.",
      );
      await assertDeliveryRecoveryGeometry(page, "recovery-resume-button");
      const resume = page.getByTestId("recovery-resume-button");
      if (test.info().project.name === "mobile-chrome") await resume.tap();
      else await resume.click();
      await afterResumeClick();
      await expect
        .poll(() => readRecovery(tmpDir, recoveryBlockId, seeded.submissionId).block, {
          timeout: 60_000,
        })
        .toMatchObject({ state: "resolved", authorized_action: "resume" });
    } else {
      await api.wsRequest("session.recover", {
        task_id: task.id,
        session_id: active.id,
        action: "resume",
      });
    }
  } catch (error) {
    await test.info().attach("native-resume-backend-log", {
      path: path.join(tmpDir, ".kandev/logs/backend-logs.log"),
      contentType: "text/plain",
    });
    await test.info().attach("native-resume-session", {
      body: JSON.stringify(await api.getTaskSession(active.id)),
      contentType: "application/json",
    });
    throw error;
  }
  if (!legacy)
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
    submission: { state: "interrupted_unknown" },
  });
  await expectJournalRetirement(seeded.journalPath, seeded.submission);
  expect(
    (await api.listSessionMessages(active.id)).messages
      .filter((item) => item.author_type === "user")
      .map((item) => item.id),
  ).toEqual(originalMessages.filter((item) => item.author_type === "user").map((item) => item.id));
  await expect(session.chat.getByText("simple mock response", { exact: false })).toHaveCount(1);
  if (legacy === "readiness") {
    const draft = session.activeChat().locator(".tiptap.ProseMirror:visible").first();
    await expect(draft).toContainText("/slow 1s");
    if (test.info().project.name === "mobile-chrome") await session.tapSubmitWhenReady();
    else await session.clickSubmitWhenReady();
  } else {
    await session.sendMessageViaButton("/slow 1s");
  }
  await expect(
    session.chat.getByText("Slow response complete after 1s.", { exact: false }),
  ).toBeVisible({ timeout: 30_000 });
}
