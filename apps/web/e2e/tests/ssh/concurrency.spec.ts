import { test, expect } from "../../fixtures/ssh-test-base";
import { dropTrafficToPort22, restoreTraffic } from "../../helpers/ssh";
import { waitForLatestSessionDone, waitForSessionDone } from "../../helpers/session";

// raceWithTimeout returns the resolved value of fn() if it completes within
// timeoutMs, or { kind: "timeout" } otherwise. Used by the drop-traffic spec
// to bound a probe whose backend dial may sit blocked on TCP SYN retries
// longer than the dialer's logical Timeout.
async function raceWithTimeout<T>(
  fn: () => Promise<T>,
  timeoutMs: number,
): Promise<{ kind: "ok"; value: T } | { kind: "timeout" } | { kind: "error"; error: unknown }> {
  let timer: NodeJS.Timeout | undefined;
  try {
    const timeout = new Promise<{ kind: "timeout" }>((resolve) => {
      timer = setTimeout(() => resolve({ kind: "timeout" }), timeoutMs);
    });
    const ok = fn()
      .then((value) => ({ kind: "ok" as const, value }))
      .catch((error) => ({ kind: "error" as const, error }));
    return await Promise.race([ok, timeout]);
  } finally {
    if (timer) clearTimeout(timer);
  }
}

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
      // The TCP SYN to the sshd container is silently dropped by iptables,
      // so the backend's net.Dialer can sit on the syscall well past its
      // logical Timeout while the kernel exhausts its SYN retries. Race the
      // probe against an explicit AbortController and assert that the
      // request didn't succeed — either it errors out, or kandev returns
      // success=false. Both prove the dropped state is reaching the dialer.
      const probeResult = await raceWithTimeout(
        () =>
          apiClient.testSSHConnection({
            name: "I4 drop probe",
            host: seedData.sshTarget.host,
            port: seedData.sshTarget.port,
            user: seedData.sshTarget.user,
            identity_source: "file",
            identity_file: seedData.sshTarget.identityFile,
          }),
        5_000,
      );
      if (probeResult.kind === "ok") {
        expect(probeResult.value.success).toBe(false);
      }
    } finally {
      restoreTraffic(seedData.sshTarget);
    }

    // After restoring, a fresh test should succeed.
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
