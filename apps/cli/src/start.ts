/**
 * Production start command for running local builds.
 *
 * This module implements the `kandev start` command, which runs the locally
 * built backend binary in production mode. Unlike `kandev dev`
 * which uses hot-reloading, this runs the optimized production builds.
 *
 * Prerequisites:
 * - Backend must be built: `make build-backend`
 * - Or simply: `make build`
 */

import fs from "node:fs";
import path from "node:path";

import {
  HEALTH_TIMEOUT_MS_RELEASE,
  resolveDataDir,
  resolveDatabasePath,
  resolveKandevHomeDir,
} from "./constants";
import { resolveHealthTimeoutMs, waitForHealth } from "./health";
import { getBinaryName } from "./platform";
import { createProcessSupervisor } from "./process";
import { buildBackendEnv, logStartupInfo, pickPorts } from "./shared";
import { launchRestartableBackend } from "./supervisor/backend";
import { openBrowser } from "./web";

export type StartOptions = {
  /** Path to the repository root directory */
  repoRoot: string;
  /** Optional preferred backend port (finds available port if not specified) */
  backendPort?: number;
  /** Optional preferred web port (finds available port if not specified) */
  webPort?: number;
  /** Show info logs from backend + web */
  verbose?: boolean;
  /** Show debug logs + agent message dumps */
  debug?: boolean;
  /** Skip browser open. Set by service units and preview environments. */
  headless?: boolean;
};

/**
 * Runs the application in production mode using local builds.
 *
 * This function:
 * 1. Validates that build artifacts exist
 * 2. Picks available ports for all services
 * 3. Starts the backend binary (with warn log level for clean output)
 * 4. Waits for the backend to be healthy before announcing readiness
 *
 * @param options - Configuration for the start command
 * @throws Error if backend binary is not found
 */
export async function runStart({
  repoRoot,
  backendPort,
  webPort,
  verbose = false,
  debug = false,
  headless = false,
}: StartOptions): Promise<void> {
  const ports = await pickPorts(backendPort, webPort);

  const backendBin = path.join(repoRoot, "apps", "backend", "bin", getBinaryName("kandev"));
  if (!fs.existsSync(backendBin)) {
    throw new Error("Backend binary not found. Run `make build` first.");
  }

  // Production mode: use warn log level for clean output unless verbose/debug
  const showOutput = verbose || debug;
  fs.mkdirSync(resolveDataDir(), { recursive: true });
  // The data dir holds the SQLite DB; keep it owner-only even if it pre-existed
  // with a looser umask-derived mode.
  fs.chmodSync(resolveDataDir(), 0o700);
  const dbPath = resolveDatabasePath();
  const logLevel =
    process.env.KANDEV_LOG_LEVEL?.trim() || (debug ? "debug" : verbose ? "info" : "warn");
  const backendEnv = buildBackendEnv({
    ports,
    logLevel,
    webProxy: false,
    extra: {
      KANDEV_DATABASE_PATH: dbPath,
      ...(debug ? { KANDEV_DEBUG_AGENT_MESSAGES: "true", KANDEV_DEBUG_PPROF_ENABLED: "true" } : {}),
    },
  });

  logStartupInfo({
    header: "start mode: using local build",
    ports,
    dbPath,
    logLevel,
  });

  const supervisor = createProcessSupervisor();
  supervisor.attachSignalHandlers();

  // Start backend: ignore stdin, show stdout only in verbose/debug mode, always show stderr
  // Stderr is always inherited to ensure error messages are visible immediately (no pipe buffering)
  const backend = await launchRestartableBackend({
    command: backendBin,
    args: [],
    cwd: path.dirname(backendBin),
    env: backendEnv,
    homeDir: resolveKandevHomeDir(),
    ports,
    mode: "start",
    stdio: showOutput ? ["ignore", "inherit", "inherit"] : ["ignore", "ignore", "inherit"],
    supervisor,
  });

  const healthTimeoutMs = resolveHealthTimeoutMs(HEALTH_TIMEOUT_MS_RELEASE);
  console.log("[kandev] starting backend...");
  await waitForHealth(ports.backendUrl, backend.proc, healthTimeoutMs);
  console.log(`[kandev] backend ready at ${ports.backendUrl}`);

  console.log("[kandev] open: " + ports.backendUrl);
  if (headless) {
    console.log(`[kandev] ready (headless) at ${ports.backendUrl}`);
    return;
  }
  openBrowser(ports.backendUrl);
}
