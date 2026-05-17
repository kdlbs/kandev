import { execFileSync, spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

/**
 * Live SSH server handle returned by startSSHServer. Tests use the host/port
 * pair to talk SSH, the identityFile for auth, and the hostFingerprint for
 * the "Trust this host" UI checkbox assertion. Pass it back to stop /
 * fault-injection helpers.
 */
export type SSHServerHandle = {
  containerId: string;
  containerName: string;
  /** 127.0.0.1 — sshd is reached over a published port. */
  host: string;
  /** Published host port. */
  port: number;
  /** Linux user inside the container. */
  user: string;
  /** Absolute path to the test-only private key. */
  identityFile: string;
  /** Absolute path to the matching public key. */
  publicKeyFile: string;
  /** SHA256:... fingerprint of the container's host key after first start. */
  hostFingerprint: string;
  /** Directory holding identityFile / publicKeyFile / authorized_keys. */
  workDir: string;
};

const SSH_BASE_PORT = 22000;

/**
 * Start a fresh sshd container for a Playwright worker. Each worker gets:
 *   - A unique published port (SSH_BASE_PORT + workerIndex).
 *   - A unique ed25519 keypair generated into workDir.
 *   - A unique container name `kandev-sshd-e2e-<workerIndex>` (labeled
 *     `kandev.managed=true` so the shared cleanup hook destroys it).
 *
 * Blocks until sshd answers TCP connections and a stable host fingerprint
 * is observable via `ssh-keyscan`.
 */
export function startSSHServer(
  workerIndex: number,
  imageTag: string,
  workDir: string,
): SSHServerHandle {
  fs.mkdirSync(workDir, { recursive: true });
  const port = SSH_BASE_PORT + workerIndex;
  const containerName = `kandev-sshd-e2e-${workerIndex}`;

  const identityFile = path.join(workDir, "id_ed25519");
  const publicKeyFile = `${identityFile}.pub`;
  generateKeypair(identityFile);

  // kandev's orchestrator builds the agent command using KANDEV_MOCK_AGENT_LINUX_BINARY,
  // which is set to the *host* absolute path of mock-agent-linux-amd64.
  // Docker e2e gets away with using that path inside the container because
  // it bind-mounts the same host file at the same path; for SSH e2e we have
  // to do the equivalent — otherwise the kandev backend tells agentctl on
  // the remote to exec a path that doesn't exist there.
  const mockAgentHostPath = path.resolve(__dirname, "../../../backend/bin/mock-agent-linux-amd64");
  if (!fs.existsSync(mockAgentHostPath)) {
    throw new Error(
      `mock-agent-linux-amd64 not found at ${mockAgentHostPath}; ` +
        `build it with 'make -C apps/backend build-mock-agent-linux'.`,
    );
  }

  // The container reads /authorized_keys on entry, so mount the public key
  // into that path read-only.
  removeContainerIfExists(containerName);
  // Bind-mount the remote workspace root to the host so agentctl + task logs
  // survive `--rm` teardown and the test runner can `cat /tmp/...` them in
  // failure summaries. The kandev user inside the container writes to its
  // own home dir at /home/kandev/.kandev, so we point the host side at the
  // worker's workDir/remote-home (created on demand).
  const remoteHome = path.join(workDir, "remote-home");
  fs.mkdirSync(remoteHome, { recursive: true });
  const containerId = execFileSync(
    "docker",
    [
      "run",
      "-d",
      "--rm",
      "--name",
      containerName,
      "--label",
      "kandev.managed=true",
      "--label",
      "kandev.e2e.role=ssh-target",
      "--cap-add",
      "NET_ADMIN", // iptables-based fault injection
      "-p",
      `127.0.0.1:${port}:22`,
      "-v",
      `${publicKeyFile}:/authorized_keys:ro`,
      "-v",
      `${mockAgentHostPath}:${mockAgentHostPath}:ro`,
      "-v",
      `${remoteHome}:/home/kandev/.kandev`,
      imageTag,
    ],
    { encoding: "utf8" },
  )
    .toString()
    .trim();

  waitForTCPOpen("127.0.0.1", port);
  const hostFingerprint = scanHostFingerprint("127.0.0.1", port);

  return {
    containerId,
    containerName,
    host: "127.0.0.1",
    port,
    user: "kandev",
    identityFile,
    publicKeyFile,
    hostFingerprint,
    workDir,
  };
}

/**
 * Tear down the container started by startSSHServer. Safe to call even if
 * the container has already exited.
 */
export function stopSSHServer(handle: SSHServerHandle): void {
  spawnSync("docker", ["rm", "-f", handle.containerName], { stdio: "ignore" });
}

/**
 * Regenerate the container's host key and bounce sshd. The container's host
 * fingerprint changes; subsequent connections that use the old pinned
 * fingerprint surface the host-key-changed error verbatim — exactly the path
 * the host-key-rotation specs care about.
 *
 * Returns the new fingerprint so the test can assert against it.
 */
export function regenerateHostKey(handle: SSHServerHandle): string {
  execInContainer(handle, [
    "sh",
    "-c",
    "rm -f /etc/ssh/ssh_host_* && ssh-keygen -A >/dev/null && pkill -HUP sshd || true",
  ]);
  // sshd needs a moment to re-bind after pkill -HUP.
  waitForTCPOpen(handle.host, handle.port);
  return scanHostFingerprint(handle.host, handle.port);
}

/**
 * Drop all TCP/22 traffic at the container's INPUT chain so any new SSH
 * connection times out. Existing keepalives fire and eventually evict the
 * pooled connection. Call restoreTraffic() to undo.
 */
export function dropTrafficToPort22(handle: SSHServerHandle): void {
  execInContainer(handle, ["iptables", "-A", "INPUT", "-p", "tcp", "--dport", "22", "-j", "DROP"]);
}

export function restoreTraffic(handle: SSHServerHandle): void {
  // Best-effort: removes the first matching rule. Idempotent across repeat calls
  // because subsequent ones report "Bad rule" via stderr and we ignore it.
  spawnSync(
    "docker",
    [
      "exec",
      handle.containerName,
      "iptables",
      "-D",
      "INPUT",
      "-p",
      "tcp",
      "--dport",
      "22",
      "-j",
      "DROP",
    ],
    { stdio: "ignore" },
  );
}

/**
 * Kill a remote process by pid inside the container. Used by recovery tests
 * to simulate "the agentctl process died" without disturbing the SSH
 * connection itself.
 */
export function killRemotePid(handle: SSHServerHandle, pid: number): void {
  execInContainer(handle, ["kill", "-9", String(pid)]);
}

/**
 * Read a file from inside the container as the kandev user. Used to assert
 * what the SSH executor wrote — uploaded agentctl path, sha256 sidecar, port
 * file, pid file. Returns the file's contents; throws on `cat` failure.
 */
export function readRemoteFile(handle: SSHServerHandle, absPath: string): string {
  return execInContainer(handle, ["sh", "-c", `cat ${shellQuote(absPath)}`]);
}

/**
 * Check whether a file or directory exists inside the container.
 */
export function remotePathExists(handle: SSHServerHandle, absPath: string): boolean {
  const res = spawnSync(
    "docker",
    ["exec", handle.containerName, "sh", "-c", `test -e ${shellQuote(absPath)}`],
    { stdio: "ignore" },
  );
  return res.status === 0;
}

/**
 * List the entries directly under a directory inside the container.
 * One basename per line; empty lines suppressed.
 */
export function listRemoteDir(handle: SSHServerHandle, absPath: string): string[] {
  const res = spawnSync(
    "docker",
    ["exec", handle.containerName, "sh", "-c", `ls -1 ${shellQuote(absPath)} 2>/dev/null || true`],
    { encoding: "utf8" },
  );
  return res.stdout
    .toString()
    .split("\n")
    .map((s) => s.trim())
    .filter(Boolean);
}

/**
 * Count how many sshd child processes are alive in the container. Specs that
 * assert "same-host sessions share one SSH connection" use this to detect
 * connection fan-in.
 */
export function countSshdConnections(handle: SSHServerHandle): number {
  const out = execInContainer(handle, ["sh", "-c", "ps -ef | grep -c 'sshd.*: kandev' || true"]);
  return parseInt(out.trim(), 10) || 0;
}

// --- internals ---

function generateKeypair(identityFile: string): void {
  if (fs.existsSync(identityFile)) {
    fs.rmSync(identityFile, { force: true });
    fs.rmSync(`${identityFile}.pub`, { force: true });
  }
  execFileSync(
    "ssh-keygen",
    ["-t", "ed25519", "-f", identityFile, "-N", "", "-C", "kandev-e2e", "-q"],
    { stdio: process.env.E2E_DEBUG ? "inherit" : "ignore" },
  );
  // Ensure private key has the strict perms ssh requires.
  fs.chmodSync(identityFile, 0o600);
}

function execInContainer(handle: SSHServerHandle, argv: string[]): string {
  const res = spawnSync("docker", ["exec", handle.containerName, ...argv], {
    encoding: "utf8",
  });
  if (res.status !== 0) {
    throw new Error(
      `docker exec ${argv.join(" ")} failed (status=${res.status}): ${res.stderr || res.stdout}`,
    );
  }
  return res.stdout;
}

function removeContainerIfExists(name: string): void {
  spawnSync("docker", ["rm", "-f", name], { stdio: "ignore" });
}

function waitForTCPOpen(host: string, port: number, timeoutMs = 30_000): void {
  const deadline = Date.now() + timeoutMs;
  let lastErr = "";
  while (Date.now() < deadline) {
    // Prefer bash's /dev/tcp probe — always available on POSIX, no `nc` dep.
    const res = spawnSync("bash", ["-c", `exec 3<>/dev/tcp/${host}/${port} && exec 3<&-`], {
      stdio: ["ignore", "ignore", "pipe"],
      encoding: "utf8",
    });
    if (res.status === 0) return;
    lastErr = (res.stderr ?? "").trim();
    sleep(250);
  }
  throw new Error(`sshd at ${host}:${port} did not open within ${timeoutMs}ms: ${lastErr}`);
}

function scanHostFingerprint(host: string, port: number): string {
  // ssh-keyscan prints the host's public key on stdout; `# host SSH-2.0-...`
  // comment lines go to stderr. We retry briefly because the first scan can
  // race the container's first sshd accept.
  const deadline = Date.now() + 15_000;
  let stdout = "";
  let stderr = "";
  while (Date.now() < deadline) {
    const keyOut = spawnSync(
      "ssh-keyscan",
      ["-p", String(port), "-t", "ed25519", "-T", "5", host],
      {
        encoding: "utf8",
      },
    );
    stdout = keyOut.stdout ?? "";
    stderr = keyOut.stderr ?? "";
    if (keyOut.status === 0 && stdout.trim()) break;
    sleep(300);
  }
  if (!stdout.trim()) {
    throw new Error(
      `ssh-keyscan ${host}:${port} returned nothing after retries; stderr=${stderr.trim()}`,
    );
  }
  const tmpKey = path.join(os.tmpdir(), `kandev-e2e-keyscan-${process.pid}-${port}`);
  fs.writeFileSync(tmpKey, stdout);
  try {
    const fp = spawnSync("ssh-keygen", ["-lf", tmpKey], { encoding: "utf8" });
    const match = fp.stdout.match(/SHA256:\S+/);
    if (!match) throw new Error(`could not parse SHA256 fingerprint from: ${fp.stdout}`);
    return match[0];
  } finally {
    fs.rmSync(tmpKey, { force: true });
  }
}

function shellQuote(s: string): string {
  return `'${s.replace(/'/g, `'\\''`)}'`;
}

function sleep(ms: number): void {
  // Synchronous sleep via the system `sleep` binary. Subprocess overhead is
  // dwarfed by the ms we're waiting, and we get an honest wall-clock pause
  // rather than CPU spin.
  spawnSync("sleep", [(ms / 1000).toFixed(3)]);
}
