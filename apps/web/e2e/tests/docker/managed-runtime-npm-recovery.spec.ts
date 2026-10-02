import { test, expect } from "../../fixtures/docker-test-base";
import { randomUUID } from "node:crypto";
import { dockerFileContent, dockerPathExists } from "../../helpers/docker";
import { waitForLatestSessionDone } from "../../helpers/session";
import {
  MANAGED_RUNTIME_CACHE_ROOT,
  managedRuntimeStartupAttemptFile,
  managedRuntimeStartupSentinels,
  prepareManagedRuntimeProfile,
  restoreE2EAgentRegistry,
} from "../../helpers/managed-runtime-recovery";
import { SessionPage } from "../../pages/session-page";

test.describe("Docker executor - managed npm runtime recovery", () => {
  test("retries online without deleting the executor tree", async ({
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
      const containerID = environment!.container_id!;
      expect(
        dockerFileContent(containerID, managedRuntimeStartupAttemptFile(launchId)).trim(),
      ).toBe("2");
      const sentinels = managedRuntimeStartupSentinels(packageSpec, launchId);
      expect(dockerPathExists(containerID, sentinels.selected)).toBe(true);
      expect(dockerFileContent(containerID, sentinels.selected)).toBe("preserved\n");
      expect(dockerFileContent(containerID, sentinels.sibling)).toBe("preserved\n");
      const launches = dockerFileContent(
        containerID,
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
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await expect(session.activeChat().getByTestId("managed-runtime-npm-recovery")).toHaveCount(0);
    } finally {
      if (profileId) await apiClient.deleteAgentProfile(profileId, true).catch(() => undefined);
      await restoreE2EAgentRegistry(backend);
    }
  });
});
