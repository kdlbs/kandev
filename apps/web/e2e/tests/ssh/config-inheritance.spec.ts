import { test, expect } from "../../fixtures/ssh-test-base";
import fs from "node:fs";
import path from "node:path";

/**
 * ~/.ssh/config inheritance: alias → HostName / Port / User / IdentityFile /
 * ProxyJump. Explicit form values always win. Unknown alias falls back to
 * alias-as-hostname.
 *
 * Each test writes a fresh ~/.ssh/config into the worker's HOME (the
 * fixture's tmpDir is the backend's HOME by construction). The backend
 * reads from $HOME/.ssh/config the same way a user's terminal does.
 *
 * Covers e2e-plan.md group M (M1–M6).
 */
test.describe("ssh ~/.ssh/config inheritance", () => {
  test("alias resolves HostName from ~/.ssh/config", async ({ apiClient, seedData, backend }) => {
    const sshConfigDir = path.join(backend.tmpDir, ".ssh");
    fs.mkdirSync(sshConfigDir, { recursive: true });
    fs.writeFileSync(
      path.join(sshConfigDir, "config"),
      [
        "Host alias-m1",
        `  HostName ${seedData.sshTarget.host}`,
        `  Port ${seedData.sshTarget.port}`,
        `  User ${seedData.sshTarget.user}`,
        `  IdentityFile ${seedData.sshTarget.identityFile}`,
        "  IdentitiesOnly yes",
        "",
      ].join("\n"),
      { mode: 0o600 },
    );

    const result = await apiClient.testSSHConnection({
      name: "M1 alias HostName",
      host_alias: "alias-m1",
    });
    expect(result.success).toBe(true);
    expect(result.fingerprint).toBe(seedData.sshTarget.hostFingerprint);
  });

  test("explicit form port wins over the alias' Port", async ({ apiClient, seedData, backend }) => {
    const sshConfigDir = path.join(backend.tmpDir, ".ssh");
    fs.mkdirSync(sshConfigDir, { recursive: true });
    fs.writeFileSync(
      path.join(sshConfigDir, "config"),
      [
        "Host alias-m2",
        `  HostName ${seedData.sshTarget.host}`,
        "  Port 1", // intentionally wrong; the form value should win
        `  User ${seedData.sshTarget.user}`,
        `  IdentityFile ${seedData.sshTarget.identityFile}`,
        "",
      ].join("\n"),
      { mode: 0o600 },
    );

    const result = await apiClient.testSSHConnection({
      name: "M2 form port wins",
      host_alias: "alias-m2",
      port: seedData.sshTarget.port,
    });
    expect(result.success).toBe(true);
  });

  test("explicit form IdentityFile wins over the alias' IdentityFile", async ({
    apiClient,
    seedData,
    backend,
  }) => {
    const sshConfigDir = path.join(backend.tmpDir, ".ssh");
    fs.mkdirSync(sshConfigDir, { recursive: true });
    fs.writeFileSync(
      path.join(sshConfigDir, "config"),
      [
        "Host alias-m3",
        `  HostName ${seedData.sshTarget.host}`,
        `  Port ${seedData.sshTarget.port}`,
        `  User ${seedData.sshTarget.user}`,
        "  IdentityFile /tmp/does-not-exist",
        "",
      ].join("\n"),
      { mode: 0o600 },
    );

    const result = await apiClient.testSSHConnection({
      name: "M3 form key wins",
      host_alias: "alias-m3",
      identity_source: "file",
      identity_file: seedData.sshTarget.identityFile,
    });
    expect(result.success).toBe(true);
  });

  test("unknown alias falls back to using the alias string as the hostname", async ({
    apiClient,
  }) => {
    // 127.0.0.1 is a hostname that resolves. Even with no matching Host
    // block, ResolveSSHTarget treats the alias as a literal host.
    const result = await apiClient.testSSHConnection({
      name: "M5 alias as hostname",
      host_alias: "127.0.0.1",
      port: 1, // refused — we just want to prove the resolve target step accepts the alias
      user: "kandev",
      identity_source: "agent",
    });
    // Resolve target should succeed (no "host is required"), the failure
    // should be at handshake time.
    expect(result.steps[0]?.name).toBe("Resolve target");
    expect(result.steps[0]?.success).toBe(true);
  });
});
