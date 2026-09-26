import { expect, test } from "../../fixtures/ssh-test-base";
import { execInContainer, readRemoteFile } from "../../helpers/ssh";
import { waitForLatestSessionDone } from "../../helpers/session";
import {
  assertExactFixtureTransport,
  credentialBrokerPublicURL,
  fixtureGitHost,
  fixtureGitProvider,
  fixtureGitSecret,
  fixtureGitUser,
  fixtureGitURL,
  installFixturePlugin,
  revokeFixtureConnection,
  runRemoteGit,
  startPluginGitFixture,
  uninstallFixturePlugin,
} from "../../helpers/plugin-git-credentials";

test.skip(
  !process.env.KANDEV_E2E_CREDENTIAL_BROKER_PUBLIC_BASE_URL?.trim(),
  "requires KANDEV_E2E_CREDENTIAL_BROKER_PUBLIC_BASE_URL reachable over HTTPS from SSH",
);

test.describe("Bitbucket plugin contract — SSH executor credential leases", () => {
  test("clones and pushes through HTTPS, then denies a connection-generation change", async ({
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
      // SSH intentionally forwards only broker-owned Git configuration to the
      // remote agentctl. Put test-only TLS/proxy transport config in the
      // target's system Git config; the credential lease still scopes the
      // original HTTPS identity rather than this local transport fixture.
      execInContainer(seedData.sshTarget, [
        "git",
        "config",
        "--system",
        "http.proxy",
        fixture.proxyURL,
      ]);
      execInContainer(seedData.sshTarget, ["git", "config", "--system", "http.sslVerify", "false"]);

      await installFixturePlugin(backend.baseUrl);
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
        "SSH fixture credential lease",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [repository.id],
          executor_profile_id: seedData.sshExecutorProfileId,
        },
      );
      if (!task.session_id) throw new Error("credentialed SSH task did not create a session");
      await waitForLatestSessionDone(apiClient, task.id, 1, "waiting for credentialed SSH clone");
      const rows = await apiClient.listSSHSessions(seedData.sshExecutorId);
      const row = rows.find((candidate) => candidate.task_id === task.id);
      if (!row?.remote_task_dir) throw new Error("credentialed SSH task has no remote directory");
      const agentctlPID = Number(
        readRemoteFile(
          seedData.sshTarget,
          `${row.remote_task_dir}/.kandev/sessions/${task.session_id}/agentctl.pid`,
        ).trim(),
      );
      expect(agentctlPID).toBeGreaterThan(0);
      const remoteURL = await runRemoteGit(
        agentctlPID,
        `cd '${row.remote_task_dir}' && git remote get-url origin`,
        seedData.sshTarget.containerName,
      );
      expect(remoteURL.status, remoteURL.output).toBe(0);
      expect(remoteURL.output.trim()).toBe(fixtureGitURL);

      const push = await runRemoteGit(
        agentctlPID,
        `cd '${row.remote_task_dir}' && printf 'pushed\\n' >> README.md && git add README.md && git -c user.name=E2E -c user.email=e2e@test.local commit -m 'credential fixture push' && git push origin HEAD:main`,
        seedData.sshTarget.containerName,
      );
      expect(push.status, push.output).toBe(0);
      expect(fixture.pushed()).toBe(true);
      assertExactFixtureTransport(fixture);

      const agentctlLog = readRemoteFile(
        seedData.sshTarget,
        `${row.remote_task_dir}/.kandev/sessions/${task.session_id}/agentctl.log`,
      );
      expect(agentctlLog).not.toContain(fixtureGitSecret);
      expect(agentctlLog).not.toContain(`${fixtureGitUser}:${fixtureGitSecret}`);
      const agentctlEnv = await runRemoteGit(agentctlPID, "env", seedData.sshTarget.containerName);
      expect(agentctlEnv.status, agentctlEnv.output).toBe(0);
      expect(agentctlEnv.output).not.toContain(fixtureGitSecret);
      expect(agentctlEnv.output).not.toContain(`${fixtureGitUser}:${fixtureGitSecret}`);

      const revoke = await revokeFixtureConnection(backend.baseUrl, seedData.workspaceId);
      expect(revoke.status).toBe(200);
      expect(await revoke.text()).not.toContain(fixtureGitSecret);
      const afterRevocation = await runRemoteGit(
        agentctlPID,
        `cd '${row.remote_task_dir}' && git fetch origin`,
        seedData.sshTarget.containerName,
      );
      expect(afterRevocation.status).not.toBe(0);
      expect(afterRevocation.output).not.toContain(fixtureGitSecret);
      expect(afterRevocation.output).not.toContain(`${fixtureGitUser}:${fixtureGitSecret}`);
    } finally {
      execInContainer(seedData.sshTarget, [
        "git",
        "config",
        "--system",
        "--unset-all",
        "http.proxy",
      ]);
      execInContainer(seedData.sshTarget, [
        "git",
        "config",
        "--system",
        "--unset-all",
        "http.sslVerify",
      ]);
      await uninstallFixturePlugin(backend.baseUrl);
      await fixture.close();
      await releaseBrokerEnv();
    }
  });
});
