/**
 * Shared utilities for CLI commands (dev, start, run).
 *
 * This module extracts common patterns used across different launch modes
 * to reduce duplication and ensure consistent behavior.
 */

import type { ChildProcess } from "node:child_process";

import { DEFAULT_AGENTCTL_PORT, DEFAULT_BACKEND_PORT, DEFAULT_WEB_PORT } from "./constants";
import { pickAvailablePort } from "./ports";
import { createProcessSupervisor } from "./process";

export type PortConfig = {
  backendPort: number;
  webPort: number;
  agentctlPort: number;
  backendUrl: string;
};

/**
 * Picks available ports for all services, using provided values or finding free ports.
 *
 * @param backendPort - Optional preferred backend port
 * @param webPort - Optional preferred web port
 * @returns Resolved ports for all services
 */
export async function pickPorts(backendPort?: number, webPort?: number): Promise<PortConfig> {
  const resolvedBackendPort = backendPort ?? (await pickAvailablePort(DEFAULT_BACKEND_PORT));
  const resolvedWebPort = webPort ?? (await pickAvailablePort(DEFAULT_WEB_PORT));
  const agentctlPort = await pickAvailablePort(DEFAULT_AGENTCTL_PORT);

  return {
    backendPort: resolvedBackendPort,
    webPort: resolvedWebPort,
    agentctlPort,
    backendUrl: `http://localhost:${resolvedBackendPort}`,
  };
}

export type BackendEnvOptions = {
  ports: PortConfig;
  /** Log level: debug, info, warn, error (default: info) */
  logLevel?: string;
  /** Additional environment variables to merge */
  extra?: Record<string, string>;
};

/**
 * Builds environment variables for the backend process.
 *
 * @param options - Configuration options for the backend environment
 * @returns Environment object for the backend process
 */
export function buildBackendEnv(options: BackendEnvOptions): NodeJS.ProcessEnv {
  const { ports, logLevel, extra } = options;
  return {
    ...process.env,
    KANDEV_SERVER_PORT: String(ports.backendPort),
    KANDEV_WEB_INTERNAL_URL: `http://localhost:${ports.webPort}`,
    KANDEV_AGENT_STANDALONE_PORT: String(ports.agentctlPort),
    ...(logLevel ? { KANDEV_LOG_LEVEL: logLevel } : {}),
    ...extra,
  };
}

export type WebEnvOptions = {
  ports: PortConfig;
  /** Set NODE_ENV to production */
  production?: boolean;
  /** Enable debug mode (NEXT_PUBLIC_KANDEV_DEBUG) */
  debug?: boolean;
};

/**
 * Builds environment variables for the web process.
 *
 * @param options - Configuration options for the web environment
 * @returns Environment object for the web process
 */
export function buildWebEnv(options: WebEnvOptions): NodeJS.ProcessEnv {
  const { ports, production = false, debug = false } = options;

  const env: NodeJS.ProcessEnv = {
    ...process.env,
    // Server-side: full localhost URL for SSR fetches (Next.js SSR → Go backend)
    KANDEV_API_BASE_URL: ports.backendUrl,
    PORT: String(ports.webPort),
    // Ensure Next.js standalone server binds to 127.0.0.1 so localhost health checks work.
    // Without this, HOSTNAME from the host environment can cause binding issues.
    HOSTNAME: "127.0.0.1",
  };

  if (production) {
    (env as Record<string, string>).NODE_ENV = "production";
    // Explicitly unset so a host-level NEXT_PUBLIC_KANDEV_API_PORT (from a .env
    // file, Docker env, or CI variable) cannot leak through the process.env
    // spread above and reintroduce the cross-origin URL problem.
    delete env.NEXT_PUBLIC_KANDEV_API_PORT;
  } else {
    // Dev mode only: browser hits the web port directly, so the client needs to
    // know the backend port for API calls. In production the Go backend
    // reverse-proxies Next.js on a single port, so the client uses same-origin
    // and this var must NOT be set — otherwise the client builds cross-origin
    // URLs like `https://host:38429/...` that aren't reachable behind a
    // reverse proxy / ingress / Cloudflare tunnel.
    env.NEXT_PUBLIC_KANDEV_API_PORT = String(ports.backendPort);
  }

  if (debug) {
    env.NEXT_PUBLIC_KANDEV_DEBUG = "true";
  }

  return env;
}

export type StartupInfoOptions = {
  /** Mode header line, e.g. "dev mode: using local repo" or "release: v0.0.12 (github latest)" */
  header: string;
  ports: PortConfig;
  /** Database file path */
  dbPath?: string;
  /** Log level being used */
  logLevel?: string;
};

/**
 * Logs a unified startup info block to the console.
 */
export function logStartupInfo(options: StartupInfoOptions): void {
  const { header, ports, dbPath, logLevel } = options;
  const backendUrl = ports.backendUrl;
  const webUrl = `http://localhost:${ports.webPort}`;
  console.log(`[kandev] ${header}`);
  console.log("[kandev] backend:", backendUrl);
  console.log("[kandev] web:", webUrl);
  console.log("[kandev] agentctl port:", ports.agentctlPort);
  console.log("[kandev] mcp url:", `${backendUrl}/mcp`);
  if (dbPath) {
    console.log("[kandev] db path:", dbPath);
  }
  if (logLevel) {
    console.log("[kandev] log level:", logLevel);
  }
}

/**
 * Attaches a standardized exit handler to a backend process.
 *
 * When the backend exits, this handler logs the exit reason and triggers
 * a graceful shutdown of all supervised processes. If the process was
 * killed by a signal, it exits with code 0; otherwise it uses the
 * process exit code (defaulting to 1).
 *
 * @param backendProc - The backend child process
 * @param supervisor - The process supervisor managing child processes
 */
export function attachBackendExitHandler(
  backendProc: ChildProcess,
  supervisor: ReturnType<typeof createProcessSupervisor>,
): void {
  backendProc.on("exit", (code, signal) => {
    console.error(`[kandev] backend exited (code=${code}, signal=${signal})`);
    const exitCode = signal ? 0 : (code ?? 1);
    void supervisor.shutdown("backend exit").then(() => process.exit(exitCode));
  });
}
