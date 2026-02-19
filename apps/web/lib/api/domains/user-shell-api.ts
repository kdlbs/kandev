import { getWebSocketClient } from "@/lib/ws/connection";
import { getBackendConfig } from "@/lib/config";

/**
 * Terminal info returned by HTTP API (for SSR).
 */
export type TerminalInfo = {
  terminal_id: string;
  process_id: string;
  running: boolean;
  label: string;
  closable: boolean;
  initial_command?: string;
};

/**
 * Fetch terminals for a session via HTTP (for SSR).
 * This endpoint auto-creates the first "Terminal" if none exist.
 */
export async function fetchTerminals(sessionId: string): Promise<TerminalInfo[]> {
  const { apiBaseUrl } = getBackendConfig();
  const url = `${apiBaseUrl}/api/v1/sessions/${sessionId}/terminals`;
  try {
    const response = await fetch(url);
    if (!response.ok) {
      console.warn("Failed to fetch terminals:", response.status);
      return [];
    }
    const data = await response.json();
    return data.terminals ?? [];
  } catch (error) {
    console.warn("Failed to fetch terminals:", error);
    return [];
  }
}

/**
 * Result of creating a new user shell.
 */
export type CreateUserShellResult = {
  terminalId: string;
  label: string;
  closable: boolean;
  initialCommand?: string;
};

/**
 * Create a new user shell terminal.
 * Backend assigns the terminal ID, label, and closable status.
 * First terminal is "Terminal" and not closable, subsequent are "Terminal 2", etc. and closable.
 * If scriptId is provided, creates a script terminal with the script's name and command.
 */
export async function createUserShell(
  sessionId: string,
  scriptId?: string,
): Promise<CreateUserShellResult> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error("WebSocket client not available");
  }

  const payload: Record<string, string> = { session_id: sessionId };
  if (scriptId) {
    payload.script_id = scriptId;
  }

  const response = (await client.request("user_shell.create", payload)) as {
    terminal_id: string;
    label: string;
    closable: boolean;
    initial_command?: string;
  };

  return {
    terminalId: response.terminal_id,
    label: response.label,
    closable: response.closable,
    initialCommand: response.initial_command,
  };
}

/**
 * Stop a user shell terminal process.
 * This is called when closing a terminal tab to clean up the backend process.
 */
export async function stopUserShell(sessionId: string, terminalId: string): Promise<void> {
  const client = getWebSocketClient();
  if (!client) {
    // WebSocket not available, silently fail (best-effort cleanup)
    return;
  }

  try {
    await client.request("user_shell.stop", {
      session_id: sessionId,
      terminal_id: terminalId,
    });
  } catch (error) {
    // Log but don't throw - this is best-effort cleanup
    console.warn("Failed to stop user shell:", error);
  }
}
