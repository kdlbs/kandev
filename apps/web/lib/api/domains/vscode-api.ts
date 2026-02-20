import { getWebSocketClient } from "@/lib/ws/connection";

/**
 * VS Code server status response.
 */
export type VscodeStatus = {
  status: "stopped" | "installing" | "starting" | "running" | "error";
  port?: number;
  url?: string;
  error?: string;
  message?: string;
};

/**
 * Start VS Code server for a session.
 * Returns the status including the proxy URL.
 * Start is non-blocking — the caller should poll getVscodeStatus() until running/error.
 */
export async function startVscode(
  sessionId: string,
  theme?: string,
): Promise<VscodeStatus> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error("WebSocket client not available");
  }

  const response = (await client.request(
    "vscode.start",
    {
      session_id: sessionId,
      theme: theme ?? "dark",
    },
    15_000,
  )) as VscodeStatus;

  return response;
}

/**
 * Stop VS Code server for a session.
 */
export async function stopVscode(sessionId: string): Promise<void> {
  const client = getWebSocketClient();
  if (!client) {
    return;
  }

  try {
    await client.request("vscode.stop", {
      session_id: sessionId,
    });
  } catch (error) {
    console.warn("Failed to stop VS Code:", error);
  }
}

/**
 * Open a file in the running VS Code instance via the backend Remote CLI.
 * This is a fire-and-forget call — errors are logged but not thrown.
 */
export async function openFileInVscode(
  sessionId: string,
  path: string,
  line?: number,
  col?: number,
): Promise<void> {
  const client = getWebSocketClient();
  if (!client) return;

  try {
    await client.request("vscode.openFile", {
      session_id: sessionId,
      path,
      line: line ?? 0,
      col: col ?? 0,
    });
  } catch (error) {
    console.warn("Failed to open file in VS Code:", error);
  }
}

/**
 * Get VS Code server status for a session.
 */
export async function getVscodeStatus(
  sessionId: string,
): Promise<VscodeStatus> {
  const client = getWebSocketClient();
  if (!client) {
    return { status: "stopped" };
  }

  try {
    const response = (await client.request("vscode.status", {
      session_id: sessionId,
    })) as VscodeStatus;
    return response;
  } catch {
    return { status: "stopped" };
  }
}
