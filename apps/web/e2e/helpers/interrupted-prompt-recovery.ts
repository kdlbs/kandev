import { createRequire } from "node:module";
import path from "node:path";
import { randomUUID } from "node:crypto";
import { expect } from "@playwright/test";

const nodeRequire = createRequire(path.join(process.cwd(), "package.json"));
type TestDatabase = {
  prepare(sql: string): { get(...args: unknown[]): unknown; run(...args: unknown[]): unknown };
  close(): void;
};

export function withDatabase<T>(tmpDir: string, read: (db: TestDatabase) => T): T {
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

export function seedInterruptedPrompt(tmpDir: string, sessionId: string) {
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

export function readRecovery(tmpDir: string, blockId: string, submissionId: string) {
  return withDatabase(tmpDir, (db) => ({
    block: db
      .prepare("SELECT state, authorized_action FROM session_recovery_blocks WHERE id = ?")
      .get(blockId),
    submission: db
      .prepare("SELECT state FROM agent_delivery_submissions WHERE id = ?")
      .get(submissionId),
  }));
}
