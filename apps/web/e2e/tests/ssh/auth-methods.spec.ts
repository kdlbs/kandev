import { test, expect } from "../../fixtures/ssh-test-base";
import { execFileSync } from "node:child_process";
import path from "node:path";
import { randomUUID } from "node:crypto";
import type { SSHPreflight } from "../../../lib/types/http-ssh-connections";
import {
  createSSHRoute,
  managedSSHRequest,
  trustAndTestSSHRoute,
} from "../../helpers/ssh-managed-fixture";

for (const [label, kind, preflightCode] of [
  ["missing key", "file", "ssh_key_file_unavailable"],
  ["encrypted key", "file", "ssh_key_file_unavailable"],
  ["unknown key", "file", null],
  ["missing agent", "agent", "ssh_agent_unavailable"],
] as const) {
  test(`managed access blocks login with ${label}`, async ({ backend, seedData }) => {
    const ref = path.join(backend.tmpDir, randomUUID());
    if (label === "encrypted key" || label === "unknown key") {
      const passphrase = label === "encrypted key" ? "fixture-passphrase" : "";
      execFileSync("ssh-keygen", ["-q", "-t", "ed25519", "-f", ref, "-N", passphrase]);
    }
    const route = await createSSHRoute(backend.baseUrl, label, seedData.sshTarget, {
      identity: { kind, reference: ref },
    });
    if (preflightCode) {
      const diagnostic = await managedSSHRequest<SSHPreflight>(
        backend.baseUrl,
        `/connections/${route.destination.id}/preflight`,
        { expected_stamp: route.stamp },
      );
      expect(diagnostic.checks).toContainEqual({ code: preflightCode, hop: "destination" });
      expect(diagnostic.ready).toBe(false);
    }
    await expect(trustAndTestSSHRoute(backend.baseUrl, route)).rejects.toThrow(
      preflightCode ? "ssh_credential_unavailable" : "ssh_authentication_rejected",
    );
  });
}
