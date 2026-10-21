import path from "node:path";
import { expect, type Page } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { DatabaseSync } from "./node-sqlite";
import { retentionEvidence } from "./delivery-retention-compatibility";
import { attachGatewayTrafficCapture } from "./ws-traffic";
import { watchWs } from "./causal-waits";
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
  options: {
    profileId?: string;
    restartOverrides?: Record<string, string>;
    historicalGap?: boolean;
  } = {},
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
  let historicalGap: { stream_id: string; sequence: number; payload: string } | undefined;
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
    if (options.historicalGap) {
      historicalGap = seedHistoricalDeliveryGap(db, primary.id);
    }
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
  if (historicalGap) {
    assertHistoricalDeliveryGap(backend, primary.id, historicalGap);
  }
  await page.reload();
  await session.waitForLoad();
  await expect(session.chat.getByText(output, { exact: false })).toBeVisible({ timeout: 30_000 });
  return { task, session, sessionId: primary.id, output };
}

// Model a projected historical terminal whose prompt authority was never retained.
// All fixture writes are confined to this worker's disposable database.
function seedHistoricalDeliveryGap(db: InstanceType<typeof DatabaseSync>, sessionId: string) {
  const gap = db
    .prepare(
      "SELECT stream_id, sequence FROM agent_delivery_inbox WHERE session_id = ? AND terminal = 0 AND projected_at IS NOT NULL AND EXISTS (SELECT 1 FROM agent_delivery_inbox later WHERE later.stream_id = agent_delivery_inbox.stream_id AND later.sequence > agent_delivery_inbox.sequence AND later.terminal = 0) ORDER BY stream_id, sequence LIMIT 1",
    )
    .get(sessionId) as { stream_id: string; sequence: number } | undefined;
  expect(gap).toBeTruthy();
  if (!gap) throw new Error("historical gap fixture needs later canonical output");
  const payload = JSON.stringify({ type: "complete", turn_id: "historical-unsettled-turn" });
  db.prepare(
    "UPDATE agent_delivery_inbox SET terminal = 1, event_type = 'complete', submission_id = 'missing-historical-submission', payload = ?, payload_bytes = 0, hash_version = 0, content_hash = '' WHERE stream_id = ? AND sequence = ?",
  ).run(Buffer.from(payload), gap.stream_id, gap.sequence);
  return { ...gap, payload };
}

function assertHistoricalDeliveryGap(
  backend: BackendContext,
  sessionId: string,
  gap: { stream_id: string; sequence: number; payload: string },
) {
  const db = new DatabaseSync(path.join(backend.tmpDir, "kandev.db"));
  try {
    const protectedRow = db
      .prepare(
        "SELECT CAST(payload AS TEXT) AS payload, payload_pruned_at FROM agent_delivery_inbox WHERE stream_id = ? AND sequence = ?",
      )
      .get(gap.stream_id, gap.sequence) as { payload: string; payload_pruned_at: unknown };
    expect(protectedRow.payload).toBe(gap.payload);
    expect(protectedRow.payload_pruned_at).toBeNull();
    const cleaned = db
      .prepare(
        "SELECT COUNT(*) AS count FROM agent_delivery_inbox WHERE session_id = ? AND stream_id = ? AND sequence > ? AND terminal = 0 AND payload_pruned_at IS NOT NULL",
      )
      .get(sessionId, gap.stream_id, gap.sequence) as { count: number };
    expect(Number(cleaned.count)).toBeGreaterThan(0);
  } finally {
    db.close();
  }
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

// An independent native recovery cause keeps this control test blocked while
// the shared runtime recovers after process loss.
function seedRetentionRecoveryBlock(backend: BackendContext, sessionId: string) {
  const db = new DatabaseSync(path.join(backend.tmpDir, "kandev.db"));
  const blockId = `retention-native-recovery:${sessionId}`;
  try {
    const inserted = db
      .prepare(
        `INSERT INTO session_recovery_blocks
       (id, session_id, incarnation_id, expected_generation, reason, state,
        consumer_reference, created_at, updated_at)
       SELECT ?, h.session_id, h.incarnation_id, h.generation, 'native_state_missing',
        'open', 'native_resume', datetime('now', '-1 minute'), datetime('now')
       FROM harness_session_generations h JOIN task_sessions s
        ON s.id = h.session_id AND s.queue_incarnation_id = h.incarnation_id
       WHERE h.session_id = ? ORDER BY h.generation DESC LIMIT 1`,
      )
      .run(blockId, sessionId);
    expect(Number(inserted.changes)).toBe(1);
  } finally {
    db.close();
  }
  return blockId;
}

function assertRetentionRecoveryBlockPreserved(backend: BackendContext, blockId: string) {
  const db = new DatabaseSync(path.join(backend.tmpDir, "kandev.db"));
  try {
    expect(
      db
        .prepare(
          "SELECT state, consumer_reference, authorized_action FROM session_recovery_blocks WHERE id = ?",
        )
        .get(blockId),
    ).toEqual({ state: "open", consumer_reference: "native_resume", authorized_action: "" });
  } finally {
    db.close();
  }
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
  const ws = watchWs(page);
  await page.reload();
  await session.waitForLoad();
  const previousIds = new Set(retentionEvidence(backend, sessionId).prompts.map((row) => row.id));
  await session.sendMessageViaButton("/slow 120");
  await waitForProjectedRecoveryPrompt(backend, sessionId, previousIds);
  const independentBlockId = seedRetentionRecoveryBlock(backend, sessionId);
  const bootID = await api.getBackendBootID();
  await observeAgentRuntimeAvailability(page);
  const killed = await api.killLocalAgentRuntimeChild();
  expect(killed.killed).toBe(true);
  await waitForAgentRuntimeReplacement(page, killed.runtime_epoch, bootID);
  assertRetentionRecoveryBlockPreserved(backend, independentBlockId);
  const banner = page.getByTestId("failed-session-banner");
  await expect(banner).toBeVisible({ timeout: 60_000 });
  const promptCount = traffic.frames.filter(
    (frame) => frame.direction === "sent" && frame.action === "message.add",
  ).length;
  expect(promptCount).toBe(1);
  const submissionsBeforeRetry = retentionEvidence(backend, sessionId).prompts.map((row) => row.id);
  const retry = banner.getByTestId("recovery-retry-connection-button");
  await expect(banner).toContainText(
    "The connection was interrupted. Kandev cannot confirm whether the agent processed your message.",
  );
  await expect(retry).toBeVisible();
  if (touch) {
    const box = await retry.boundingBox();
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
    expect(box?.width ?? 0).toBeGreaterThanOrEqual(44);
  }
  const recoveryResponse = ws.waitForResponse("session.recover");
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
  expect((await recoveryResponse).payload).toMatchObject({
    session_id: sessionId,
    outcome: "blocked",
    reason: "independent_recovery_block",
  });
  assertRetentionRecoveryBlockPreserved(backend, independentBlockId);
  await expect(banner.getByTestId("delivery-recovery-result")).toHaveText(
    "Recovery is blocked. Resolve the recovery issue before retrying.",
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
  assertRetentionRecoveryBlockPreserved(backend, independentBlockId);
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
