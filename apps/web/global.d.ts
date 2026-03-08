export {};

declare global {
  interface Window {
    // Port injection for dev mode (browser on web port, API on backend port)
    __KANDEV_API_PORT?: string;
    // Debug mode flag (injected at runtime by layout.tsx)
    __KANDEV_DEBUG?: boolean;
  }
}
