import { execFileSync, spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { SSH_E2E_IMAGE_TAG } from "../fixtures/ssh-image";
import type { SSHServerHandle } from "./ssh";

/**
 * Two-container setup for ProxyJump end-to-end tests:
 *
 *   kandev backend  --ssh-->  bastion (port published)  --tcp--> target (no port published)
 *
 * `bastion` lives on a private Docker network and has a published port the
 * host can dial. `target` is on the same private network only — there is no
 * port-forward from the host to it, so any successful connection to target
 * MUST have traversed the bastion. That makes the assertion honest: a green
 * test proves we actually proxied, not that we side-stepped.
 *
 * Each test worker gets its own network and pair of containers so concurrent
 * workers don't fight over container names or IP ranges.
 */
export type SSHBastionHandles = {
  bastion: SSHServerHandle;
  target: SSHServerHandle;
  /** Docker network name; teardown removes it. */
  networkName: string;
  /**
   * Hostname the kandev backend uses for the target. Resolves inside the
   * private network only — supplying this as `host` to the SSH executor
   * proves the connection went through the bastion.
   */
  targetInternalHost: string;
};

const BASTION_BASE_PORT = 23000;

export function startBastionAndTarget(workerIndex: number, workDir: string): SSHBastionHandles {
  fs.mkdirSync(workDir, { recursive: true });
  const networkName = `kandev-e2e-ssh-net-${workerIndex}`;
  ensureNetwork(networkName);

  // The SSH executor's ProxyJump string doesn't carry a separate identity for
  // the bastion (matching OpenSSH semantics — per-host identities come from
  // ~/.ssh/config or ssh-agent). For these tests we share one keypair across
  // bastion + target so a single -i flag authenticates both hops.
  const sharedIdentityFile = path.join(workDir, "shared_id_ed25519");
  generateKeypair(sharedIdentityFile);

  const bastionPort = BASTION_BASE_PORT + workerIndex * 2;
  const bastion = launchInNetwork({
    name: `kandev-bastion-${workerIndex}`,
    role: "bastion",
    networkName,
    hostPort: bastionPort,
    workDir: path.join(workDir, "bastion"),
    identityFile: sharedIdentityFile,
  });

  const target = launchInNetwork({
    name: `kandev-target-${workerIndex}`,
    role: "target",
    networkName,
    hostPort: null, // not reachable from host
    workDir: path.join(workDir, "target"),
    identityFile: sharedIdentityFile,
  });

  return {
    bastion,
    target,
    networkName,
    targetInternalHost: target.containerName,
  };
}

export function stopBastionAndTarget(handles: SSHBastionHandles): void {
  spawnSync("docker", ["rm", "-f", handles.bastion.containerName], { stdio: "ignore" });
  spawnSync("docker", ["rm", "-f", handles.target.containerName], { stdio: "ignore" });
  spawnSync("docker", ["network", "rm", handles.networkName], { stdio: "ignore" });
}

// --- internals ---

function ensureNetwork(name: string): void {
  // `docker network create` errors if it exists; check first to be idempotent.
  const inspect = spawnSync("docker", ["network", "inspect", name], { stdio: "ignore" });
  if (inspect.status === 0) return;
  execFileSync("docker", ["network", "create", "--driver", "bridge", name], {
    stdio: process.env.E2E_DEBUG ? "inherit" : "ignore",
  });
}

interface LaunchOpts {
  name: string;
  role: "bastion" | "target";
  networkName: string;
  hostPort: number | null;
  workDir: string;
  /** Pre-generated identity file to mount as authorized_keys. */
  identityFile: string;
}

function launchInNetwork(opts: LaunchOpts): SSHServerHandle {
  fs.mkdirSync(opts.workDir, { recursive: true });

  const identityFile = opts.identityFile;
  const publicKeyFile = `${identityFile}.pub`;

  spawnSync("docker", ["rm", "-f", opts.name], { stdio: "ignore" });

  const args = [
    "run",
    "-d",
    "--rm",
    "--name",
    opts.name,
    "--network",
    opts.networkName,
    "--label",
    "kandev.managed=true",
    "--label",
    `kandev.e2e.role=ssh-${opts.role}`,
    "--cap-add",
    "NET_ADMIN",
    "-v",
    `${publicKeyFile}:/authorized_keys:ro`,
  ];
  if (opts.hostPort !== null) {
    args.push("-p", `127.0.0.1:${opts.hostPort}:22`);
  }
  args.push(SSH_E2E_IMAGE_TAG);

  const containerId = execFileSync("docker", args, { encoding: "utf8" }).toString().trim();

  // For the bastion, wait for its published port to open and grab the host
  // fingerprint. For the target, we don't expose anything on the host, so
  // grab the fingerprint from inside the network via a one-shot helper
  // container on the same network.
  let hostFingerprint = "";
  if (opts.hostPort !== null) {
    waitForTCPOpen("127.0.0.1", opts.hostPort);
    hostFingerprint = scanFingerprintViaHost("127.0.0.1", opts.hostPort);
  } else {
    waitForTCPOpenInsideNetwork(opts.networkName, opts.name, 22);
    hostFingerprint = scanFingerprintInsideNetwork(opts.networkName, opts.name);
  }

  return {
    containerId,
    containerName: opts.name,
    host: opts.hostPort !== null ? "127.0.0.1" : opts.name,
    port: opts.hostPort ?? 22,
    user: "kandev",
    identityFile,
    publicKeyFile,
    hostFingerprint,
    workDir: opts.workDir,
  };
}

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
  fs.chmodSync(identityFile, 0o600);
}

function waitForTCPOpen(host: string, port: number, timeoutMs = 15_000): void {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (spawnSync("nc", ["-z", host, String(port)], { stdio: "ignore" }).status === 0) return;
    spawnSync("sleep", ["0.15"]);
  }
  throw new Error(`${host}:${port} did not open within ${timeoutMs}ms`);
}

function waitForTCPOpenInsideNetwork(
  networkName: string,
  targetName: string,
  targetPort: number,
  timeoutMs = 15_000,
): void {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const res = spawnSync(
      "docker",
      [
        "run",
        "--rm",
        "--network",
        networkName,
        "alpine:3.20",
        "sh",
        "-c",
        `nc -z ${targetName} ${targetPort}`,
      ],
      { stdio: "ignore" },
    );
    if (res.status === 0) return;
    spawnSync("sleep", ["0.2"]);
  }
  throw new Error(
    `${targetName}:${targetPort} on ${networkName} did not open within ${timeoutMs}ms`,
  );
}

function scanFingerprintViaHost(host: string, port: number): string {
  // sshd often accepts TCP before it's finished negotiating host keys —
  // ssh-keyscan can come back empty on the first try right after the
  // container starts. Retry a few times before giving up so the bastion
  // fixture isn't flaky on a cold daemon.
  const deadline = Date.now() + 15_000;
  let lastErr = "";
  while (Date.now() < deadline) {
    const keyOut = spawnSync("ssh-keyscan", ["-p", String(port), "-t", "ed25519", host], {
      encoding: "utf8",
      stdio: ["ignore", "pipe", "pipe"],
    });
    if (keyOut.stdout?.trim()) {
      return parseFingerprint(keyOut.stdout);
    }
    lastErr = keyOut.stderr ?? "";
    spawnSync("sleep", ["0.3"]);
  }
  throw new Error(`scanFingerprintViaHost: empty output (stderr=${lastErr})`);
}

function scanFingerprintInsideNetwork(networkName: string, targetName: string): string {
  // openssh-client is the package that ships ssh-keyscan on Alpine. (The
  // historical "openssh-keyscan" name doesn't exist in repos.) Pin to a
  // known-good Alpine and surface install/keyscan stderr so failures are
  // diagnosable instead of empty.
  const res = spawnSync(
    "docker",
    [
      "run",
      "--rm",
      "--network",
      networkName,
      "alpine:3.20",
      "sh",
      "-c",
      `apk add --no-cache openssh-client >/dev/null 2>&1 && ssh-keyscan -t ed25519 ${targetName}`,
    ],
    { encoding: "utf8" },
  );
  if (!res.stdout?.trim()) {
    throw new Error(
      `scanFingerprintInsideNetwork: empty output (status=${res.status}, stderr=${res.stderr})`,
    );
  }
  return parseFingerprint(res.stdout);
}

function parseFingerprint(keyscanOutput: string): string {
  if (!keyscanOutput.trim()) {
    throw new Error("ssh-keyscan produced no output");
  }
  const tmp = path.join(os.tmpdir(), `kandev-e2e-keyscan-${process.pid}-${Date.now()}`);
  fs.writeFileSync(tmp, keyscanOutput);
  try {
    const fp = spawnSync("ssh-keygen", ["-lf", tmp], { encoding: "utf8" });
    const match = fp.stdout.match(/SHA256:\S+/);
    if (!match) throw new Error(`could not parse SHA256 fingerprint from: ${fp.stdout}`);
    return match[0];
  } finally {
    fs.rmSync(tmp, { force: true });
  }
}
