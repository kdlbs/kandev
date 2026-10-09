import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { watchWs } from "../../helpers/causal-waits";
import { createSettledHistoryTask } from "../../helpers/session-entry-recovery";
import {
  seedInterruptedPrompt,
  expectJournalRetirement,
  readRecovery,
  withDatabase,
} from "../../helpers/interrupted-prompt-recovery";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.11
export async function verifyReadOnlyResume(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  options: {
    tmpDir: string;
    capture: PrAssetCapture;
    mobile: boolean;
    restart: () => Promise<void>;
  },
) {
  const { tmpDir, capture, mobile } = options;
  const task = await createSettledHistoryTask(api, seed, "Resume restored workspace");
  const sessionId = task.session_id!;
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
  const { blockId, submissionId, journalPath, submission } = seedInterruptedPrompt(
    tmpDir,
    sessionId,
  );
  withDatabase(tmpDir, (db) => {
    db.prepare(
      "UPDATE task_sessions SET state = 'WAITING_FOR_INPUT', error_message = '', metadata = json_remove(metadata, '$.last_agent_error', '$.agent_delivery_recovery') WHERE id = ?",
    ).run(sessionId);
  });
  const seeded = (await api.listTaskSessions(task.id)).sessions.find((s) => s.id === sessionId)!;
  expect(seeded.state).toBe("WAITING_FOR_INPUT");
  expect(seeded.error_message).toBeFalsy();
  const ws = watchWs(page);
  await page.goto(`/t/${task.id}`);
  const notice = page.getByTestId("session-recovery-notice");
  await expect(notice).toBeVisible({ timeout: 45_000 });
  await expect(notice).toContainText("Workspace restored in read-only mode");
  await expect(page.getByTestId("recovery-resume-button")).toHaveCount(0);
  const resume = notice.getByTestId("session-recovery-notice-resume");
  await expect(resume).toBeEnabled();
  await expect(page.getByTestId("session-recovery-details")).not.toHaveAttribute("open", "");
  if (mobile) {
    const bounds = await resume.boundingBox();
    expect(bounds?.height).toBeGreaterThanOrEqual(44);
  }
  await capture.screenshot("resume-action", {
    caption: "Resume remains available after workspace-only restoration.",
  });
  const recovered = ws.waitForResponse("session.recover", { timeout: 60_000 });
  if (mobile) await resume.tap();
  else await resume.click();
  await recovered;
  await expect(notice).toHaveCount(0);
  expect(readRecovery(tmpDir, blockId, submissionId)).toMatchObject({
    block: { state: "resolved", authorized_action: "resume" },
    submission: { state: "interrupted_unknown" },
  });
  await expectJournalRetirement(journalPath, submission);
  const resumed = (await api.listTaskSessions(task.id)).sessions.find((s) => s.id === sessionId)!;
  expect(resumed.metadata?.acp).toEqual(original.metadata?.acp);
  await options.restart();
  await page.reload();
  await expect(notice).toHaveCount(0);
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
