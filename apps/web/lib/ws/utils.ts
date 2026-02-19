/**
 * Convert HTTP URL to WebSocket URL
 * http://localhost:8080 → ws://localhost:8080/ws
 * https://api.example.com → wss://api.example.com/ws
 */
export function httpToWebSocketUrl(baseUrl: string): string {
  try {
    const url = new URL(baseUrl);
    const protocol = url.protocol === "https:" ? "wss:" : "ws:";
    return `${protocol}//${url.host}/ws`;
  } catch {
    throw new Error(`Invalid URL format: ${baseUrl}`);
  }
}

/**
 * Validate backend URL format
 */
export function isValidBackendUrl(url: string): boolean {
  try {
    const parsed = new URL(url);
    return parsed.protocol === "http:" || parsed.protocol === "https:";
  } catch {
    return false;
  }
}
