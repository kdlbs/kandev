export type AppConfig = {
  apiBaseUrl: string;
  mcpServerUrl: string;
};

const DEFAULT_API_BASE_URL = 'http://localhost:8080';
const DEFAULT_MCP_SERVER_URL = 'http://localhost:9090/sse';
export const DEBUG_UI = process.env.NEXT_PUBLIC_KANDEV_DEBUG === 'true';

export function getBackendConfig(): AppConfig {
  if (typeof window === 'undefined') {
    return {
      apiBaseUrl: process.env.KANDEV_API_BASE_URL ?? DEFAULT_API_BASE_URL,
      mcpServerUrl: process.env.KANDEV_MCP_SERVER_URL ?? DEFAULT_MCP_SERVER_URL,
    };
  }
  if (window.__KANDEV_API_BASE_URL || window.__KANDEV_MCP_SERVER_URL) {
    return {
      apiBaseUrl: window.__KANDEV_API_BASE_URL ?? DEFAULT_API_BASE_URL,
      mcpServerUrl: window.__KANDEV_MCP_SERVER_URL ?? DEFAULT_MCP_SERVER_URL,
    };
  }
  return {
    apiBaseUrl: process.env.NEXT_PUBLIC_KANDEV_API_BASE_URL ?? DEFAULT_API_BASE_URL,
    mcpServerUrl: process.env.NEXT_PUBLIC_KANDEV_MCP_SERVER_URL ?? DEFAULT_MCP_SERVER_URL,
  };
}
