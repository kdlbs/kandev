import fs from "node:fs";
import { execFileSync } from "node:child_process";
import { test, expect } from "../../fixtures/ssh-test-base";
import { randomUUID } from "node:crypto";
import { readRemoteFile, remotePathExists } from "../../helpers/ssh";
import { waitForLatestSessionDone, waitForSessionState } from "../../helpers/session";
import { dwell } from "../../helpers/causal-waits";
import {
  MANAGED_RUNTIME_CACHE_ROOT,
  managedRuntimeStartupAttemptFile,
  managedRuntimeStartupSentinels,
  prepareManagedRuntimeProfile,
  prepareIncompleteManagedRuntimeProfile,
  restoreE2EAgentRegistry,
} from "../../helpers/managed-runtime-recovery";
import { SessionPage } from "../../pages/session-page";

test.describe("SSH executor - managed npm runtime recovery", () => {
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
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "private runtime on execution host");

    expect(readRemoteFile(seedData.sshTarget, fixture.sentinels.selected)).toBe("preserved\n");
    expect(readRemoteFile(seedData.sshTarget, fixture.sentinels.sibling)).toBe("preserved\n");
    const privateProject = readRemoteFile(
      seedData.sshTarget,
      `${fixture.cacheRoot}/kandev-e2e-private-roots`,
    ).trim();
    expect(privateProject).toMatch(
      /(?:\/kandev-managed-runtime-[^/]+|\/managed-runtime-repairs\/v1\/generations\/[a-f0-9]{32})\/project$/,
    );
    expect(
      readRemoteFile(
        seedData.sshTarget,
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

test("reusable verified cache repair keeps the published runtime across an agent process exit", async ({
  apiClient,
  backend,
  seedData,
  testPage,
}) => {
  test.setTimeout(300_000);
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
      "SSH reusable npm repair after process exit",
      fixture.profile.id,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.sshExecutorProfileId,
      },
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "first recovery on the remote host");
    const privateProject = readRemoteFile(
      seedData.sshTarget,
      `${fixture.cacheRoot}/kandev-e2e-private-roots`,
    )
      .trim()
      .split("\n")
      .at(-1)!;
    expect(privateProject).toMatch(
      /(?:\/kandev-managed-runtime-[^/]+|\/managed-runtime-repairs\/v1\/generations\/[a-f0-9]{32})\/project$/,
    );
    const requestsAfterRecovery = fixture.registry.requests();
    expect(requestsAfterRecovery).toBeGreaterThan(0);
    execFileSync("docker", [
      "exec",
      seedData.sshTarget.containerId,
      "sh",
      "-c",
      'for p in /proc/[0-9]*; do if grep -qa "mock[-]agent" "$p/cmdline" 2>/dev/null; then kill -9 "${p#/proc/}"; fi; done',
    ]);
    await dwell(
      2_000,
      "poll-interval",
      "wait for the killed remote agent process to settle before resume",
    );
    if (!task.session_id) throw new Error("managed runtime task did not return a session ID");
    await apiClient.launchSession(
      { task_id: task.id, session_id: task.session_id, intent: "resume" },
      120_000,
    );
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId: task.session_id,
      expectedState: "WAITING_FOR_INPUT",
      message: "remote agent process restart should reuse the published repair",
      timeout: 120_000,
    });
    expect(fixture.registry.requests()).toBe(requestsAfterRecovery);
    expect(
      readRemoteFile(
        seedData.sshTarget,
        `${privateProject}/../cache/_npx/${fixture.sharedRoot.split("/").at(-1)}/package.json`,
      ),
    ).toContain("dependencies");
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForDirectInput();
  } finally {
    await testPage.goto("/");
    await fixture.releaseBackendEnv();
    await apiClient.deleteAgentProfile(fixture.profile.id, true);
    await fixture.registry.close();
    fs.rmSync(fixture.cacheRoot, { recursive: true, force: true });
    for (const fixturePath of fixture.hostFixturePaths) fs.rmSync(fixturePath, { force: true });
  }
});
