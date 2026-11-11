import { execFile, spawn } from "node:child_process";
import { randomUUID } from "node:crypto";
import path from "node:path";
import { promisify } from "node:util";
import { expect, test, type Page } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import { SessionPage } from "../pages/session-page";
import { attachGatewayTrafficCapture } from "./ws-traffic";
import { attachMessageAddCapture } from "./ws-capture";
import { countSessionMessageAdds } from "./silent-restart-recovery";
import {
  findSessionJournal,
  withDatabase,
  type JournalRecord,
} from "./interrupted-prompt-recovery";

export type IncidentShape = "matching" | "independent";

const execFileAsync = promisify(execFile);

async function readJournalSubmission(journalPath: string, submissionId: string) {
  const fixtureBinary = path.resolve(
    process.cwd(),
    `../backend/bin/mock-agent${process.platform === "win32" ? ".exe" : ""}`,
  );
  return new Promise<JournalRecord>((resolve, reject) => {
    const child = spawn(fixtureBinary, ["--delivery-journal-fixture", "read"], {
      stdio: ["pipe", "pipe", "pipe"],
    });
    let stdout = "";
    let stderr = "";
    let didTimeout = false;
    const timeout = setTimeout(() => {
      didTimeout = true;
      child.kill("SIGKILL");
    }, 15_000);
    child.stdout.setEncoding("utf8").on("data", (chunk: string) => {
      stdout += chunk;
    });
    child.stderr.setEncoding("utf8").on("data", (chunk: string) => {
      stderr += chunk;
    });
    child.once("error", (error) => {
      clearTimeout(timeout);
      reject(error);
    });
    child.once("close", (code, signal) => {
      clearTimeout(timeout);
      if (didTimeout) {
        reject(new Error("journal fixture read timed out"));
        return;
      }
      if (code !== 0) {
        reject(new Error(`journal fixture exited with ${code ?? signal}: ${stderr}`));
        return;
      }
      try {
        resolve(JSON.parse(stdout) as JournalRecord);
      } catch (error) {
        reject(error);
      }
    });
    child.stdin.end(JSON.stringify({ path: journalPath, submission: { id: submissionId } }));
  });
}

type RepeatedNativeResumeFixture = {
  submission_id: string;
  bound_block_id: string;
  generic_block_id: string;
  independent_block_id?: string;
  historical_submission_id: string;
  historical_block_id: string;
  historical_stream_id: string;
  historical_payload: string;
  historical_payload_hash: string;
  incarnation_id: string;
  generation: number;
  stream_id: string;
  payload: string;
  payload_hash: string;
};

async function seedSuccessorGenerationInterruption(
  tmpDir: string,
  sessionId: string,
  shape: IncidentShape,
) {
  const journalPath = findSessionJournal(tmpDir, sessionId);
  const submissionId = `prompt:${randomUUID()}`;
  const boundBlockId = randomUUID();
  const genericBlockId = randomUUID();
  const historicalSubmissionId = `prompt:${randomUUID()}`;
  const historicalBlockId = randomUUID();
  const independentBlockId = shape === "independent" ? randomUUID() : undefined;
  const payload = Buffer.from(
    JSON.stringify({ text: "A distinct successor-generation instruction." }),
  );
  const fixtureBinary = path.resolve(
    process.cwd(),
    `../backend/bin/e2e-delivery-fixture${process.platform === "win32" ? ".exe" : ""}`,
  );
  let fixtureOutput: string;
  try {
    const result = await execFileAsync(
      fixtureBinary,
      ["-test.run=^TestRepeatedNativeResumeFixture$", "-test.timeout=30s", "-test.v"],
      {
        cwd: path.resolve(process.cwd(), "../backend"),
        env: {
          ...process.env,
          KANDEV_E2E_REPEAT_RESUME_ROOT: tmpDir,
          KANDEV_E2E_REPEAT_RESUME_SESSION: sessionId,
          KANDEV_E2E_REPEAT_RESUME_SHAPE: shape,
          KANDEV_E2E_REPEAT_RESUME_SUBMISSION: submissionId,
          KANDEV_E2E_REPEAT_RESUME_BOUND_BLOCK: boundBlockId,
          KANDEV_E2E_REPEAT_RESUME_GENERIC_BLOCK: genericBlockId,
          KANDEV_E2E_REPEAT_RESUME_HISTORICAL_SUBMISSION: historicalSubmissionId,
          KANDEV_E2E_REPEAT_RESUME_HISTORICAL_BLOCK: historicalBlockId,
          KANDEV_E2E_REPEAT_RESUME_INDEPENDENT_BLOCK: independentBlockId ?? "",
          KANDEV_E2E_REPEAT_RESUME_PAYLOAD: payload.toString("base64"),
        },
        encoding: "utf8",
        timeout: 35_000,
        maxBuffer: 1 << 20,
      },
    );
    fixtureOutput = result.stdout;
  } catch (failure) {
    const output = failure as { stdout?: string; stderr?: string };
    throw new Error(
      `repeated Resume fixture failed:\n${output.stdout ?? ""}\n${output.stderr ?? ""}`,
      { cause: failure },
    );
  }
  const marker = "KANDEV_REPEATED_NATIVE_RESUME_FIXTURE:";
  const fixtureLine = fixtureOutput.split("\n").find((line) => line.startsWith(marker));
  if (!fixtureLine) throw new Error("repeated Resume fixture returned no seed evidence");
  const seeded = JSON.parse(fixtureLine.slice(marker.length)) as RepeatedNativeResumeFixture;
  const fixture = {
    sessionId,
    historicalSubmissionId: seeded.historical_submission_id,
    historicalBlockId: seeded.historical_block_id,
    historicalStreamId: seeded.historical_stream_id,
    historicalPayload: Buffer.from(seeded.historical_payload, "base64"),
    historicalHash: seeded.historical_payload_hash,
    submissionId: seeded.submission_id,
    blockId: seeded.bound_block_id,
    genericBlockId: seeded.generic_block_id,
    independentBlockId: seeded.independent_block_id,
    generation: seeded.generation,
    incarnationId: seeded.incarnation_id,
    streamId: seeded.stream_id,
    payload: Buffer.from(seeded.payload, "base64"),
    hash: seeded.payload_hash,
  };
  const historicalPeer: JournalRecord = {
    id: fixture.historicalSubmissionId,
    session_id: sessionId,
    incarnation_id: fixture.incarnationId,
    stream_id: fixture.historicalStreamId,
    harness_generation: fixture.generation - 1,
    hash: fixture.historicalHash,
    payload: fixture.historicalPayload.toString("base64"),
    state: "interrupted_unknown",
  };
  const peerSubmission: JournalRecord = {
    id: fixture.submissionId,
    session_id: sessionId,
    incarnation_id: fixture.incarnationId,
    stream_id: fixture.streamId,
    harness_generation: fixture.generation,
    hash: fixture.hash,
    payload: fixture.payload.toString("base64"),
    state: "interrupted_unknown",
  };

  const restored = withDatabase(tmpDir, (db) => {
    const session = db
      .prepare("SELECT metadata FROM task_sessions WHERE id = ?")
      .get(sessionId) as { metadata: string };
    const metadata = JSON.parse(session.metadata) as Record<string, unknown>;
    const recovery = metadata.agent_delivery_recovery as {
      phase?: string;
      harness_generation?: number;
    };
    const previous = db
      .prepare("SELECT state, outcome, payload_hash FROM agent_delivery_submissions WHERE id = ?")
      .get(fixture.historicalSubmissionId) as {
      state: string;
      outcome: string;
      payload_hash: string;
    };
    return { recovery, previous };
  });
  expect(restored.recovery.phase).toBe("restored");
  expect(restored.recovery.harness_generation).toBe(fixture.generation - 1);
  expect(restored.previous.state).toBe("interrupted_unknown");
  return {
    ...fixture,
    journalPath,
    previousSubmission: historicalPeer,
    currentSubmission: peerSubmission,
    previousOutcome: restored.previous.outcome,
    previousHash: restored.previous.payload_hash,
  };
}

function readRepeatedNativeResumeState(
  tmpDir: string,
  fixture: Awaited<ReturnType<typeof seedSuccessorGenerationInterruption>>,
) {
  return withDatabase(tmpDir, (db) => ({
    latestGeneration: db
      .prepare(
        "SELECT MAX(generation) AS generation FROM harness_session_generations WHERE session_id = ? AND incarnation_id = ?",
      )
      .get(fixture.sessionId, fixture.incarnationId) as { generation: number },
    blocks: db
      .prepare(
        "SELECT id, reason, consumer_reference, state, authorized_action FROM session_recovery_blocks WHERE session_id = ? AND expected_generation = ? AND state = 'open' ORDER BY id",
      )
      .all(fixture.sessionId, fixture.generation),
    old: db
      .prepare(
        "SELECT state, outcome, payload_hash, payload, terminal_sequence, terminal_stream_id FROM agent_delivery_submissions WHERE id = ?",
      )
      .get(fixture.historicalSubmissionId),
    current: db
      .prepare(
        "SELECT state, outcome, payload_hash, payload, terminal_sequence, terminal_stream_id FROM agent_delivery_submissions WHERE id = ?",
      )
      .get(fixture.submissionId),
    session: db
      .prepare("SELECT metadata FROM task_sessions WHERE id = ?")
      .get(fixture.sessionId) as { metadata: string },
  }));
}

export async function verifyRepeatedNativeResume(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  backend: BackendContext,
  shape: IncidentShape,
) {
  test.setTimeout(180_000);
  const release = await backend.useEnv({
    KANDEV_FEATURES_AGENT_SURVIVAL: "true",
    KANDEV_LOG_LEVEL: "debug",
  });
  let taskId: string | undefined;
  try {
    const task = await api.createTaskWithAgent(
      seed.workspaceId,
      "Repeated native resume",
      seed.agentProfileId,
      {
        workflow_id: seed.workflowId,
        workflow_step_id: seed.startStepId,
        repository_ids: [seed.repositoryId],
        executor_profile_id: seed.worktreeExecutorProfileId,
        start_agent: false,
      },
    );
    taskId = task.id;
    await page.goto(`/t/${task.id}`);
    const session = new SessionPage(page);
    await session.waitForLoad();
    const messageAdds = attachMessageAddCapture(page);
    await session.sendMessageViaButton("/e2e:simple-message");
    await session.expectChatResponseVisible("simple mock response", 0, { timeout: 30_000 });
    await session.waitForChatIdle({ timeout: 30_000 });
    const active = (await api.listTaskSessions(task.id)).sessions.find(
      (candidate) => candidate.is_primary,
    );
    if (!active) throw new Error("created task has no primary session");
    const nativeBefore = (await api.getTaskSession(active.id)).session.metadata?.acp;
    const messagesBeforeResume = countSessionMessageAdds(messageAdds.frames, task.id, active.id);
    const bootId = await api.getBackendBootID();
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
    const fixture = await seedSuccessorGenerationInterruption(backend.tmpDir, active.id, shape);
    expect(await api.getBackendBootID()).toBe(bootId);
    const traffic = attachGatewayTrafficCapture(page);
    await page.reload();
    await session.waitForLoad();
    await expect
      .poll(async () => (await api.getTaskSession(active.id)).session.state)
      .toBe("FAILED");
    await expect(page.getByTestId("failed-session-banner")).toBeVisible();
    const resume = page.getByTestId("recovery-resume-button");
    await expect(resume).toBeVisible();
    expect(
      traffic.frames.filter(
        (frame) =>
          frame.direction === "sent" &&
          frame.action === "session.recover" &&
          frame.sessionId === active.id,
      ),
    ).toHaveLength(0);

    if (test.info().project.name === "mobile-chrome") await resume.tap();
    else await resume.click();

    const expectedBlockCount = shape === "independent" ? 3 : 0;
    try {
      await expect
        .poll(() => readRepeatedNativeResumeState(backend.tmpDir, fixture).blocks.length, {
          timeout: 60_000,
        })
        .toBe(expectedBlockCount);
    } catch (error) {
      const state = readRepeatedNativeResumeState(backend.tmpDir, fixture);
      const sessionState = (await api.getTaskSession(active.id)).session;
      const recoveryErrorElement = page.getByTestId("session-recovery-error");
      const recoveryError = (await recoveryErrorElement.isVisible())
        ? await recoveryErrorElement.textContent()
        : null;
      const peer = await readJournalSubmission(fixture.journalPath, fixture.submissionId);
      throw new Error(
        `repeated Resume did not settle its fences: ${JSON.stringify({
          generation: state.latestGeneration,
          blocks: state.blocks,
          session_state: sessionState.state,
          agent_delivery_recovery: sessionState.metadata?.agent_delivery_recovery,
          recovery_error: recoveryError,
          peer,
          error: error instanceof Error ? error.message : String(error),
        })}`,
        { cause: error },
      );
    }
    await expect
      .poll(
        async () =>
          (await readJournalSubmission(fixture.journalPath, fixture.submissionId)).retired,
      )
      .toBe(true);
    const afterResume = readRepeatedNativeResumeState(backend.tmpDir, fixture);
    const [oldPeer, currentPeer] = await Promise.all([
      readJournalSubmission(fixture.journalPath, fixture.historicalSubmissionId),
      readJournalSubmission(fixture.journalPath, fixture.submissionId),
    ]);
    expect(oldPeer).toMatchObject({
      retired: true,
      state: fixture.previousSubmission.state,
      payload: fixture.previousSubmission.payload,
    });
    expect(currentPeer).toMatchObject({
      retired: true,
      state: fixture.currentSubmission.state,
      payload: fixture.currentSubmission.payload,
    });
    expect(afterResume.old).toMatchObject({
      state: "interrupted_unknown",
      outcome: fixture.previousOutcome,
      payload_hash: fixture.previousHash,
      terminal_sequence: 0,
      terminal_stream_id: "",
    });
    expect(afterResume.current).toMatchObject({
      state: "interrupted_unknown",
      outcome: "prompt_dispatch_failed",
      payload_hash: fixture.hash,
      terminal_sequence: 0,
      terminal_stream_id: "",
    });
    expect(Buffer.from(afterResume.old.payload as Uint8Array).toString("base64")).toBe(
      fixture.previousSubmission.payload,
    );
    expect(Buffer.from(afterResume.current.payload as Uint8Array).toString("utf8")).toBe(
      fixture.payload.toString("utf8"),
    );
    const metadata = JSON.parse(afterResume.session.metadata) as Record<string, unknown>;
    const recovery = metadata.agent_delivery_recovery as {
      phase?: string;
      harness_generation?: number;
    };
    expect(recovery.phase).toBe("restored");
    expect(recovery.harness_generation).toBe(fixture.generation - 1);

    if (shape === "independent") {
      await expect(page.getByTestId("failed-session-banner")).toBeVisible();
      await expect(page.getByTestId("session-recovery-error")).toBeVisible();
      await expect(resume).toBeEnabled();
      await page.reload();
      await session.waitForLoad();
      const retry = page.getByTestId("recovery-resume-button");
      await expect(retry).toBeVisible();
      if (test.info().project.name === "mobile-chrome") await retry.tap();
      else await retry.click();
      await expect(retry).toBeDisabled();
      await expect
        .poll(
          () =>
            traffic.frames.filter(
              (frame) =>
                frame.direction === "sent" &&
                frame.action === "session.recover" &&
                frame.sessionId === active.id,
            ).length,
        )
        .toBe(2);
      await expect(retry).toBeEnabled();
      await expect(page.getByTestId("session-recovery-error")).toBeVisible();
      await expect
        .poll(() => readRepeatedNativeResumeState(backend.tmpDir, fixture).blocks.length)
        .toBe(3);
      return;
    }

    await expect(page.getByTestId("failed-session-banner")).toHaveCount(0);
    await page.reload();
    await session.waitForLoad();
    await expect(page.getByTestId("failed-session-banner")).toHaveCount(0);
    expect((await api.getTaskSession(active.id)).session.metadata?.acp).toEqual(nativeBefore);
    expect(countSessionMessageAdds(messageAdds.frames, task.id, active.id)).toBe(
      messagesBeforeResume,
    );
    await expect(session.chat.getByText("Slow response complete", { exact: false })).toHaveCount(0);

    await session.sendMessageViaButton("/slow 1s");
    await expect(
      session.chat.getByText("Slow response complete after 1s.", { exact: false }),
    ).toBeVisible({ timeout: 30_000 });
    await session.waitForChatIdle({ timeout: 30_000 });
    const messages = (await api.listSessionMessages(active.id)).messages;
    expect(
      messages.filter(
        (message) =>
          message.author_type === "user" && message.content.trim() === "/e2e:simple-message",
      ),
    ).toHaveLength(1);
    expect(countSessionMessageAdds(messageAdds.frames, task.id, active.id)).toBe(
      messagesBeforeResume + 1,
    );
    expect(
      traffic.frames.filter(
        (frame) =>
          frame.direction === "sent" &&
          frame.action === "session.recover" &&
          frame.sessionId === active.id,
      ),
    ).toHaveLength(1);
  } finally {
    try {
      if (taskId) await api.deleteTask(taskId);
    } finally {
      await release();
    }
  }
}
