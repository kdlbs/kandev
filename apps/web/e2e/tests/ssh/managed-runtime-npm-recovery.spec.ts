import { test, expect } from "../../fixtures/ssh-test-base";
import { readRemoteFile, remotePathExists } from "../../helpers/ssh";
import { waitForLatestSessionDone } from "../../helpers/session";
import {
  MANAGED_RUNTIME_CACHE_ROOT,
  MANAGED_RUNTIME_PACKAGE_SPEC,
  managedRuntimeExecutionCacheKey,
  prepareManagedRuntimeProfile,
  restoreE2EAgentRegistry,
} from "../../helpers/managed-runtime-recovery";
import { SessionPage } from "../../pages/session-page";

test.describe("SSH executor - managed npm runtime recovery", () => {
  test("repairs the remote cache and completes the original session", async ({
    apiClient,
    backend,
    seedData,
    testPage,
  }) => {
    test.setTimeout(240_000);
    let profileId = "";
    try {
      const profile = await prepareManagedRuntimeProfile(apiClient, backend);
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
      const target = `${MANAGED_RUNTIME_CACHE_ROOT}/_npx/${managedRuntimeExecutionCacheKey()}`;
      const sibling = `${MANAGED_RUNTIME_CACHE_ROOT}/_npx/0123456789abcdef`;
      expect(remotePathExists(seedData.sshTarget, `${target}/stale-marker`)).toBe(false);
      expect(readRemoteFile(seedData.sshTarget, `${target}/fresh-marker`)).toBe("fresh\n");
      expect(readRemoteFile(seedData.sshTarget, `${sibling}/sibling-marker`)).toBe("sibling\n");
      const onlineInvocations = readRemoteFile(
        seedData.sshTarget,
        `${MANAGED_RUNTIME_CACHE_ROOT}/online-invocations`,
      );
      expect(onlineInvocations.trim().split(/\r?\n/)).toEqual([MANAGED_RUNTIME_PACKAGE_SPEC]);
      expect(MANAGED_RUNTIME_PACKAGE_SPEC).toBe("opencode-ai@1.18.18");

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
