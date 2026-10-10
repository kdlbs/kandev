import { createRequire } from "node:module";
import path from "node:path";
import { expect } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { randomUUID } from "node:crypto";
import { readdirSync, existsSync } from "node:fs";

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

type JournalRecord = {
  id: string;
  session_id: string;
  incarnation_id: string;
  harness_generation: number;
  hash: string;
  payload: string;
  state: string;
  retired?: boolean;
};

function journalFixture(
  mode: "seed" | "read",
  journalPath: string,
  submission: Partial<JournalRecord>,
): JournalRecord {
  const binary = path.resolve(__dirname, "../../../backend/bin/mock-agent");
  return JSON.parse(
    execFileSync(binary, ["--delivery-journal-fixture", mode], {
      input: JSON.stringify({ path: journalPath, submission }),
      encoding: "utf8",
      stdio: ["pipe", "pipe", "pipe"],
    }),
  );
}

function findSessionJournal(tmpDir: string, sessionId: string) {
  const root = path.join(tmpDir, ".kandev", "agentctl-journals");
  for (const owner of readdirSync(root)) {
    const journalPath = path.join(root, owner, "delivery.bbolt");
    if (!existsSync(journalPath)) continue;
    const sourceId = withDatabase(
      tmpDir,
      (db) =>
        db
          .prepare(
            "SELECT id FROM agent_delivery_submissions WHERE session_id = ? ORDER BY created_at DESC LIMIT 1",
          )
          .get(sessionId) as { id: string },
    );
    try {
      if (journalFixture("read", journalPath, { id: sourceId.id }).session_id === sessionId)
        return journalPath;
    } catch {
      // Another test-owned environment's journal does not contain this submission.
    }
  }
  throw new Error(`No retained journal found for session ${sessionId}`);
}

export function seedInterruptedPrompt(tmpDir: string, sessionId: string) {
  const journalPath = findSessionJournal(tmpDir, sessionId);
  const seeded = withDatabase(tmpDir, (db) => {
    const source = db
      .prepare(
        `SELECT id, incarnation_id, harness_generation, payload_hash, payload
       FROM agent_delivery_submissions WHERE session_id = ? ORDER BY created_at DESC LIMIT 1`,
      )
      .get(sessionId) as {
      id: string;
      incarnation_id: string;
      harness_generation: number;
      payload_hash: string;
      payload: string | Uint8Array;
    };
    const blockId = randomUUID();
    const submissionId = `prompt:${randomUUID()}`;
    const submission: JournalRecord = {
      id: submissionId,
      session_id: sessionId,
      incarnation_id: source.incarnation_id,
      harness_generation: source.harness_generation,
      hash: source.payload_hash,
      payload: Buffer.from(source.payload).toString("base64"),
      state: "interrupted_unknown",
    };
    journalFixture("seed", journalPath, submission);
    db.prepare(
      `INSERT INTO agent_delivery_submissions
      (id, session_id, incarnation_id, harness_generation, owner_generation, payload_hash, payload, state, outcome, created_at, updated_at)
      SELECT ?, session_id, incarnation_id, harness_generation, owner_generation, payload_hash, payload,
      'interrupted_unknown', 'prompt_dispatch_failed', created_at, updated_at FROM agent_delivery_submissions WHERE id = ?`,
    ).run(submissionId, source.id);
    db.prepare(
      `INSERT INTO session_recovery_blocks
      (id, session_id, incarnation_id, expected_generation, reason, state, consumer_reference, delivery_submission_id, created_at, updated_at)
      VALUES (?, ?, ?, ?, 'unknown_prompt_outcome', 'open', 'agent_delivery', ?, ?, ?)`,
    ).run(
      blockId,
      sessionId,
      source.incarnation_id,
      source.harness_generation,
      submissionId,
      new Date().toISOString(),
      new Date().toISOString(),
    );
    return { blockId, submissionId, journalPath, submission };
  });
  expect(journalFixture("read", journalPath, seeded.submission)).toMatchObject({
    id: seeded.submissionId,
    state: "interrupted_unknown",
  });
  return seeded;
}

export async function expectJournalRetirement(journalPath: string, submission: JournalRecord) {
  await expect
    .poll(() => journalFixture("read", journalPath, submission))
    .toMatchObject({ ...submission, retired: true });
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
