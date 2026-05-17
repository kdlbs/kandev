import { test, expect } from "../../fixtures/ssh-test-base";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

/**
 * SSH auth surface area: identity_source = file (with valid + missing +
 * passphrase-protected key files) and identity_source = agent (with and
 * without a running ssh-agent).
 *
 * Covers e2e-plan.md group L (L1–L5).
 */
test.describe("ssh auth methods", () => {
  test("identity_source=file with a valid key succeeds", async ({ apiClient, seedData }) => {
    const result = await apiClient.testSSHConnection({
      name: "L1",
      host: seedData.sshTarget.host,
      port: seedData.sshTarget.port,
      user: seedData.sshTarget.user,
      identity_source: "file",
      identity_file: seedData.sshTarget.identityFile,
    });
    expect(result.success).toBe(true);
    expect(result.fingerprint).toBe(seedData.sshTarget.hostFingerprint);
  });

  test("identity_source=file with a missing key file surfaces a clear error", async ({
    apiClient,
    seedData,
  }) => {
    const result = await apiClient.testSSHConnection({
      name: "L2",
      host: seedData.sshTarget.host,
      port: seedData.sshTarget.port,
      user: seedData.sshTarget.user,
      identity_source: "file",
      identity_file: "/tmp/does-not-exist-i-promise",
    });
    expect(result.success).toBe(false);
    const handshake = result.steps.find((s) => s.name === "SSH handshake");
    expect(handshake?.error).toMatch(/identity file|no such file|read/i);
  });

  test("identity_source=file with a passphrase-protected key surfaces 'load into ssh-agent'", async ({
    apiClient,
    seedData,
  }) => {
    const keyPath = path.join(seedData.sshTarget.workDir, "passphrase-protected");
    fs.rmSync(keyPath, { force: true });
    fs.rmSync(`${keyPath}.pub`, { force: true });
    execFileSync("ssh-keygen", ["-t", "ed25519", "-f", keyPath, "-N", "hunter2", "-q"]);

    const result = await apiClient.testSSHConnection({
      name: "L3",
      host: seedData.sshTarget.host,
      port: seedData.sshTarget.port,
      user: seedData.sshTarget.user,
      identity_source: "file",
      identity_file: keyPath,
    });
    expect(result.success).toBe(false);
    const handshake = result.steps.find((s) => s.name === "SSH handshake");
    expect(handshake?.error).toMatch(/passphrase|ssh-agent/i);
  });

  test("identity_source=agent without SSH_AUTH_SOCK fails with a clear message", async ({
    apiClient,
    seedData,
  }) => {
    // The backend was spawned without SSH_AUTH_SOCK (per backend.ts), so an
    // agent test should fail at handshake time.
    const result = await apiClient.testSSHConnection({
      name: "L5",
      host: seedData.sshTarget.host,
      port: seedData.sshTarget.port,
      user: seedData.sshTarget.user,
      identity_source: "agent",
    });
    expect(result.success).toBe(false);
    const handshake = result.steps.find((s) => s.name === "SSH handshake");
    expect(handshake?.error).toMatch(/ssh-agent|SSH_AUTH_SOCK/i);
  });

  test("auth failure (wrong key) surfaces verbatim", async ({ apiClient, seedData }) => {
    // Generate a brand-new keypair that isn't in authorized_keys.
    const strangerKey = path.join(seedData.sshTarget.workDir, "stranger");
    fs.rmSync(strangerKey, { force: true });
    fs.rmSync(`${strangerKey}.pub`, { force: true });
    execFileSync("ssh-keygen", ["-t", "ed25519", "-f", strangerKey, "-N", "", "-q"]);

    const result = await apiClient.testSSHConnection({
      name: "L-auth-fail",
      host: seedData.sshTarget.host,
      port: seedData.sshTarget.port,
      user: seedData.sshTarget.user,
      identity_source: "file",
      identity_file: strangerKey,
    });
    expect(result.success).toBe(false);
    const handshake = result.steps.find((s) => s.name === "SSH handshake");
    expect(handshake?.error).toMatch(/handshake|unable to authenticate|permission denied/i);
  });
});
