import { spawnSync } from "node:child_process";
import fs from "node:fs";
import net from "node:net";
import os from "node:os";
import path from "node:path";
import { once } from "node:events";

import { ApiClient } from "../helpers/api-client";
import { dwell } from "../helpers/causal-waits";

const REPO_ROOT = path.resolve(__dirname, "../../../..");
const HOST_BACKEND_BIN = path.join(REPO_ROOT, "apps/backend/bin/kandev");
const LINUX_BACKEND_BIN = path.join(REPO_ROOT, "apps/backend/bin/kandev-linux-amd64");
const AGENTCTL_BIN = path.join(REPO_ROOT, "apps/backend/bin/agentctl-linux-amd64");
const MOCK_AGENT_BIN = path.join(REPO_ROOT, "apps/backend/bin/mock-agent-linux-amd64");
const WEB_DIST = path.join(REPO_ROOT, "apps/web/dist");

function backendBinary(): string {
  const override = process.env.KANDEV_E2E_BACKEND_BIN?.trim();
  if (override) {
    return override;
  }
  if (process.platform === "linux") {
    return HOST_BACKEND_BIN;
  }
  // The managed host build is a native binary on macOS. The disposable
  // backend runs in Debian, so require a linux/amd64 build explicitly.
  return LINUX_BACKEND_BIN;
}

export type ContainerizedBackend = {
  baseUrl: string;
  api: ApiClient;
  backendContainerID: string;
  dbPath: string;
  logPath: string;
  homeDir: string;
  docker: (args: string[], allowFailure?: boolean) => string;
  dockerStatus: (args: string[]) => number;
  dockerInspectPID: (containerID: string) => number;
  pidNamespace: (pid: number) => string;
  standaloneWorkerPIDs: () => string[];
  sqlite: (sql: string) => string;
  stop: () => Promise<void>;
};

async function freePort(): Promise<number> {
  const server = net.createServer();
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  const port = (server.address() as net.AddressInfo).port;
  await new Promise<void>((resolve, reject) =>
    server.close((error) => (error ? reject(error) : resolve())),
  );
  return port;
}

async function waitFor(
  description: string,
  timeoutMs: number,
  check: () => boolean | Promise<boolean>,
): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  let lastError: unknown;
  while (Date.now() < deadline) {
    try {
      if (await check()) return;
    } catch (error) {
      lastError = error;
    }
    await dwell(250, "poll-interval", "waiting for the disposable containerized backend topology");
  }
  throw new Error(`${description} timed out${lastError ? `: ${String(lastError)}` : ""}`);
}

function run(command: string, args: string[]): string {
  const result = spawnSync(command, args, { encoding: "utf8" });
  if (result.status !== 0) {
    throw new Error(`${command} ${args.join(" ")} failed: ${result.stderr || result.stdout}`);
  }
  return result.stdout.trim();
}

/**
 * Starts a disposable Docker-in-Docker container and a Kandev backend
 * container inside it. The outer container owns its own network namespace, so
 * this never creates, reuses, or removes the host's docker0 bridge. The
 * backend and every worker are distinct containers on the inner daemon.
 */
export async function startContainerizedBackend(): Promise<ContainerizedBackend> {
  const backendBin = backendBinary();
  for (const [label, file] of [
    ["linux kandev backend", backendBin],
    ["agentctl helper", AGENTCTL_BIN],
    ["mock agent", MOCK_AGENT_BIN],
    ["web assets", path.join(WEB_DIST, "index.html")],
  ] as const) {
    if (!fs.existsSync(file)) {
      throw new Error(
        `${label} is missing at ${file}; run the managed E2E build first ` +
          "(non-Linux hosts also need `make -C apps/backend build-kandev-linux`)",
      );
    }
  }

  const root = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-containerized-backend-"));
  const homeDir = path.join(root, "home");
  const tempRoot = path.join(root, "temp");
  const worktreeRoot = path.join(tempRoot, "worktrees");
  const repoCloneRoot = path.join(tempRoot, "repos");
  const systemTempRoot = path.join(tempRoot, "system-temporary");
  for (const dir of [homeDir, tempRoot, worktreeRoot, repoCloneRoot, systemTempRoot]) {
    fs.mkdirSync(dir, { recursive: true });
  }

  const backendPort = await freePort();
  const dindAPIPort = await freePort();
  const scope = `cb-${Date.now()}`;
  const dindName = `kandev-cb-dind-${scope}`;
  const backendName = `kandev-cb-backend-${scope}`;
  const dindURL = `tcp://127.0.0.1:${dindAPIPort}`;

  const hostDocker = (args: string[], allowFailure = false): string => {
    const result = spawnSync("docker", args, { encoding: "utf8", timeout: 180_000 });
    if (result.status !== 0 && !allowFailure) {
      throw new Error(`host docker ${args.join(" ")} failed: ${result.stderr || result.stdout}`);
    }
    return result.stdout.trim();
  };

  const docker = (args: string[], allowFailure = false): string => {
    const result = spawnSync("docker", ["-H", dindURL, ...args], {
      encoding: "utf8",
      timeout: 180_000,
    });
    if (result.status !== 0 && !allowFailure) {
      throw new Error(`inner docker ${args.join(" ")} failed: ${result.stderr || result.stdout}`);
    }
    return result.stdout.trim();
  };

  const stop = async (): Promise<void> => {
    // Remove the dind container's anonymous daemon-storage volume too, so
    // repeated runs on a persistent Docker host do not leak images/layers.
    hostDocker(["rm", "-f", "-v", dindName], true);
    try {
      fs.rmSync(root, { recursive: true, force: true, maxRetries: 10, retryDelay: 200 });
    } catch (error) {
      console.warn(`containerized backend cleanup left ${root}: ${String(error)}`);
    }
  };

  try {
    hostDocker(["pull", "-q", "docker:26-dind"]);
    // The outer dind container publishes only the backend API and its own
    // loopback daemon control port. Workers never bind a host port.
    hostDocker([
      "run",
      "-d",
      "--privileged",
      "--name",
      dindName,
      "-e",
      "DOCKER_TLS_CERTDIR=",
      "-p",
      `127.0.0.1:${dindAPIPort}:2375`,
      "-p",
      `127.0.0.1:${backendPort}:${backendPort}`,
      "-v",
      `${REPO_ROOT}:${REPO_ROOT}:ro`,
      "-v",
      `${homeDir}:${homeDir}`,
      "-v",
      `${tempRoot}:${tempRoot}`,
      "docker:26-dind",
    ]);

    await waitFor("disposable Docker daemon startup", 60_000, () => {
      const result = spawnSync("docker", ["-H", dindURL, "info"], {
        encoding: "utf8",
      });
      return result.status === 0;
    });
    docker(["pull", "-q", "debian:bookworm-slim"]);
    docker(["pull", "-q", "alpine:3.20"]);
    // sqlite3 is used only to seed/assert the disposable database. It runs
    // inside the dind container so it sees exactly the backend's filesystem.
    hostDocker(["exec", dindName, "apk", "add", "--no-cache", "sqlite"]);

    const env = [
      `HOME=${homeDir}`,
      `KANDEV_HOME_DIR=${homeDir}`,
      `KANDEV_SERVER_PORT=${backendPort}`,
      `KANDEV_WEB_DIST_DIR=${WEB_DIST}`,
      `KANDEV_DATABASE_PATH=${path.join(homeDir, "kandev.db")}`,
      "KANDEV_E2E_MOCK=true",
      "KANDEV_DOCKER_ENABLED=true",
      "KANDEV_DOCKER_HOST=tcp://127.0.0.1:2375",
      `KANDEV_AGENTCTL_LINUX_BINARY=${AGENTCTL_BIN}`,
      `KANDEV_MOCK_AGENT_LINUX_BINARY=${MOCK_AGENT_BIN}`,
      `KANDEV_E2E_DOCKER_SCOPE=${scope}`,
      "KANDEV_WORKTREE_ENABLED=true",
      `KANDEV_WORKTREE_BASEPATH=${worktreeRoot}`,
      `KANDEV_REPOCLONE_BASEPATH=${repoCloneRoot}`,
      `KANDEV_E2E_SYSTEM_TEMP_ROOT=${systemTempRoot}`,
      `PATH=${path.join(REPO_ROOT, "apps/backend/bin")}:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`,
      "KANDEV_LOG_LEVEL=debug",
      "GIT_AUTHOR_NAME=E2E Test",
      "GIT_AUTHOR_EMAIL=e2e@test.local",
      "GIT_COMMITTER_NAME=E2E Test",
      "GIT_COMMITTER_EMAIL=e2e@test.local",
    ].flatMap((value) => ["-e", value]);

    const backendContainerID = docker([
      "run",
      "-d",
      "--name",
      backendName,
      "--network",
      "host",
      "-v",
      `${REPO_ROOT}:${REPO_ROOT}:ro`,
      "-v",
      `${homeDir}:${homeDir}`,
      "-v",
      `${tempRoot}:${tempRoot}`,
      ...env,
      "debian:bookworm-slim",
      backendBin,
      "__backend",
    ]);

    const baseUrl = `http://127.0.0.1:${backendPort}`;
    const logPath = path.join(homeDir, "logs", "backend-logs.log");
    try {
      await waitFor("containerized backend readiness", 90_000, async () => {
        try {
          const res = await fetch(`${baseUrl}/ready`);
          return res.ok;
        } catch {
          return false;
        }
      });
    } catch (error) {
      const logs = docker(["logs", backendContainerID], true);
      throw new Error(`${String(error)}\nbackend logs:\n${logs}`);
    }

    return {
      baseUrl,
      api: new ApiClient(baseUrl),
      backendContainerID,
      dbPath: path.join(homeDir, "kandev.db"),
      logPath,
      homeDir,
      docker,
      dockerStatus: (args: string[]) =>
        spawnSync("docker", ["-H", dindURL, ...args], { encoding: "utf8", timeout: 120_000 })
          .status ?? 1,
      dockerInspectPID: (containerID: string) =>
        Number(docker(["inspect", "-f", "{{.State.Pid}}", containerID])),
      pidNamespace: (pid: number) =>
        run("docker", ["exec", dindName, "readlink", `/proc/${pid}/ns/pid`]),
      standaloneWorkerPIDs: () => {
        const script = [
          "for entry in /proc/[0-9]*; do",
          '  [ "$entry" = "/proc/$$" ] && continue',
          '  if tr "\\0" " " < "$entry/cmdline" 2>/dev/null | grep -q "mock-agent"; then',
          '    echo "${entry#/proc/}"',
          "  fi",
          "done",
        ].join("\n");
        return docker(["exec", backendContainerID, "sh", "-c", script])
          .split("\n")
          .map((pid) => pid.trim())
          .filter(Boolean);
      },
      sqlite: (sql: string) =>
        hostDocker(["exec", dindName, "sqlite3", path.join(homeDir, "kandev.db"), sql]),
      stop,
    };
  } catch (error) {
    await stop();
    throw error;
  }
}
