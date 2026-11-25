import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/ssh-test-base";
import {
  insertLegacySSHFixture,
  legacySSHFixtureConfig,
  readSSHRoute,
} from "../../helpers/ssh-managed-fixture";
import {
  dropTrafficToPort22,
  execInContainer,
  restoreTraffic,
  type SSHServerHandle,
} from "../../helpers/ssh";
import { startBastionAndTarget, stopBastionAndTarget } from "../../helpers/ssh-bastion";
import { pollUntil } from "../../helpers/poll-until";

// @covers AC-EXECUTORS-SSH-MIGRATION-001.1, 001.3, 001.4
for (const topology of ["direct", "bastion"] as const) {
  test(`offline ${topology} import launches on first use through the original profile`, async ({
    backend,
    apiClient,
    seedData,
  }, testInfo) => {
    test.setTimeout(240_000);
    const handles =
      topology === "bastion"
        ? startBastionAndTarget(
            testInfo.workerIndex,
            path.join(backend.tmpDir, "migration-bastion"),
            { separateIdentities: true },
          )
        : undefined;
    const target = handles?.target ?? seedData.sshTarget;
    const ingress = handles?.bastion ?? target;
    let restoreSSHConfig = () => {};
    const config: Record<string, string> = { ...legacySSHFixtureConfig(target), ssh_shell: "bash" };
    const publicHostKey = "/etc/ssh/ssh_host_ed25519_key.pub";
    const fingerprint = execInContainer(target, ["ssh-keygen", "-lf", publicHostKey]).split(
      /\s+/,
    )[1];
    expect(fingerprint).toMatch(/^SHA256:/);
    config.ssh_host_fingerprint = fingerprint;
    try {
      if (handles) {
        config.ssh_proxy_jump = legacyBastionAlias;
        restoreSSHConfig = prepareHistoricalBastion(path.join(backend.tmpDir, ".ssh"), ingress);
        const rewrite = execInContainer(seedData.sshTarget, [
          "git",
          "config",
          "--system",
          "--get-regexp",
          "^url\\..*\\.insteadof$",
        ]).trim();
        const separator = rewrite.indexOf(" ");
        expect(separator).toBeGreaterThan(0);
        execInContainer(target, [
          "git",
          "config",
          "--system",
          rewrite.slice(0, separator),
          rewrite.slice(separator + 1),
        ]);
      }

      const { executorId, profileId } = insertLegacySSHFixture(
        backend.tmpDir,
        `Historical ${topology} executor`,
        config,
      );
      dropTrafficToPort22(ingress);
      try {
        await backend.restart();
      } finally {
        restoreTraffic(ingress);
      }
      const imported = await apiClient.getExecutor(executorId);
      const id = imported.config?.ssh_connection_id;
      expect(id).toBeTruthy();
      expect(imported.config).toEqual({
        ssh_shell: "bash",
        ssh_connection_id: id,
        ssh_connection_stamp: expect.any(String),
      });
      const route = await readSSHRoute(backend.baseUrl, id!);
      expect(route.destination.fingerprint).toBe(fingerprint);
      expect(route.destination.verified_route).toBeFalsy();
      expect(route.destination.imported_pending).toBe(true);
      if (handles) {
        expect(route.bastion).toMatchObject({
          host: ingress.host,
          port: ingress.port,
          user: ingress.user,
          fingerprint: ingress.hostFingerprint,
        });
      }

      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        `First launch after ${topology} import`,
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: profileId,
        },
      );
      const result = await pollUntil(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return { state: sessions[0]?.state, error: sessions[0]?.error_message };
        },
        ({ state }) =>
          ["COMPLETED", "WAITING_FOR_INPUT", "FAILED", "CANCELLED"].includes(state ?? ""),
        120_000,
        "Imported SSH workload",
      );
      expect(result, result.error).toMatchObject({
        state: expect.stringMatching(/^(COMPLETED|WAITING_FOR_INPUT)$/),
      });
      const verified = await readSSHRoute(backend.baseUrl, id!);
      expect(verified.destination.verified_route).toBe(verified.stamp);
      expect(verified.destination.imported_pending).toBeFalsy();
    } finally {
      if (handles) {
        try {
          await apiClient.e2eReset(seedData.workspaceId, [seedData.workflowId]);
        } finally {
          stopBastionAndTarget(handles);
          restoreSSHConfig();
        }
      }
    }
  });
}

const legacyBastionAlias = "fixture-migration-bastion";

function prepareHistoricalBastion(directory: string, bastion: SSHServerHandle) {
  const config = path.join(directory, "config");
  const knownHosts = path.join(directory, "known_hosts");
  const previous = [config, knownHosts].map((file) => ({
    file,
    bytes: fs.existsSync(file) ? fs.readFileSync(file) : undefined,
  }));
  const key = execInContainer(bastion, ["cat", "/etc/ssh/ssh_host_ed25519_key.pub"]);
  fs.mkdirSync(directory, { recursive: true });
  fs.appendFileSync(
    config,
    [
      "",
      `Host ${legacyBastionAlias}`,
      `  HostName ${bastion.host}`,
      `  Port ${bastion.port}`,
      `  User ${bastion.user}`,
      `  IdentityFile "${bastion.identityFile}"`,
      "",
    ].join("\n"),
  );
  fs.appendFileSync(knownHosts, `\n[${bastion.host}]:${bastion.port} ${key.trim()}\n`);
  return () => {
    for (const { file, bytes } of previous) {
      if (bytes) fs.writeFileSync(file, bytes);
      else fs.rmSync(file, { force: true });
    }
  };
}
