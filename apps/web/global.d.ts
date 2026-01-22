export {};

declare global {
  interface Window {
    __KANDEV_API_BASE_URL?: string;
    __KANDEV_MCP_SERVER_URL?: string;
  }
}
