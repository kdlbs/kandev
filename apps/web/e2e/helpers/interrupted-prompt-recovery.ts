import { createRequire } from "node:module";
import path from "node:path";
import { expect } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { randomUUID } from "node:crypto";
import { readdirSync, existsSync } from "node:fs";

const nodeRequire = createRequire(path.join(process.cwd(), "package.json"));
type TestDatabase = {
  exec(sql: string): void;
  prepare(sql: string): {
    get(...args: unknown[]): unknown;
    all(...args: unknown[]): unknown[];
    run(...args: unknown[]): unknown;
  };
  close(): void;
};

export function withDatabase<T>(tmpDir: string, read: (db: TestDatabase) => T): T {
  const { DatabaseSync } = nodeRequire("node:sqlite") as {
    DatabaseSync: new (databasePath: string) => TestDatabase;
  };
  const db = new DatabaseSync(path.join(tmpDir, "kandev.db"));
  try {
    db.exec("PRAGMA busy_timeout = 5000");
    return read(db);
  } finally {
    db.close();
  }
}

export type JournalRecord = {
  id: string;
  session_id: string;
  incarnation_id: string;
  stream_id: string;
  harness_generation: number;
  hash: string;
  payload: string;
  state: string;
  retired?: boolean;
};

type SourceSubmission = {
  id: string;
  incarnation_id: string;
  harness_generation: number;
  payload_hash: string;
  payload: string | Uint8Array;
};

type SessionSeed = { task_id: string; metadata: string };

type PromptTurnBinding = {
  original_runtime?: Record<string, unknown>;
  prompt_generation?: number;
  task_id?: string;
  session_id?: string;
  execution_id?: string;
  incarnation_id?: string;
  harness_generation?: number;
  submission_id?: string;
  stream_id?: string;
};

type PromptSeedRuntime = {
  originalRuntime: Record<string, unknown>;
  promptGeneration: number;
};

type SeededPrompt = {
  blockId: string;
  submissionId: string;
  messageId: string | undefined;
  journalPath: string;
  submission: JournalRecord;
  originalRuntime: Record<string, unknown> | undefined;
};

export function journalFixture(
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

export function findSessionJournal(tmpDir: string, sessionId: string) {
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

export function seedInterruptedPrompt(
  tmpDir: string,
  sessionId: string,
  recoveryExecutionId?: string,
  options: { createCanonicalMessage?: boolean } = {},
): SeededPrompt {
  const journalPath = findSessionJournal(tmpDir, sessionId);
  const seeded = withDatabase(tmpDir, (db) =>
    seedInterruptedPromptInDatabase(db, journalPath, sessionId, recoveryExecutionId, options),
  );
  expect(journalFixture("read", journalPath, seeded.submission)).toMatchObject({
    id: seeded.submissionId,
    state: "interrupted_unknown",
  });
  return seeded;
}

function seedInterruptedPromptInDatabase(
  db: TestDatabase,
  journalPath: string,
  sessionId: string,
  recoveryExecutionId: string | undefined,
  options: { createCanonicalMessage?: boolean },
): SeededPrompt {
  const source = readLatestSourceSubmission(db, sessionId);
  const sourcePeer = journalFixture("read", journalPath, { id: source.id });
  const message = options.createCanonicalMessage
    ? { messageId: randomUUID(), turnId: randomUUID() }
    : undefined;
  const submissionId = "prompt:" + (message?.messageId ?? randomUUID());
  const session = readSessionSeed(db, sessionId);
  const runtime = recoveryExecutionId
    ? preparePromptSeedRuntime({
        db,
        sessionId,
        taskId: session.task_id,
        executionId: recoveryExecutionId,
        source,
        sourcePeer,
        submissionId,
        turnId: message?.turnId,
      })
    : undefined;

  if (message) {
    insertCanonicalPromptMessage({
      db,
      sessionId,
      taskId: session.task_id,
      turnId: message.turnId,
      messageId: message.messageId,
      source,
    });
  }
  const submission = buildInterruptedSubmission(sessionId, source, sourcePeer, submissionId);
  journalFixture("seed", journalPath, submission);
  insertInterruptedSubmission(db, source, submission, message?.messageId);
  const blockId = insertInterruptedRecoveryBlock(db, sessionId, source, sourcePeer, submissionId);
  if (runtime && recoveryExecutionId) {
    persistUncertainRecoveryMetadata({
      db,
      session,
      sessionId,
      executionId: recoveryExecutionId,
      source,
      sourcePeer,
      submissionId,
      runtime,
    });
  }
  return {
    blockId,
    submissionId,
    messageId: message?.messageId,
    journalPath,
    submission,
    originalRuntime: runtime?.originalRuntime,
  };
}

function readLatestSourceSubmission(db: TestDatabase, sessionId: string): SourceSubmission {
  return db
    .prepare(
      "SELECT id, incarnation_id, harness_generation, payload_hash, payload\n" +
        "FROM agent_delivery_submissions WHERE session_id = ? ORDER BY created_at DESC LIMIT 1",
    )
    .get(sessionId) as SourceSubmission;
}

function readSessionSeed(db: TestDatabase, sessionId: string): SessionSeed {
  return db
    .prepare("SELECT task_id, metadata FROM task_sessions WHERE id = ?")
    .get(sessionId) as SessionSeed;
}

function preparePromptSeedRuntime({
  db,
  sessionId,
  taskId,
  executionId,
  source,
  sourcePeer,
  submissionId,
  turnId,
}: {
  db: TestDatabase;
  sessionId: string;
  taskId: string;
  executionId: string;
  source: SourceSubmission;
  sourcePeer: JournalRecord;
  submissionId: string;
  turnId: string | undefined;
}): PromptSeedRuntime {
  const running = db
    .prepare("SELECT agent_execution_id, metadata FROM executors_running WHERE session_id = ?")
    .get(sessionId) as { agent_execution_id: string; metadata: string } | undefined;
  if (!running) throw persistedRuntimeOwnershipError();

  const runtimeMetadata = JSON.parse(running.metadata || "{}") as Record<string, unknown>;
  const binding = runtimeMetadata.delivery_prompt_turn_binding as PromptTurnBinding | undefined;
  if (
    !binding ||
    !hasMatchingPromptTurnBinding({
      running,
      binding,
      taskId,
      sessionId,
      executionId,
      source,
      sourcePeer,
    })
  ) {
    throw persistedRuntimeOwnershipError();
  }
  const originalRuntime = binding.original_runtime;
  if (!hasUsableRuntimeIdentity(originalRuntime)) throw persistedRuntimeOwnershipError();

  const promptGeneration = (binding.prompt_generation ?? 0) + 1;
  runtimeMetadata.delivery_prompt_turn_binding = {
    ...binding,
    prompt_generation: promptGeneration,
    submission_id: submissionId,
    turn_id: turnId ?? "e2e-interrupted-" + randomUUID(),
  };
  db.prepare(
    "UPDATE executors_running SET metadata = ? WHERE session_id = ? AND agent_execution_id = ?",
  ).run(JSON.stringify(runtimeMetadata), sessionId, executionId);
  return { originalRuntime, promptGeneration };
}

function persistedRuntimeOwnershipError(): Error {
  return new Error("interrupted prompt fixture lacks matching persisted runtime ownership proof");
}

function hasMatchingPromptTurnBinding({
  running,
  binding,
  taskId,
  sessionId,
  executionId,
  source,
  sourcePeer,
}: {
  running: { agent_execution_id: string; metadata: string };
  binding: PromptTurnBinding;
  taskId: string;
  sessionId: string;
  executionId: string;
  source: SourceSubmission;
  sourcePeer: JournalRecord;
}): boolean {
  return (
    running.agent_execution_id === executionId &&
    binding.task_id === taskId &&
    binding.session_id === sessionId &&
    binding.execution_id === executionId &&
    binding.incarnation_id === source.incarnation_id &&
    binding.harness_generation === source.harness_generation &&
    binding.submission_id === source.id &&
    binding.stream_id === sourcePeer.stream_id
  );
}

function hasUsableRuntimeIdentity(
  identity: Record<string, unknown> | undefined,
): identity is Record<string, unknown> {
  return Boolean(
    identity &&
    typeof identity.pid === "number" &&
    identity.pid > 0 &&
    typeof identity.group_id === "number" &&
    identity.group_id > 0 &&
    typeof identity.session_id === "number" &&
    identity.session_id > 0 &&
    typeof identity.birth_token === "string" &&
    identity.birth_token.length > 0,
  );
}

function insertCanonicalPromptMessage({
  db,
  sessionId,
  taskId,
  turnId,
  messageId,
  source,
}: {
  db: TestDatabase;
  sessionId: string;
  taskId: string;
  turnId: string;
  messageId: string;
  source: SourceSubmission;
}): void {
  const now = new Date().toISOString();
  const prompt = JSON.parse(Buffer.from(source.payload).toString("utf8")) as { text?: unknown };
  const content = typeof prompt.text === "string" ? prompt.text : "Interrupted prompt";
  db.prepare(
    "INSERT INTO task_session_turns\n" +
      "(id, task_session_id, task_id, started_at, created_at, updated_at)\n" +
      "VALUES (?, ?, ?, ?, ?, ?)",
  ).run(turnId, sessionId, taskId, now, now, now);
  db.prepare(
    "INSERT INTO task_session_messages\n" +
      "(id, task_session_id, task_id, turn_id, author_type, content, type, metadata, created_at, updated_at)\n" +
      "VALUES (?, ?, ?, ?, 'user', ?, 'message', '{}', ?, ?)",
  ).run(messageId, sessionId, taskId, turnId, content, now, now);
}

function buildInterruptedSubmission(
  sessionId: string,
  source: SourceSubmission,
  sourcePeer: JournalRecord,
  submissionId: string,
): JournalRecord {
  return {
    id: submissionId,
    session_id: sessionId,
    incarnation_id: source.incarnation_id,
    stream_id: sourcePeer.stream_id,
    harness_generation: source.harness_generation,
    hash: source.payload_hash,
    payload: Buffer.from(source.payload).toString("base64"),
    state: "interrupted_unknown",
  };
}

function insertInterruptedSubmission(
  db: TestDatabase,
  source: SourceSubmission,
  submission: JournalRecord,
  messageId: string | undefined,
): void {
  db.prepare(
    "INSERT INTO agent_delivery_submissions\n" +
      "(id, session_id, incarnation_id, harness_generation, owner_generation, dispatch_attempt_id, payload_hash, payload, state, outcome, created_at, updated_at)\n" +
      "SELECT ?, session_id, incarnation_id, harness_generation, owner_generation, ?, payload_hash, payload,\n" +
      "'interrupted_unknown', 'prompt_dispatch_failed', created_at, updated_at FROM agent_delivery_submissions WHERE id = ?",
  ).run(submission.id, messageId ?? "", source.id);
}

function insertInterruptedRecoveryBlock(
  db: TestDatabase,
  sessionId: string,
  source: SourceSubmission,
  sourcePeer: JournalRecord,
  submissionId: string,
): string {
  const blockId = randomUUID();
  db.prepare(
    "INSERT INTO session_recovery_blocks\n" +
      "(id, session_id, incarnation_id, expected_generation, reason, state, consumer_reference, delivery_submission_id, delivery_stream_id, created_at, updated_at)\n" +
      "VALUES (?, ?, ?, ?, 'unknown_prompt_outcome', 'open', 'agent_delivery', ?, ?, ?, ?)",
  ).run(
    blockId,
    sessionId,
    source.incarnation_id,
    source.harness_generation,
    submissionId,
    sourcePeer.stream_id,
    new Date().toISOString(),
    new Date().toISOString(),
  );
  return blockId;
}

function persistUncertainRecoveryMetadata({
  db,
  session,
  sessionId,
  executionId,
  source,
  sourcePeer,
  submissionId,
  runtime,
}: {
  db: TestDatabase;
  session: SessionSeed;
  sessionId: string;
  executionId: string;
  source: SourceSubmission;
  sourcePeer: JournalRecord;
  submissionId: string;
  runtime: PromptSeedRuntime;
}): void {
  const metadata = JSON.parse(session.metadata || "{}") as Record<string, unknown>;
  metadata.agent_delivery_recovery = {
    original_runtime: runtime.originalRuntime,
    phase: "uncertain",
    revision: 1,
    session_id: sessionId,
    agent_execution_id: executionId,
    submission_id: submissionId,
    stream_id: sourcePeer.stream_id,
    incarnation_id: source.incarnation_id,
    harness_generation: source.harness_generation,
    prompt_generation: runtime.promptGeneration,
    updated_at: new Date().toISOString(),
  };
  metadata.last_agent_error = {
    code: "DURABLE_DELIVERY_UNCERTAIN",
    details: submissionId,
    agent_execution_id: executionId,
    message: "Delivery was interrupted.",
    occurred_at: new Date().toISOString(),
  };
  db.prepare("UPDATE task_sessions SET metadata = ? WHERE id = ?").run(
    JSON.stringify(metadata),
    sessionId,
  );
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
