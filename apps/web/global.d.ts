export {};

declare global {
  interface Window {
    // Port injection for dev mode (browser on web port, API on backend port)
    __KANDEV_API_PORT?: string;
    // Debug mode flag (injected by the Go shell or derived from boot payload runtime config)
    __KANDEV_DEBUG?: boolean;
  }
}
