import { test, expect, type Page } from "@playwright/test";
import path from "node:path";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import { SessionPage } from "../pages/session-page";
import {
  seedInterruptedPrompt,
  readRecovery,
  expectJournalRetirement,
} from "./interrupted-prompt-recovery";

// @covers AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.4
export async function verifyNativeResumeClearsRecoveryNotice(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  tmpDir: string,
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
  const seeded = seedInterruptedPrompt(tmpDir, active.id, active.agent_execution_id);
  await page.reload();
  await session.waitForLoad();
  await expect(page.getByTestId("failed-session-banner")).toContainText(
    "The prompt outcome is uncertain.",
  );
  try {
    await api.wsRequest("session.recover", {
      task_id: task.id,
      session_id: active.id,
      action: "resume",
    });
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
  await session.sendMessageViaButton("/slow 1s");
  await expect(
    session.chat.getByText("Slow response complete after 1s.", { exact: false }),
  ).toBeVisible({ timeout: 30_000 });
}
