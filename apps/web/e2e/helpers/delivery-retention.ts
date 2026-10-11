import path from "node:path";
import { expect, type Page } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { DatabaseSync } from "./node-sqlite";
import { retentionEvidence } from "./delivery-retention-compatibility";
import { attachGatewayTrafficCapture } from "./ws-traffic";
import {
  observeAgentRuntimeAvailability,
  waitForAgentRuntimeReplacement,
} from "./agent-runtime-availability";
import { SessionPage } from "../pages/session-page";

export async function pruneSavedDelivery(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  backend: BackendContext,
  options: { profileId?: string; restartOverrides?: Record<string, string> } = {},
) {
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    "Delivery retention saved history",
    options.profileId ?? seed.agentProfileId,
    {
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
      executor_profile_id: seed.worktreeExecutorProfileId,
      start_agent: false,
    },
  );
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.sendMessageViaButton("/slow 1s");
  const output = "Slow response complete after 1s.";
  await expect(session.chat.getByText(output, { exact: false })).toBeVisible({ timeout: 60_000 });
  const primary = (await api.listTaskSessions(task.id)).sessions.find((item) => item.is_primary);
  if (!primary) throw new Error("task has no primary session");
  const canonicalBefore = (await api.listSessionMessages(primary.id)).messages;
  const dbPath = path.join(backend.tmpDir, "kandev.db");
  const readCount = (sql: string) => {
    const db = new DatabaseSync(dbPath);
    try {
      return Number((db.prepare(sql).get(primary.id) as { count: number }).count);
    } finally {
      db.close();
    }
  };
  await expect
    .poll(
      () =>
        readCount(
          "SELECT COUNT(*) AS count FROM agent_delivery_submissions WHERE session_id = ? AND fully_settled_at IS NOT NULL",
        ),
      { timeout: 30_000 },
    )
    .toBeGreaterThan(0);
  const db = new DatabaseSync(dbPath);
  try {
    const policy = db
      .prepare("SELECT value FROM settings WHERE key = 'tool_payload_retention'")
      .get() as { value: string } | undefined;
    expect(policy ? JSON.parse(policy.value).policy.enabled : false).toBe(false);
    expect(
      readCount(
        "SELECT COUNT(*) AS count FROM agent_delivery_inbox WHERE session_id = ? AND LENGTH(payload) > 0",
      ),
    ).toBeGreaterThan(0);
    db.prepare(
      "UPDATE agent_delivery_inbox SET projected_at = datetime('now', '-48 hours') WHERE session_id = ? AND projected_at IS NOT NULL",
    ).run(primary.id);
    db.prepare(
      "UPDATE agent_delivery_submissions SET fully_settled_at = datetime('now', '-48 hours') WHERE session_id = ? AND fully_settled_at IS NOT NULL",
    ).run(primary.id);
  } finally {
    db.close();
  }
  await backend.restart({
    ...options.restartOverrides,
    KANDEV_TASK_DELIVERY_RETENTION_INTERVAL: "10s",
    KANDEV_TASK_DELIVERY_RETENTION_REPLAY_AGE: "720h",
  });
  try {
    await expect
      .poll(
        () =>
          readCount(
            "SELECT COUNT(*) AS count FROM agent_delivery_inbox WHERE session_id = ? AND payload_pruned_at IS NOT NULL",
          ),
        { timeout: 90_000 },
      )
      .toBeGreaterThan(0);
  } catch (error) {
    throw new Error(
      `${String(error)}\nDelivery evidence: ${JSON.stringify(retentionEvidence(backend, primary.id))}`,
    );
  }
  expect((await api.listSessionMessages(primary.id)).messages).toEqual(canonicalBefore);
  await page.reload();
  await session.waitForLoad();
  await expect(session.chat.getByText(output, { exact: false })).toBeVisible({ timeout: 30_000 });
  return { task, session, sessionId: primary.id, output };
}

async function waitForProjectedRecoveryPrompt(
  backend: BackendContext,
  sessionId: string,
  previousIds: Set<unknown>,
) {
  const newPrompts = () =>
    retentionEvidence(backend, sessionId).prompts.filter((row) => !previousIds.has(row.id));
  await expect.poll(newPrompts, { timeout: 30_000 }).toHaveLength(1);
  const submissionId = newPrompts()[0].id;
  await expect
    .poll(
      () => {
        const db = new DatabaseSync(path.join(backend.tmpDir, "kandev.db"));
        try {
          const row = db
            .prepare(
              "SELECT COUNT(*) AS count FROM agent_delivery_inbox i JOIN agent_delivery_cursors c ON c.stream_id = i.stream_id AND c.session_id = i.session_id JOIN agent_delivery_submissions s ON s.id = i.submission_id AND s.session_id = i.session_id WHERE i.session_id = ? AND i.submission_id = ? AND i.event_type = 'message_chunk' AND i.projected_at IS NOT NULL AND i.sequence <= c.projected_sequence AND s.state IN ('accepted', 'dispatching') AND s.fully_settled_at IS NULL",
            )
            .get(sessionId, submissionId) as { count: number };
          return Number(row.count);
        } finally {
          db.close();
        }
      },
      { timeout: 30_000 },
    )
    .toBeGreaterThan(0);
}

export async function assertRetentionRecoveryControls(
  page: Page,
  api: ApiClient,
  session: SessionPage,
  sessionId: string,
  controls: { backend: BackendContext; touch: boolean },
) {
  const { backend, touch } = controls;
  const traffic = attachGatewayTrafficCapture(page);
  await page.reload();
  await session.waitForLoad();
  const previousIds = new Set(retentionEvidence(backend, sessionId).prompts.map((row) => row.id));
  await session.sendMessageViaButton("/slow 120");
  await waitForProjectedRecoveryPrompt(backend, sessionId, previousIds);
  const bootID = await api.getBackendBootID();
  await observeAgentRuntimeAvailability(page);
  const killed = await api.killLocalAgentRuntimeChild();
  expect(killed.killed).toBe(true);
  await waitForAgentRuntimeReplacement(page, killed.runtime_epoch, bootID);
  const banner = page.getByTestId("failed-session-banner");
  await expect(banner).toBeVisible({ timeout: 60_000 });
  const promptCount = traffic.frames.filter(
    (frame) => frame.direction === "sent" && frame.action === "message.add",
  ).length;
  expect(promptCount).toBe(1);
  const submissionsBeforeRetry = retentionEvidence(backend, sessionId).prompts.map((row) => row.id);
  const retry = banner.getByTestId("recovery-retry-connection-button");
  await expect(banner).toContainText("Delivery was interrupted. The prompt outcome is uncertain.");
  await expect(retry).toBeVisible();
  if (touch) {
    const box = await retry.boundingBox();
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
    expect(box?.width ?? 0).toBeGreaterThanOrEqual(44);
  }
  if (touch) await retry.tap();
  else await retry.click();
  await expect
    .poll(() =>
      traffic.frames.some(
        (frame) =>
          frame.direction === "sent" &&
          frame.action === "session.recover" &&
          frame.sessionId === sessionId,
      ),
    )
    .toBe(true);
  await expect(banner.getByTestId("delivery-recovery-result")).toHaveText(
    "Recovery is blocked. Resolve the recovery issue before retrying.",
    { timeout: 30_000 },
  );
  expect(
    traffic.frames.filter((frame) => frame.direction === "sent" && frame.action === "message.add"),
  ).toHaveLength(promptCount);
  expect(retentionEvidence(backend, sessionId).prompts.map((row) => row.id)).toEqual(
    submissionsBeforeRetry,
  );
  expect(
    (await api.listSessionMessages(sessionId)).messages.filter(
      (row) => row.author_type === "user" && row.content.trim() === "/slow 120",
    ),
  ).toHaveLength(1);
  const stop = banner.getByTestId("recovery-stop-button");
  await expect(stop).toBeEnabled();
  if (touch) {
    const box = await stop.boundingBox();
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
    expect(box?.width ?? 0).toBeGreaterThanOrEqual(44);
    await stop.tap();
  } else await stop.click();
  await expect(banner.getByTestId("delivery-stop-outcome-unconfirmed")).toBeVisible({
    timeout: 10_000,
  });
  await expect
    .poll(() =>
      traffic.frames.some(
        (frame) =>
          frame.direction === "sent" &&
          frame.action === "session.stop" &&
          frame.sessionId === sessionId,
      ),
    )
    .toBe(true);
}
