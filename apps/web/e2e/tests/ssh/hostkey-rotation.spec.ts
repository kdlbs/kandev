import { test, expect } from "../../fixtures/ssh-test-base";
import { regenerateHostKey } from "../../helpers/ssh";

/**
 * Host-key rotation: regenerate the container's host key and assert
 * (1) the test endpoint surfaces the new fingerprint (no silent re-pin),
 * (2) a subsequent task launch against the still-pinned executor fails with
 * "host key changed", and (3) re-running Test Connection + re-trusting
 * restores function.
 *
 * Covers e2e-plan.md group K (K1–K4).
 */
test.describe("ssh executor — host key rotation", () => {
  test("regenerating the host key surfaces a new fingerprint on the next test", async ({
    apiClient,
    seedData,
  }) => {
    const before = seedData.sshTarget.hostFingerprint;
    const newFp = regenerateHostKey(seedData.sshTarget);
    expect(newFp).not.toBe(before);

    const result = await apiClient.testSSHConnection({
      name: "K1 after rekey",
      host: seedData.sshTarget.host,
      port: seedData.sshTarget.port,
      user: seedData.sshTarget.user,
      identity_source: "file",
      identity_file: seedData.sshTarget.identityFile,
    });
    expect(result.success).toBe(true);
    expect(result.fingerprint).toBe(newFp);
  });

  test("launching against a stale pinned fingerprint fails with host-key-changed", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    // Pin a deliberately wrong fingerprint to simulate the user having
    // trusted an old key before the rekey.
    await apiClient.updateExecutor(seedData.sshExecutorId, {
      config: {
        ssh_host: seedData.sshTarget.host,
        ssh_port: String(seedData.sshTarget.port),
        ssh_user: seedData.sshTarget.user,
        ssh_identity_source: "file",
        ssh_identity_file: seedData.sshTarget.identityFile,
        ssh_host_fingerprint: "SHA256:was-the-old-key-but-no-more",
      },
    });

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "K2 stale fingerprint",
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
      .poll(
        async () => {
          const env = await apiClient.getTaskEnvironment(task.id);
          return env?.error_message ?? null;
        },
        { timeout: 60_000 },
      )
      .toMatch(/host key changed/i);
  });

  test("re-trusting the new fingerprint restores function", async ({ apiClient, seedData }) => {
    test.setTimeout(240_000);
    // Rotate, observe, re-trust.
    const newFp = regenerateHostKey(seedData.sshTarget);
    const test1 = await apiClient.testSSHConnection({
      name: "K3 observe new",
      host: seedData.sshTarget.host,
      port: seedData.sshTarget.port,
      user: seedData.sshTarget.user,
      identity_source: "file",
      identity_file: seedData.sshTarget.identityFile,
    });
    expect(test1.fingerprint).toBe(newFp);

    await apiClient.updateExecutor(seedData.sshExecutorId, {
      config: {
        ssh_host: seedData.sshTarget.host,
        ssh_port: String(seedData.sshTarget.port),
        ssh_user: seedData.sshTarget.user,
        ssh_identity_source: "file",
        ssh_identity_file: seedData.sshTarget.identityFile,
        ssh_host_fingerprint: newFp,
      },
    });

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "K3 after re-trust",
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
  });
});
