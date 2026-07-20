export {};

declare global {
  interface Window {
    // Port injection for dev mode (browser on web port, API on backend port)
    __KANDEV_API_PORT?: string;
    // Debug mode flag (injected by the Go shell or derived from boot payload runtime config)
    __KANDEV_DEBUG?: boolean;
    // E2E-only state exposure flag set by Playwright init scripts.
    __KANDEV_E2E_EXPOSE_STORE__?: boolean;
  }
}
