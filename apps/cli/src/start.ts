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
import {
  buildBackendEnv,
  createOutputRingBuffer,
  logStartupInfo,
  pickBackendPorts,
  resolveBackendLogLevels,
} from "./shared";
import { launchRestartableBackend } from "./supervisor/backend";
import { openBrowser } from "./web";

export type StartOptions = {
  /** Path to the repository root directory */
  repoRoot: string;
  /** Optional preferred backend port (finds available port if not specified) */
  backendPort?: number;
  /** Show backend info logs on stdout */
  verbose?: boolean;
  /** Retain backend debug logs in its file + agent message dumps */
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
  verbose = false,
  debug = false,
  headless = false,
}: StartOptions): Promise<void> {
  const ports = await pickBackendPorts(backendPort);

  const backendBin = path.join(repoRoot, "apps", "backend", "bin", getBinaryName("kandev"));
  if (!fs.existsSync(backendBin)) {
    throw new Error("Backend binary not found. Run `make build` first.");
  }

  // File logs retain info by default. Verbose only changes the stdout sink;
  // debug lowers the file threshold while stdout remains warn+.
  fs.mkdirSync(resolveDataDir(), { recursive: true });
  // The data dir holds the SQLite DB; keep it owner-only even if it pre-existed
  // with a looser umask-derived mode.
  fs.chmodSync(resolveDataDir(), 0o700);
  const dbPath = resolveDatabasePath();
  const levels = resolveBackendLogLevels({ verbose, debug });
  const logLevel = levels.file;
  const backendEnv = buildBackendEnv({
    ports,
    logLevel,
    consoleLogLevel: levels.console,
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

  const output = createOutputRingBuffer(process.stdout);
  const backend = await launchRestartableBackend({
    command: backendBin,
    args: [],
    cwd: path.dirname(backendBin),
    env: backendEnv,
    homeDir: resolveKandevHomeDir(),
    ports,
    mode: "start",
    stdio: ["ignore", "pipe", "inherit"],
    supervisor,
    onProcess: (proc) => output.attach(proc.stdout),
  });

  const healthTimeoutMs = resolveHealthTimeoutMs(HEALTH_TIMEOUT_MS_RELEASE);
  console.log("[kandev] starting backend...");
  await waitForHealth(ports.backendUrl, backend.proc, healthTimeoutMs, () => {
    const buffered = output.read();
    if (buffered.trim().length === 0) return;
    console.error("[kandev] --- backend stdout (last captured output) ---");
    console.error(buffered.trimEnd());
    console.error("[kandev] --- end backend stdout ---");
  });
  console.log(`[kandev] backend ready at ${ports.backendUrl}`);

  console.log("[kandev] open: " + ports.backendUrl);
  if (headless) {
    console.log(`[kandev] ready (headless) at ${ports.backendUrl}`);
    return;
  }
  openBrowser(ports.backendUrl);
}
