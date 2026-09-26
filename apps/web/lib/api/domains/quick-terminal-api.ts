import { fetchJson, type ApiRequestOptions } from "../client";
import type { QuickTerminalStatus, QuickTerminalTab } from "@/lib/state/slices/ui/types";

export type QuickTerminalTabResponse = {
  tabId: string;
  workspaceId: string;
  sessionId: string | null;
  sequence: number;
  status: QuickTerminalStatus;
  exitCode?: number;
  error?: string;
};

export type ListQuickTerminalTabsResponse = {
  tabs: QuickTerminalTabResponse[];
};

export type QuickTerminalLifecycleUpdate = {
  sessionId?: string | null;
  status: QuickTerminalStatus;
  exitCode?: number | null;
  error?: string | null;
};

export async function listQuickTerminalTabs(
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<ListQuickTerminalTabsResponse> {
  const params = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<ListQuickTerminalTabsResponse>(
    `/api/v1/quick-terminal-tabs?${params.toString()}`,
    options,
  );
}

export async function createQuickTerminalTab(
  tabId: string,
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<QuickTerminalTabResponse> {
  return fetchJson<QuickTerminalTabResponse>("/api/v1/quick-terminal-tabs", {
    ...options,
    init: {
      method: "POST",
      body: JSON.stringify({ tab_id: tabId, workspace_id: workspaceId }),
      ...(options?.init ?? {}),
    },
  });
}

export async function updateQuickTerminalTab(
  tabId: string,
  update: QuickTerminalLifecycleUpdate,
  options?: ApiRequestOptions,
): Promise<QuickTerminalTabResponse> {
  return fetchJson<QuickTerminalTabResponse>(`/api/v1/quick-terminal-tabs/${tabId}`, {
    ...options,
    init: {
      method: "PATCH",
      body: JSON.stringify({
        session_id: update.sessionId ?? null,
        status: update.status,
        exit_code: update.exitCode ?? null,
        error: update.error ?? "",
      }),
      ...(options?.init ?? {}),
    },
  });
}

export async function deleteQuickTerminalTab(
  tabId: string,
  options?: ApiRequestOptions,
): Promise<void> {
  await fetchJson<{ ok: boolean }>(`/api/v1/quick-terminal-tabs/${tabId}`, {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}

export function toQuickTerminalTab(tab: QuickTerminalTabResponse): QuickTerminalTab {
  return {
    tabId: tab.tabId,
    workspaceId: tab.workspaceId,
    sessionId: tab.sessionId,
    sequence: tab.sequence,
    status: tab.status,
    ...(tab.exitCode === undefined ? {} : { exitCode: tab.exitCode }),
    ...(tab.error ? { error: tab.error } : {}),
  };
}
