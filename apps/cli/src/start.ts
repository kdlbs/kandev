/**
 * Production start command for running local builds.
 *
 * This module implements the `kandev start` command, which runs the locally
 * built backend binary and web app in production mode. Unlike `kandev dev`
 * which uses hot-reloading, this runs the optimized production builds.
 *
 * Prerequisites:
 * - Backend must be built: `make build-backend`
 * - Web app must be built: `make build-web`
 * - Or simply: `make build` (builds both)
 */

import { spawn } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

import { DATA_DIR, HEALTH_TIMEOUT_MS_RELEASE } from "./constants";
import { resolveHealthTimeoutMs, waitForHealth } from "./health";
import { getBinaryName } from "./platform";
import { createProcessSupervisor } from "./process";
import {
  attachBackendExitHandler,
  buildBackendEnv,
  buildWebEnv,
  pickPorts,
} from "./shared";
import { launchWebApp } from "./web";

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
};

/**
 * Runs the application in production mode using local builds.
 *
 * This function:
 * 1. Validates that build artifacts exist
 * 2. Picks available ports for all services
 * 3. Starts the backend binary (with warn log level for clean output)
 * 4. Starts the web app via `pnpm start`
 * 5. Waits for the backend to be healthy before announcing readiness
 *
 * @param options - Configuration for the start command
 * @throws Error if backend binary or web build is not found
 */
export async function runStart({
  repoRoot,
  backendPort,
  webPort,
  verbose = false,
  debug = false,
}: StartOptions): Promise<void> {
  const ports = await pickPorts(backendPort, webPort);

  const backendBin = path.join(repoRoot, "apps", "backend", "bin", getBinaryName("kandev"));
  if (!fs.existsSync(backendBin)) {
    throw new Error("Backend binary not found. Run `make build` first.");
  }

  // Check for standalone build (Next.js standalone output)
  const webServerPath = path.join(repoRoot, "apps", "web", ".next", "standalone", "web", "server.js");
  if (!fs.existsSync(webServerPath)) {
    throw new Error("Web standalone build not found. Run `make build` first.");
  }
  const webStandaloneDir = path.dirname(webServerPath);
  const webStaticDir = path.join(repoRoot, "apps", "web", ".next", "static");
  const standaloneStaticDir = path.join(webStandaloneDir, ".next", "static");
  if (fs.existsSync(webStaticDir) && !fs.existsSync(standaloneStaticDir)) {
    fs.mkdirSync(path.dirname(standaloneStaticDir), { recursive: true });
    try {
      fs.symlinkSync(webStaticDir, standaloneStaticDir, "junction");
    } catch (err) {
      console.warn(
        `[kandev] failed to link Next.js static assets: ${err instanceof Error ? err.message : String(err)}`,
      );
    }
  }

  // Link public directory (fonts, images, etc.) into standalone output
  const webPublicDir = path.join(repoRoot, "apps", "web", "public");
  const standalonePublicDir = path.join(webStandaloneDir, "public");
  if (fs.existsSync(webPublicDir) && !fs.existsSync(standalonePublicDir)) {
    try {
      fs.symlinkSync(webPublicDir, standalonePublicDir, "junction");
    } catch (err) {
      console.warn(
        `[kandev] failed to link public assets: ${err instanceof Error ? err.message : String(err)}`,
      );
    }
  }

  // Production mode: use warn log level for clean output unless verbose/debug
  const showOutput = verbose || debug;
  const dbPath = path.join(DATA_DIR, "kandev.db");
  const backendEnv = buildBackendEnv({
    ports,
    logLevel: debug ? "debug" : verbose ? "info" : "warn",
    extra: {
      KANDEV_DB_PATH: dbPath,
      ...(debug ? { KANDEV_DEBUG_AGENT_MESSAGES: "true" } : {}),
    },
  });
  const webEnv = buildWebEnv({ ports, includeMcp: true, production: true, debug });

  const supervisor = createProcessSupervisor();
  supervisor.attachSignalHandlers();

  // Start backend with piped stdio (quiet mode unless verbose/debug)
  const backendProc = spawn(backendBin, [], {
    cwd: path.dirname(backendBin),
    env: backendEnv,
    stdio: showOutput ? ["ignore", "inherit", "inherit"] : ["ignore", "pipe", "pipe"],
  });
  supervisor.children.push(backendProc);

  // Forward stderr only (warnings/errors) when quiet
  if (!showOutput) {
    backendProc.stderr?.pipe(process.stderr);
  }

  attachBackendExitHandler(backendProc, supervisor);

  // Use standalone server.js directly (not pnpm start)
  const webUrl = `http://localhost:${ports.webPort}`;
  launchWebApp({
    command: "node",
    args: [webServerPath],
    cwd: webStandaloneDir,
    env: webEnv,
    url: webUrl,
    supervisor,
    label: "web",
    quiet: !showOutput,
  });

  const healthTimeoutMs = resolveHealthTimeoutMs(HEALTH_TIMEOUT_MS_RELEASE);
  await waitForHealth(ports.backendUrl, backendProc, healthTimeoutMs);

  // Print clean summary
  console.log("");
  console.log("[kandev] Server started successfully");
  console.log("");
  console.log(`  Web:      ${webUrl}`);
  console.log(`  API:      ${ports.backendUrl}`);
  console.log(`  MCP:      ${ports.mcpUrl}`);
  console.log(`  Database: ${dbPath}`);
  console.log("");
}
