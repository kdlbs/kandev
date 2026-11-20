import { randomUUID } from "node:crypto";
import { expect } from "@playwright/test";
import { withDatabase, journalFixture, type JournalRecord } from "./interrupted-prompt-recovery";
import type { ApiClient } from "./api-client";

export async function seedAbsentPromptsAndCheckAdmission(
  api: ApiClient,
  tmpDir: string,
  seed: { taskId: string; sessionId: string; sourceId: string; blockId: string },
  enabled: boolean,
) {
  if (!enabled) return [];
  const { taskId, sessionId, sourceId, blockId } = seed;
  const ids = seedAbsentPrompts(tmpDir, sourceId, blockId);
  const before = sessionSubmissionIds(tmpDir, sessionId);
  const blockedPrompt = "Do not dispatch while recovery is blocked";
  await api.addUserMessage(taskId, sessionId, blockedPrompt);
  await expect
    .poll(
      async () =>
        (await api.listSessionMessages(sessionId)).messages.find(
          (message) => message.content === blockedPrompt,
        )?.metadata?.delivery_status,
    )
    .toBe("blocked");
  expect(sessionSubmissionIds(tmpDir, sessionId)).toEqual(before);
  expectAbsentPromptsBlocked(tmpDir, blockId, ids[1]);
  return ids;
}

export async function expectAbsentPromptsRetired(
  tmpDir: string,
  journalPath: string,
  source: JournalRecord,
  ids: string[],
) {
  expectAbsentPromptsRetained(tmpDir, ids);
  for (const id of ids) {
    await expect
      .poll(() => journalFixture("read", journalPath, { id }))
      .toMatchObject({
        ...source,
        id,
        payload: null,
        retired: true,
      });
  }
}

export function seedAbsentPrompts(tmpDir: string, sourceId: string, blockId: string) {
  const ids = [randomUUID(), randomUUID()].map((id) => `prompt:${id}`);
  withDatabase(tmpDir, (db) => {
    for (const id of ids) {
      db.prepare(
        `INSERT INTO agent_delivery_submissions
        (id, session_id, incarnation_id, harness_generation, owner_generation, payload_hash, payload, state, outcome, created_at, updated_at)
        SELECT ?, session_id, incarnation_id, harness_generation, owner_generation, payload_hash, payload,
        'interrupted_unknown', 'prompt_dispatch_failed', created_at, updated_at
        FROM agent_delivery_submissions WHERE id = ?`,
      ).run(id, sourceId);
    }
    db.prepare("UPDATE session_recovery_blocks SET delivery_submission_id = ? WHERE id = ?").run(
      ids[1],
      blockId,
    );
  });
  return ids;
}

export function expectAbsentPromptsRetained(tmpDir: string, ids: string[]) {
  withDatabase(tmpDir, (db) => {
    for (const id of ids) {
      expect(
        db
          .prepare(
            "SELECT state, outcome, terminal_sequence, fully_settled_at FROM agent_delivery_submissions WHERE id = ?",
          )
          .get(id),
      ).toMatchObject({
        state: "interrupted_unknown",
        outcome: "prompt_dispatch_failed",
        terminal_sequence: 0,
        fully_settled_at: null,
      });
    }
  });
}

export function sessionSubmissionIds(tmpDir: string, sessionId: string) {
  return withDatabase(tmpDir, (db) =>
    db
      .prepare("SELECT id FROM agent_delivery_submissions WHERE session_id = ? ORDER BY id")
      .all(sessionId),
  );
}

export function expectAbsentPromptsBlocked(tmpDir: string, blockId: string, submissionId: string) {
  withDatabase(tmpDir, (db) => {
    expect(
      db
        .prepare("SELECT state, delivery_submission_id FROM session_recovery_blocks WHERE id = ?")
        .get(blockId),
    ).toEqual({
      state: "open",
      delivery_submission_id: submissionId,
    });
  });
}
