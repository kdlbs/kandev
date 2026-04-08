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
const BACKEND_BASE_PORT = 18080;
const FRONTEND_BASE_PORT = 13000;
const HEALTH_TIMEOUT_MS = 30_000;
const HEALTH_POLL_MS = 250;

export type BackendContext = {
  port: number;
  baseUrl: string;
  frontendPort: number;
  frontendUrl: string;
  tmpDir: string;
  /** Kill the backend process and respawn with the same config (DB, ports, tmpDir persist). */
  restart: () => Promise<void>;
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
      const dataDir = path.join(tmpDir, ".kandev");
      const dbPath = path.join(tmpDir, "kandev.db");
      const worktreeBase = path.join(tmpDir, "worktrees");
      const repoCloneBase = path.join(tmpDir, "repos");

      fs.mkdirSync(dataDir, { recursive: true });
      fs.mkdirSync(worktreeBase, { recursive: true });
      fs.mkdirSync(repoCloneBase, { recursive: true });

      // Write a minimal .gitconfig so git doesn't prompt for identity
      // and disable signing to avoid SSH/GPG key lookups in the isolated HOME.
      fs.writeFileSync(
        path.join(tmpDir, ".gitconfig"),
        "[user]\n  name = E2E Test\n  email = e2e@test.local\n[commit]\n  gpgsign = false\n[tag]\n  gpgsign = false\n",
      );

      // Give each worker its own agentctl port range, offset from the default
      // range (10001-10100) to avoid conflicts with a running dev instance.
      const agentctlPortBase = 30001 + workerInfo.workerIndex * 50;
      const agentctlPortMax = agentctlPortBase + 49;

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

      const backendEnv = {
        ...stripGitHubTokens(process.env as Record<string, string>),
        PATH: `${shimDir}:${originalPath}`,
        KANDEV_E2E_ORIGINAL_PATH: originalPath,
        KANDEV_E2E_GIT_DELAY_FILE: shimDelayFile,
        HOME: tmpDir,
        KANDEV_DATA_DIR: dataDir,
        KANDEV_SERVER_PORT: String(backendPort),
        KANDEV_DATABASE_PATH: dbPath,
        KANDEV_MOCK_AGENT: "only",
        KANDEV_MOCK_GITHUB: "true",
        KANDEV_DOCKER_ENABLED: "false",
        KANDEV_WORKTREE_ENABLED: "true",
        KANDEV_WORKTREE_BASEPATH: worktreeBase,
        KANDEV_REPOCLONE_BASEPATH: repoCloneBase,
        KANDEV_LOG_LEVEL: process.env.KANDEV_LOG_LEVEL ?? "warn",
        AGENTCTL_INSTANCE_PORT_BASE: String(agentctlPortBase),
        AGENTCTL_INSTANCE_PORT_MAX: String(agentctlPortMax),
        GIT_AUTHOR_NAME: "E2E Test",
        GIT_AUTHOR_EMAIL: "e2e@test.local",
        GIT_COMMITTER_NAME: "E2E Test",
        GIT_COMMITTER_EMAIL: "e2e@test.local",
      };

      const debug = !!process.env.E2E_DEBUG;
      const baseUrl = `http://localhost:${backendPort}`;

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
      const restart = async () => {
        await killProcessGroup(backendProc);
        // Wait for OS to release the TCP port — Linux TIME_WAIT can exceed 500ms under load
        await new Promise((r) => setTimeout(r, 2_000));
        backendProc = spawnBackendProcess(backendEnv, debug, backendPort);
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
