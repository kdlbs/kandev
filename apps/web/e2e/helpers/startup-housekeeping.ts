import { randomUUID } from "node:crypto";
import { expect, type Page } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { withDatabase } from "./interrupted-prompt-recovery";
import {
  captureSilentRestartSnapshot,
  expectRestartConversationIdentity,
} from "./silent-restart-recovery";
import { attachGatewayTrafficCapture } from "./ws-traffic";
import { waitForAgentMessage } from "./session";
import { SessionPage } from "../pages/session-page";

export async function verifyStartupHousekeeping(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  backend: BackendContext,
) {
  const release = await backend.useEnv({ KANDEV_FEATURES_AGENT_SURVIVAL: "true" });
  const prefix = `housekeeping-${randomUUID()}`;
  const taskIds: string[] = [];
  try {
    const snapshots = [];
    for (let index = 0; index < 2; index++) {
      const task = await api.createTaskWithAgent(
        seed.workspaceId,
        `Startup conversation ${index + 1}`,
        seed.agentProfileId,
        {
          description: "/slow 120",
          workflow_id: seed.workflowId,
          workflow_step_id: seed.startStepId,
          repository_ids: [seed.repositoryId],
          executor_profile_id: seed.worktreeExecutorProfileId,
        },
      );
      taskIds.push(task.id);
      const session = (await api.listTaskSessions(task.id)).sessions.find((row) => row.is_primary)!;
      await waitForAgentMessage(api, session.id, "Running slow response");
      await page.goto(`/t/${task.id}`);
      const chat = new SessionPage(page);
      await chat.waitForLoad();
      await expect(chat.chat.getByText("Running slow response", { exact: false })).toBeVisible();
      snapshots.push(await captureSilentRestartSnapshot(api, task.id, session.id));
    }
    withDatabase(backend.tmpDir, (db) => {
      db.prepare("PRAGMA busy_timeout=10000").run();
      for (const kind of ["history", "remote"]) {
        db.prepare(
          `INSERT INTO tasks(id,workspace_id,workflow_id,title,state,archived_at,created_at,updated_at)
      SELECT ?,workspace_id,workflow_id,?, 'IN_PROGRESS',?,created_at,updated_at FROM tasks WHERE id=?`,
        ).run(
          `${prefix}-${kind}`,
          `Startup ${kind}`,
          kind === "history" ? new Date().toISOString() : null,
          taskIds[0],
        );
      }
      for (let index = 0; index < 101; index++) {
        const remote = index === 100,
          id = `${prefix}-${index}`,
          taskId = `${prefix}-${remote ? "remote" : "history"}`;
        db.prepare(
          `INSERT INTO task_sessions(id,task_id,agent_profile_id,queue_incarnation_id,state,started_at,updated_at)
      SELECT ?,?,?,?, ?,started_at,updated_at FROM task_sessions WHERE id=?`,
        ).run(
          id,
          taskId,
          seed.agentProfileId,
          id,
          remote ? "RUNNING" : "COMPLETED",
          snapshots[0].sessionId,
        );
        db.prepare(
          `INSERT INTO executors_running(id,session_id,task_id,executor_id,runtime,status,resumable,resume_token,agent_execution_id,created_at,updated_at)
      VALUES(?,?,?,?,?,?,1,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
        ).run(
          id,
          id,
          taskId,
          remote ? "unavailable-remote-executor" : "",
          remote ? "ssh" : "standalone",
          remote ? "running" : "stopped",
          `retained-${index}`,
          remote ? `${prefix}-owner` : "",
        );
      }
    });
    const traffic = attachGatewayTrafficCapture(page);
    await backend.restart();
    const ready = await page.request.get(`${backend.baseUrl}/ready`);
    expect(ready.ok()).toBe(true);
    const body = await ready.json();
    expect(body.startup.phase).toBe("ready");
    for (const id of [
      "queue_reconciliation",
      "turn_reconciliation",
      "route_reconciliation",
      "runtime_inventory",
      "task_reconciliation",
    ]) {
      expect(body.startup.completed_steps).toContainEqual(
        expect.objectContaining({ id: `startup.${id}`, completed: true }),
      );
    }
    const elapsed = body.startup.elapsed_ms;
    expect(
      (await (await page.request.get(`${backend.baseUrl}/ready`)).json()).startup.elapsed_ms,
    ).toBe(elapsed);
    expect(
      withDatabase(backend.tmpDir, (db) =>
        db
          .prepare("SELECT status,agent_execution_id FROM executors_running WHERE session_id=?")
          .get(`${prefix}-100`),
      ),
    ).toEqual({ status: "running", agent_execution_id: `${prefix}-owner` });
    await page.reload();
    await new SessionPage(page).waitForLoad();
    for (let index = 0; index < 2; index++) {
      await expectRestartConversationIdentity({
        page,
        api,
        taskId: taskIds[index],
        snapshot: snapshots[index],
        traffic: traffic.frames,
        expectedState: "RUNNING",
        executionDisposition: "preserved",
      });
    }
  } finally {
    withDatabase(backend.tmpDir, (db) => {
      db.prepare("DELETE FROM executors_running WHERE id LIKE ?").run(`${prefix}%`);
      db.prepare("DELETE FROM task_sessions WHERE id LIKE ?").run(`${prefix}%`);
      db.prepare("DELETE FROM tasks WHERE id LIKE ?").run(`${prefix}%`);
    });
    await release();
  }
}
