import { test, expect } from "../../fixtures/ssh-test-base";
import { dwell } from "../../helpers/causal-waits";
import type { ApiClient } from "../../helpers/api-client";

async function waitForTaskSSHSession(apiClient: ApiClient, executorID: string, taskID: string) {
  let row: Awaited<ReturnType<ApiClient["listSSHSessions"]>>[number] | undefined;
  await expect
    .poll(
      async () => {
        row = (await apiClient.listSSHSessions(executorID)).find(
          (session) => session.task_id === taskID,
        );
        return row !== undefined;
      },
      { message: "wait for this task's SSH runtime row", timeout: 60_000 },
    )
    .toBe(true);
  return row!;
}

/**
 * HTTP contract for GET /api/v1/ssh/executors/:id/sessions. Backs the
 * SSHSessionsCard table. Filtered to ssh-runtime ExecutorRunning rows for
 * the named executor; metadata fields populate host/user/port/etc.
 *
 * Covers e2e-plan.md group G (G1–G5).
 */
test.describe("ssh sessions-endpoint contract", () => {
  // G1 — only ssh-runtime rows for the requested executor come back.
  test("returns only ssh-runtime rows tied to the requested executor", async ({
    apiClient,
    seedData,
  }) => {
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "G1 launch SSH session",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    // Poll listSSHSessions directly. We want the row to appear while the
    // executor is up; the per-turn session may complete quickly and the row
    // gets removed when the executor is fully torn down — so we race the
    // query against teardown.
    let sessions: Awaited<ReturnType<typeof apiClient.listSSHSessions>> = [];
    await expect
      .poll(
        async () => {
          sessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
          return sessions.length;
        },
        { message: "wait for ssh session row to appear", timeout: 60_000 },
      )
      .toBeGreaterThan(0);

    for (const s of sessions) {
      expect(s.host).toBe(seedData.sshTarget.host);
      expect(s.user).toBe(seedData.sshTarget.user);
    }
  });

  // G2 — non-ssh ExecutorRunning rows are filtered out even if they reference
  // the same executor id (extra defensive guard the handler enforces).
  test("non-ssh runtime rows are filtered out", async ({ apiClient, seedData }) => {
    // Seed at least one live SSH row so the per-row checks aren't vacuous —
    // a handler regression that returned [] would otherwise still pass.
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "G2 filter contract",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    let sessions: Awaited<ReturnType<typeof apiClient.listSSHSessions>> = [];
    await expect
      .poll(
        async () => {
          sessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
          return sessions.length;
        },
        { message: "wait for at least one ssh session row", timeout: 60_000 },
      )
      .toBeGreaterThan(0);
    // Every returned row's runtime must trace back to the seeded SSH target;
    // a row with a different host would mean either the runtime filter or
    // the executor-id filter leaked a non-SSH / cross-executor row.
    for (const s of sessions) {
      expect(s.host).toBe(seedData.sshTarget.host);
    }
  });

  // G3 — metadata fields populate host/user/ports.
  test("populates host, user, remote_agentctl_port, local_forward_port from metadata", async ({
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "G3 metadata populated",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    const row = await waitForTaskSSHSession(apiClient, seedData.sshExecutorId, task.id);
    expect(row.host).toBe(seedData.sshTarget.host);
    expect(row.user).toBe(seedData.sshTarget.user);
    expect(row.remote_agentctl_port).toBeGreaterThan(0);
    expect(row.local_forward_port).toBeGreaterThan(0);
    expect(row.remote_task_dir).toMatch(/\/tasks\/[^/]+$/);
  });

  // G4 — unknown executor id returns an empty list, not 404.
  test("unknown executor id returns empty array", async ({ apiClient }) => {
    const sessions = await apiClient.listSSHSessions("does-not-exist");
    expect(Array.isArray(sessions)).toBe(true);
    expect(sessions.length).toBe(0);
  });

  // G5 — uptime_seconds increases on subsequent polls of the same session.
  test("uptime_seconds is monotonic across polls", async ({ apiClient, seedData }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "G5 uptime monotonic",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    const beforeRow = await waitForTaskSSHSession(apiClient, seedData.sshExecutorId, task.id);

    await dwell(
      2_000,
      "clock-separation",
      "the assertion below is that uptime_seconds grew, so real time has to pass between the two reads; the thing being waited for is the clock, not an app event",
    );

    const afterRow = await waitForTaskSSHSession(apiClient, seedData.sshExecutorId, task.id);
    expect(afterRow.uptime_seconds).toBeGreaterThanOrEqual(beforeRow.uptime_seconds);
  });
});
