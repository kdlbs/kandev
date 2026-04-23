export type AppConfig = {
  apiBaseUrl: string;
};

// Server-side default (used during SSR - always localhost since SSR runs on same machine)
const DEFAULT_API_BASE_URL = "http://localhost:38429";

export const DEBUG_UI =
  process.env.NEXT_PUBLIC_KANDEV_DEBUG === "true" ||
  (typeof window !== "undefined" && window.__KANDEV_DEBUG === true);

export function getBackendConfig(): AppConfig {
  // Server-side: use env vars or localhost defaults (SSR runs on same machine as backend)
  if (typeof window === "undefined") {
    return {
      apiBaseUrl: process.env.KANDEV_API_BASE_URL ?? DEFAULT_API_BASE_URL,
    };
  }

  // Client-side URL resolution:
  // 1. Port-based URL via __KANDEV_API_PORT (dev mode: browser on :37429, API on :38429)
  // 2. Same-origin (production: Go reverse-proxies Next.js on single port)
  //    Works for any hosting scenario: localhost, custom domain, Tailscale, etc.
  if (window.__KANDEV_API_PORT) {
    const port = parseInt(window.__KANDEV_API_PORT, 10);
    if (Number.isInteger(port) && port > 0 && port <= 65535) {
      const protocol = window.location.protocol;
      return { apiBaseUrl: `${protocol}//${window.location.hostname}:${port}` };
    }
  }

  return { apiBaseUrl: window.location.origin };
}
