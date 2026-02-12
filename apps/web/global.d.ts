export {};

declare global {
  interface Window {
    // Full URL overrides (for custom domains/proxies)
    __KANDEV_API_BASE_URL?: string;
    __KANDEV_MCP_SERVER_URL?: string;
    // Port-only injection (client builds URL from window.location.hostname + port)
    __KANDEV_API_PORT?: string;
    __KANDEV_MCP_PORT?: string;
    // Debug mode flag (injected at runtime by layout.tsx)
    __KANDEV_DEBUG?: boolean;
  }
}
