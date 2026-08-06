import { getBackendConfig } from "@/lib/config";
import { fetchJson, type ApiRequestOptions } from "../client";

export type AgentLoginSession = {
  session_id: string;
  agent_id: string;
  cmd: string[];
  running: boolean;
  started_at: string;
  finished_at?: string;
  exit_code?: number;
};

export type HostShellStartOptions = ApiRequestOptions & {
  /** Stable browser-local identity used to isolate Quick Chat terminal tabs. */
  clientId?: string;
};

export async function startAgentLogin(
  agentName: string,
  size: { cols: number; rows: number },
  options?: ApiRequestOptions,
): Promise<AgentLoginSession> {
  return fetchJson<AgentLoginSession>(`/api/v1/agent-login/agents/${agentName}/start`, {
    ...options,
    init: {
      method: "POST",
      body: JSON.stringify(size),
      ...(options?.init ?? {}),
    },
  });
}

export async function stopAgentLogin(sessionID: string): Promise<void> {
  await fetchJson<{ ok: boolean }>(`/api/v1/agent-login/sessions/${sessionID}/stop`, {
    init: { method: "POST" },
  });
}

export async function resizeAgentLogin(
  sessionID: string,
  size: { cols: number; rows: number },
): Promise<void> {
  await fetchJson<{ ok: boolean }>(`/api/v1/agent-login/sessions/${sessionID}/resize`, {
    init: { method: "POST", body: JSON.stringify(size) },
  });
}

/**
 * Build the bi-directional WS URL for streaming a login session.
 * Derives the host from the backend config (NOT window.location) so dev mode
 * — where the browser is on :37429 and the API is on :38429 — routes to the
 * Go backend, not the web dev server.
 */
export function agentLoginStreamUrl(sessionID: string): string {
  const { apiBaseUrl } = getBackendConfig();
  const url = new URL(apiBaseUrl);
  const proto = url.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${url.host}/api/v1/agent-login/sessions/${sessionID}/stream`;
}

/**
 * Start a plain host shell PTY (spawns $SHELL, or bash/sh fallback). Reuses
 * the same session manager as agent-login, so stop/resize/stream all use the
 * same session-ID-based endpoints.
 */
export async function startHostShell(
  size: { cols: number; rows: number },
  options?: HostShellStartOptions,
): Promise<AgentLoginSession> {
  const { clientId, ...requestOptions } = options ?? {};
  return fetchJson<AgentLoginSession>("/api/v1/host-shell/start", {
    ...requestOptions,
    init: {
      method: "POST",
      body: JSON.stringify({ ...size, ...(clientId ? { client_id: clientId } : {}) }),
      ...(requestOptions.init ?? {}),
    },
  });
}

export async function getAgentLoginStatus(sessionID: string): Promise<AgentLoginSession> {
  return fetchJson<AgentLoginSession>(`/api/v1/agent-login/sessions/${sessionID}/status`);
}
