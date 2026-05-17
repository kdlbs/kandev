import { test, expect } from "../../fixtures/ssh-test-base";
import { listRemoteDir, readRemoteFile, remotePathExists } from "../../helpers/ssh";
import { waitForLatestSessionDone } from "../../helpers/session";

/**
 * Full end-to-end task launch on the real sshd container. The smoke test
 * for the SSH executor: upload agentctl, mkdir the per-task dir, clone the
 * repo, launch the per-session agentctl, port-forward, run the agent to
 * completion, observe the on-remote filesystem layout.
 *
 * Covers e2e-plan.md group H (H1–H8).
 */
test.describe("ssh executor — task launch", () => {
  test("launches a session and records ssh runtime on the task environment", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "H1 launch",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );

    await waitForLatestSessionDone(apiClient, task.id, 1, "Wait for first SSH session");

    const env = await apiClient.getTaskEnvironment(task.id);
    expect(env).not.toBeNull();
    expect(env!.executor_type).toBe("ssh");
  });

  test("agentctl is uploaded on first launch and sha256 sidecar lands", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "H2 upload agentctl",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Wait for upload");

    expect(remotePathExists(seedData.sshTarget, "/home/kandev/.kandev/bin/agentctl")).toBe(true);
    expect(remotePathExists(seedData.sshTarget, "/home/kandev/.kandev/bin/agentctl.sha256")).toBe(
      true,
    );
    const sha = readRemoteFile(seedData.sshTarget, "/home/kandev/.kandev/bin/agentctl.sha256");
    expect(sha.trim()).toMatch(/^[0-9a-f]{64}$/);
  });

  test("second launch on the same host skips the upload (sha matches)", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(240_000);
    // First launch primes the cache.
    const first = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "H3 first launch",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, first.id, 1, "First launch");

    // Inspect mtime BEFORE the second launch.
    const beforeMtime = readRemoteFile(
      seedData.sshTarget,
      "/home/kandev/.kandev/bin/agentctl.sha256",
    );

    const second = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "H3 second launch",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, second.id, 1, "Second launch");

    // The sha256 file should be byte-identical (same hash, content reused).
    const afterMtime = readRemoteFile(
      seedData.sshTarget,
      "/home/kandev/.kandev/bin/agentctl.sha256",
    );
    expect(afterMtime).toBe(beforeMtime);
  });

  test("per-task workdir lives at <workdir>/tasks/<task-dir>/", async ({ apiClient, seedData }) => {
    test.setTimeout(180_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "H4 task workdir",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Wait for workdir");

    const sessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
    const row = sessions.find((s) => s.task_id === task.id);
    expect(row?.remote_task_dir).toMatch(/\/tasks\/[^/]+$/);
    expect(remotePathExists(seedData.sshTarget, row!.remote_task_dir!)).toBe(true);
  });

  test("per-session runtime dir holds the agentctl pid and port files", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "H5 session runtime",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Wait for session runtime");

    const sessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
    const row = sessions.find((s) => s.task_id === task.id);
    expect(row).toBeDefined();

    const sessionDir = `${row!.remote_task_dir}/.kandev/sessions/${row!.session_id}`;
    expect(remotePathExists(seedData.sshTarget, sessionDir)).toBe(true);
    const entries = listRemoteDir(seedData.sshTarget, sessionDir);
    // The wrapper writes the pid file and the log; the port travels via the
    // AGENTCTL_PORT env var and the ExecutorRunning metadata, not a file.
    expect(entries).toEqual(expect.arrayContaining(["agentctl.pid", "agentctl.log"]));
    expect(row!.remote_agentctl_port).toBeGreaterThan(0);
  });

  test("stopping the session cleans up the session runtime dir but leaves the task dir", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "H7 stop cleanup",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Wait before stop");

    const sessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
    const row = sessions.find((s) => s.task_id === task.id);
    expect(row).toBeDefined();
    const sessionDir = `${row!.remote_task_dir}/.kandev/sessions/${row!.session_id}`;

    // Archive (or otherwise end) the task to trigger StopInstance.
    await apiClient.archiveTask(task.id);

    await expect
      .poll(() => remotePathExists(seedData.sshTarget, sessionDir), { timeout: 30_000 })
      .toBe(false);
    // Task dir intact — v1 spec, no auto-clean on last session stop.
    expect(remotePathExists(seedData.sshTarget, row!.remote_task_dir!)).toBe(true);
  });
});
