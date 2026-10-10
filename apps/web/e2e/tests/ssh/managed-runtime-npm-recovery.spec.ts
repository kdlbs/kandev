import { test, expect } from "../../fixtures/ssh-test-base";
import { randomUUID } from "node:crypto";
import { readRemoteFile, remotePathExists } from "../../helpers/ssh";
import { waitForLatestSessionDone } from "../../helpers/session";
import {
  MANAGED_RUNTIME_CACHE_ROOT,
  managedRuntimeStartupAttemptFile,
  managedRuntimeStartupSentinels,
  prepareManagedRuntimeProfile,
  restoreE2EAgentRegistry,
} from "../../helpers/managed-runtime-recovery";
import { SessionPage } from "../../pages/session-page";

test.describe("SSH executor - managed npm runtime recovery", () => {
  test("retries online without deleting the remote tree", async ({
    apiClient,
    backend,
    seedData,
    testPage,
  }) => {
    test.setTimeout(240_000);
    let profileId = "";
    try {
      const launchId = randomUUID();
      const { profile, packageSpec } = await prepareManagedRuntimeProfile(apiClient, backend, {
        startupMode: "transient",
        launchId,
      });
      profileId = profile.id;
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "SSH managed npm recovery",
        profile.id,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: seedData.sshExecutorProfileId,
        },
      );

      await waitForLatestSessionDone(apiClient, task.id, 1, "Wait for SSH managed recovery");
      const session = (await apiClient.listSSHSessions(seedData.sshExecutorId)).find(
        (candidate) => candidate.task_id === task.id,
      );
      expect(session?.remote_task_dir).toBeTruthy();
      expect(
        readRemoteFile(seedData.sshTarget, managedRuntimeStartupAttemptFile(launchId)).trim(),
      ).toBe("2");
      const sentinels = managedRuntimeStartupSentinels(packageSpec, launchId);
      expect(remotePathExists(seedData.sshTarget, sentinels.selected)).toBe(true);
      expect(readRemoteFile(seedData.sshTarget, sentinels.selected)).toBe("preserved\n");
      expect(readRemoteFile(seedData.sshTarget, sentinels.sibling)).toBe("preserved\n");
      const launches = readRemoteFile(
        seedData.sshTarget,
        `${MANAGED_RUNTIME_CACHE_ROOT}/kandev-e2e-launches`,
      )
        .trim()
        .split(/\r?\n/)
        .filter((line) => line.startsWith(`${launchId}\t`));
      expect(launches.map((line) => line.split("\t")[3])).toEqual([
        "--prefer-offline",
        "--prefer-online",
      ]);

      await testPage.goto(`/t/${task.id}`);
      const page = new SessionPage(testPage);
      await page.waitForLoad();
      await expect(page.activeChat().getByTestId("managed-runtime-npm-recovery")).toHaveCount(0);
    } finally {
      if (profileId) await apiClient.deleteAgentProfile(profileId, true).catch(() => undefined);
      await restoreE2EAgentRegistry(backend);
    }
  });
});
