import { test, expect } from "../../fixtures/ssh-test-base";
import { killRemotePid, readRemoteFile } from "../../helpers/ssh";
import { waitForLatestSessionDone } from "../../helpers/session";

/**
 * Backend-restart recovery. Persisted ExecutorRunning metadata
 * (ssh_host, ssh_remote_agentctl_pid, ssh_remote_agentctl_port, etc.) drives
 * ResumeRemoteInstance — re-dial the SSH connection, re-forward the recorded
 * remote port, verify `kill -0 pid`, and reattach the stream manager.
 *
 * Covers e2e-plan.md group J (J1–J4).
 */
test.describe("ssh executor — recovery after restart", () => {
  test("live session survives a backend restart", async ({ apiClient, seedData, backend }) => {
    test.setTimeout(240_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "J1 backend restart",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Pre-restart launch");

    const beforeSessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
    const beforeRow = beforeSessions.find((s) => s.task_id === task.id);
    expect(beforeRow).toBeDefined();
    const beforeRemotePort = beforeRow!.remote_agentctl_port;

    await backend.restart();

    await expect
      .poll(
        async () => {
          const sessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
          const row = sessions.find((s) => s.task_id === task.id);
          // Resume should land back on the same remote port (the agentctl
          // process is unchanged) but a *new* local forward port.
          return row && row.remote_agentctl_port === beforeRemotePort
            ? row.local_forward_port
            : null;
        },
        { timeout: 60_000 },
      )
      .toBeGreaterThan(0);
  });

  test("backend restart with a dead remote agentctl marks the execution stopped", async ({
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(240_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "J2 dead agentctl",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Launch before kill");

    const sessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
    const row = sessions.find((s) => s.task_id === task.id);
    expect(row).toBeDefined();
    const sessionDir = `${row!.remote_task_dir}/.kandev/sessions/${row!.session_id}`;
    const pidStr = readRemoteFile(seedData.sshTarget, `${sessionDir}/agentctl.pid`).trim();
    const pid = parseInt(pidStr, 10);
    expect(pid).toBeGreaterThan(0);

    killRemotePid(seedData.sshTarget, pid);

    await backend.restart();

    await expect
      .poll(
        async () => {
          const after = await apiClient.listSSHSessions(seedData.sshExecutorId);
          const r = after.find((s) => s.task_id === task.id);
          return r?.status ?? "absent";
        },
        { timeout: 60_000 },
      )
      .not.toBe("running");
  });

  test("persisted metadata keys are present in ExecutorRunning after launch", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "J4 persisted metadata",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Launch for metadata");

    const sessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
    const row = sessions.find((s) => s.task_id === task.id);
    expect(row?.host).toBe(seedData.sshTarget.host);
    expect(row?.user).toBe(seedData.sshTarget.user);
    expect(row?.remote_agentctl_port).toBeGreaterThan(0);
    expect(row?.local_forward_port).toBeGreaterThan(0);
    expect(row?.remote_task_dir).toMatch(/\/tasks\//);
  });
});
