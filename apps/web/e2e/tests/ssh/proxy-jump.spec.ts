import { test, expect } from "../../fixtures/ssh-test-base";
import { startBastionAndTarget, stopBastionAndTarget } from "../../helpers/ssh-bastion";
import path from "node:path";

/**
 * ProxyJump end-to-end: two-container topology where target is only
 * reachable from inside the bastion's Docker network. A successful test
 * against `target` MUST have traversed the bastion. Chained ProxyJump
 * (a -> b -> target) is out of v1 scope; we assert it fails clearly.
 *
 * Covers e2e-plan.md group N (N1–N3). Bastion topology is set up per-test
 * because it's heavier than the default fixture's single-sshd model.
 */
test.describe("ssh executor — ProxyJump", () => {
  test("direct connect (no ProxyJump) succeeds against the bastion's own port", async ({
    apiClient,
    backend,
  }, testInfo) => {
    test.setTimeout(180_000);
    const bastion = startBastionAndTarget(
      testInfo.workerIndex,
      path.join(backend.tmpDir, "ssh-bastion-direct"),
    );
    try {
      const result = await apiClient.testSSHConnection({
        name: "N0 direct",
        host: bastion.bastion.host,
        port: bastion.bastion.port,
        user: bastion.bastion.user,
        identity_source: "file",
        identity_file: bastion.bastion.identityFile,
      });
      expect(result.success).toBe(true);
    } finally {
      stopBastionAndTarget(bastion);
    }
  });

  test("ProxyJump through bastion reaches the otherwise-unreachable target", async ({
    apiClient,
    backend,
  }, testInfo) => {
    test.setTimeout(240_000);
    const handles = startBastionAndTarget(
      testInfo.workerIndex,
      path.join(backend.tmpDir, "ssh-bastion-jump"),
    );
    try {
      // Without ProxyJump, target should be unreachable from the host.
      const direct = await apiClient.testSSHConnection({
        name: "N1 direct (should fail)",
        host: handles.targetInternalHost,
        port: 22,
        user: handles.target.user,
        identity_source: "file",
        identity_file: handles.target.identityFile,
      });
      expect(direct.success).toBe(false);

      // With ProxyJump set to the bastion, target should be reachable.
      // Note: the SSH executor parses ProxyJump as a host:port string. The
      // bastion is reachable from the host on 127.0.0.1:<bastionPort>.
      const viaJump = await apiClient.testSSHConnection({
        name: "N1 via jump",
        host: handles.targetInternalHost,
        port: 22,
        user: handles.target.user,
        identity_source: "file",
        identity_file: handles.target.identityFile,
        proxy_jump: `${handles.bastion.user}@${handles.bastion.host}:${handles.bastion.port}`,
      });
      expect(viaJump.success).toBe(true);
      expect(viaJump.fingerprint).toBe(handles.target.hostFingerprint);
    } finally {
      stopBastionAndTarget(handles);
    }
  });

  test("chained ProxyJump (a -> b -> target) fails with a clear error", async ({
    apiClient,
    backend,
  }, testInfo) => {
    test.setTimeout(180_000);
    const handles = startBastionAndTarget(
      testInfo.workerIndex,
      path.join(backend.tmpDir, "ssh-bastion-chained"),
    );
    try {
      // Comma-separated chained ProxyJump is intentionally unsupported in
      // v1. The first hop will accept, the second hop should fail because
      // ResolveSSHTarget treats the value as a single bastion identifier.
      const result = await apiClient.testSSHConnection({
        name: "N3 chained",
        host: handles.targetInternalHost,
        port: 22,
        user: handles.target.user,
        identity_source: "file",
        identity_file: handles.target.identityFile,
        proxy_jump: `${handles.bastion.user}@${handles.bastion.host}:${handles.bastion.port},another-hop`,
      });
      expect(result.success).toBe(false);
      const handshake = result.steps.find((s) => s.name === "SSH handshake");
      expect(handshake?.error).toMatch(/bastion|resolve|connect|chain/i);
    } finally {
      stopBastionAndTarget(handles);
    }
  });
});
