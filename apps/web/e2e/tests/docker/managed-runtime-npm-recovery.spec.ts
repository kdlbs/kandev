import fs from "node:fs";
import { execFileSync } from "node:child_process";
import { test, expect } from "../../fixtures/docker-test-base";
import { randomUUID } from "node:crypto";
import { dockerFileContent, dockerPathExists } from "../../helpers/docker";
import { waitForLatestSessionDone } from "../../helpers/session";
import {
  MANAGED_RUNTIME_CACHE_ROOT,
  managedRuntimeStartupAttemptFile,
  managedRuntimeStartupSentinels,
  prepareManagedRuntimeProfile,
  prepareIncompleteManagedRuntimeProfile,
  restoreE2EAgentRegistry,
} from "../../helpers/managed-runtime-recovery";
import { SessionPage } from "../../pages/session-page";

test.describe("Docker executor - managed npm runtime recovery", () => {
  test.afterEach(async ({ backend }, testInfo) => {
    if (testInfo.status !== testInfo.expectedStatus) {
      await testInfo
        .attach("managed-runtime-recovery-backend.log", {
          path: backend.logPath,
          contentType: "text/plain",
        })
        .catch(() => undefined);
    }
  });

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

test("privately prepares the exact runtime on the execution host after a missing manifest", async ({
  apiClient,
  backend,
  seedData,
  testPage,
}) => {
  test.setTimeout(240_000);
  const gateway =
    process.platform === "linux"
      ? execFileSync(
          "docker",
          ["network", "inspect", "bridge", "--format", "{{(index .IPAM.Config 0).Gateway}}"],
          { encoding: "utf8" },
        ).trim()
      : "host.docker.internal";
  const launchId = randomUUID();
  const fixture = await prepareIncompleteManagedRuntimeProfile(apiClient, backend, {
    launchId,
    registryHost: gateway,
  });
  try {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Executor private npm recovery",
      fixture.profile.id,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.dockerExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "private runtime on execution host");
    const environment = await apiClient.getTaskEnvironment(task.id);
    expect(environment?.container_id).toBeTruthy();
    const containerID = environment!.container_id!;
    expect(dockerFileContent(containerID, fixture.sentinels.selected)).toBe("preserved\n");
    expect(dockerFileContent(containerID, fixture.sentinels.sibling)).toBe("preserved\n");
    const privateProject = dockerFileContent(
      containerID,
      `${fixture.cacheRoot}/kandev-e2e-private-roots`,
    ).trim();
    expect(privateProject).toContain("/kandev-managed-runtime-");
    expect(
      dockerFileContent(
        containerID,
        `${privateProject}/../cache/_npx/${fixture.sharedRoot.split("/").at(-1)}/package.json`,
      ),
    ).toContain("dependencies");
    expect(fixture.registry.requests()).toBeGreaterThan(0);
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.expectChatResponseVisible("simple mock response", 0);
    await session.sendMessage("/e2e:simple-message");
    await session.expectChatResponseVisible("simple mock response", 1);
  } finally {
    await testPage.goto("/");
    await fixture.releaseBackendEnv();
    await apiClient.deleteAgentProfile(fixture.profile.id, true);
    await fixture.registry.close();
    fs.rmSync(fixture.cacheRoot, { recursive: true, force: true });
    for (const fixturePath of fixture.hostFixturePaths) fs.rmSync(fixturePath, { force: true });
  }
});
