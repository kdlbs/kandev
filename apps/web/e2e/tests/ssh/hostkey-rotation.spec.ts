import { test, expect } from "../../fixtures/ssh-test-base";
import { regenerateHostKey } from "../../helpers/ssh";
import { waitForLatestSessionDone } from "../../helpers/session";
import {
  createTestedSSHRoute,
  observeSSHRoute,
  readSSHRoute,
  sshBinding,
} from "../../helpers/ssh-managed-fixture";

test("rotation keeps the old pin until a replacement connection is verified and bound", async ({
  apiClient,
  backend,
  seedData,
}) => {
  test.setTimeout(240_000);
  const old = (await apiClient.getExecutor(seedData.sshExecutorId)).config!;
  const id = old.ssh_connection_id;
  const route = await readSSHRoute(backend.baseUrl, id);
  const startTask = (title: string) =>
    apiClient.createTaskWithAgent(seedData.workspaceId, title, seedData.agentProfileId, {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      executor_profile_id: seedData.sshExecutorProfileId,
    });

  regenerateHostKey(seedData.sshTarget);
  try {
    await expect(async () => {
      const observed = await observeSSHRoute(backend.baseUrl, route);
      expect(observed.fingerprint).not.toBe(route.destination.fingerprint);
    }).toPass({ timeout: 15_000 });
    expect((await readSSHRoute(backend.baseUrl, id)).destination.fingerprint).toBe(
      route.destination.fingerprint,
    );

    const failed = await startTask("Launch against stale SSH pin");
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(failed.id);
          return sessions[0]?.error_message ?? "";
        },
        { timeout: 60_000 },
      )
      .toMatch(/host.key.changed/i);
  } finally {
    const replacement = await createTestedSSHRoute(
      backend.baseUrl,
      "Replacement connection",
      seedData.sshTarget,
    );
    await apiClient.updateExecutor(seedData.sshExecutorId, { config: sshBinding(replacement) });
    seedData.sshTarget.hostFingerprint = replacement.destination.fingerprint;
  }

  const task = await startTask("Launch against verified replacement");
  await waitForLatestSessionDone(apiClient, task.id, 1, "Host-key rotation recovery");
});
