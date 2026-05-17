import { execFileSync, spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

/**
 * Image used by the SSH executor's e2e tests. Built once per machine and
 * reused. Tag is stable so Docker's layer cache survives across runs.
 *
 * The image carries:
 *   - openssh-server + openssh-sftp-server (the remote target)
 *   - git + bash + sudo + coreutils (for the SSH executor's prepare scripts)
 *   - iproute2 + iptables (for fault-injection: drop traffic mid-session)
 *   - a pre-baked `mock-agent` binary at /usr/local/bin/mock-agent so the
 *     agentctl process the SSH executor uploads has something to spawn when
 *     the orchestrator asks for an agent subprocess.
 *
 * The `agentctl` binary is NOT baked in — the e2e tests verify the actual
 * SFTP upload + sha256-cache path the production code uses.
 */
export const SSH_E2E_IMAGE_TAG = "kandev-sshd:e2e";

const ENTRYPOINT = `#!/bin/sh
set -e

# Generate a fresh host key on first start so each test run gets a new
# fingerprint. Persisted across container restarts within the same test so
# fingerprint-pinning works end-to-end.
ssh-keygen -A >/dev/null

# Worker mounts its public key in here; copy into the kandev user's
# authorized_keys with the right perms so sshd accepts it.
if [ -f /authorized_keys ]; then
  mkdir -p /home/kandev/.ssh
  cp /authorized_keys /home/kandev/.ssh/authorized_keys
  chown -R kandev:kandev /home/kandev/.ssh
  chmod 700 /home/kandev/.ssh
  chmod 600 /home/kandev/.ssh/authorized_keys
fi

# Optional second authorized_keys for a different user (passphrase-protected
# key tests load a second key into ssh-agent and want it accepted too).
if [ -f /authorized_keys.extra ]; then
  cat /authorized_keys.extra >> /home/kandev/.ssh/authorized_keys
fi

# Run sshd in the foreground so docker logs and exit-codes propagate.
mkdir -p /var/run/sshd /var/empty
exec /usr/sbin/sshd -D -e
`;

const DOCKERFILE = `FROM alpine:3.20
RUN apk add --no-cache \\
    openssh-server \\
    openssh-sftp-server \\
    git \\
    bash \\
    sudo \\
    coreutils \\
    ca-certificates \\
    iproute2 \\
    iptables \\
 && adduser -D -s /bin/bash kandev \\
 && echo "kandev ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers \\
 && mkdir -p /home/kandev/.ssh /var/empty /var/run/sshd \\
 && chown -R kandev:kandev /home/kandev \\
 && rm -f /etc/ssh/ssh_host_*  # generated fresh by entrypoint on each first-start

# The mock-agent binary is supplied by the test runner (linux/amd64 build the
# Go suite already produces). Without it agentctl can't spawn an agent
# subprocess; with it, the SSH executor can drive a full task end-to-end.
COPY mock-agent-linux-amd64 /usr/local/bin/mock-agent
RUN chmod +x /usr/local/bin/mock-agent

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 22
ENTRYPOINT ["/entrypoint.sh"]
`;

/**
 * Returns true when a Docker daemon is reachable. Reused from docker-probe;
 * duplicated here so callers can ask the "SSH-specific" question without
 * implying anything about the Docker executor.
 */
export function hasSSHContainerSupport(): boolean {
  return spawnSync("docker", ["info"], { stdio: "ignore" }).status === 0;
}

/**
 * Build the kandev-sshd:e2e image. Idempotent: Docker layer caching makes
 * repeated builds near-instant when the Dockerfile and mock-agent haven't
 * changed.
 *
 * Throws if the mock-agent linux/amd64 binary isn't present — that's a
 * setup error the global-setup pre-flight should have caught.
 */
export function buildE2ESSHImage(): void {
  const mockAgentPath = path.resolve(__dirname, "../../../backend/bin/mock-agent-linux-amd64");
  if (!fs.existsSync(mockAgentPath)) {
    throw new Error(
      `mock-agent-linux-amd64 not found at ${mockAgentPath}; build it with ` +
        `'make -C apps/backend build-mock-agent-linux' before running container e2e tests.`,
    );
  }

  const ctxDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-e2e-sshd-img-"));
  try {
    fs.writeFileSync(path.join(ctxDir, "Dockerfile"), DOCKERFILE);
    fs.writeFileSync(path.join(ctxDir, "entrypoint.sh"), ENTRYPOINT);
    fs.copyFileSync(mockAgentPath, path.join(ctxDir, "mock-agent-linux-amd64"));
    execFileSync("docker", ["build", "-t", SSH_E2E_IMAGE_TAG, ctxDir], {
      stdio: process.env.E2E_DEBUG ? "inherit" : "ignore",
    });
  } finally {
    fs.rmSync(ctxDir, { recursive: true, force: true });
  }
}
