import { test as base } from "@playwright/test";
import { type ChildProcess, spawn } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const BACKEND_DIR = path.resolve(__dirname, "../../../../apps/backend");
const WEB_DIR = path.resolve(__dirname, "../..");
const KANDEV_BIN = path.join(BACKEND_DIR, "bin", "kandev");
const STANDALONE_SERVER = path.join(WEB_DIR, ".next/standalone/web/server.js");
const STANDALONE_STATIC_DIR = path.join(WEB_DIR, ".next/standalone/web/.next/static");
const SOURCE_STATIC_DIR = path.join(WEB_DIR, ".next/static");
// Auto-derive from PID if not explicitly set — prevents port clashes between concurrent test runs
// Modulo 30 keeps agentctl ports under 65535 (30001 + 30*1000 = 60001 max)
const rawPortOffset = process.env.E2E_PORT_OFFSET;
const E2E_PORT_OFFSET = rawPortOffset === undefined ? process.pid % 30 : Number(rawPortOffset);
if (!Number.isInteger(E2E_PORT_OFFSET) || E2E_PORT_OFFSET < 0 || E2E_PORT_OFFSET > 29) {
  throw new Error(`E2E_PORT_OFFSET must be an integer 0-29, got: ${rawPortOffset}`);
}
const BACKEND_BASE_PORT = 18080 + E2E_PORT_OFFSET;
const FRONTEND_BASE_PORT = 13000 + E2E_PORT_OFFSET;
const HEALTH_TIMEOUT_MS = 30_000;
const HEALTH_POLL_MS = 250;

export type BackendContext = {
  port: number;
  baseUrl: string;
  frontendPort: number;
  frontendUrl: string;
  tmpDir: string;
  /**
   * Kill the backend process and respawn with the same config (DB, ports,
   * tmpDir persist). The captured env is rebuilt from the baseline snapshot
   * on every call, so `envOverrides` only apply to this restart — they do
   * NOT leak into a subsequent restart. Call `restart()` with no args to
   * revert to the baseline env. Only in-memory execution state (running
   * agents, WS connections) is lost.
   */
  restart: (envOverrides?: Record<string, string>) => Promise<void>;
};

async function waitForHealth(url: string, timeoutMs: number, proc?: ChildProcess): Promise<void> {
  const deadline = Date.now() + timeoutMs;

  // Track process exit so we can fail fast instead of polling for the full timeout.
  let processExited = false;
  let processExitCode: number | null = null;
  const onExit = (code: number | null) => {
    processExited = true;
    processExitCode = code;
  };
  proc?.once("exit", onExit);

  try {
    while (Date.now() < deadline) {
      if (processExited) {
        throw new Error(
          `Backend process exited with code ${processExitCode} while waiting for health at ${url}`,
        );
      }
      try {
        const res = await fetch(url);
        if (res.ok) return;
      } catch {
        // not ready yet
      }
      await new Promise((r) => setTimeout(r, HEALTH_POLL_MS));
    }
    throw new Error(`Service did not become healthy at ${url} within ${timeoutMs}ms`);
  } finally {
    proc?.off("exit", onExit);
  }
}

function killProcess(proc: ChildProcess): Promise<void> {
  return new Promise<void>((resolve) => {
    proc.kill("SIGTERM");
    const timeout = setTimeout(() => {
      proc.kill("SIGKILL");
      resolve();
    }, 5_000);
    proc.on("exit", () => {
      clearTimeout(timeout);
      resolve();
    });
  });
}

/**
 * Polls until the given TCP port is no longer accepting connections, or until
 * timeoutMs elapses. Used after killProcessGroup to avoid a fixed sleep: the
 * OS may hold the port in TIME_WAIT for up to 60 s under heavy load, and
 * sleeping a fixed 2 s races against that window.
 */
async function waitForPortFree(port: number, timeoutMs = 10_000): Promise<void> {
  const { createConnection } = await import("node:net");
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const free = await new Promise<boolean>((resolve) => {
      const sock = createConnection({ port, host: "127.0.0.1" });
      sock.once("connect", () => {
        sock.destroy();
        resolve(false); // port still occupied
      });
      sock.once("error", () => resolve(true)); // ECONNREFUSED → port is free
    });
    if (free) return;
    await new Promise((r) => setTimeout(r, 100));
  }
  // Timeout expired — proceed anyway; the new process will fail-fast if the
  // port is still held and waitForHealth will surface the error.
}

/**
 * Kills an entire process group. Used for the backend process which is spawned
 * with `detached: true` so it becomes a process group leader. Sending signals
 * to the negative PID targets all processes in that group (backend + agentctl).
 * The 7s grace period gives agentctl time to cascade cleanup to agent process groups.
 */
function killProcessGroup(proc: ChildProcess): Promise<void> {
  return new Promise<void>((resolve) => {
    if (!proc.pid) {
      resolve();
      return;
    }

    const pid = proc.pid;

    try {
      process.kill(-pid, "SIGTERM");
    } catch {
      // Process group may already be gone
      resolve();
      return;
    }

    const timeout = setTimeout(() => {
      try {
        process.kill(-pid, "SIGKILL");
      } catch {
        // Already dead
      }
      resolve();
    }, 7_000);

    proc.on("exit", () => {
      clearTimeout(timeout);
      resolve();
    });
  });
}

/**
 * Spawn a backend process with the given environment. Returns the child process.
 * The process is spawned with `detached: true` so it becomes a process group leader.
 */
function spawnBackendProcess(
  env: Record<string, string>,
  debug: boolean,
  port: number,
): ChildProcess {
  const proc = spawn(KANDEV_BIN, [], {
    env: env as unknown as NodeJS.ProcessEnv,
    stdio: ["ignore", "pipe", "pipe"],
    detached: true,
  });

  const logFile = debug ? fs.createWriteStream(`/tmp/e2e-backend-${port}.log`) : null;
  proc.once("exit", () => {
    logFile?.end();
  });
  proc.stderr?.on("data", (chunk: Buffer) => {
    if (debug) {
      process.stderr.write(`[backend:${port}] ${chunk.toString()}`);
      logFile?.write(chunk);
    }
  });
  proc.stdout?.on("data", (chunk: Buffer) => {
    if (debug) {
      process.stderr.write(`[backend-log:${port}] ${chunk.toString()}`);
      logFile?.write(chunk);
    }
  });

  return proc;
}

/**
 * Worker-scoped fixture that spawns an isolated backend process and
 * a dedicated Next.js frontend. Each Playwright worker gets its own
 * backend on a unique port with an isolated HOME, database, and data
 * directory, plus its own frontend with SSR routed to that backend.
 */
export const backendFixture = base.extend<object, { backend: BackendContext }>({
  backend: [
    async ({}, use, workerInfo) => {
      const backendPort = BACKEND_BASE_PORT + workerInfo.workerIndex;
      const frontendPort = FRONTEND_BASE_PORT + workerInfo.workerIndex;
      const tmpDir = fs.mkdtempSync(
        path.join(os.tmpdir(), `kandev-e2e-${workerInfo.workerIndex}-`),
      );
      const homeDir = path.join(tmpDir, ".kandev");
      const dbPath = path.join(tmpDir, "kandev.db");
      const worktreeBase = path.join(tmpDir, "worktrees");
      const repoCloneBase = path.join(tmpDir, "repos");

      fs.mkdirSync(homeDir, { recursive: true });
      fs.mkdirSync(worktreeBase, { recursive: true });
      fs.mkdirSync(repoCloneBase, { recursive: true });

      // Write a minimal .gitconfig so git doesn't prompt for identity
      // and disable signing to avoid SSH/GPG key lookups in the isolated HOME.
      fs.writeFileSync(
        path.join(tmpDir, ".gitconfig"),
        "[user]\n  name = E2E Test\n  email = e2e@test.local\n[commit]\n  gpgsign = false\n[tag]\n  gpgsign = false\n",
      );

      // Give each worker its own agentctl port range, offset from the default
      // range (41001-41100) to avoid conflicts with a running dev instance.
      // The async cleanup of agent instances runs after each test deletes its
      // tasks, so during a 60+ test shard the in-flight cleanup queue can hold
      // several dozen ports at any given moment. 200 ports per worker keeps
      // headroom for that without overflowing the 65535 port space. With the
      // current playwright config (workers: 1, so workerIndex == 0) and shard
      // offsets capped at 29, the highest port used is 30001 + 29*1000 + 199
      // = 59200. The `workerIndex * 200` term is defensive for the case where
      // a future config sets workers > 1.
      const agentctlPortBase = 30001 + E2E_PORT_OFFSET * 1000 + workerInfo.workerIndex * 200;
      const agentctlPortMax = agentctlPortBase + 199;

      // Install a `git` shim that can sleep on `fetch`/`pull` before execing
      // the real git binary. Tests that need to simulate slow network git
      // operations write a millisecond value to `${tmpDir}/git-delay-ms`; the
      // shim reads it on every invocation and sleeps the matching duration.
      // When the file is absent the shim is a transparent passthrough, so
      // other tests in the same worker are unaffected.
      const shimDir = path.join(tmpDir, "bin");
      const shimPath = path.join(shimDir, "git");
      const shimDelayFile = path.join(tmpDir, "git-delay-ms");
      const originalPath = process.env.PATH ?? "";
      fs.mkdirSync(shimDir, { recursive: true });
      fs.writeFileSync(
        shimPath,
        `#!/bin/sh
if [ -f "$KANDEV_E2E_GIT_DELAY_FILE" ] && { [ "$1" = "fetch" ] || [ "$1" = "pull" ]; }; then
  delay_ms=$(cat "$KANDEV_E2E_GIT_DELAY_FILE" 2>/dev/null)
  case "$delay_ms" in
    ''|*[!0-9]*) ;;
    *)
      if [ "$delay_ms" -gt 0 ]; then
        sleep_secs=$((delay_ms / 1000))
        [ "$sleep_secs" -lt 1 ] && sleep_secs=1
        sleep "$sleep_secs"
      fi
      ;;
  esac
fi
export PATH="$KANDEV_E2E_ORIGINAL_PATH"
exec git "$@"
`,
        { mode: 0o755 },
      );

      // Opt-in: Docker E2E project or KANDEV_E2E_DOCKER=1 enables real
      // container execution. Default is off so the regular suite stays fast
      // and runs without a Docker daemon.
      const dockerEnabled =
        workerInfo.project.name === "docker" || process.env.KANDEV_E2E_DOCKER === "1";
      const mockAgentLinuxBinary = path.join(BACKEND_DIR, "bin", "mock-agent-linux-amd64");
      const agentctlLinuxBinary = path.join(BACKEND_DIR, "bin", "agentctl-linux-amd64");

      const backendEnv = {
        ...stripGitHubTokens(process.env as Record<string, string>),
        // Prepend the kandev bin dir so the host utility probe can locate
        // the `mock-agent` binary via PATH. In production that dir is the
        // same as the running kandev binary's dir, but e2e spawns via an
        // absolute path and doesn't inherit that location.
        PATH: `${shimDir}:${path.join(BACKEND_DIR, "bin")}:${originalPath}`,
        KANDEV_E2E_ORIGINAL_PATH: originalPath,
        KANDEV_E2E_GIT_DELAY_FILE: shimDelayFile,
        HOME: tmpDir,
        KANDEV_HOME_DIR: homeDir,
        KANDEV_SERVER_PORT: String(backendPort),
        KANDEV_DATABASE_PATH: dbPath,
        // Profile selector. KANDEV_E2E_MOCK=true tells the backend to
        // apply the `e2e:` profile from profiles.yaml at startup —
        // which sets KANDEV_MOCK_AGENT, KANDEV_MOCK_GITHUB/JIRA/LINEAR,
        // KANDEV_FEATURES_OFFICE, AGENTCTL_AUTO_APPROVE_PERMISSIONS,
        // KANDEV_PLAN_COALESCE_WINDOW_MS, etc. We don't re-set those
        // here. KANDEV_MOCK_PROVIDERS stays opt-in per-spec because it
        // changes agent counts; the five office-routing-* specs pass
        // it to backend.restart() when needed (see
        // registry.RoutableProviderIDs).
        KANDEV_E2E_MOCK: "true",
        KANDEV_DOCKER_ENABLED: dockerEnabled ? "true" : "false",
        // When Docker is on, point the lifecycle resolvers at the linux/amd64
        // binaries the test runner pre-built, so containers can bind-mount them.
        ...(dockerEnabled
          ? {
              KANDEV_AGENTCTL_LINUX_BINARY: agentctlLinuxBinary,
              KANDEV_MOCK_AGENT_LINUX_BINARY: mockAgentLinuxBinary,
            }
          : {}),
        KANDEV_WORKTREE_ENABLED: "true",
        KANDEV_WORKTREE_BASEPATH: worktreeBase,
        KANDEV_REPOCLONE_BASEPATH: repoCloneBase,
        KANDEV_LOG_LEVEL: process.env.KANDEV_LOG_LEVEL ?? "warn",
        AGENTCTL_INSTANCE_PORT_BASE: String(agentctlPortBase),
        AGENTCTL_INSTANCE_PORT_MAX: String(agentctlPortMax),
        // AGENTCTL_AUTO_APPROVE_PERMISSIONS=true and
        // KANDEV_PLAN_COALESCE_WINDOW_MS=2000 are applied by the
        // backend's profile loader (profiles.yaml `e2e:` column).
        // Specs that need different values (e.g.
        // permission-approval.spec.ts setting auto-approve=false) set
        // process.env.X before spawn — that already flows through the
        // `...stripGitHubTokens(process.env)` spread above, and the
        // backend's ApplyProfile leaves already-set vars alone.
        GIT_AUTHOR_NAME: "E2E Test",
        GIT_AUTHOR_EMAIL: "e2e@test.local",
        GIT_COMMITTER_NAME: "E2E Test",
        GIT_COMMITTER_EMAIL: "e2e@test.local",
      };

      const debug = !!process.env.E2E_DEBUG;
      const baseUrl = `http://localhost:${backendPort}`;

      // Snapshot the baseline env so `restart(envOverrides)` rebuilds from
      // a clean copy each call instead of accumulating leftover keys (e.g.
      // KANDEV_MOCK_PROVIDERS, KANDEV_PROVIDER_FAILURES) from prior tests.
      const baselineEnv = { ...backendEnv } as Record<string, string>;

      // --- Spawn backend ---
      let backendProc = spawnBackendProcess(backendEnv, debug, backendPort);
      await waitForHealth(`${baseUrl}/health`, HEALTH_TIMEOUT_MS);

      // Ensure Next.js static assets are available to the standalone server.
      // The CLI start command does this via symlink; replicate it here.
      // Use try/catch to handle concurrent workers racing to create the same symlink.
      if (fs.existsSync(SOURCE_STATIC_DIR) && !fs.existsSync(STANDALONE_STATIC_DIR)) {
        fs.mkdirSync(path.dirname(STANDALONE_STATIC_DIR), { recursive: true });
        try {
          fs.symlinkSync(SOURCE_STATIC_DIR, STANDALONE_STATIC_DIR, "junction");
        } catch (err: unknown) {
          if ((err as NodeJS.ErrnoException).code !== "EEXIST") throw err;
        }
      }

      // --- Spawn frontend (Next.js standalone server) ---
      const frontendProc: ChildProcess = spawn("node", [STANDALONE_SERVER], {
        cwd: WEB_DIR,
        env: {
          ...(process.env as unknown as Record<string, string>),
          KANDEV_API_BASE_URL: baseUrl,
          NEXT_PUBLIC_KANDEV_API_PORT: String(backendPort),
          PORT: String(frontendPort),
          HOSTNAME: "localhost",
          NODE_ENV: "production",
        } as unknown as NodeJS.ProcessEnv,
        stdio: ["ignore", "pipe", "pipe"],
      });

      frontendProc.stderr?.on("data", (chunk: Buffer) => {
        if (debug) process.stderr.write(`[frontend:${frontendPort}] ${chunk.toString()}`);
      });

      const frontendUrl = `http://localhost:${frontendPort}`;
      await waitForHealth(frontendUrl, HEALTH_TIMEOUT_MS);

      /**
       * Kill the backend process group and respawn with the same config.
       * SQLite DB, tmpDir, and all persisted data survive the restart.
       * Only in-memory execution state (running agents, WS connections) is lost.
       */
      const restart = async (envOverrides?: Record<string, string>) => {
        // Rebuild from the baseline snapshot so a previous restart's
        // overrides don't leak into this one (e.g. KANDEV_MOCK_PROVIDERS
        // set by the routing specs would otherwise stick for the rest of
        // the worker's lifetime and register canonical agent IDs that
        // sibling specs count).
        const nextEnv: Record<string, string> = { ...baselineEnv };
        if (envOverrides) {
          for (const [k, v] of Object.entries(envOverrides)) {
            nextEnv[k] = v;
          }
        }
        await killProcessGroup(backendProc);
        // Poll until the OS releases the TCP port rather than sleeping a fixed
        // 2 s. TIME_WAIT can linger for 30–120 s under load; the probe exits
        // as soon as the port stops accepting connections (typically <200 ms).
        await waitForPortFree(backendPort);
        backendProc = spawnBackendProcess(nextEnv, debug, backendPort);
        // Pass the process so waitForHealth fails fast if it exits (e.g. port still in use)
        await waitForHealth(`${baseUrl}/health`, HEALTH_TIMEOUT_MS, backendProc);
      };

      try {
        await use({ port: backendPort, baseUrl, frontendPort, frontendUrl, tmpDir, restart });
      } finally {
        // Shutdown frontend first (simple process), then backend (process group)
        await killProcess(frontendProc);
        await killProcessGroup(backendProc);

        // Cleanup temp directory — ignore errors (backend may still hold files briefly)
        try {
          fs.rmSync(tmpDir, { recursive: true, force: true });
        } catch {
          // Non-fatal: OS may not have released all file handles yet
        }
      }
    },
    { scope: "worker", timeout: 60_000 },
  ],
});

/** Strip GH_TOKEN / GITHUB_TOKEN so the mock client is used. */
function stripGitHubTokens(env: Record<string, string>): Record<string, string> {
  const cleaned = { ...env };
  delete cleaned.GH_TOKEN;
  delete cleaned.GITHUB_TOKEN;
  return cleaned;
}
