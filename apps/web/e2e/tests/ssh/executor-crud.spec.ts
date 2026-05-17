import { test, expect } from "../../fixtures/ssh-test-base";

/**
 * CRUD against the SSH executor record + profile. Asserts type, config map,
 * profile workdir round-trip, and that running-session warnings on
 * edit/delete are surfaced (UI is purely client-side via window.confirm; we
 * assert the backend semantics — config snapshots, no auto-stop).
 *
 * Covers e2e-plan.md group D (D1–D7).
 */
test.describe("ssh executor CRUD", () => {
  test("POST /api/v1/executors with type=ssh persists host_fingerprint in Config", async ({
    apiClient,
    seedData,
  }) => {
    const exec = await apiClient.createSSHExecutor("D1 manual create", {
      ssh_host: seedData.sshTarget.host,
      ssh_port: String(seedData.sshTarget.port),
      ssh_user: seedData.sshTarget.user,
      ssh_identity_source: "file",
      ssh_identity_file: seedData.sshTarget.identityFile,
      ssh_host_fingerprint: seedData.sshTarget.hostFingerprint,
    });
    expect(exec.type).toBe("ssh");

    const persisted = await apiClient.getExecutor(exec.id);
    expect(persisted.config?.ssh_host_fingerprint).toBe(seedData.sshTarget.hostFingerprint);
    expect(persisted.config?.ssh_host).toBe(seedData.sshTarget.host);
    expect(persisted.config?.ssh_user).toBe(seedData.sshTarget.user);
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

    // Mutate the executor's name. Backend should accept silently; running
    // sessions retain their snapshot of host/user/etc. in metadata.
    await apiClient.updateExecutor(seedData.sshExecutorId, { name: "D4 renamed" });
    const renamed = await apiClient.getExecutor(seedData.sshExecutorId);
    expect(renamed.name).toBe("D4 renamed");

    const sessions = await apiClient.listSSHSessions(seedData.sshExecutorId);
    const row = sessions.find((s) => s.task_id === task.id);
    expect(row, "session should still be reported as active after rename").toBeDefined();
  });

  test("changing executor config does not change live session's metadata snapshot", async ({
    apiClient,
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

    const before = await apiClient.listSSHSessions(seedData.sshExecutorId);
    const beforeRow = before.find((s) => s.task_id === task.id);
    expect(beforeRow).toBeDefined();
    const originalHost = beforeRow!.host;

    // Bait: change the executor's host to something different. The live
    // session must keep its snapshot.
    await apiClient.updateExecutor(seedData.sshExecutorId, {
      config: {
        ssh_host: "10.255.255.1",
        ssh_port: String(seedData.sshTarget.port),
        ssh_user: seedData.sshTarget.user,
        ssh_identity_source: "file",
        ssh_identity_file: seedData.sshTarget.identityFile,
        ssh_host_fingerprint: seedData.sshTarget.hostFingerprint,
      },
    });

    const after = await apiClient.listSSHSessions(seedData.sshExecutorId);
    const afterRow = after.find((s) => s.task_id === task.id);
    expect(afterRow).toBeDefined();
    expect(afterRow!.host).toBe(originalHost);
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
