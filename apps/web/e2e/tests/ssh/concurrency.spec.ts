import { test, expect } from "../../fixtures/ssh-test-base";
import { dropTrafficToPort22, restoreTraffic } from "../../helpers/ssh";
import { waitForLatestSessionDone, waitForSessionDone } from "../../helpers/session";

/**
 * Two sessions on the same task share the task workdir but get independent
 * agentctl ports + local forwards. Two tasks on the same host get separate
 * task dirs. Dropping all sshd traffic mid-session evicts the pooled
 * connection (keepalive timeout) and the next op reconnects.
 *
 * Covers e2e-plan.md group I (I1–I4).
 */
test.describe("ssh executor — concurrency", () => {
  test("two sessions on the same task share the task workdir with distinct ports", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(240_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "I1 shared workdir",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "First session");

    // Spawn a second session on the same task.
    const launched = await apiClient.launchSession({
      task_id: task.id,
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.sshExecutorProfileId,
      workflow_step_id: seedData.startStepId,
      prompt: "/e2e:simple-message",
    });
    await waitForSessionDone(apiClient, task.id, launched.session_id, "Second session");

    const sessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
    const rows = sessions.filter((s) => s.task_id === task.id);
    expect(rows.length).toBeGreaterThanOrEqual(2);

    const taskDirs = new Set(rows.map((r) => r.remote_task_dir));
    const remotePorts = new Set(rows.map((r) => r.remote_agentctl_port));
    const localPorts = new Set(rows.map((r) => r.local_forward_port));
    expect(taskDirs.size).toBe(1);
    expect(remotePorts.size).toBe(rows.length);
    expect(localPorts.size).toBe(rows.length);
  });

  test("two different tasks on the same host get distinct task dirs", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(240_000);
    const taskA = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "I2 task A",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    const taskB = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "I2 task B",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, taskA.id, 1, "Task A");
    await waitForLatestSessionDone(apiClient, taskB.id, 1, "Task B");

    const sessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
    const dirA = sessions.find((s) => s.task_id === taskA.id)?.remote_task_dir;
    const dirB = sessions.find((s) => s.task_id === taskB.id)?.remote_task_dir;
    expect(dirA).toBeTruthy();
    expect(dirB).toBeTruthy();
    expect(dirA).not.toBe(dirB);
  });

  test("dropping all sshd traffic mid-session triggers reconnect on next op", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(240_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "I4 conn drop",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Launch before drop");

    dropTrafficToPort22(seedData.sshTarget);
    try {
      // A fresh test against the same host should now fail (eviction). It
      // takes ~30s for the keepalive to time out and evict.
      const result = await apiClient.testSSHConnection({
        name: "I4 drop probe",
        host: seedData.sshTarget.host,
        port: seedData.sshTarget.port,
        user: seedData.sshTarget.user,
        identity_source: "file",
        identity_file: seedData.sshTarget.identityFile,
      });
      expect(result.success).toBe(false);
    } finally {
      restoreTraffic(seedData.sshTarget);
    }

    // After restoring, a fresh test should succeed (pool re-dials).
    await expect
      .poll(
        async () => {
          const r = await apiClient.testSSHConnection({
            name: "I4 restore probe",
            host: seedData.sshTarget.host,
            port: seedData.sshTarget.port,
            user: seedData.sshTarget.user,
            identity_source: "file",
            identity_file: seedData.sshTarget.identityFile,
          });
          return r.success;
        },
        { timeout: 60_000 },
      )
      .toBe(true);
  });
});
