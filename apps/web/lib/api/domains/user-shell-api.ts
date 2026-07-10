import { getWebSocketClient } from "@/lib/ws/connection";
import { getBackendConfig } from "@/lib/config";

const WS_UNAVAILABLE = "WebSocket client not available";

/**
 * Terminal info returned by the HTTP/WS list endpoint.
 *
 * Backend uses a discriminated union (kind: ordinary | fixed | script).
 * Ordinary terminals carry seq + custom_name + state; fixed (bottom-panel)
 * and script terminals omit those.
 *
 * Legacy keys (process_id, running, label, closable) remain in the type as
 * optional so older agentctl responses still decode. New UI reads kind +
 * seq + display_name + state + pty_status.
 */
export type TerminalInfo = {
  terminal_id?: string; // legacy SSR shape (env-keyed handler)
  id?: string; // new task-keyed handler
  kind?: "ordinary" | "fixed" | "script";
  seq?: number;
  display_name?: string;
  custom_name?: string | null;
  state?: "open" | "parked";
  pty_status?: "running" | "stopped";

  // Legacy fields kept for env-keyed SSR fallback.
  process_id?: string;
  running?: boolean;
  label?: string;
  closable?: boolean;
  initial_command?: string;
};

/**
 * Fetch terminals for a task. Preferred path is the new task-keyed endpoint
 * (`/tasks/:taskId/terminals`) which returns the full union, including the
 * ordinary terminals' DB metadata. Falls back to the legacy env-keyed
 * endpoint if no taskId is supplied (during transition).
 */
export async function fetchTerminals(
  taskID: string,
  environmentId?: string,
): Promise<TerminalInfo[]> {
  const { apiBaseUrl } = getBackendConfig();
  if (!taskID) {
    if (!environmentId) return [];
    return fetchTerminalsByEnv(environmentId);
  }
  const params = environmentId ? `?task_environment_id=${encodeURIComponent(environmentId)}` : "";
  const url = `${apiBaseUrl}/api/v1/tasks/${encodeURIComponent(taskID)}/terminals${params}`;
  try {
    const response = await fetch(url);
    if (!response.ok) {
      console.warn("Failed to fetch terminals (task-keyed):", response.status);
      // Fall back to legacy path so older backends still work mid-rollout.
      return environmentId ? fetchTerminalsByEnv(environmentId) : [];
    }
    const data = await response.json();
    return data.terminals ?? [];
  } catch (error) {
    console.warn("Failed to fetch terminals (task-keyed):", error);
    return environmentId ? fetchTerminalsByEnv(environmentId) : [];
  }
}

async function fetchTerminalsByEnv(environmentId: string): Promise<TerminalInfo[]> {
  const { apiBaseUrl } = getBackendConfig();
  const url = `${apiBaseUrl}/api/v1/environments/${environmentId}/terminals`;
  try {
    const response = await fetch(url);
    if (!response.ok) {
      console.warn("Failed to fetch terminals (env-keyed):", response.status);
      return [];
    }
    const data = await response.json();
    return data.terminals ?? [];
  } catch (error) {
    console.warn("Failed to fetch terminals (env-keyed):", error);
    return [];
  }
}

/**
 * Result of creating a new user shell. For ordinary terminals the backend
 * also returns seq + display_name + state; legacy / script paths return the
 * older label + closable fields.
 */
export type CreateUserShellResult = {
  terminalId: string;
  kind?: "ordinary" | "fixed" | "script";
  seq?: number;
  displayName?: string;
  state?: "open" | "parked";
  ptyStatus?: "running" | "stopped";
  label?: string;
  closable?: boolean;
  initialCommand?: string;
};

/**
 * Options for {@link createUserShell}.
 * - `taskId` is required for ordinary terminals (DB-backed) — without it
 *   the create falls back to the legacy non-persistent path.
 * - `scriptId` runs a stored RepositoryScript.
 * - `command` (with optional `label`) runs an arbitrary command — script
 *   terminal.
 */
export type CreateUserShellOptions = {
  taskId?: string;
  scriptId?: string;
  command?: string;
  label?: string;
};

export async function createUserShell(
  environmentId: string,
  options?: CreateUserShellOptions,
): Promise<CreateUserShellResult> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_UNAVAILABLE);
  }

  const payload: Record<string, string> = { task_environment_id: environmentId };
  if (options?.taskId) payload.task_id = options.taskId;
  if (options?.scriptId) payload.script_id = options.scriptId;
  if (options?.command) payload.command = options.command;
  if (options?.label) payload.label = options.label;

  const response = (await client.request("user_shell.create", payload)) as {
    terminal_id: string;
    kind?: "ordinary" | "fixed" | "script";
    seq?: number;
    display_name?: string;
    state?: "open" | "parked";
    pty_status?: "running" | "stopped";
    label?: string;
    closable?: boolean;
    initial_command?: string;
  };

  return {
    terminalId: response.terminal_id,
    kind: response.kind,
    seq: response.seq,
    displayName: response.display_name,
    state: response.state,
    ptyStatus: response.pty_status,
    label: response.label,
    closable: response.closable,
    initialCommand: response.initial_command,
  };
}

/**
 * Destroy (hard-kill PTY + delete row) an ordinary terminal. Also serves
 * the legacy `user_shell.stop` action for non-ordinary terminals — backend
 * accepts either name.
 *
 * Rejects (rather than silently resolving) on a missing WS client so the
 * caller's `.then()` doesn't fire and remove the tab from the strip
 * without a frame ever being sent. Mirrors rename/park/resume's contract.
 *
 * `taskId` is sent so the backend can verify ownership and reject
 * cross-task destroys. Optional for non-managed ids (bottom-panel,
 * script-*) where the service guard short-circuits.
 *
 * Falls back to `user_shell.stop` if the server rejects `user_shell.destroy`
 * as an unknown action — preserves compatibility during the rollout
 * window where older backends only register the legacy action name.
 */
export async function destroyUserShell(
  environmentId: string,
  terminalId: string,
  taskId?: string,
): Promise<void> {
  const client = getWebSocketClient();
  if (!client) throw new Error(WS_UNAVAILABLE);
  const payload: Record<string, string> = {
    task_environment_id: environmentId,
    terminal_id: terminalId,
  };
  if (taskId) payload.task_id = taskId;
  try {
    await client.request("user_shell.destroy", payload);
  } catch (error) {
    if (isUnknownActionError(error)) {
      await client.request("user_shell.stop", payload);
      return;
    }
    throw error;
  }
}

/**
 * Heuristic check for "server doesn't know this action" responses. The
 * WS dispatcher returns an error whose message contains "unknown action"
 * or "no handler" depending on backend version; both forms are matched
 * by case-insensitive substring search.
 */
function isUnknownActionError(error: unknown): boolean {
  const msg = error instanceof Error ? error.message : String(error);
  const lower = msg.toLowerCase();
  return lower.includes("unknown action") || lower.includes("no handler");
}

/** Legacy name retained for the bottom-panel + script paths. */
export const stopUserShell = destroyUserShell;

/**
 * Rename an ordinary terminal. Pass `null` to clear the custom name and
 * revert to the derived "Terminal {seq}" label.
 *
 * `taskId` lets the backend verify the terminal belongs to the supplied
 * task; a mismatch rejects the rename instead of silently mutating
 * another task's row.
 */
export async function renameUserShell(
  terminalId: string,
  customName: string | null,
  taskId?: string,
): Promise<void> {
  const client = getWebSocketClient();
  if (!client) throw new Error(WS_UNAVAILABLE);
  const payload: Record<string, unknown> = {
    terminal_id: terminalId,
    custom_name: customName,
  };
  if (taskId) payload.task_id = taskId;
  await client.request("user_shell.rename", payload);
}

/**
 * Park an ordinary terminal — hides the tab from the panel strip but
 * leaves the PTY running so the user can resume later. `taskId` enforces
 * ownership server-side.
 */
export async function parkUserShell(terminalId: string, taskId?: string): Promise<void> {
  const client = getWebSocketClient();
  if (!client) throw new Error(WS_UNAVAILABLE);
  const payload: Record<string, string> = { terminal_id: terminalId };
  if (taskId) payload.task_id = taskId;
  await client.request("user_shell.park", payload);
}

export async function resumeUserShell(terminalId: string, taskId?: string): Promise<void> {
  const client = getWebSocketClient();
  if (!client) throw new Error(WS_UNAVAILABLE);
  const payload: Record<string, string> = { terminal_id: terminalId };
  if (taskId) payload.task_id = taskId;
  await client.request("user_shell.resume", payload);
}
