import { writeFileSync } from "node:fs";
import path from "node:path";
import { withDatabase } from "./interrupted-prompt-recovery";
import { expect, test, type Page } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { createDelayedResumeProfile } from "./session-resume-prompt-queue";
import { SessionPage } from "../pages/session-page";
import { attachGatewayTrafficCapture } from "./ws-traffic";
import {
  observeAgentRuntimeAvailability,
  waitForAgentRuntimeReplacement,
} from "./agent-runtime-availability";
import {
  captureSilentRestartSnapshot,
  expectRestartConversationIdentity,
  expectSilentRestartRestoration,
} from "./silent-restart-recovery";

// @covers AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.2
// @covers AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.3
// @covers AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.5
export async function verifyAutomaticRestartReconciliation(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  backend: BackendContext,
  resumeDelay?: string,
) {
  const release = await backend.useEnv({
    KANDEV_FEATURES_AGENT_SURVIVAL: "true",
    KANDEV_LOG_LEVEL: "debug",
  });
  let delayedProfile: string | undefined;
  let createdTask: string | undefined;
  let siblingTask: string | undefined;
  try {
    if (resumeDelay) delayedProfile = await createDelayedResumeProfile(api, resumeDelay);
    const task = await api.createTaskWithAgent(
      seed.workspaceId,
      "Automatic crash recovery",
      delayedProfile ?? seed.agentProfileId,
      {
        workflow_id: seed.workflowId,
        workflow_step_id: seed.startStepId,
        repository_ids: [seed.repositoryId],
        executor_profile_id: seed.worktreeExecutorProfileId,
        start_agent: false,
      },
    );
    createdTask = task.id;
    await page.goto(`/t/${task.id}`);
    const session = new SessionPage(page);
    await session.waitForLoad();
    await session.sendMessageViaButton("/slow 120");
    await expect(session.chat.getByText("Running slow response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });
    const listed = await api.listTaskSessions(task.id);
    const active = listed.sessions.find((candidate) => candidate.is_primary);
    if (!active) throw new Error("created task has no primary session");
    const snapshot = await captureSilentRestartSnapshot(api, task.id, active.id);
    const traffic = attachGatewayTrafficCapture(page);
    const bootId = await api.getBackendBootID();
    await observeAgentRuntimeAvailability(page);
    const killedRuntime = await api.killLocalAgentRuntimeChild();
    expect(killedRuntime.killed).toBe(true);
    await waitForAgentRuntimeReplacement(page, killedRuntime.runtime_epoch, bootId);
    const sibling = resumeDelay ? await createRecoverySibling(api, seed) : undefined;
    siblingTask = sibling?.taskId;
    try {
      await expectSilentRestartRestoration(page, api, task.id, snapshot, traffic.frames);
    } catch (error) {
      const evidencePath = path.join(backend.tmpDir, "recovery-evidence.json");
      const checkpoint = withDatabase(backend.tmpDir, (db) =>
        db
          .prepare(
            "SELECT checkpoint_json FROM session_restore_attempts WHERE session_id = ? ORDER BY created_at DESC LIMIT 1",
          )
          .get(active.id),
      );
      const owner = withDatabase(backend.tmpDir, (db) =>
        db
          .prepare(
            "SELECT agent_execution_id, json_extract(metadata, '$.delivery_prompt_turn_binding') AS prompt_binding FROM executors_running WHERE session_id = ?",
          )
          .get(active.id),
      );
      writeFileSync(
        evidencePath,
        JSON.stringify(
          {
            checkpoint,
            owner,
            blocks: withDatabase(backend.tmpDir, (db) =>
              db
                .prepare("SELECT * FROM session_recovery_blocks WHERE session_id = ?")
                .all(active.id),
            ),
            sessions: await api.listTaskSessions(task.id),
          },
          null,
          2,
        ),
      );
      await test
        .info()
        .attach("recovery-evidence", { path: evidencePath, contentType: "application/json" });
      await test
        .info()
        .attach("recovery-backend-log", { path: backend.logPath, contentType: "text/plain" });
      await test.info().attach("recovery-file-log", {
        path: path.join(backend.tmpDir, ".kandev/logs/backend-logs.log"),
        contentType: "text/plain",
      });
      await test.info().attach("recovery-sessions", {
        body: JSON.stringify(await api.listTaskSessions(task.id)),
        contentType: "application/json",
      });
      throw error;
    }
    expect(await api.getBackendBootID()).toBe(bootId);
    if (sibling) await verifyRecoverySibling(page, api, sibling);
    await page.goto(`/t/${task.id}`);
    await session.waitForLoad();
    const restored = await captureSilentRestartSnapshot(api, task.id, active.id);
    await backend.restart();
    await page.reload();
    await session.waitForLoad();
    await expectRestartConversationIdentity({
      page,
      api,
      taskId: task.id,
      snapshot: restored,
      traffic: traffic.frames,
      expectedState: "WAITING_FOR_INPUT",
      executionDisposition: "preserved",
    });
    await expect(page.getByTestId("failed-session-banner")).toHaveCount(0);
    await session.sendMessageViaButton("/e2e:simple-message");
    await session.expectChatResponseVisible("simple mock response", 0, { timeout: 30_000 });
    await expect
      .poll(
        async () =>
          (await api.listTaskSessions(task.id)).sessions.find((item) => item.id === active.id)
            ?.state,
      )
      .toBe("WAITING_FOR_INPUT");
    const messages = (await api.listSessionMessages(active.id)).messages;
    expect(
      messages.filter(
        (message) => message.author_type === "user" && message.content.trim() === "/slow 120",
      ),
    ).toHaveLength(1);
    expect(
      messages.filter(
        (message) =>
          message.author_type === "user" && message.content.trim() === "/e2e:simple-message",
      ),
    ).toHaveLength(1);
    await expect(session.chat.getByText("Slow response complete", { exact: false })).toHaveCount(0);
  } finally {
    try {
      if (siblingTask) await api.deleteTask(siblingTask);
      if (delayedProfile) {
        if (createdTask) await api.deleteTask(createdTask);
        await api.deleteAgentProfile(delayedProfile, true);
      }
    } finally {
      await release();
    }
  }
}

async function createRecoverySibling(api: ApiClient, seed: SeedData) {
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    "Healthy recovery sibling",
    seed.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
      executor_profile_id: seed.worktreeExecutorProfileId,
    },
  );
  if (!task.session_id) throw new Error("healthy sibling has no session");
  await expect
    .poll(async () => (await api.getTaskSession(task.session_id!)).session.state, {
      timeout: 30_000,
    })
    .toBe("WAITING_FOR_INPUT");
  return {
    taskId: task.id,
    snapshot: await captureSilentRestartSnapshot(api, task.id, task.session_id),
  };
}

async function verifyRecoverySibling(
  page: Page,
  api: ApiClient,
  sibling: Awaited<ReturnType<typeof createRecoverySibling>>,
) {
  await page.goto(`/t/${sibling.taskId}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.expectChatResponseVisible("simple mock response", 0);
  await session.sendMessageViaButton("/e2e:simple-message");
  await session.expectChatResponseVisible("simple mock response", 1, { timeout: 30_000 });
  await session.waitForChatIdle({ timeout: 30_000 });
  const after = await captureSilentRestartSnapshot(api, sibling.taskId, sibling.snapshot.sessionId);
  expect(after.agentExecutionId).toBe(sibling.snapshot.agentExecutionId);
  expect(after.nativeSessionId).toBe(sibling.snapshot.nativeSessionId);
}
