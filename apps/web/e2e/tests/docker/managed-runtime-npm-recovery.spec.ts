import { test, expect } from "../../fixtures/docker-test-base";
import { dockerFileContent, dockerPathExists } from "../../helpers/docker";
import { waitForLatestSessionDone } from "../../helpers/session";
import {
  MANAGED_RUNTIME_CACHE_ROOT,
  MANAGED_RUNTIME_PACKAGE_SPEC,
  managedRuntimeExecutionCacheKey,
  prepareManagedRuntimeProfile,
  restoreE2EAgentRegistry,
} from "../../helpers/managed-runtime-recovery";
import { SessionPage } from "../../pages/session-page";

test.describe("Docker executor - managed npm runtime recovery", () => {
  test("repairs the executor cache and completes the original session", async ({
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
        "Docker managed npm recovery",
        profile.id,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: seedData.dockerExecutorProfileId,
        },
      );

      await waitForLatestSessionDone(apiClient, task.id, 1, "Wait for Docker managed recovery");
      const environment = await apiClient.getTaskEnvironment(task.id);
      expect(environment?.container_id).toBeTruthy();
      const target = `${MANAGED_RUNTIME_CACHE_ROOT}/_npx/${managedRuntimeExecutionCacheKey()}`;
      const sibling = `${MANAGED_RUNTIME_CACHE_ROOT}/_npx/0123456789abcdef`;
      const containerID = environment!.container_id!;
      expect(dockerPathExists(containerID, `${target}/stale-marker`)).toBe(false);
      expect(dockerFileContent(containerID, `${target}/fresh-marker`)).toBe("fresh\n");
      expect(dockerFileContent(containerID, `${sibling}/sibling-marker`)).toBe("sibling\n");
      const onlineInvocations = dockerFileContent(
        containerID,
        `${MANAGED_RUNTIME_CACHE_ROOT}/online-invocations`,
      );
      expect(onlineInvocations.trim().split(/\r?\n/)).toEqual([MANAGED_RUNTIME_PACKAGE_SPEC]);

      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await expect(session.activeChat().getByTestId("managed-runtime-npm-recovery")).toHaveCount(0);
    } finally {
      if (profileId) await apiClient.deleteAgentProfile(profileId, true).catch(() => undefined);
      await restoreE2EAgentRegistry(backend);
    }
  });
});
