export type AppConfig = {
  apiBaseUrl: string;
  mcpServerUrl: string;
};

// Default ports
const DEFAULT_API_PORT = 8080;
const DEFAULT_MCP_PORT = 9090;

// Server-side defaults (used during SSR - always localhost since SSR runs on same machine)
const DEFAULT_API_BASE_URL = `http://localhost:${DEFAULT_API_PORT}`;
const DEFAULT_MCP_SERVER_URL = `http://localhost:${DEFAULT_MCP_PORT}/sse`;

export const DEBUG_UI =
  process.env.NEXT_PUBLIC_KANDEV_DEBUG === "true" ||
  (typeof window !== "undefined" && window.__KANDEV_DEBUG === true);

/**
 * Build URL from current page hostname and port.
 * This allows accessing the app from any device (iPhone, Tailscale, etc.)
 * without manual configuration.
 */
function buildClientUrl(port: number, path = ""): string {
  const protocol = window.location.protocol === "https:" ? "https:" : "http:";
  return `${protocol}//${window.location.hostname}:${port}${path}`;
}

export function getBackendConfig(): AppConfig {
  // Server-side: use env vars or localhost defaults (SSR runs on same machine as backend)
  if (typeof window === "undefined") {
    return {
      apiBaseUrl: process.env.KANDEV_API_BASE_URL ?? DEFAULT_API_BASE_URL,
      mcpServerUrl: process.env.KANDEV_MCP_SERVER_URL ?? DEFAULT_MCP_SERVER_URL,
    };
  }

  // Client-side: Build URLs from current hostname + port
  // Priority:
  // 1. Explicit full URL override via window.__KANDEV_API_BASE_URL (for custom domains/proxies)
  // 2. Port from window.__KANDEV_API_PORT (injected by layout.tsx from server env)
  // 3. Port from NEXT_PUBLIC env var (build-time)
  // 4. Default port
  // All port-based options build URL from window.location.hostname

  if (window.__KANDEV_API_BASE_URL) {
    // Full URL override - use as-is (for custom domains/proxies)
    return {
      apiBaseUrl: window.__KANDEV_API_BASE_URL,
      mcpServerUrl: window.__KANDEV_MCP_SERVER_URL || buildClientUrl(DEFAULT_MCP_PORT, "/sse"),
    };
  }

  // Build URLs dynamically from current hostname + port
  const apiPort =
    parseInt(window.__KANDEV_API_PORT || "", 10) ||
    parseInt(process.env.NEXT_PUBLIC_KANDEV_API_PORT || "", 10) ||
    DEFAULT_API_PORT;

  const mcpPort =
    parseInt(window.__KANDEV_MCP_PORT || "", 10) ||
    parseInt(process.env.NEXT_PUBLIC_KANDEV_MCP_PORT || "", 10) ||
    DEFAULT_MCP_PORT;

  return {
    apiBaseUrl: buildClientUrl(apiPort),
    mcpServerUrl: buildClientUrl(mcpPort, "/sse"),
  };
}
