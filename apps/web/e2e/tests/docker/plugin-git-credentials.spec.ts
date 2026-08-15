import { spawnSync } from "node:child_process";
import { expect, test } from "../../fixtures/docker-test-base";
import { waitForLatestSessionDone } from "../../helpers/session";
import {
  assertExactFixtureTransport,
  credentialBrokerPublicURL,
  fixtureGitHost,
  fixtureGitURL,
  fixtureGitSecret,
  fixtureGitUser,
  fixtureGitProvider,
  installFixturePlugin,
  revokeFixtureConnection,
  runDockerGit,
  startPluginGitFixture,
  uninstallFixturePlugin,
} from "../../helpers/plugin-git-credentials";
import { E2E_IMAGE_TAG } from "../../fixtures/docker-probe";

test.skip(
  !process.env.KANDEV_E2E_CREDENTIAL_BROKER_PUBLIC_BASE_URL?.trim(),
  "requires KANDEV_E2E_CREDENTIAL_BROKER_PUBLIC_BASE_URL reachable over HTTPS from Docker",
);

test.describe("Bitbucket plugin contract — Docker executor credential leases", () => {
  test("clones and pushes through an exact provider lease, then rejects its revoked generation", async ({
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(240_000);
    const publicBrokerURL = credentialBrokerPublicURL(backend.port);
    if (!publicBrokerURL) throw new Error("credential broker public URL is required");

    const releaseBrokerEnv = await backend.useEnv({
      KANDEV_GITHUB_CREDENTIAL_BROKER_PUBLIC_BASE_URL: publicBrokerURL,
    });
    const fixture = await startPluginGitFixture(backend.tmpDir);
    try {
      await installFixturePlugin(backend.baseUrl);
      const { executors } = await apiClient.listExecutors();
      const dockerExecutor = executors.find((executor) => executor.type === "local_docker");
      expect(dockerExecutor).toBeTruthy();
      const profile = await apiClient.createExecutorProfile(dockerExecutor!.id, {
        name: "E2E provider-neutral Git credentials",
        config: { image_tag: E2E_IMAGE_TAG },
        prepare_script: "",
        cleanup_script: "",
        env_vars: [
          ...fixture.profileGitEnv,
          { key: "NO_PROXY", value: new URL(publicBrokerURL).hostname },
        ],
      });
      const repository = await apiClient.createRepository(
        seedData.workspaceId,
        fixture.checkoutPath,
        "main",
        {
          name: "TEAM/fixture",
          provider: fixtureGitProvider,
          provider_host: `https://${fixtureGitHost}`,
          provider_owner: "TEAM",
          provider_name: "fixture",
        },
      );
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Docker fixture credential lease",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [repository.id],
          executor_profile_id: profile.id,
        },
      );
      await waitForLatestSessionDone(
        apiClient,
        task.id,
        1,
        "waiting for credentialed Docker clone",
      );
      const environment = await apiClient.getTaskEnvironment(task.id);
      expect(environment?.container_id).toBeTruthy();
      const remoteURL = await runDockerGit(
        environment!.container_id!,
        "cd /workspace && git remote get-url origin",
      );
      expect(remoteURL.status, remoteURL.output).toBe(0);
      expect(remoteURL.output.trim()).toBe(fixtureGitURL);

      const push = await runDockerGit(
        environment!.container_id!,
        "cd /workspace && printf 'pushed\\n' >> README.md && git add README.md && git -c user.name=E2E -c user.email=e2e@test.local commit -m 'credential fixture push' && git push origin HEAD:main",
      );
      expect(push.status, push.output).toBe(0);
      expect(fixture.pushed()).toBe(true);
      assertExactFixtureTransport(fixture);

      const inspect = spawnSync("docker", ["inspect", environment!.container_id!], {
        encoding: "utf8",
      });
      const taskResponse = await apiClient.rawRequest("GET", `/api/v1/tasks/${task.id}`);
      const taskBody = await taskResponse.text();
      expect(`${inspect.stdout}${taskBody}`).not.toContain(fixtureGitSecret);
      expect(`${inspect.stdout}${taskBody}`).not.toContain(`${fixtureGitUser}:${fixtureGitSecret}`);

      const revoke = await revokeFixtureConnection(backend.baseUrl, seedData.workspaceId);
      expect(revoke.status).toBe(200);
      expect(await revoke.text()).not.toContain(fixtureGitSecret);

      const afterRevocation = await runDockerGit(
        environment!.container_id!,
        "cd /workspace && git fetch origin",
      );
      expect(afterRevocation.status).not.toBe(0);
      expect(afterRevocation.output).not.toContain(fixtureGitSecret);
      expect(afterRevocation.output).not.toContain(`${fixtureGitUser}:${fixtureGitSecret}`);
      const logs = spawnSync("docker", ["logs", environment!.container_id!], { encoding: "utf8" });
      expect(`${logs.stdout}${logs.stderr}`).not.toContain(fixtureGitSecret);
    } finally {
      await uninstallFixturePlugin(backend.baseUrl);
      await fixture.close();
      await releaseBrokerEnv();
    }
  });
});
