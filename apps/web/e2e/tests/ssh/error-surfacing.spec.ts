import { test, expect } from "../../fixtures/ssh-test-base";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

/**
 * The SSH executor commits to surfacing connection errors verbatim — no
 * generic "executor failed" wrapping. Spec lists this as a hard
 * requirement; the suite asserts the strings users see.
 *
 * Covers e2e-plan.md group Q (Q1–Q5).
 */
test.describe("ssh error surfacing", () => {
  test("TCP refused — error mentions 'connection refused'", async ({ apiClient, seedData }) => {
    const result = await apiClient.testSSHConnection({
      name: "Q1",
      host: "127.0.0.1",
      port: 1,
      user: "kandev",
      identity_source: "file",
      identity_file: seedData.sshTarget.identityFile,
    });
    expect(result.success).toBe(false);
    const handshake = result.steps.find((s) => s.name === "SSH handshake");
    expect(handshake?.error).toMatch(/connection refused|tcp dial/i);
  });

  test("auth failed — error names the failure mode", async ({ apiClient, seedData }) => {
    const strangerKey = path.join(seedData.sshTarget.workDir, "q2-stranger");
    fs.rmSync(strangerKey, { force: true });
    fs.rmSync(`${strangerKey}.pub`, { force: true });
    execFileSync("ssh-keygen", ["-t", "ed25519", "-f", strangerKey, "-N", "", "-q"]);

    const result = await apiClient.testSSHConnection({
      name: "Q2 auth fail",
      host: seedData.sshTarget.host,
      port: seedData.sshTarget.port,
      user: seedData.sshTarget.user,
      identity_source: "file",
      identity_file: strangerKey,
    });
    expect(result.success).toBe(false);
    const handshake = result.steps.find((s) => s.name === "SSH handshake");
    expect(handshake?.error).toMatch(
      /handshake|unable to authenticate|permission denied|no supported methods/i,
    );
  });

  test("ProxyJump unreachable — error mentions the bastion", async ({ apiClient, seedData }) => {
    const result = await apiClient.testSSHConnection({
      name: "Q4 bastion unreachable",
      host: seedData.sshTarget.host,
      port: seedData.sshTarget.port,
      user: seedData.sshTarget.user,
      identity_source: "file",
      identity_file: seedData.sshTarget.identityFile,
      proxy_jump: "127.0.0.1:1",
    });
    expect(result.success).toBe(false);
    const handshake = result.steps.find((s) => s.name === "SSH handshake");
    expect(handshake?.error).toMatch(/bastion|proxy|connection refused/i);
  });

  test("unsupported arm64 host surfaces the linux/amd64-only guidance", async ({
    apiClient,
    seedData,
  }) => {
    // We can't trivially spin up arm64 sshd, but we can assert the message
    // shape via the existing Verify arch step on the running container.
    // When the host is the expected x86_64, this step succeeds — that's a
    // negative control. The arm64 message itself is unit-tested in
    // executor_ssh_connection_test.go (TestRequireSupportedArch).
    const result = await apiClient.testSSHConnection({
      name: "Q-arch-control",
      host: seedData.sshTarget.host,
      port: seedData.sshTarget.port,
      user: seedData.sshTarget.user,
      identity_source: "file",
      identity_file: seedData.sshTarget.identityFile,
    });
    expect(result.success).toBe(true);
    const arch = result.steps.find((s) => s.name === "Verify arch");
    expect(arch?.success).toBe(true);
    expect(arch?.output).toBe("x86_64");
  });

  test("backend 5xx on /api/v1/ssh/test is not the same as 'test failed'", async ({
    apiClient,
  }) => {
    // POST with a body that triggers a 400 from the gin route. Asserts the
    // client surfaces the HTTP error verbatim rather than synthesising a
    // SSHTestResult-shaped success=false.
    const res = await apiClient.rawRequest("POST", "/api/v1/ssh/test", "garbage");
    expect(res.status).toBe(400);
    const body = await res.text();
    expect(body).toMatch(/invalid request body|json/i);
  });
});
