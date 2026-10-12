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

// @covers AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.4
export async function verifyNativeResumeClearsRecoveryNotice(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  tmpDir: string,
  legacy = false,
) {
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
  if (legacy) {
    withDatabase(tmpDir, (db) => {
      db.prepare(
        "UPDATE agent_delivery_submissions SET state = 'dispatching', outcome = 'dispatching' WHERE id = ?",
      ).run(seeded.submissionId);
      db.prepare(
        "UPDATE session_recovery_blocks SET reason = 'unresolved_durable_work', delivery_submission_id = '', delivery_stream_id = '' WHERE id = ?",
      ).run(seeded.blockId);
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
  await page.reload();
  await session.waitForLoad();
  await expect(page.getByTestId("failed-session-banner")).toContainText(
    "The prompt outcome is uncertain.",
  );
  try {
    if (legacy) {
      await assertDeliveryRecoveryGeometry(page, "recovery-resume-button");
      const resume = page.getByTestId("recovery-resume-button");
      if (test.info().project.name === "mobile-chrome") await resume.tap();
      else await resume.click();
      await expect
        .poll(() => readRecovery(tmpDir, seeded.blockId, seeded.submissionId).block, {
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
  expect(readRecovery(tmpDir, seeded.blockId, seeded.submissionId)).toMatchObject({
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
  await session.sendMessageViaButton("/slow 1s");
  await expect(
    session.chat.getByText("Slow response complete after 1s.", { exact: false }),
  ).toBeVisible({ timeout: 30_000 });
}
