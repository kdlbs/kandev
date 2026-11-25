import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import { createDecipheriv, randomUUID } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { devices, type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/remote-docker-test-base";
import type { RemoteDockerSeedData } from "../../fixtures/remote-docker-test-base";
import type { ApiClient } from "../../helpers/api-client";
import {
  readRecovery,
  seedInterruptedPrompt,
  withDatabase,
} from "../../helpers/interrupted-prompt-recovery";
import { waitForAgentMessage, waitForLatestSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { watchWs } from "../../helpers/causal-waits";
import { setupPage } from "../../fixtures/test-base";
import type { BackendContext } from "../../fixtures/backend";

/**
 * Same-session recovery on a Docker daemon reached over SSH.
 *
 * The first session is stopped without destroying its container, then resumed
 * with a new execution ID. The resumed launch must reconcile the previous
 * agentctl journal owner before opening the same journal, reattach to the same
 * container, and keep work that exists only inside that container.
 */
function containerExists(containerId: string): boolean {
  const out = execFileSync("docker", ["ps", "-aq", "--no-trunc", "--filter", `id=${containerId}`], {
    encoding: "utf8",
  })
    .toString()
    .trim();
  return out !== "";
}

function execInContainer(containerId: string, script: string): string {
  return execFileSync("docker", ["exec", containerId, "sh", "-lc", script], {
    encoding: "utf8",
  })
    .toString()
    .trim();
}

function execInContainerWithInput(containerId: string, script: string, input: string): string {
  try {
    return execFileSync("docker", ["exec", "-i", containerId, "sh", "-lc", script], {
      encoding: "utf8",
      input,
    })
      .toString()
      .trim();
  } catch {
    throw new Error("Authenticated agentctl request failed inside the task container");
  }
}

function findRepositoryRoot(containerId: string): string {
  const root = execInContainer(
    containerId,
    "repo=$(find /workspace -maxdepth 3 -name .git | head -1); " +
      'if [ -n "$repo" ]; then dirname "$repo"; else echo /workspace; fi',
  );
  expect(root, "the task container has no writable workspace").toMatch(/^\/workspace/);
  return root;
}

function journalPath(sessionId: string): string {
  return `/var/lib/kandev/agentctl-journals/${sessionId}/delivery.bbolt`;
}

function processIsAlive(pid: number): boolean {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

function seedUncertainQueueClaim(
  tmpDir: string,
  taskId: string,
  sessionId: string,
  incarnationId: string,
) {
  const entryId = `queue-uncertain-${randomUUID()}`;
  const attemptId = `attempt-${randomUUID()}`;
  const createdAt = new Date().toISOString();
  const message = JSON.stringify({
    id: entryId,
    session_id: sessionId,
    session_incarnation_id: incarnationId,
    task_id: taskId,
    position: 1,
    content: "separate queued instruction",
    model: "",
    plan_mode: false,
    attachments: [],
    queued_at: createdAt,
    queued_by: "user",
  });
  const claim = {
    entry_id: entryId,
    session_id: sessionId,
    attempt_id: attemptId,
    message_json: message,
    accepted: 0,
    protocol_mode: "legacy",
    submission_id: "",
    payload_hash: "",
    created_at: createdAt,
  };
  withDatabase(tmpDir, (db) => {
    db.exec(`
      CREATE TABLE IF NOT EXISTS queue_dispatch_claims (
        entry_id TEXT PRIMARY KEY,
        session_id TEXT NOT NULL,
        attempt_id TEXT NOT NULL,
        message_json TEXT NOT NULL,
        accepted INTEGER NOT NULL DEFAULT 0,
        protocol_mode TEXT NOT NULL DEFAULT 'legacy',
        submission_id TEXT NOT NULL DEFAULT '',
        payload_hash TEXT NOT NULL DEFAULT '',
        created_at TIMESTAMP NOT NULL
      )
    `);
    db.prepare(
      `
      INSERT INTO queue_dispatch_claims
        (entry_id, session_id, attempt_id, message_json, accepted, protocol_mode, submission_id, payload_hash, created_at)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    `,
    ).run(
      claim.entry_id,
      claim.session_id,
      claim.attempt_id,
      claim.message_json,
      claim.accepted,
      claim.protocol_mode,
      claim.submission_id,
      claim.payload_hash,
      claim.created_at,
    );
  });
  return claim;
}

function readQueueClaim(tmpDir: string, entryId: string) {
  return withDatabase(tmpDir, (db) =>
    db
      .prepare(
        `SELECT entry_id, session_id, attempt_id, message_json, accepted, protocol_mode,
                submission_id, payload_hash, created_at
         FROM queue_dispatch_claims WHERE entry_id = ?`,
      )
      .get(entryId),
  );
}

function readQueuedMessage(tmpDir: string, entryId: string) {
  return withDatabase(tmpDir, (db) =>
    db
      .prepare(
        "SELECT id, session_id, task_id, content, queued_by FROM queued_messages WHERE id = ?",
      )
      .get(entryId),
  );
}

function readExecutorRunning(tmpDir: string, sessionId: string) {
  return withDatabase(
    tmpDir,
    (db) =>
      db
        .prepare("SELECT status, agent_execution_id FROM executors_running WHERE session_id = ?")
        .get(sessionId) as { status: string; agent_execution_id: string } | undefined,
  );
}

function readDeliveryCursor(tmpDir: string, sessionId: string) {
  return withDatabase(
    tmpDir,
    (db) =>
      db
        .prepare(
          `SELECT stream_id, projected_sequence, remote_high_water
           FROM agent_delivery_cursors WHERE session_id = ? ORDER BY updated_at DESC LIMIT 1`,
        )
        .get(sessionId) as
        | { stream_id: string; projected_sequence: number; remote_high_water: number }
        | undefined,
  );
}

function readSecretValue(tmpDir: string, secretID: string): string | undefined {
  if (!secretID) return undefined;
  const row = withDatabase(
    tmpDir,
    (db) =>
      db.prepare("SELECT encrypted_value, nonce FROM secrets WHERE id = ?").get(secretID) as
        | { encrypted_value: Uint8Array; nonce: Uint8Array }
        | undefined,
  );
  if (!row) return undefined;
  const encrypted = Buffer.from(row.encrypted_value);
  const key = fs.readFileSync(path.join(tmpDir, ".kandev", "data", "master.key"));
  const decipher = createDecipheriv("aes-256-gcm", key, Buffer.from(row.nonce));
  decipher.setAuthTag(encrypted.subarray(-16));
  return Buffer.concat([decipher.update(encrypted.subarray(0, -16)), decipher.final()]).toString();
}

function readAgentctlCredentials(
  tmpDir: string,
  sessionId: string,
): { agentctlToken: string; controlToken: string } {
  const references = withDatabase(tmpDir, (db) => {
    const runtime = db
      .prepare(
        `SELECT e.container_control_auth_token_secret_id AS control_secret_id, r.metadata
         FROM executors_running r
         JOIN task_sessions s ON s.id = r.session_id
         JOIN task_environments e ON e.id = s.task_environment_id
         WHERE r.session_id = ?`,
      )
      .get(sessionId) as { control_secret_id: string; metadata: string } | undefined;
    const metadata = runtime ? (JSON.parse(runtime.metadata) as Record<string, unknown>) : {};
    const agentctlSecretID = metadata.env_secret_id_AGENTCTL_AUTH_TOKEN;
    return {
      agentctlSecretID: typeof agentctlSecretID === "string" ? agentctlSecretID : "",
      controlSecretID: runtime?.control_secret_id ?? "",
    };
  });
  const agentctlToken = readSecretValue(tmpDir, references.agentctlSecretID);
  const controlToken = readSecretValue(tmpDir, references.controlSecretID);
  expect(agentctlToken, "the fixture agentctl credential is missing").toBeTruthy();
  expect(controlToken, "the fixture control credential is missing").toBeTruthy();
  return { agentctlToken: agentctlToken!, controlToken: controlToken! };
}

async function findManagedAgentPID(
  tmpDir: string,
  sessionId: string,
  containerId: string,
): Promise<{ pid: number; agentctlPort: number; authToken: string }> {
  const runtime = withDatabase(
    tmpDir,
    (db) =>
      db
        .prepare("SELECT agentctl_url FROM executors_running WHERE session_id = ?")
        .get(sessionId) as { agentctl_url: string } | undefined,
  );
  expect(runtime?.agentctl_url, "the runtime agentctl endpoint is missing").toBeTruthy();
  const credentials = readAgentctlCredentials(tmpDir, sessionId);
  const status = await readAuthenticatedAgentctlJSON<{
    agent_status?: string;
    process_info?: { pid?: number };
  }>(runtime!.agentctl_url, "/api/v1/status", credentials.agentctlToken);
  if (status.agent_status !== "running" || !Number.isInteger(status.process_info?.pid)) {
    throw new Error("agentctl did not report a running managed process");
  }
  const controlPort = Number(execInContainer(containerId, 'printf "%s" "$AGENTCTL_PORT"'));
  const agentctlPort = readManagedAgentInstancePort(
    containerId,
    controlPort,
    sessionId,
    credentials.controlToken,
  );
  return { pid: status.process_info!.pid!, agentctlPort, authToken: credentials.agentctlToken };
}

async function readAuthenticatedAgentctlJSON<T>(
  endpoint: string,
  route: string,
  token: string,
): Promise<T> {
  let response: Response;
  try {
    response = await fetch(new URL(route, endpoint), {
      headers: { authorization: `Bearer ${token}` },
    });
  } catch {
    throw new Error(`Could not read authenticated agentctl route ${route}`);
  }
  if (!response.ok) throw new Error(`Could not read authenticated agentctl route ${route}`);
  return (await response.json()) as T;
}

function readManagedAgentInstancePort(
  containerId: string,
  controlPort: number,
  sessionId: string,
  authToken: string,
): number {
  expect(Number.isInteger(controlPort) && controlPort > 0, "agentctl control port is missing").toBe(
    true,
  );
  const response = execInContainerWithInput(
    containerId,
    `IFS= read -r token; printf 'header = "Authorization: Bearer %s"\\n' "$token" | ` +
      `curl --config - --max-time 3 -fsS http://127.0.0.1:${controlPort}/api/v1/instances`,
    `${authToken}\n`,
  );
  const payload = JSON.parse(response) as {
    instances?: Array<{ id: string; port: number; session_id?: string }>;
  };
  const instances =
    payload.instances?.filter((candidate) => candidate.session_id === sessionId) ?? [];
  expect(instances, "the control server has no unique instance for this session").toHaveLength(1);
  const instance = instances[0];
  expect(Number.isInteger(instance?.port) && instance!.port > 0).toBe(true);
  return instance!.port;
}

async function readAgentctlAssociationStatus(
  sessionId: string,
  containerId: string,
  agentctlPort: number,
  authToken: string,
) {
  expect(
    Number.isInteger(agentctlPort) && agentctlPort > 0,
    "the task session agentctl port is missing",
  ).toBe(true);
  const result = execInContainerWithInput(
    containerId,
    `IFS= read -r token; printf 'header = "Authorization: Bearer %s"\\n' "$token" | ` +
      `curl --config - --max-time 3 -sS -w '\\n__HTTP_STATUS__%{http_code}' ` +
      `http://127.0.0.1:${agentctlPort}/api/v1/agent/session 2>/dev/null || true`,
    `${authToken}\n`,
  );
  const statusMarker = "\n__HTTP_STATUS__";
  const marker = result.lastIndexOf(statusMarker);
  const httpStatus = marker < 0 ? "no HTTP response" : result.slice(marker + statusMarker.length);
  if (httpStatus !== "200") {
    const agentctlStatus = execInContainerWithInput(
      containerId,
      `IFS= read -r token; printf 'header = "Authorization: Bearer %s"\\n' "$token" | ` +
        `curl --config - --max-time 3 -sS -o /dev/null -w '%{http_code}' ` +
        `http://127.0.0.1:${agentctlPort}/api/v1/status 2>/dev/null || true`,
      `${authToken}\n`,
    );
    const healthStatus = execInContainer(
      containerId,
      `curl --max-time 3 -sS -o /dev/null -w '%{http_code}' ` +
        `http://127.0.0.1:${agentctlPort}/health || true`,
    );
    throw new Error(
      `agentctl association returned HTTP ${httpStatus}; status returned ${agentctlStatus}; ` +
        `health returned ${healthStatus} on instance port ${agentctlPort}`,
    );
  }
  const association = JSON.parse(result.slice(0, marker)) as {
    session_id: string;
    agent_status: string;
  };
  expect(association.session_id).toBe(sessionId);
  return association.agent_status;
}

function readAgentctlDeliveryHighWater(
  containerId: string,
  agentctlPort: number,
  authToken: string,
  streamId: string,
): number {
  const response = execInContainerWithInput(
    containerId,
    `IFS= read -r token; printf 'header = "Authorization: Bearer %s"\\n' "$token" | ` +
      `curl --config - --max-time 3 -fsS ` +
      `"http://127.0.0.1:${agentctlPort}/api/v1/agent/delivery?stream_id=${encodeURIComponent(streamId)}"`,
    `${authToken}\n`,
  );
  const descriptor = JSON.parse(response) as { stream?: { high_water?: number } };
  return descriptor.stream?.high_water ?? 0;
}

function containerProcessIsAlive(containerId: string, pid: number): boolean {
  return execInContainer(containerId, `test -e /proc/${pid} && echo true || echo false`) === "true";
}

function readProjectedTailCounts(
  tmpDir: string,
  streamId: string,
  afterSequence: number,
  throughSequence: number,
) {
  return withDatabase(
    tmpDir,
    (db) =>
      db
        .prepare(
          `SELECT COUNT(*) AS count, COUNT(DISTINCT sequence) AS distinct_count
         FROM agent_delivery_inbox
         WHERE stream_id = ? AND sequence > ? AND sequence <= ? AND projected_at IS NOT NULL`,
        )
        .get(streamId, afterSequence, throughSequence) as { count: number; distinct_count: number },
  );
}

function getNativeSessionId(session: { metadata?: Record<string, unknown> | null }): string {
  const acp = session.metadata?.acp;
  const nativeSessionId =
    typeof acp === "object" && acp !== null
      ? (acp as Record<string, unknown>).session_id
      : undefined;
  expect(nativeSessionId, "the session has no saved native ACP identity").toEqual(
    expect.any(String),
  );
  expect(nativeSessionId).not.toBe("");
  return nativeSessionId as string;
}

function expectExplicitResumePreservedUncertainty(
  tmpDir: string,
  blockId: string,
  submissionId: string,
) {
  const recovery = readRecovery(tmpDir, blockId, submissionId);
  expect(recovery.submission).toEqual({ state: "interrupted_unknown" });
  expect([
    { state: "open", authorized_action: "" },
    { state: "resolved", authorized_action: "resume" },
  ]).toContainEqual(recovery.block);
}

type RecoveryWireOutcome = { type: "response" | "error"; payload: unknown };

function isResumedAgentBootMessage(message: { type?: string; metadata?: Record<string, unknown> }) {
  return (
    message.type === "script_execution" &&
    message.metadata?.script_type === "agent_boot" &&
    message.metadata?.is_resuming === true
  );
}

function isSessionRestorationNotice(message: { metadata?: Record<string, unknown> }) {
  return message.metadata?.kind === "session_restored_after_interruption";
}

async function requestSessionResume(baseUrl: string, taskId: string, sessionId: string) {
  const requestId = `e2e-resume-${randomUUID()}`;
  const socket = new WebSocket(baseUrl.replace(/^http/, "ws") + "/ws");
  return new Promise<RecoveryWireOutcome>((resolve, reject) => {
    let settled = false;
    const timeout = setTimeout(() => {
      if (settled) return;
      settled = true;
      socket.close();
      reject(new Error("session.recover resume timed out"));
    }, 90_000);
    const finish = (outcome: RecoveryWireOutcome) => {
      if (settled) return;
      settled = true;
      clearTimeout(timeout);
      socket.close();
      resolve(outcome);
    };
    const fail = (error: Error) => {
      if (settled) return;
      settled = true;
      clearTimeout(timeout);
      socket.close();
      reject(error);
    };
    socket.addEventListener("open", () => {
      socket.send(
        JSON.stringify({
          id: requestId,
          type: "request",
          action: "session.recover",
          payload: { task_id: taskId, session_id: sessionId, action: "resume" },
          timestamp: new Date().toISOString(),
        }),
      );
    });
    socket.addEventListener("message", (event) => {
      let frame: { id?: string; type?: string; payload?: unknown };
      try {
        frame = JSON.parse(String(event.data)) as typeof frame;
      } catch (error) {
        fail(error instanceof Error ? error : new Error(String(error)));
        return;
      }
      if (frame.id !== requestId) return;
      if (frame.type === "response" || frame.type === "error") {
        finish({ type: frame.type, payload: frame.payload });
      } else {
        fail(new Error(`unexpected session.recover response type: ${String(frame.type)}`));
      }
    });
    socket.addEventListener("error", () => fail(new Error("session.recover websocket failed")));
  });
}

async function requestSessionResumeAfterRestart(
  baseUrl: string,
  taskId: string,
  sessionId: string,
) {
  let latest: RecoveryWireOutcome | undefined;
  await expect
    .poll(
      async () => {
        latest = await requestSessionResume(baseUrl, taskId, sessionId);
        if (latest.type === "response") return "ready";
        const payload = latest.payload as {
          details?: { kind?: string; retryable?: boolean };
        };
        if (
          payload.details?.kind === "session_recovery_in_progress" &&
          payload.details.retryable === true
        ) {
          return "reconciling";
        }
        throw new Error(`session Resume failed: ${JSON.stringify(latest.payload)}`);
      },
      {
        timeout: 90_000,
        message: "Wait for backend restart reconciliation before requesting Resume",
      },
    )
    .toBe("ready");
  return latest!;
}

async function seedRemoteLossAndRestart(
  backend: BackendContext,
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
  containerId: string,
) {
  const { session } = await apiClient.getTaskSession(sessionId);
  const executionId = session.agent_execution_id;
  expect(executionId, "the remote execution identity is missing").toBeTruthy();
  await apiClient.addUserMessage(taskId, sessionId, "/slow 30");
  await waitForAgentMessage(apiClient, sessionId, "Running slow response", 30_000);
  const beforeMessages = (await apiClient.listSessionMessages(sessionId)).messages;

  const pid = backend.pid();
  expect(pid, "the test-owned backend PID is unavailable").toBeTruthy();
  process.kill(pid!, "SIGKILL");
  await expect
    .poll(() => processIsAlive(pid!), {
      timeout: 30_000,
      message: "Wait for the owned backend to stop",
    })
    .toBe(false);

  execFileSync("docker", ["stop", "--time", "0", containerId]);
  const localJournalDir = path.join(backend.tmpDir, ".kandev", "agentctl-journals", sessionId);
  fs.mkdirSync(localJournalDir, { recursive: true });
  const localJournal = path.join(localJournalDir, "delivery.bbolt");
  execFileSync("docker", ["cp", `${containerId}:${journalPath(sessionId)}`, localJournal]);
  const interrupted = seedInterruptedPrompt(backend.tmpDir, sessionId, executionId, {
    allowUnverifiableRuntimeIdentity: true,
  });
  execFileSync("docker", ["cp", localJournal, `${containerId}:${journalPath(sessionId)}`]);
  const queueClaim = seedUncertainQueueClaim(
    backend.tmpDir,
    taskId,
    sessionId,
    session.queue_incarnation_id,
  );
  await backend.restart();
  return { interrupted, queueClaim, executionId, beforeMessages };
}

async function verifyRemoteLossRecoveryUI(
  page: Page,
  apiClient: ApiClient,
  backend: BackendContext,
  seedData: RemoteDockerSeedData,
  phone: boolean,
) {
  test.setTimeout(300_000);
  const { task, sessionId, containerId } = await prepareRecoverableTask(
    apiClient,
    seedData,
    phone ? "remote docker loss recovery phone" : "remote docker loss recovery desktop",
  );
  const { interrupted, queueClaim, executionId, beforeMessages } = await seedRemoteLossAndRestart(
    backend,
    apiClient,
    task.id,
    sessionId,
    containerId,
  );
  expect(containerExists(containerId), "the test should retain the stopped task container").toBe(
    true,
  );

  const ws = watchWs(page);
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await expect
    .poll(() => readExecutorRunning(backend.tmpDir, sessionId))
    .toEqual({ status: "stopped", agent_execution_id: executionId });
  await expect(session.recoveryRetryConnectionButton()).toBeVisible();
  const retryResponse = ws.waitForResponse("session.recover", {
    requestWhere: (payload) =>
      payload.action === "retry_connection" &&
      payload.task_id === task.id &&
      payload.session_id === sessionId,
  });
  if (phone) {
    const retryBox = await session.recoveryRetryConnectionButton().boundingBox();
    expect(retryBox?.height).toBeGreaterThanOrEqual(44);
    await session.recoveryRetryConnectionButton().tap();
  } else {
    await session.recoveryRetryConnectionButton().click();
  }
  await retryResponse;
  await expect(page.getByTestId("delivery-recovery-result")).toHaveText(
    "Recovery is blocked. Resolve the recovery issue before retrying.",
  );
  expect(readRecovery(backend.tmpDir, interrupted.blockId, interrupted.submissionId)).toEqual({
    block: { state: "open", authorized_action: "" },
    submission: { state: "interrupted_unknown" },
  });
  expect(readQueueClaim(backend.tmpDir, queueClaim.entry_id)).toEqual(queueClaim);

  const resumeOutcome = await requestSessionResume(backend.baseUrl, task.id, sessionId);
  expect(resumeOutcome.type, JSON.stringify(resumeOutcome.payload)).toBe("response");
  expect(resumeOutcome.payload).toMatchObject({
    session_id: sessionId,
    agent_execution_id: expect.any(String),
  });

  const afterAttempt = (await apiClient.listSessionMessages(sessionId)).messages;
  expect(
    afterAttempt.filter(
      (message) => !isResumedAgentBootMessage(message) && !isSessionRestorationNotice(message),
    ),
  ).toEqual(beforeMessages);
  expect(afterAttempt.filter(isResumedAgentBootMessage)).toHaveLength(1);
  expect((await apiClient.getTaskSession(sessionId)).session.id).toBe(sessionId);
  expectExplicitResumePreservedUncertainty(
    backend.tmpDir,
    interrupted.blockId,
    interrupted.submissionId,
  );
  expect(readQueueClaim(backend.tmpDir, queueClaim.entry_id)).toBeUndefined();
  expect(readQueuedMessage(backend.tmpDir, queueClaim.entry_id)).toMatchObject({
    id: queueClaim.entry_id,
    session_id: sessionId,
    task_id: task.id,
    content: "separate queued instruction",
    queued_by: "user",
  });

  await page.reload();
  await session.waitForLoad();
  expect((await apiClient.getTaskSession(sessionId)).session.task_id).toBe(task.id);
  const afterReloadMessages = (await apiClient.listSessionMessages(sessionId)).messages;
  expect(
    afterReloadMessages.filter(
      (message) => !isResumedAgentBootMessage(message) && !isSessionRestorationNotice(message),
    ),
  ).toEqual(beforeMessages);
  expect(afterReloadMessages.filter(isResumedAgentBootMessage)).toHaveLength(1);
  expectExplicitResumePreservedUncertainty(
    backend.tmpDir,
    interrupted.blockId,
    interrupted.submissionId,
  );
  expect(readQueueClaim(backend.tmpDir, queueClaim.entry_id)).toBeUndefined();
  expect(readQueuedMessage(backend.tmpDir, queueClaim.entry_id)).toMatchObject({
    id: queueClaim.entry_id,
    session_id: sessionId,
    task_id: task.id,
    content: "separate queued instruction",
    queued_by: "user",
  });
  if (phone) await assertNoDocumentHorizontalOverflow(page, "phone remote loss recovery");
}

const journalLockReady = "/tmp/kandev-journal-lock-ready";
const journalLockWaiting = "/tmp/kandev-journal-lock-waiting";
const journalLockPipe = "/tmp/kandev-journal-lock-pipe";

function startJournalLock(containerId: string, path: string): ChildProcess {
  return spawn(
    "docker",
    [
      "exec",
      containerId,
      "sh",
      "-lc",
      `rm -f ${journalLockReady} ${journalLockWaiting} ${journalLockPipe}; ` +
        `mkfifo ${journalLockPipe}; touch ${journalLockWaiting}; ` +
        `flock -x ${path} -c 'touch ${journalLockReady}; read line < ${journalLockPipe}'`,
    ],
    { stdio: "ignore" },
  );
}

async function releaseJournalLock(containerId: string, holder: ChildProcess, path: string) {
  execInContainer(
    containerId,
    `timeout 5 sh -c "printf 'release\\n' > ${journalLockPipe}" || true`,
  );
  holder.kill();
  await expect
    .poll(
      () =>
        execInContainer(
          containerId,
          `flock -n ${path} -c true >/dev/null 2>&1 && echo free || echo locked`,
        ),
      { timeout: 30_000, message: "Wait for the external journal lock to release" },
    )
    .toBe("free");
}

async function prepareRecoverableTask(
  apiClient: ApiClient,
  seedData: RemoteDockerSeedData,
  title: string,
) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      executor_profile_id: seedData.remoteDockerExecutorProfileId,
    },
  );
  await waitForLatestSessionDone(apiClient, task.id, 1, "Wait for the remote Docker session");
  const { sessions } = await apiClient.listTaskSessions(task.id);
  const sessionId = sessions[0]?.id;
  expect(sessionId, "the launched session is missing").toBeTruthy();
  await sendMarkerTurn(apiClient, task.id, sessionId!);

  const env = await apiClient.getTaskEnvironment(task.id);
  const containerId = env?.container_id;
  expect(containerId, "no container recorded for the task").toBeTruthy();
  expect(containerExists(containerId!)).toBe(true);

  // Work that exists only in the container: a committed marker and an
  // untracked marker. Neither is reachable from any backend checkout.
  const repoRoot = findRepositoryRoot(containerId!);
  const markerCommit = execInContainer(
    containerId!,
    `cd ${repoRoot} && ` +
      "if [ ! -d .git ] && [ ! -f .git ]; then git init -q; fi && " +
      "printf 'recovery marker\\n' > recovery-marker.txt && " +
      "git add recovery-marker.txt && " +
      "git -c user.name='E2E Test' -c user.email='e2e@test.local' commit -q -m 'recovery marker' && " +
      "git rev-parse HEAD",
  );
  expect(markerCommit).toMatch(/^[0-9a-f]{40}$/);
  execInContainer(
    containerId!,
    `cd ${repoRoot} && printf 'untracked marker\\n' > untracked-recovery-marker.txt`,
  );

  return { task, sessionId: sessionId!, containerId: containerId!, repoRoot, markerCommit };
}

async function stopSessionAndWait(apiClient: ApiClient, taskId: string, sessionId: string) {
  await apiClient.stopSession({ session_id: sessionId });
  await expect
    .poll(async () => (await apiClient.listTaskSessions(taskId)).sessions[0]?.state, {
      timeout: 60_000,
      message: "Wait for the stopped session",
    })
    .toBe("CANCELLED");
}

async function waitForResumedSession(apiClient: ApiClient, taskId: string, sessionId: string) {
  await expect
    .poll(
      async () =>
        (await apiClient.listTaskSessions(taskId)).sessions.find(
          (session) => session.id === sessionId,
        )?.state,
      { timeout: 90_000, message: "Wait for the resumed session to become ready" },
    )
    .toBe("WAITING_FOR_INPUT");
  const { session } = await apiClient.getTaskSession(sessionId);
  expect(
    session.session_recovery_blocks ?? [],
    "a recovered session must have no current block",
  ).toEqual([]);
}

async function killFixtureBackend(backend: BackendContext): Promise<number> {
  const pid = backend.pid();
  expect(pid, "the test-owned backend PID is unavailable").toBeTruthy();
  process.kill(pid!, "SIGKILL");
  await expect
    .poll(() => processIsAlive(pid!), {
      timeout: 30_000,
      message: "Wait for the owned backend to stop",
    })
    .toBe(false);
  return pid!;
}

async function sendMarkerTurn(
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
): Promise<string> {
  const marker = `resume-marker-${randomUUID()}`;
  await apiClient.addUserMessage(taskId, sessionId, `e2e:message("${marker}")`);
  await waitForAgentMessage(apiClient, sessionId, marker, 120_000);
  return marker;
}

test.describe("remote docker executor — recoverable resume", () => {
  test("failed session without a resume token keeps its Remote Docker workspace", async ({
    apiClient,
    seedData,
    backend,
    testPage,
  }) => {
    test.setTimeout(300_000);
    const { task, sessionId, containerId, repoRoot, markerCommit } = await prepareRecoverableTask(
      apiClient,
      seedData,
      "remote docker failed session without resume token",
    );
    const managedAgent = await findManagedAgentPID(backend.tmpDir, sessionId, containerId);
    await expect
      .poll(
        () =>
          readAgentctlAssociationStatus(
            sessionId,
            containerId,
            managedAgent.agentctlPort,
            managedAgent.authToken,
          ),
        {
          timeout: 15_000,
          message: "the fixture host should reach the live agentctl port",
        },
      )
      .toBe("running");

    const backendPid = await killFixtureBackend(backend);
    let backendStopped = true;
    try {
      const runtimeCheckpoint = withDatabase(backend.tmpDir, (db) => {
        db.prepare("UPDATE task_sessions SET state = 'FAILED', error_message = ? WHERE id = ?").run(
          "Agent process exited",
          sessionId,
        );
        db.prepare(
          "UPDATE executors_running SET resumable = 0, resume_token = '' WHERE session_id = ?",
        ).run(sessionId);
        return db
          .prepare(
            "SELECT container_id, agentctl_url, resumable, resume_token FROM executors_running WHERE session_id = ?",
          )
          .get(sessionId) as
          | { container_id: string; agentctl_url: string; resumable: number; resume_token: string }
          | undefined;
      });
      expect(runtimeCheckpoint).toMatchObject({
        container_id: containerId,
        resumable: 0,
        resume_token: "",
      });
      expect(runtimeCheckpoint?.agentctl_url).toMatch(/^\S+$/);
      execInContainer(containerId, `kill -9 ${managedAgent.pid}`);
      await expect
        .poll(() => containerProcessIsAlive(containerId, managedAgent.pid), {
          timeout: 15_000,
          message: "the fixture mock-agent should stop while its workspace stays attached",
        })
        .toBe(false);
      await expect
        .poll(
          () =>
            readAgentctlAssociationStatus(
              sessionId,
              containerId,
              managedAgent.agentctlPort,
              managedAgent.authToken,
            ),
          {
            timeout: 30_000,
            message: "authenticated agentctl association should report the stopped session",
          },
        )
        .toBe("stopped");

      await backend.restart();
      backendStopped = false;
      expect(processIsAlive(backendPid)).toBe(false);
      expect(containerExists(containerId), "startup removed the retained task container").toBe(
        true,
      );
      expect((await apiClient.getTaskEnvironment(task.id))?.container_id).toBe(containerId);
      expect(execInContainer(containerId, `cd ${repoRoot} && git rev-parse HEAD`)).toBe(
        markerCommit,
      );
      expect(
        execInContainer(containerId, `cat ${repoRoot}/untracked-recovery-marker.txt`),
      ).toContain("untracked marker");

      await testPage.goto(`/t/${task.id}?sessionId=${sessionId}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      const recoveryFailure = testPage.getByRole("heading", { name: "Session recovery failed" });
      await expect(recoveryFailure).toBeVisible();
      await expect(testPage.getByRole("button", { name: /Agent failed/ })).toBeVisible();
      await expect(session.recoveryResumeButton()).toBeVisible();
      await testPage.reload();
      await session.waitForLoad();
      await expect(
        testPage.getByRole("heading", { name: "Session recovery failed" }),
      ).toBeVisible();
      await expect(testPage.getByRole("button", { name: /Agent failed/ })).toBeVisible();
      await session.recoveryResumeButton().click();
      await waitForResumedSession(apiClient, task.id, sessionId);
      await testPage.reload();
      await session.waitForLoad();
      await expect(session.anyIdleInput()).toBeVisible();
      await sendMarkerTurn(apiClient, task.id, sessionId);

      expect(containerExists(containerId), "Resume replaced the Remote Docker container").toBe(
        true,
      );
      expect((await apiClient.getTaskEnvironment(task.id))?.container_id).toBe(containerId);
      expect(execInContainer(containerId, `cd ${repoRoot} && git rev-parse HEAD`)).toBe(
        markerCommit,
      );
      expect(
        execInContainer(containerId, `cat ${repoRoot}/untracked-recovery-marker.txt`),
      ).toContain("untracked marker");
    } finally {
      if (backendStopped) await backend.restart();
    }
  });

  test("failed tokenless session releases recovery after its recorded container exits", async ({
    apiClient,
    seedData,
    backend,
    testPage,
  }) => {
    test.setTimeout(240_000);
    const { task, sessionId, containerId } = await prepareRecoverableTask(
      apiClient,
      seedData,
      "remote docker failed tokenless exited container",
    );
    const backendPid = await killFixtureBackend(backend);
    let backendStopped = true;
    try {
      const checkpoint = withDatabase(backend.tmpDir, (db) => {
        db.prepare("UPDATE task_sessions SET state = 'FAILED', error_message = ? WHERE id = ?").run(
          "Agent process exited",
          sessionId,
        );
        db.prepare(
          "UPDATE executors_running SET resumable = 0, resume_token = '' WHERE session_id = ?",
        ).run(sessionId);
        return db
          .prepare(
            "SELECT container_id, resumable, resume_token FROM executors_running WHERE session_id = ?",
          )
          .get(sessionId) as
          | { container_id: string; resumable: number; resume_token: string }
          | undefined;
      });
      expect(checkpoint).toMatchObject({
        container_id: containerId,
        resumable: 0,
        resume_token: "",
      });
      execFileSync("docker", ["stop", containerId], { encoding: "utf8" });

      await backend.restart();
      backendStopped = false;
      expect(processIsAlive(backendPid)).toBe(false);
      expect(containerExists(containerId), "startup removed the recorded exited container").toBe(
        true,
      );
      expect((await apiClient.getTaskEnvironment(task.id))?.container_id).toBe(containerId);
      await expect
        .poll(() => readExecutorRunning(backend.tmpDir, sessionId)?.status, {
          timeout: 30_000,
          message: "the definitively exited recorded container should release runtime recovery",
        })
        .toBe("stopped");
      const recovered = readExecutorRunning(backend.tmpDir, sessionId);
      expect(recovered?.agent_execution_id).toBeTruthy();
      expect((await apiClient.getTaskSession(sessionId)).session.state).toBe("FAILED");
      await testPage.goto(`/t/${task.id}?sessionId=${sessionId}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await expect(
        testPage.getByRole("heading", { name: "Session recovery failed" }),
      ).toBeVisible();
      await expect(testPage.getByRole("button", { name: /Agent failed/ })).toBeVisible();
      await testPage.reload();
      await session.waitForLoad();
      await expect(
        testPage.getByRole("heading", { name: "Session recovery failed" }),
      ).toBeVisible();
      await expect(testPage.getByRole("button", { name: /Agent failed/ })).toBeVisible();
    } finally {
      if (backendStopped) await backend.restart();
    }
  });

  test("stopped agent resumes after restart in the same Remote Docker workspace", async ({
    apiClient,
    seedData,
    backend,
    testPage,
  }) => {
    test.setTimeout(300_000);
    const { task, sessionId, containerId, repoRoot, markerCommit } = await prepareRecoverableTask(
      apiClient,
      seedData,
      "remote docker stopped agent restart recovery",
    );
    const originalSession = (await apiClient.getTaskSession(sessionId)).session;
    const originalNativeSessionId = getNativeSessionId(originalSession);
    const managedAgent = await findManagedAgentPID(backend.tmpDir, sessionId, containerId);
    await expect
      .poll(
        () =>
          readAgentctlAssociationStatus(
            sessionId,
            containerId,
            managedAgent.agentctlPort,
            managedAgent.authToken,
          ),
        {
          timeout: 15_000,
          message: "the fixture host should reach the live agentctl port",
        },
      )
      .toBe("running");

    await apiClient.addUserMessage(task.id, sessionId, "/slow 30");
    await waitForAgentMessage(apiClient, sessionId, "Running slow response", 30_000);
    const initialCursor = readDeliveryCursor(backend.tmpDir, sessionId);
    expect(initialCursor, "the prompt has no projected delivery cursor").toBeTruthy();
    const streamId = initialCursor!.stream_id;
    const initialRemoteHighWater = readAgentctlDeliveryHighWater(
      containerId,
      managedAgent.agentctlPort,
      managedAgent.authToken,
      streamId,
    );
    expect(initialRemoteHighWater).toBeGreaterThanOrEqual(initialCursor!.projected_sequence);

    const backendPid = await killFixtureBackend(backend);
    let backendStopped = true;
    try {
      const outageCursor = initialCursor!.projected_sequence;
      // /slow 30 completes only after this outage window and leaves its final
      // output in the retained agentctl journal before the process is stopped.
      await expect
        .poll(
          () =>
            readAgentctlDeliveryHighWater(
              containerId,
              managedAgent.agentctlPort,
              managedAgent.authToken,
              streamId,
            ),
          {
            timeout: 40_000,
            message:
              "the slow command should append output to the retained journal during the outage",
          },
        )
        .toBeGreaterThan(initialRemoteHighWater);
      execInContainer(containerId, `kill -9 ${managedAgent.pid}`);
      await expect
        .poll(() => containerProcessIsAlive(containerId, managedAgent.pid), {
          timeout: 15_000,
          message: "the exact fixture mock-agent process should stop during the backend outage",
        })
        .toBe(false);
      await expect
        .poll(
          () =>
            readAgentctlAssociationStatus(
              sessionId,
              containerId,
              managedAgent.agentctlPort,
              managedAgent.authToken,
            ),
          {
            timeout: 30_000,
            message: "authenticated agentctl association should report the stopped session",
          },
        )
        .toBe("stopped");
      const stoppedRemoteHighWater = readAgentctlDeliveryHighWater(
        containerId,
        managedAgent.agentctlPort,
        managedAgent.authToken,
        streamId,
      );
      expect(stoppedRemoteHighWater).toBeGreaterThan(initialRemoteHighWater);

      await backend.restart();
      backendStopped = false;
      expect(processIsAlive(backendPid)).toBe(false);
      await expect
        .poll(async () => (await apiClient.getTaskSession(sessionId)).session.state, {
          timeout: 90_000,
          message: "startup recovery should settle the stopped agent session",
        })
        .toBe("WAITING_FOR_INPUT");

      const settledSession = (await apiClient.getTaskSession(sessionId)).session;
      expect(getNativeSessionId(settledSession)).toBe(originalNativeSessionId);
      expect(readExecutorRunning(backend.tmpDir, sessionId)?.status).toBe("stopped");
      expect(containerExists(containerId)).toBe(true);
      expect((await apiClient.getTaskEnvironment(task.id))?.container_id).toBe(containerId);

      await expect
        .poll(() => readDeliveryCursor(backend.tmpDir, sessionId)?.projected_sequence ?? 0, {
          timeout: 30_000,
          message: "startup replay should project the retained output from the stopped agent",
        })
        .toBeGreaterThan(outageCursor);
      const settledCursor = readDeliveryCursor(backend.tmpDir, sessionId)!;
      expect(settledCursor.stream_id).toBe(streamId);
      expect(settledCursor.projected_sequence).toBeGreaterThan(outageCursor);
      expect(settledCursor.projected_sequence).toBeGreaterThanOrEqual(stoppedRemoteHighWater);
      const tail = readProjectedTailCounts(
        backend.tmpDir,
        streamId,
        outageCursor,
        stoppedRemoteHighWater,
      );
      expect(tail.count).toBeGreaterThan(0);
      expect(tail.distinct_count).toBe(tail.count);

      await testPage.goto(`/t/${task.id}?sessionId=${sessionId}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await expect(session.anyIdleInput()).toBeVisible();
      await expect(session.activeChat()).toContainText("Running slow response");
      await testPage.reload();
      await session.waitForLoad();
      await expect(session.anyIdleInput()).toBeVisible();
      await expect(session.activeChat()).toContainText("Running slow response");

      const resumeOutcome = await requestSessionResumeAfterRestart(
        backend.baseUrl,
        task.id,
        sessionId,
      );
      expect(resumeOutcome.type, JSON.stringify(resumeOutcome.payload)).toBe("response");
      await waitForResumedSession(apiClient, task.id, sessionId);
      expect(getNativeSessionId((await apiClient.getTaskSession(sessionId)).session)).toBe(
        originalNativeSessionId,
      );
      await sendMarkerTurn(apiClient, task.id, sessionId);

      expect(containerExists(containerId), "Resume replaced the Remote Docker container").toBe(
        true,
      );
      expect((await apiClient.getTaskEnvironment(task.id))?.container_id).toBe(containerId);
      expect(execInContainer(containerId, `cd ${repoRoot} && git rev-parse HEAD`)).toBe(
        markerCommit,
      );
      expect(
        execInContainer(containerId, `cat ${repoRoot}/untracked-recovery-marker.txt`),
      ).toContain("untracked marker");

      await apiClient.deleteTask(task.id);
      await expect
        .poll(() => containerExists(containerId), {
          message: "container survived task deletion on the remote daemon",
          timeout: 90_000,
        })
        .toBe(false);
    } finally {
      if (backendStopped) await backend.restart();
    }
  });

  test("same-session resume keeps the container, journal, and unpushed work", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(300_000);
    const { task, sessionId, containerId, repoRoot, markerCommit } = await prepareRecoverableTask(
      apiClient,
      seedData,
      "remote docker recoverable resume",
    );

    await stopSessionAndWait(apiClient, task.id, sessionId);
    const resumed = await apiClient.launchSession(
      {
        task_id: task.id,
        session_id: sessionId,
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.remoteDockerExecutorProfileId,
        intent: "resume",
      },
      90_000,
    );
    expect(resumed.session_id).toBe(sessionId);
    expect(resumed.agent_execution_id).toMatch(/\S/);
    await expect
      .poll(
        async () =>
          (await apiClient.listTaskSessions(task.id)).sessions.find(
            (session) => session.id === sessionId,
          )?.agent_execution_id,
        { timeout: 60_000, message: "Wait for the replacement execution identity" },
      )
      .toBe(resumed.agent_execution_id);
    await waitForResumedSession(apiClient, task.id, sessionId);
    await sendMarkerTurn(apiClient, task.id, sessionId);

    const resumedEnv = await apiClient.getTaskEnvironment(task.id);
    expect(resumedEnv?.container_id, "resume launched a new container").toBe(containerId);
    expect(containerExists(containerId)).toBe(true);
    expect(findRepositoryRoot(containerId)).toBe(repoRoot);
    expect(execInContainer(containerId, `cd ${repoRoot} && git rev-parse HEAD`)).toBe(markerCommit);
    expect(execInContainer(containerId, `cat ${repoRoot}/recovery-marker.txt`)).toContain(
      "recovery marker",
    );
    expect(execInContainer(containerId, `cat ${repoRoot}/untracked-recovery-marker.txt`)).toContain(
      "untracked marker",
    );

    await apiClient.deleteTask(task.id);
    await expect
      .poll(() => containerExists(containerId), {
        message: "container survived task deletion on the remote daemon",
        timeout: 90_000,
      })
      .toBe(false);
  });

  test("blocked journal acquisition preserves the workspace and retries after release", async ({
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(300_000);
    const { task, sessionId, containerId, repoRoot, markerCommit } = await prepareRecoverableTask(
      apiClient,
      seedData,
      "remote docker blocked journal recovery",
    );

    await stopSessionAndWait(apiClient, task.id, sessionId);
    const path = journalPath(sessionId);
    const holder = startJournalLock(containerId, path);
    try {
      await expect
        .poll(
          () =>
            execInContainer(
              containerId,
              `test -f ${journalLockWaiting} && echo waiting || echo absent`,
            ),
          { timeout: 30_000, message: "Wait for the external journal lock to start" },
        )
        .toBe("waiting");

      const before = (await apiClient.getTaskSession(sessionId)).session.agent_execution_id;
      let launchError: unknown;
      let launchResult: { state?: string } | undefined;
      try {
        launchResult = await apiClient.launchSession(
          {
            task_id: task.id,
            session_id: sessionId,
            agent_profile_id: seedData.agentProfileId,
            executor_profile_id: seedData.remoteDockerExecutorProfileId,
            intent: "resume",
          },
          90_000,
        );
      } catch (error) {
        launchError = error;
      }
      expect(
        launchError !== undefined || launchResult?.state !== "WAITING_FOR_INPUT",
        "a held journal lock must not produce a ready session",
      ).toBe(true);

      const launchErrorText = String(launchError);
      expect(launchErrorText).toMatch(
        /delivery journal unavailable: open delivery journal: lock_timeout/i,
      );
      const backendLog = fs.readFileSync(backend.logPath, "utf8");
      const failedLaunchRecord = backendLog
        .split("\n")
        .find(
          (line) =>
            line.includes("failed to relaunch agent for session") &&
            line.includes(sessionId) &&
            line.includes("lock_timeout"),
        );
      expect(
        failedLaunchRecord,
        "the backend must persist the journal failure for this session",
      ).toBeTruthy();
      const blockedExecutionID = /instance ([0-9a-f-]{36}) delivery journal unavailable/.exec(
        failedLaunchRecord!,
      )?.[1];
      expect(blockedExecutionID, "the journal failure must name the blocked attempt").toBeTruthy();
      expect(blockedExecutionID).not.toBe(before ?? "");
      expect((await apiClient.getTaskSession(sessionId)).session.state).toBe("CANCELLED");

      await expect
        .poll(
          () =>
            execInContainer(
              containerId,
              `test -f ${journalLockReady} && echo ready || echo waiting`,
            ),
          { timeout: 60_000, message: "Wait for the blocked writer to acquire the released lock" },
        )
        .toBe("ready");

      expect(containerExists(containerId), "blocked recovery removed the workspace").toBe(true);
      expect(execInContainer(containerId, `cd ${repoRoot} && git rev-parse HEAD`)).toBe(
        markerCommit,
      );
      expect(execInContainer(containerId, `cat ${repoRoot}/recovery-marker.txt`)).toContain(
        "recovery marker",
      );
    } finally {
      await releaseJournalLock(containerId, holder, path);
    }

    // The reported incident crossed a backend restart. The container, journal,
    // and durable execution record must survive it before the retry.
    await backend.restart();

    const resumed = await apiClient.launchSession(
      {
        task_id: task.id,
        session_id: sessionId,
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.remoteDockerExecutorProfileId,
        intent: "resume",
      },
      90_000,
    );
    expect(resumed.session_id).toBe(sessionId);
    await waitForResumedSession(apiClient, task.id, sessionId);
    await sendMarkerTurn(apiClient, task.id, sessionId);
    expect((await apiClient.getTaskEnvironment(task.id))?.container_id).toBe(containerId);
    expect(execInContainer(containerId, `cd ${repoRoot} && git rev-parse HEAD`)).toBe(markerCommit);
    expect(execInContainer(containerId, `cat ${repoRoot}/untracked-recovery-marker.txt`)).toContain(
      "untracked marker",
    );

    await apiClient.deleteTask(task.id);
    await expect
      .poll(() => containerExists(containerId), {
        message: "container survived task deletion on the remote daemon",
        timeout: 90_000,
      })
      .toBe(false);
  });
});

test.describe("remote docker confirmed absence recovery", () => {
  test.describe.configure({ retries: 0 });

  test("desktop Resume reaches recovery after the exact task container stops", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    await verifyRemoteLossRecoveryUI(testPage, apiClient, backend, seedData, false);
  });

  test("phone Resume reaches recovery after the exact task container stops", async ({
    browser,
    apiClient,
    seedData,
    backend,
  }) => {
    const context = await browser.newContext({
      ...devices["Pixel 5"],
      baseURL: backend.frontendUrl,
    });
    try {
      const page = await context.newPage();
      await setupPage(page, backend);
      await verifyRemoteLossRecoveryUI(page, apiClient, backend, seedData, true);
    } finally {
      await context.close();
    }
  });
});
