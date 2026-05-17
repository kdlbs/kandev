/**
 * Shared utilities for CLI commands (dev, start, run).
 *
 * This module extracts common patterns used across different launch modes
 * to reduce duplication and ensure consistent behavior.
 */

import type { ChildProcess } from "node:child_process";
import os from "node:os";

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

    // Auto-allow the host's own LAN / VPN addresses so a dev hitting the dev
    // server from another device on the same network (Tailscale, LAN IP, WSL
    // mirrored mode, etc.) passes Next.js's allowedDevOrigins check and HMR
    // works. The user can still extend the list via NEXT_ALLOWED_DEV_ORIGINS.
    // Skip the assignment when there's nothing to add — keeps the env clean
    // for the loopback-only case.
    const merged = mergeAllowedDevOrigins(
      process.env.NEXT_ALLOWED_DEV_ORIGINS,
      listHostNetworkAddresses(),
    );
    if (merged) env.NEXT_ALLOWED_DEV_ORIGINS = merged;
  }

  if (debug) {
    env.NEXT_PUBLIC_KANDEV_DEBUG = "true";
  }

  return env;
}

/**
 * Returns the host's non-loopback, non-internal IPv4/IPv6 addresses. Used to
 * auto-populate Next.js `allowedDevOrigins` so the dev server accepts
 * connections from LAN / Tailscale / SSH-forwarded clients.
 */
export function listHostNetworkAddresses(): string[] {
  const v4: string[] = [];
  const v6: string[] = [];
  const seen = new Set<string>();
  const interfaces = os.networkInterfaces();
  for (const addrs of Object.values(interfaces)) {
    if (!addrs) continue;
    for (const addr of addrs) {
      if (addr.internal) continue;
      // Skip link-local IPv6 (fe80::/10) and link-local IPv4 (169.254.0.0/16,
      // RFC 3927) — neither is reachable from a remote machine, and the
      // 169.254 range in particular is what Hyper-V assigns to its phantom
      // WSL adapter, which clutters the startup output. The regex covers the
      // full /10 (fe80::–febf::); OS stacks only assign fe80::/64 in practice
      // but a stricter check is the same effort and removes the surprise.
      if (addr.family === "IPv6" && /^fe[89ab]/i.test(addr.address)) continue;
      if (addr.family === "IPv4" && addr.address.startsWith("169.254.")) continue;
      if (seen.has(addr.address)) continue;
      seen.add(addr.address);
      if (addr.family === "IPv4") v4.push(addr.address);
      else v6.push(addr.address);
    }
  }
  // IPv4 first — LAN + Tailscale IPv4 are what people usually want.
  return [...v4, ...v6];
}

function mergeAllowedDevOrigins(existing: string | undefined, extra: string[]): string {
  const set = new Set<string>();
  if (existing) {
    for (const s of existing.split(",")) {
      const trimmed = s.trim();
      if (trimmed) set.add(trimmed);
    }
  }
  for (const s of extra) set.add(s);
  return [...set].join(",");
}

export type StartupInfoOptions = {
  /** Mode header line, e.g. "dev mode: using local repo" or "release: v0.0.12 (github latest)" */
  header: string;
  ports: PortConfig;
  /**
   * Which port a user actually opens in a browser. In `start`/`run` the Go
   * backend reverse-proxies Next.js so the entry point is the backend port;
   * in `dev` the browser hits Next.js directly. Network URLs are only listed
   * under this port — the other one is internal-only. Defaults to "backend".
   */
  primary?: "backend" | "web";
  /** Database file path */
  dbPath?: string;
  /** Log level being used */
  logLevel?: string;
};

/**
 * Logs a unified startup info block to the console.
 *
 * Shows only the URL the user actually opens — start/run modes have the Go
 * backend reverse-proxy Next.js on a single port, dev mode hits Next.js
 * directly. The other port and the agentctl port are internal plumbing and
 * would only mislead. Below the URL, lists the same port on each non-loopback
 * interface (LAN, Tailscale) so a user opening the app remotely sees the
 * right address.
 */
export function logStartupInfo(options: StartupInfoOptions): void {
  const { header, ports, primary = "backend", dbPath, logLevel } = options;
  const primaryPort = primary === "web" ? ports.webPort : ports.backendPort;
  const primaryUrl = `http://localhost:${primaryPort}`;
  const networkHosts = listHostNetworkAddresses();

  console.log(`[kandev] ${header}`);
  console.log("[kandev] url:", primaryUrl);
  for (const url of networkUrlsForPort(primaryPort, networkHosts)) {
    console.log("[kandev]   network:", url);
  }
  console.log("[kandev] mcp:", `${ports.backendUrl}/mcp`);
  if (dbPath) {
    console.log("[kandev] db:", dbPath);
  }
  if (logLevel) {
    console.log("[kandev] log level:", logLevel);
  }
}

/**
 * Builds `http://<host>:<port>` URLs from a list of host addresses, wrapping
 * IPv6 addresses in brackets per RFC 3986.
 */
export function networkUrlsForPort(port: number, hosts: string[]): string[] {
  return hosts.map((host) => {
    const formatted = host.includes(":") ? `[${host}]` : host;
    return `http://${formatted}:${port}`;
  });
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
