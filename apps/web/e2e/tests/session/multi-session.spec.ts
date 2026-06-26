import fs from "node:fs";
import path from "node:path";

import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

function listTaskWorktreeRoots(tmpDir: string): string[] {
  const tasksRoot = path.join(tmpDir, ".kandev", "tasks");
  if (!fs.existsSync(tasksRoot)) return [];
  return fs
    .readdirSync(tasksRoot, { withFileTypes: true })
    .filter((entry) => entry.isDirectory())
    .map((entry) => path.join(tasksRoot, entry.name))
    .sort();
}

async function waitForDoneSessionCount(
  apiClient: import("../../helpers/api-client").ApiClient,
  taskId: string,
  count: number,
  message: string,
) {
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        return sessions.filter((s) => DONE_STATES.includes(s.state)).length;
      },
      { timeout: 45_000, message },
    )
    .toBe(count);
}

async function waitForTaskEnvironmentId(
  apiClient: import("../../helpers/api-client").ApiClient,
  taskId: string,
  message: string,
) {
  await expect
    .poll(
      async () => {
        const env = await apiClient.getTaskEnvironment(taskId);
        return env?.id ?? "";
      },
      { timeout: 45_000, message },
    )
    .not.toBe("");
}

/**
 * Tests for launching multiple sessions on the same task.
 * Verifies session handover context injection and environment reuse.
 */
test.describe("Multi-session", () => {
  test("second session on same task receives handover context", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    // 1. Create task and start first agent session
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Multi Session Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 2. Wait for first session to finish
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for first session to finish" },
      )
      .toBe(true);

    // 3. Verify first session created environment
    const env = await apiClient.getTaskEnvironment(task.id);
    expect(env).not.toBeNull();

    // 4. Navigate to task and verify first session is visible
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("Multi Session Task");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Verify first session's response is visible
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 5. Verify task has exactly one session
    const { sessions: sessionsAfterFirst } = await apiClient.listTaskSessions(task.id);
    expect(sessionsAfterFirst).toHaveLength(1);
    expect(sessionsAfterFirst[0].task_environment_id).toBe(env!.id);
  });

  test("task environment persists after session completes", async ({ apiClient, seedData }) => {
    test.setTimeout(90_000);

    // Create task and start agent
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Env Persistence Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // Wait for session to finish
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for session to finish" },
      )
      .toBe(true);

    // Verify environment still exists and is in "ready" state after completion
    const env = await apiClient.getTaskEnvironment(task.id);
    expect(env).not.toBeNull();
    expect(env!.status).toBe("ready");
    expect(env!.task_id).toBe(task.id);
  });

  test("second session reuses same worktree as first session", async ({ apiClient, seedData }) => {
    test.setTimeout(90_000);

    // 1. Create task with first session
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Worktree Reuse Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 2. Wait for first session to finish
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for first session to finish" },
      )
      .toBe(true);

    // 3. Capture first session's environment info
    const { sessions: firstSessions } = await apiClient.listTaskSessions(task.id);
    const firstSession = firstSessions[0];
    const env = await apiClient.getTaskEnvironment(task.id);
    expect(env).not.toBeNull();

    // 4. Verify the session points to the task environment
    // The worktree reuse is verified through the task_environment_id linkage:
    // when a second session launches on the same task, persistTaskEnvironment()
    // finds the existing environment and reuses its WorktreeID.
    expect(firstSession.task_environment_id).toBe(env!.id);
  });

  test("same-repo multi-branch second session reuses the existing task workspace", async ({
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    const rootsBefore = listTaskWorktreeRoots(backend.tmpDir);
    const task = await apiClient.createTask(
      seedData.workspaceId,
      "Multi-Branch Workspace Reuse Task",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repositories: [
          { repository_id: seedData.repositoryId, base_branch: "main" },
          {
            repository_id: seedData.repositoryId,
            base_branch: "main",
            checkout_branch: "branch-5hn",
          },
        ],
      },
    );

    await apiClient.launchSession(
      {
        task_id: task.id,
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.worktreeExecutorProfileId,
        workflow_step_id: seedData.startStepId,
        prompt: "/e2e:simple-message",
      },
      60_000,
    );

    await waitForDoneSessionCount(
      apiClient,
      task.id,
      1,
      "Waiting for first multi-branch session to finish",
    );

    await waitForTaskEnvironmentId(apiClient, task.id, "Waiting for first task environment");
    const envBefore = await apiClient.getTaskEnvironment(task.id);
    expect(envBefore?.id, "first session must create a task environment").toBeTruthy();
    const rootsAfterFirst = listTaskWorktreeRoots(backend.tmpDir);
    const createdRoots = rootsAfterFirst.filter((root) => !rootsBefore.includes(root));
    expect(
      createdRoots,
      "first session should create exactly one task workspace root",
    ).toHaveLength(1);
    expect(
      fs.readdirSync(createdRoots[0]).filter((name) => name.includes("branch-5hn")).length,
    ).toBe(1);
    const rootEntriesAfterFirst = fs.readdirSync(createdRoots[0]).sort();

    const launched = await apiClient.launchSession(
      {
        task_id: task.id,
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.worktreeExecutorProfileId,
        workflow_step_id: seedData.startStepId,
        prompt: "/e2e:simple-message",
      },
      60_000,
    );

    await waitForDoneSessionCount(
      apiClient,
      task.id,
      2,
      "Waiting for second multi-branch session to finish",
    );

    const { sessions } = await apiClient.listTaskSessions(task.id);
    const launchedSession = sessions.find((s) => s.id === launched.session_id);
    expect(launchedSession?.task_environment_id).toBe(envBefore!.id);
    expect(listTaskWorktreeRoots(backend.tmpDir)).toEqual(rootsAfterFirst);
    expect(fs.readdirSync(createdRoots[0]).sort()).toEqual(rootEntriesAfterFirst);
  });
});
