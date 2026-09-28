import path from "node:path";
import type { Page } from "@playwright/test";
import { DatabaseSync } from "./node-sqlite";
import type { ApiClient } from "./api-client";
import {
  createFixtureExecutorProfile,
  installFixtureExecutorProvider,
} from "./plugin-executor-profile";

export type PluginExecutorStatusState = "expired" | "unknown" | "unavailable" | "cleanup_pending";

export async function seedPluginExecutorStatusTask(
  page: Page,
  options: {
    backend: { tmpDir: string };
    apiClient: ApiClient;
    seedData: {
      workspaceId: string;
      workflowId: string;
      startStepId: string;
      agentProfileId: string;
    };
    state: PluginExecutorStatusState;
    touch: boolean;
  },
) {
  const { backend, apiClient, seedData, state, touch } = options;
  const executor = await installFixtureExecutorProvider(page, apiClient);
  await page.goto("/settings/executors");
  const profile = await createFixtureExecutorProfile(page, executor, touch);
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    `Plugin executor ${state} status`,
    seedData.agentProfileId,
    {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      executor_id: executor.id,
      executor_profile_id: profile.id,
      start_agent: false,
    },
  );
  let sessions = await apiClient.listTaskSessions(task.id);
  if (sessions.sessions.length === 0) {
    await apiClient.seedTaskSession(task.id, {
      state: "COMPLETED",
      agentProfileId: seedData.agentProfileId,
      completedAt: "2026-09-26T10:00:00Z",
    });
    sessions = await apiClient.listTaskSessions(task.id);
  }
  const sessionId = sessions.sessions[0]?.id ?? task.session_id;
  if (!sessionId) throw new Error("The plugin status task requires a session for executor status");
  const sqlite = new DatabaseSync(path.join(backend.tmpDir, "kandev.db"));
  try {
    sqlite.exec("PRAGMA busy_timeout = 10000");
    sqlite
      .prepare(
        "UPDATE task_sessions SET executor_id = ?, state = 'COMPLETED', completed_at = CURRENT_TIMESTAMP WHERE id = ?",
      )
      .run(executor.id, sessionId);
  } finally {
    sqlite.close();
  }

  await page.route(`**/api/v1/tasks/${task.id}/environment/live`, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        environment: {
          id: "plugin-environment-e2e",
          task_id: task.id,
          repository_id: "",
          executor_type: "plugin_remote",
          executor_id: executor.id,
          executor_profile_id: profile.id,
          agent_execution_id: sessionId,
          control_port: 0,
          status: "ready",
          created_at: "2026-09-26T10:00:00Z",
        },
        plugin_executor: {
          state,
          retention: state === "unknown" ? "unknown" : "bounded",
          expires_at: "2026-09-26T18:00:00Z",
          deadline_passed: state === "expired" || state === "unknown",
          reason: state === "unavailable" ? "provider_unavailable" : undefined,
        },
      }),
    });
  });
  return { taskId: task.id, executor, profileId: profile.id, sessionId };
}
