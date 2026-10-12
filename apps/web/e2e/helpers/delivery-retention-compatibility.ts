import path from "node:path";
import { createHash, randomUUID } from "node:crypto";
import { chmodSync, copyFileSync, mkdirSync, readFileSync } from "node:fs";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { expect, type Page } from "@playwright/test";
import { SessionPage } from "../pages/session-page";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { DatabaseSync } from "./node-sqlite";

export function assertFrozenRetentionBinary(binary: string) {
  expect(createHash("sha256").update(readFileSync(binary)).digest("hex")).toBe(
    "3df1e310220f0a7588453c062f2fa90aa0a611dd7277cc77c76dd4f8bc3a0517",
  );
}

export function prepareFrozenRetentionRuntime(backend: BackendContext, frozenBinary: string) {
  assertFrozenRetentionBinary(frozenBinary);
  const runtimeDir = path.join(backend.tmpDir, `release-n-runtime-${randomUUID()}`);
  const binary = path.join(runtimeDir, "kandev");
  const syntheticHelper = path.resolve(__dirname, "../../../backend/bin/mock-agent");
  mkdirSync(runtimeDir);
  copyFileSync(frozenBinary, binary);
  copyFileSync(syntheticHelper, path.join(runtimeDir, "mock-agent"));
  chmodSync(binary, 0o755);
  chmodSync(path.join(runtimeDir, "mock-agent"), 0o755);
  assertFrozenRetentionBinary(binary);
  return {
    binary,
    provenance: {
      frozenBinary,
      frozenSha256: createHash("sha256").update(readFileSync(binary)).digest("hex"),
      syntheticHelper,
      syntheticHelperSha256: createHash("sha256")
        .update(readFileSync(syntheticHelper))
        .digest("hex"),
      syntheticHelperRole: "current E2E mock provider; Release N backend binary is unchanged",
    },
  };
}

export async function projectOrdinaryRetentionStream(backend: BackendContext, taskId: string) {
  const result = await promisify(execFile)(
    "go",
    [
      "test",
      "-tags",
      "sqlite_fts5",
      "./internal/task/repository/sqlite",
      "-run",
      "^TestRetentionBrowserFixtureDriver$",
      "-count=1",
      "-v",
    ],
    {
      cwd: path.resolve(__dirname, "../../../backend"),
      env: {
        ...process.env,
        KANDEV_RETENTION_BROWSER_FIXTURE_DB: path.join(backend.tmpDir, "kandev.db"),
        KANDEV_RETENTION_BROWSER_FIXTURE_TASK_ID: taskId,
      },
      timeout: 60_000,
    },
  ).catch((error: Error & { stdout?: string; stderr?: string }) => {
    throw new Error(`${error.message}\n${error.stdout ?? ""}\n${error.stderr ?? ""}`);
  });
  expect(result.stdout).toContain("--- PASS: TestRetentionBrowserFixtureDriver");
  return "retention-browser-settled-session";
}

export function retentionEvidence(backend: BackendContext, sessionId: string) {
  const db = new DatabaseSync(path.join(backend.tmpDir, "kandev.db"));
  try {
    return {
      marker: db.prepare("SELECT * FROM agent_delivery_retention_state WHERE id = 1").get(),
      cursors: db
        .prepare(
          "SELECT stream_id, incarnation_id, harness_generation, received_sequence, projected_sequence, remote_high_water, retired_sequence FROM agent_delivery_cursors WHERE session_id = ? ORDER BY stream_id",
        )
        .all(sessionId),
      hashes: db
        .prepare(
          "SELECT stream_id, sequence, event_type, hash_version, content_hash, payload_pruned_at, LENGTH(payload) AS payload_length FROM agent_delivery_inbox WHERE session_id = ? ORDER BY stream_id, sequence",
        )
        .all(sessionId),
      effects: db
        .prepare(
          "SELECT effect_key, stream_id, sequence, effect_type, state, session_id, incarnation_id, harness_generation, submission_id, turn_id, outcome, created_at, completed_at FROM agent_delivery_effects WHERE session_id = ? ORDER BY effect_key",
        )
        .all(sessionId),
      prompts: db
        .prepare(
          "SELECT id, payload_hash, state, outcome, payload_pruned_at, fully_settled_at, terminal_stream_id, terminal_sequence, terminal_turn_id, created_at, updated_at, LENGTH(payload) AS payload_length FROM agent_delivery_submissions WHERE session_id = ? ORDER BY id",
        )
        .all(sessionId),
    };
  } finally {
    db.close();
  }
}

export function expectRetainedEvidencePreserved(
  before: ReturnType<typeof retentionEvidence>,
  after: ReturnType<typeof retentionEvidence>,
  options: { allowNewPrompts?: boolean } = {},
) {
  expect(after.marker).toEqual(before.marker);
  if (!options.allowNewPrompts) expect(after.prompts).toHaveLength(before.prompts.length);
  for (const old of before.prompts)
    expect(after.prompts.find((row) => row.id === old.id)).toEqual(old);
  for (const old of before.effects)
    expect(after.effects.find((row) => row.effect_key === old.effect_key)).toEqual(old);
  for (const old of before.hashes)
    expect(
      after.hashes.find((row) => row.stream_id === old.stream_id && row.sequence === old.sequence),
    ).toEqual(old);
  for (const old of before.cursors) {
    const current = after.cursors.find((row) => row.stream_id === old.stream_id);
    expect(current).toBeDefined();
    expect(current).toEqual({
      ...old,
      received_sequence: current?.received_sequence,
      projected_sequence: current?.projected_sequence,
      remote_high_water: current?.remote_high_water,
    });
    expect(Number(current?.received_sequence)).toBeGreaterThanOrEqual(
      Number(old.received_sequence),
    );
    expect(Number(current?.projected_sequence)).toBeGreaterThanOrEqual(
      Number(old.projected_sequence),
    );
    expect(Number(current?.remote_high_water)).toBeGreaterThanOrEqual(
      Number(old.remote_high_water),
    );
  }
}

export function retentionJob(backend: BackendContext, sessionId: string) {
  const db = new DatabaseSync(path.join(backend.tmpDir, "kandev.db"));
  try {
    return db
      .prepare(
        "SELECT session_id, incarnation_id, phase, last_stream_id, last_sequence FROM agent_delivery_cleanup_jobs WHERE session_id = ?",
      )
      .get(sessionId);
  } finally {
    db.close();
  }
}

export async function retireSavedDelivery(
  backend: BackendContext,
  sessionId: string,
  restartOverrides: Record<string, string> = {},
) {
  await backend.restart({ ...restartOverrides, KANDEV_TASK_DELIVERY_RETENTION_INTERVAL: "10s" });
  try {
    await expect
      .poll(
        () => {
          const evidence = retentionEvidence(backend, sessionId);
          return (
            evidence.cursors.some((row) => Number(row.retired_sequence) > 0) &&
            evidence.prompts.some((row) => row.payload_pruned_at !== null)
          );
        },
        { timeout: 90_000 },
      )
      .toBe(true);
  } catch (error) {
    throw new Error(
      `${String(error)}\nDelivery evidence: ${JSON.stringify(retentionEvidence(backend, sessionId))}`,
    );
  }
}

export async function prepareRetainedRetirementTask(page: Page, api: ApiClient, seed: SeedData) {
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    "Release N deletion journal",
    seed.agentProfileId,
    {
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
      executor_profile_id: seed.worktreeExecutorProfileId,
      start_agent: false,
    },
  );
  const prepared = await api.launchSession({
    task_id: task.id,
    intent: "prepare",
    agent_profile_id: seed.agentProfileId,
    executor_profile_id: seed.worktreeExecutorProfileId,
    launch_workspace: true,
  });
  expect(prepared.session_id).toBeTruthy();
  const sessionId = prepared.session_id;
  await page.goto(`/t/${task.id}?sessionId=${sessionId}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.sendMessageViaButton("/slow 1s");
  await expect(
    session.chat.getByText("Slow response complete after 1s.", { exact: false }),
  ).toBeVisible({ timeout: 60_000 });
  const primary = (await api.listTaskSessions(task.id)).sessions.find((row) => row.is_primary);
  if (!primary) throw new Error("Release N deletion task has no primary session");
  expect(primary.id).toBe(sessionId);
  await expect
    .poll(
      async () =>
        (await api.listTaskSessions(task.id)).sessions.find((row) => row.id === sessionId)?.state,
      { timeout: 30_000 },
    )
    .toBe("WAITING_FOR_INPUT");
  return { taskId: task.id, sessionId };
}

function manualPromptEvidence(backend: BackendContext, sessionId: string) {
  const db = new DatabaseSync(path.join(backend.tmpDir, "kandev.db"));
  try {
    return db
      .prepare(
        "SELECT s.id, s.state, (SELECT COUNT(*) FROM agent_delivery_inbox i JOIN agent_delivery_cursors c ON c.stream_id = i.stream_id AND c.session_id = i.session_id WHERE i.session_id = s.session_id AND i.submission_id = s.id AND i.event_type = 'message_chunk' AND i.projected_at IS NOT NULL AND i.sequence <= c.projected_sequence) AS projected_events FROM agent_delivery_submissions s WHERE s.session_id = ? ORDER BY s.id",
      )
      .all(sessionId);
  } finally {
    db.close();
  }
}

export async function createPendingRetirementOnReleaseN(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  fixture: {
    backend: BackendContext;
    retainedTask: { taskId: string; sessionId: string };
  },
) {
  const { backend, retainedTask } = fixture;
  const { taskId, sessionId } = retainedTask;
  const previousIds = new Set(manualPromptEvidence(backend, sessionId).map((row) => row.id));
  expect(previousIds.size).toBe(1);
  await page.goto(`/t/${taskId}?sessionId=${sessionId}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.sendMessageViaButton("/slow 2s");
  await expect(
    session.chat.getByText("Slow response complete after 2s.", { exact: false }),
  ).toBeVisible({ timeout: 60_000 });
  await expect
    .poll(
      () => {
        const fresh = manualPromptEvidence(backend, sessionId).filter(
          (row) => !previousIds.has(row.id),
        );
        return (
          fresh.length === 1 &&
          fresh[0].state === "completed" &&
          Number(fresh[0].projected_events) > 0
        );
      },
      { timeout: 30_000 },
    )
    .toBe(true);
  await expect
    .poll(
      async () =>
        (await api.listTaskSessions(taskId)).sessions.find((row) => row.id === sessionId)?.state,
      { timeout: 30_000 },
    )
    .toBe("WAITING_FOR_INPUT");
  expect(
    (await api.listSessionMessages(sessionId)).messages.filter(
      (row) => row.author_type === "user" && row.content.trim() === "/slow 2s",
    ),
  ).toHaveLength(1);
  expect(retentionEvidence(backend, sessionId).hashes.length).toBeGreaterThan(0);
  await api.deleteTask(taskId, { discardWorktreeChanges: true });
  const job = retentionJob(backend, sessionId);
  expect(job?.phase).toBe("pending");
  const successor = await api.createTask(seed.workspaceId, "Release N deterministic ID fence", {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
    agent_profile_id: seed.agentProfileId,
    repository_ids: [seed.repositoryId],
  });
  const admittedId = randomUUID();
  const admitted = await api.seedTaskSession(successor.id, {
    state: "WAITING_FOR_INPUT",
    sessionId: admittedId,
    agentProfileId: seed.agentProfileId,
    repositoryId: seed.repositoryId,
  });
  expect(admitted.session_id).toBe(admittedId);
  await expect(
    api.seedTaskSession(successor.id, {
      state: "WAITING_FOR_INPUT",
      sessionId,
      agentProfileId: seed.agentProfileId,
      repositoryId: seed.repositoryId,
    }),
  ).rejects.toThrow(/agent delivery identity is retiring/);
  expect(retentionJob(backend, sessionId)).toEqual(job);
  return { sessionId, job };
}
