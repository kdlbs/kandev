import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/ssh-test-base";
import {
  insertLegacySSHFixture,
  managedSSHRequest,
  readSSHRoute,
} from "../../helpers/ssh-managed-fixture";
import type { SSHIdentity, SSHRoute } from "../../../lib/types/http-ssh-connections";

// @covers AC-EXECUTORS-SSH-MIGRATION-001.3, AC-EXECUTORS-SSH-MIGRATION-001.4
test("upgrade resolves saved aliases, preserves explicit overrides and flags an unknown alias", async ({
  backend,
  apiClient,
  seedData,
}) => {
  const target = seedData.sshTarget;
  const dir = path.join(backend.tmpDir, ".ssh");
  const file = path.join(dir, "config");
  const previous = fs.existsSync(file) ? fs.readFileSync(file) : undefined;
  fs.mkdirSync(dir, { recursive: true });
  fs.appendFileSync(
    file,
    [
      "",
      "Host inherited-fixture",
      `  HostName ${target.host}`,
      `  User ${target.user}`,
      `  Port ${target.port}`,
      `  IdentityFile ${target.identityFile}`,
      "Host overridden-fixture",
      `  HostName ${target.host}`,
      "  User unused",
      "  Port 1",
      "  IdentityFile /unavailable/fixture",
      "",
    ].join("\n"),
    { mode: 0o600 },
  );
  try {
    const routes = [
      insertLegacySSHFixture(backend.tmpDir, "Inherited", {
        ssh_host_alias: "inherited-fixture",
        ssh_host_fingerprint: target.hostFingerprint,
      }),
      insertLegacySSHFixture(backend.tmpDir, "Explicit", {
        ssh_host_alias: "overridden-fixture",
        ssh_user: target.user,
        ssh_port: String(target.port),
        ssh_identity_source: "file",
        ssh_identity_file: target.identityFile,
        ssh_host_fingerprint: target.hostFingerprint,
      }),
    ];
    const missing = insertLegacySSHFixture(backend.tmpDir, "Missing alias", {
      ssh_host_alias: "no-such-fixture",
    });
    await backend.restart();
    for (const { executorId } of routes) {
      const executor = await apiClient.getExecutor(executorId);
      const route = await readSSHRoute(backend.baseUrl, executor.config!.ssh_connection_id);
      expect(route.destination).toMatchObject({
        host: target.host,
        port: target.port,
        user: target.user,
        fingerprint: target.hostFingerprint,
      });
      const identity = await managedSSHRequest<SSHIdentity>(
        backend.baseUrl,
        `/identities/${route.destination.identity_id}`,
      );
      expect(identity).toMatchObject({ kind: "file", reference: target.identityFile });
      const verified = await managedSSHRequest<SSHRoute>(
        backend.baseUrl,
        `/connections/${route.destination.id}/test`,
        { expected_stamp: route.stamp },
      );
      expect(verified.destination.verified_route).toBe(verified.stamp);
    }
    const executor = await apiClient.getExecutor(missing.executorId);
    const unavailable = await readSSHRoute(backend.baseUrl, executor.config!.ssh_connection_id);
    expect(unavailable.destination.repair_reason).toBe("ssh_migration_alias_missing");
    expect(unavailable.destination.verified_route).toBeFalsy();
  } finally {
    if (previous) fs.writeFileSync(file, previous);
    else fs.rmSync(file, { force: true });
  }
});
