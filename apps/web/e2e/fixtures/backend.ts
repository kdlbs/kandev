import { test as base } from "@playwright/test";
import { type ChildProcess, execFile, spawn } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { BackendFixtureEnvOverrides, createScopedEnvUse } from "./backend-env";
import { E2E_DOCKER_SCOPE } from "./docker-probe";
import { dwell } from "../helpers/causal-waits";

const BACKEND_DIR = path.resolve(__dirname, "../../../../apps/backend");
const WEB_DIR = path.resolve(__dirname, "../..");
// Lets a local or CI run exercise a freshly built backend without replacing
// the default artifact that `make build` provides for ordinary E2E runs.
const KANDEV_BIN = process.env.KANDEV_E2E_BIN || path.join(BACKEND_DIR, "bin", "kandev");
const WEB_DIST_DIR = path.join(WEB_DIR, "dist");
// Auto-derive from PID if not explicitly set — prevents port clashes between concurrent test runs
// Modulo 30 keeps agentctl ports under 65535 (30001 + 30*1000 = 60001 max)
const rawPortOffset = process.env.E2E_PORT_OFFSET;
const E2E_PORT_OFFSET = rawPortOffset === undefined ? process.pid % 30 : Number(rawPortOffset);
if (!Number.isInteger(E2E_PORT_OFFSET) || E2E_PORT_OFFSET < 0 || E2E_PORT_OFFSET > 29) {
  throw new Error(`E2E_PORT_OFFSET must be an integer 0-29, got: ${rawPortOffset}`);
}
const BACKEND_BASE_PORT = 18080 + E2E_PORT_OFFSET;
const HEALTH_TIMEOUT_MS = 30_000;
const HEALTH_POLL_MS = 250;

/**
 * Returns true when the current run is the heavyweight container-backed
 * Playwright project (Docker executor + SSH executor tests live here). The
 * project was renamed `docker` → `containers` when SSH e2e tests joined it;
 * the legacy name + env var are honored as deprecated aliases for one
 * release. See apps/web/e2e/README.md.
 */
function isContainerProjectActive(projectName: string): boolean {
  if (projectName === "containers" || projectName === "docker") return true;
  if (process.env.KANDEV_E2E_CONTAINERS === "1") return true;
  if (process.env.KANDEV_E2E_DOCKER === "1") return true;
  return false;
}

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
  /**
   * Verify the worker backend is serving requests and recover it when a
   * previous test left the process unavailable.
   */
  ensureReady: () => Promise<void>;
  /**
   * Applies test-owned process environment values to the current backend and
   * every later restart until the returned release callback is awaited.
   */
  useEnv: (overrides: Record<string, string>) => Promise<() => Promise<void>>;
};

function observeProcessExit(proc?: ChildProcess): {
  state: { exited: boolean; exitCode: number | null; error?: Error };
  dispose: () => void;
} {
  const state: { exited: boolean; exitCode: number | null; error?: Error } = {
    exited: proc?.exitCode !== null && proc?.exitCode !== undefined,
    exitCode: proc?.exitCode ?? null,
  };
  const onExit = (code: number | null) => {
    state.exited = true;
    state.exitCode = code;
  };
  const onError = (error: Error) => {
    state.error = error;
  };
  proc?.once("exit", onExit);
  proc?.once("error", onError);

  // Close the gap between inspecting exitCode and subscribing to the event.
  // Node records exitCode before emitting `exit`, so this second read catches
  // a child that exited between those two operations.
  if (proc?.exitCode !== null && proc?.exitCode !== undefined) {
    state.exited = true;
    state.exitCode = proc.exitCode;
  }

  return {
    state,
    dispose: () => {
      proc?.off("exit", onExit);
      proc?.off("error", onError);
    },
  };
}

export async function waitForHealth(
  url: string,
  timeoutMs: number,
  proc?: ChildProcess,
): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  const processExit = observeProcessExit(proc);

  try {
    while (Date.now() < deadline) {
      if (processExit.state.error) {
        throw processExit.state.error;
      }
      if (processExit.state.exited) {
        throw new Error(
          `Backend process exited with code ${processExit.state.exitCode} while waiting for health at ${url}`,
        );
      }
      try {
        const res = await fetch(url);
        if (res.ok) return;
      } catch {
        // not ready yet
      }
      await dwell(
        HEALTH_POLL_MS,
        "poll-interval",
        "sampling interval for the backend health probe; the process is still starting, so there is nothing to subscribe to and the only signal is the port answering",
      );
    }
    throw new Error(`Service did not become healthy at ${url} within ${timeoutMs}ms`);
  } finally {
    processExit.dispose();
  }
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
    await dwell(
      100,
      "poll-interval",
      "sampling interval while waiting for the previous backend to release its port; the OS publishes nothing when a socket is finally freed",
    );
  }
  // Timeout expired — proceed anyway; the new process will fail-fast if the
  // port is still held and waitForHealth will surface the error.
}

type WindowsTreeKiller = (pid: number, done: (error?: Error) => void) => void;
type ProcessAliveProbe = (pid: number) => boolean;

const taskkillProcessTree: WindowsTreeKiller = (pid, done) => {
  execFile("taskkill", ["/PID", String(pid), "/T", "/F"], (error) => done(error ?? undefined));
};

const isProcessAlive: ProcessAliveProbe = (pid) => {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
};

/**
 * Kills the backend and every child process it owns. POSIX uses the detached
 * process group; Windows needs taskkill because negative-PID signals are not
 * supported there.
 */
export function killProcessGroup(
  proc: ChildProcess,
  platform: NodeJS.Platform = process.platform,
  killWindowsTree: WindowsTreeKiller = taskkillProcessTree,
  processIsAlive: ProcessAliveProbe = isProcessAlive,
): Promise<void> {
  return new Promise<void>((resolve, reject) => {
    if (!proc.pid) {
      resolve();
      return;
    }

    const pid = proc.pid;

    if (platform === "win32") {
      killWindowsTree(pid, (error) => {
        if (error && processIsAlive(pid)) {
          reject(error);
          return;
        }
        resolve();
      });
      return;
    }

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

type BackendFixtureLifecycle = {
  stopProcess?: (proc: ChildProcess) => Promise<void>;
  removeTempRoot?: (tmpDir: string) => void;
};

function removeOwnedTempRoot(tmpDir: string): void {
  fs.rmSync(tmpDir, {
    recursive: true,
    force: true,
    maxRetries: 3,
    retryDelay: 100,
  });
}

export async function runOwnedBackendFixture<T>(
  tmpDir: string,
  run: (registerProcess: (proc: ChildProcess) => void) => Promise<T>,
  lifecycle: BackendFixtureLifecycle = {},
): Promise<T> {
  const stopProcess = lifecycle.stopProcess ?? killProcessGroup;
  const removeTempRoot = lifecycle.removeTempRoot ?? removeOwnedTempRoot;
  let backendProc: ChildProcess | undefined;
  let result: T | undefined;
  const failures: unknown[] = [];

  try {
    result = await run((proc) => {
      backendProc = proc;
    });
  } catch (error) {
    failures.push(error);
  }

  if (backendProc) {
    try {
      await stopProcess(backendProc);
    } catch (error) {
      failures.push(error);
    }
  }

  try {
    removeTempRoot(tmpDir);
  } catch (error) {
    failures.push(new Error(`Failed to remove E2E temporary root ${tmpDir}`, { cause: error }));
  }

  if (failures.length === 1) throw failures[0];
  if (failures.length > 1) {
    throw new AggregateError(failures, "Backend fixture failed and cleanup did not complete");
  }

  return result as T;
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
  const proc = spawn(KANDEV_BIN, ["__backend"], {
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
 * a Go-served SPA frontend. Each Playwright worker gets its own
 * backend on a unique port with an isolated HOME, database, and data
 * directory. Browser traffic hits that same backend, which serves the
 * Vite assets and route-aware boot payload.
 */
export const backendFixture = base.extend<object, { backend: BackendContext }>({
  backend: [
    async ({ browserName: _browserName }, use, workerInfo) => {
      const backendPort = BACKEND_BASE_PORT + workerInfo.workerIndex;
      const frontendPort = backendPort;
      const tmpDir = fs.mkdtempSync(
        path.join(os.tmpdir(), `kandev-e2e-${workerInfo.workerIndex}-`),
      );
      let backendProc: ChildProcess | undefined;

      await runOwnedBackendFixture(tmpDir, async (registerProcess) => {
        const homeDir = path.join(tmpDir, ".kandev");
        const dbPath = path.join(tmpDir, "kandev.db");
        const worktreeBase = path.join(tmpDir, "worktrees");
        const repoCloneBase = path.join(tmpDir, "managed-repos");

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
        //
        // The shim logic lives in `git-shim.mjs` (Node) so it behaves the same
        // on macOS, Linux, and Windows and never depends on the developer's
        // login shell (bash/zsh/fish) or a POSIX `sh`. Only a tiny launcher is
        // generated here to hand control to Node: an extensionless `git`
        // shebang file on POSIX, and a `git.cmd` on Windows (extensionless
        // files aren't executable via PATHEXT there).
        const shimDir = path.join(tmpDir, "bin");
        const shimScript = path.join(__dirname, "git-shim.mjs");
        const shimDelayFile = path.join(tmpDir, "git-delay-ms");
        const shimGitLabPushFile = path.join(tmpDir, "gitlab-push-remote");
        const shimGitLabPushRecordFile = path.join(tmpDir, "gitlab-push-record");
        const originalPath = process.env.PATH ?? "";
        fs.mkdirSync(shimDir, { recursive: true });
        writeGitShimLauncher(shimDir, shimScript);

        // Opt-in: Docker E2E project or KANDEV_E2E_DOCKER=1 enables real
        // container execution. Default is off so the regular suite stays fast
        // and runs without a Docker daemon. See e2e/README.md.
        const dockerEnabled = isContainerProjectActive(workerInfo.project.name);
        const mockAgentLinuxBinary = path.join(BACKEND_DIR, "bin", "mock-agent-linux-amd64");
        const agentctlLinuxBinary = path.join(BACKEND_DIR, "bin", "agentctl-linux-amd64");

        const backendEnv = {
          ...sanitizeInheritedEnv(process.env as Record<string, string>),
          // Prepend the kandev bin dir so the host utility probe can locate
          // the `mock-agent` binary via PATH. In production that dir is the
          // same as the running kandev binary's dir, but e2e spawns via an
          // absolute path and doesn't inherit that location.
          PATH: [shimDir, path.join(BACKEND_DIR, "bin"), originalPath]
            .filter(Boolean)
            .join(path.delimiter),
          KANDEV_E2E_ORIGINAL_PATH: originalPath,
          KANDEV_E2E_GIT_DELAY_FILE: shimDelayFile,
          KANDEV_E2E_GITLAB_PUSH_FILE: shimGitLabPushFile,
          KANDEV_E2E_GITLAB_PUSH_RECORD_FILE: shimGitLabPushRecordFile,
          KANDEV_E2E_GITLAB_REMOTE_URL: `http://localhost:${backendPort}/platform/kandev.git`,
          HOME: tmpDir,
          KANDEV_HOME_DIR: homeDir,
          KANDEV_SERVER_PORT: String(backendPort),
          KANDEV_WEB_DIST_DIR: WEB_DIST_DIR,
          KANDEV_DATABASE_PATH: dbPath,
          // Profile selector. KANDEV_E2E_MOCK=true tells the backend to
          // apply the `e2e:` profile from profiles.yaml at startup —
          // which sets the mock agent and third-party provider flags,
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
                KANDEV_E2E_DOCKER_SCOPE: E2E_DOCKER_SCOPE,
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
          // `...sanitizeInheritedEnv(process.env)` spread above, and the
          // backend's ApplyProfile leaves already-set vars alone. (Note:
          // KANDEV_FEATURES_* and KANDEV_WEB_TITLE_PREFIX are exceptions —
          // they are stripped from the inherited env so the e2e profile
          // always controls the baseline.)
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
        const scopedEnv = new BackendFixtureEnvOverrides();

        // --- Spawn backend ---
        backendProc = spawnBackendProcess(scopedEnv.apply(baselineEnv), debug, backendPort);
        registerProcess(backendProc);
        // /ready (not /health) — /health flips green as soon as the listener
        // is bound, before routes are wired; tests that immediately issue API
        // requests need the readiness contract instead.
        await waitForHealth(`${baseUrl}/ready`, HEALTH_TIMEOUT_MS, backendProc);
        const frontendUrl = baseUrl;

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
          const nextEnv = scopedEnv.apply(baselineEnv, envOverrides);
          const runningProcess = backendProc;
          if (!runningProcess) throw new Error("Backend process is not running");
          await killProcessGroup(runningProcess);
          // Poll until the OS releases the TCP port rather than sleeping a fixed
          // 2 s. TIME_WAIT can linger for 30–120 s under load; the probe exits
          // as soon as the port stops accepting connections (typically <200 ms).
          await waitForPortFree(backendPort);
          backendProc = spawnBackendProcess(nextEnv, debug, backendPort);
          registerProcess(backendProc);
          // Pass the process so waitForHealth fails fast if it exits (e.g. port still in use).
          // /ready, not /health — see the comment on the initial spawn above.
          await waitForHealth(`${baseUrl}/ready`, HEALTH_TIMEOUT_MS, backendProc);
        };

        let recovery: Promise<void> | null = null;
        const ensureReady = async () => {
          try {
            await waitForHealth(`${baseUrl}/ready`, 5_000, backendProc);
            return;
          } catch {
            // A worker can outlive a backend process that a prior test left
            // stopped. Restart the isolated fixture before the next page is
            // created so its setup requests do not hit a refused port.
            recovery ??= restart().finally(() => {
              recovery = null;
            });
            await recovery;
          }
        };

        const useEnv = createScopedEnvUse(scopedEnv, restart);

        await use({
          port: backendPort,
          baseUrl,
          frontendPort,
          frontendUrl,
          tmpDir,
          restart,
          ensureReady,
          useEnv,
        });
      });
    },
    { scope: "worker", timeout: 60_000 },
  ],
});

/**
 * Write a small launcher named `git` (POSIX) or `git.cmd` (Windows) into
 * `shimDir` that hands control to the Node `git-shim.mjs`. Kept minimal and
 * platform-specific because the interesting logic lives in the .mjs; this only
 * bridges a PATH `git` lookup to `node git-shim.mjs "$@"`. `process.execPath`
 * is the Node binary already running the E2E suite, so no separate Node install
 * is assumed on PATH.
 */
function writeGitShimLauncher(shimDir: string, shimScript: string): void {
  const node = process.execPath;
  if (process.platform === "win32") {
    // %* forwards all args verbatim; extensionless files aren't executable via
    // PATHEXT on Windows, so a .cmd wrapper is required for exec.Command("git").
    const launcher = `@echo off\r\n"${node}" "${shimScript}" %*\r\n`;
    fs.writeFileSync(path.join(shimDir, "git.cmd"), launcher);
    return;
  }
  // POSIX: an extensionless `git` shebang launcher. `#!/bin/sh` is only the
  // launcher interpreter (guaranteed present on macOS/Linux) — the shim body is
  // Node, so the developer's login shell (bash/zsh/fish) is irrelevant.
  const launcher = `#!/bin/sh\nexec "${node}" "${shimScript}" "$@"\n`;
  fs.writeFileSync(path.join(shimDir, "git"), launcher, { mode: 0o755 });
}

/** Strip GH_TOKEN / GITHUB_TOKEN so the mock client is used. */
// Sanitize the inherited environment before handing it to the e2e backend.
// Three classes of vars must not leak through the `...process.env` spread:
//   - GitHub tokens — tests must hit the mock GitHub, never a real token.
//   - KANDEV_FEATURES_* flags — these are profile-managed (profiles.yaml `e2e:`
//     column turns them on). When the suite is launched from inside a kandev
//     task, the parent process exports KANDEV_FEATURES_OFFICE=false; left in
//     place it survives the spread and, because the backend's ApplyProfile
//     leaves already-set vars alone, disables Office (and any future feature)
//     in the test backend → /api/v1/office/* 404s. Dropping the whole
//     KANDEV_FEATURES_* namespace lets the e2e profile govern feature flags so
//     the suite always exercises them, regardless of where it's launched.
//   - KANDEV_WEB_TITLE_PREFIX — the browser identity is profile-managed in
//     this suite. Explicit per-test values are applied after this baseline is
//     sanitized through `backend.restart(overrides)`.
//   - PATH casing aliases — Windows commonly inherits `Path`; retaining it
//     beside the fixture's new `PATH` makes child-process lookup order
//     ambiguous. The caller restores one canonical PATH after sanitizing.
function sanitizeInheritedEnv(env: Record<string, string>): Record<string, string> {
  const cleaned = { ...env };
  delete cleaned.GH_TOKEN;
  delete cleaned.GITHUB_TOKEN;
  for (const key of Object.keys(cleaned)) {
    if (key === "KANDEV_WEB_TITLE_PREFIX" || key.startsWith("KANDEV_FEATURES_")) {
      delete cleaned[key];
    }
    if (key.toUpperCase() === "PATH") delete cleaned[key];
  }
  return cleaned;
}
