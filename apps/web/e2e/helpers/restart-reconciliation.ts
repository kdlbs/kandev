import { writeFileSync } from "node:fs";
import path from "node:path";
import { withDatabase } from "./interrupted-prompt-recovery";
import { expect, test, type Page } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { SessionPage } from "../pages/session-page";
import { attachGatewayTrafficCapture } from "./ws-traffic";
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
) {
  const release = await backend.useEnv({
    KANDEV_FEATURES_AGENT_SURVIVAL: "true",
    KANDEV_LOG_LEVEL: "debug",
  });
  try {
    const task = await api.createTaskWithAgent(
      seed.workspaceId,
      "Automatic crash recovery",
      seed.agentProfileId,
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
    expect((await api.killLocalAgentRuntimeChild()).killed).toBe(true);
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
    await release();
  }
}
