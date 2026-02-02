/**
 * Shared utilities for CLI commands (dev, start, run).
 *
 * This module extracts common patterns used across different launch modes
 * to reduce duplication and ensure consistent behavior.
 */

import type { ChildProcess } from "node:child_process";

import {
  DEFAULT_AGENTCTL_PORT,
  DEFAULT_BACKEND_PORT,
  DEFAULT_MCP_PORT,
  DEFAULT_WEB_PORT,
} from "./constants";
import { pickAvailablePort } from "./ports";
import { createProcessSupervisor } from "./process";

export type PortConfig = {
  backendPort: number;
  webPort: number;
  agentctlPort: number;
  mcpPort: number;
  backendUrl: string;
  mcpUrl: string;
};

/**
 * Picks available ports for all services, using provided values or finding free ports.
 *
 * @param backendPort - Optional preferred backend port
 * @param webPort - Optional preferred web port
 * @returns Resolved ports for all services
 */
export async function pickPorts(
  backendPort?: number,
  webPort?: number,
): Promise<PortConfig> {
  const resolvedBackendPort = backendPort ?? (await pickAvailablePort(DEFAULT_BACKEND_PORT));
  const resolvedWebPort = webPort ?? (await pickAvailablePort(DEFAULT_WEB_PORT));
  const agentctlPort = await pickAvailablePort(DEFAULT_AGENTCTL_PORT);
  const mcpPort = await pickAvailablePort(DEFAULT_MCP_PORT);

  return {
    backendPort: resolvedBackendPort,
    webPort: resolvedWebPort,
    agentctlPort,
    mcpPort,
    backendUrl: `http://localhost:${resolvedBackendPort}`,
    mcpUrl: `http://localhost:${mcpPort}/sse`,
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
    KANDEV_AGENT_STANDALONE_PORT: String(ports.agentctlPort),
    KANDEV_AGENT_MCP_SERVER_PORT: String(ports.mcpPort),
    ...(logLevel ? { KANDEV_LOG_LEVEL: logLevel } : {}),
    ...extra,
  };
}

export type WebEnvOptions = {
  ports: PortConfig;
  /** Include MCP URL environment variables (used in dev/start modes) */
  includeMcp?: boolean;
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
  const { ports, includeMcp = false, production = false, debug = false } = options;

  const env: NodeJS.ProcessEnv = {
    ...process.env,
    KANDEV_API_BASE_URL: ports.backendUrl,
    NEXT_PUBLIC_KANDEV_API_BASE_URL: ports.backendUrl,
    PORT: String(ports.webPort),
  };

  if (includeMcp) {
    env.KANDEV_MCP_SERVER_URL = ports.mcpUrl;
    env.NEXT_PUBLIC_KANDEV_MCP_SERVER_URL = ports.mcpUrl;
  }

  if (production) {
    (env as Record<string, string>).NODE_ENV = "production";
  }

  if (debug) {
    env.NEXT_PUBLIC_KANDEV_DEBUG = "true";
  }

  return env;
}

/**
 * Logs port configuration to the console.
 *
 * @param mode - The launch mode name (e.g., "dev", "production", "release")
 * @param modeDescription - Human-readable description of the mode
 * @param ports - Port configuration to log
 * @param includeMcp - Whether to log MCP-related ports
 */
export function logPortConfig(
  mode: string,
  modeDescription: string,
  ports: PortConfig,
  includeMcp = false,
): void {
  console.log(`[kandev] ${mode} mode: ${modeDescription}`);
  console.log("[kandev] backend port:", ports.backendPort);
  console.log("[kandev] web port:", ports.webPort);
  console.log("[kandev] agentctl port:", ports.agentctlPort);
  if (includeMcp) {
    console.log("[kandev] mcp port:", ports.mcpPort);
    console.log("[kandev] mcp url:", ports.mcpUrl);
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
    const exitCode = signal ? 0 : code ?? 1;
    void supervisor.shutdown("backend exit").then(() => process.exit(exitCode));
  });
}
