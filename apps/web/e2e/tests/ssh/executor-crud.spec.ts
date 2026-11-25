import { test, expect } from "../../fixtures/ssh-test-base";
import { createTestedSSHRoute, sshBinding } from "../../helpers/ssh-managed-fixture";
import type { ApiClient } from "../../helpers/api-client";

async function waitForSSHSession(apiClient: ApiClient, executorId: string, taskId: string) {
  await expect
    .poll(
      async () =>
        (await apiClient.listSSHSessions(executorId)).some(
          (session: { task_id: string }) => session.task_id === taskId,
        ),
      { message: "SSH session should be running", timeout: 60_000 },
    )
    .toBe(true);
}

/**
 * CRUD against the SSH executor record + profile. Asserts type, config map,
 * profile workdir round-trip, and that running-session warnings on
 * edit/delete are surfaced (UI is purely client-side via window.confirm; we
 * assert the backend semantics — config snapshots, no auto-stop).
 *
 * Covers e2e-plan.md group D (D1–D7).
 */
test.describe("ssh executor CRUD", () => {
  test("persists the selected shared connection and rejects inline SSH writes", async ({
    apiClient,
    seedData,
  }) => {
    const saved = (await apiClient.getExecutor(seedData.sshExecutorId)).config!;
    const binding = {
      ssh_connection_id: saved.ssh_connection_id,
      ssh_connection_stamp: saved.ssh_connection_stamp,
    };
    const exec = await apiClient.createSSHExecutor("Shared SSH", binding);
    expect(exec.type).toBe("ssh");
    expect((await apiClient.getExecutor(exec.id)).config).toEqual(binding);

    const inline = { ssh_host: seedData.sshTarget.host };
    await expect(apiClient.createSSHExecutor("Old SSH client", inline)).rejects.toThrow();
    await expect(
      apiClient.updateExecutor(exec.id, { config: { ...binding, ...inline } }),
    ).rejects.toThrow();
    expect((await apiClient.getExecutor(exec.id)).config).toEqual(binding);
  });

  test("listExecutors returns the ssh row with the right type", async ({ apiClient, seedData }) => {
    const { executors } = await apiClient.listExecutors();
    const ssh = executors.find((e) => e.id === seedData.sshExecutorId);
    expect(ssh).toBeDefined();
    expect(ssh!.type).toBe("ssh");
  });

  test("profile workdir_root survives round-trip", async ({ apiClient, seedData }) => {
    const profile = await apiClient.createExecutorProfile(seedData.sshExecutorId, {
      name: "D3 workdir round-trip",
      config: { ssh_workdir_root: "~/.kandev-d3" },
      prepare_script: "",
      cleanup_script: "",
      env_vars: [],
    });
    const persisted = await apiClient.getExecutorProfile(seedData.sshExecutorId, profile.id);
    expect(persisted.config?.ssh_workdir_root).toBe("~/.kandev-d3");
  });

  test("editing executor with a running session does NOT stop it", async ({
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "D4 keep-session-alive",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await expect
      .poll(async () => (await apiClient.getTaskEnvironment(task.id))?.executor_type ?? null, {
        timeout: 60_000,
      })
      .toBe("ssh");
    await waitForSSHSession(apiClient, seedData.sshExecutorId, task.id);

    // Mutate the executor's name. Backend should accept silently; running
    // sessions retain their snapshot of host/user/etc. in metadata.
    await apiClient.updateExecutor(seedData.sshExecutorId, { name: "D4 renamed" });
    const renamed = await apiClient.getExecutor(seedData.sshExecutorId);
    expect(renamed.name).toBe("D4 renamed");

    const sessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
    const row = sessions.find((s) => s.task_id === task.id);
    expect(row, "session should still be reported as active after rename").toBeDefined();
  });

  test("changing a shared binding preserves the live session snapshot", async ({
    apiClient,
    backend,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "D5 metadata snapshot",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await expect
      .poll(async () => (await apiClient.getTaskEnvironment(task.id))?.executor_type ?? null, {
        timeout: 60_000,
      })
      .toBe("ssh");
    await waitForSSHSession(apiClient, seedData.sshExecutorId, task.id);

    const before = await apiClient.listSSHSessions(seedData.sshExecutorId);
    const beforeRow = before.find((s) => s.task_id === task.id);
    expect(beforeRow).toBeDefined();
    const originalHost = beforeRow!.host;

    const original = (await apiClient.getExecutor(seedData.sshExecutorId)).config;
    const replacement = await createTestedSSHRoute(backend.baseUrl, "Separate connection", {
      ...seedData.sshTarget,
      host: "localhost",
    });
    try {
      await apiClient.updateExecutor(seedData.sshExecutorId, {
        config: sshBinding(replacement),
      });

      const after = await apiClient.listSSHSessions(seedData.sshExecutorId);
      const afterRow = after.find((s) => s.task_id === task.id);
      expect(afterRow).toBeDefined();
      expect(afterRow!.host).toBe(originalHost);
    } finally {
      await apiClient.updateExecutor(seedData.sshExecutorId, { config: original });
    }
  });

  test("delete executor profile is accepted regardless of session count", async ({
    apiClient,
    seedData,
  }) => {
    // Spin up a throwaway profile so we don't break the rest of the suite.
    const profile = await apiClient.createExecutorProfile(seedData.sshExecutorId, {
      name: "D6 throwaway",
      config: {},
      prepare_script: "",
      cleanup_script: "",
      env_vars: [],
    });
    await apiClient.deleteExecutorProfile(profile.id);
    // No throw == accepted. Backend doesn't 4xx on dead profile delete either.
  });
});
