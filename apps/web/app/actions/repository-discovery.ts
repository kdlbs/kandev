"use server";

import { fetchJson } from "@/lib/api/client";
import { getBackendConfig } from "@/lib/config";
import type { DesktopDiscoveryRoot, RepositoryDiscoveryResponse } from "@/lib/types/http";

const { apiBaseUrl } = getBackendConfig();

export type RepositoryDiscoveryRefreshTrigger = "manual_refresh" | "stale_refresh";

export async function getRepositoryDiscoveryAction(
  workspaceId: string,
  root?: string,
): Promise<RepositoryDiscoveryResponse> {
  const params = root ? `?root=${encodeURIComponent(root)}` : "";
  return fetchJson<RepositoryDiscoveryResponse>(
    `${apiBaseUrl}/api/v1/workspaces/${workspaceId}/repositories/discovery${params}`,
  );
}

export async function refreshRepositoryDiscoveryAction(
  workspaceId: string,
  root?: string,
  trigger: RepositoryDiscoveryRefreshTrigger = "manual_refresh",
): Promise<RepositoryDiscoveryResponse> {
  const params = new URLSearchParams();
  if (root) params.set("root", root);
  params.set("trigger", trigger);
  return fetchJson<RepositoryDiscoveryResponse>(
    `${apiBaseUrl}/api/v1/workspaces/${workspaceId}/repositories/discovery/refresh?${params.toString()}`,
    { init: { method: "POST" } },
  );
}

export async function listDesktopDiscoveryRootsAction(): Promise<{
  roots: DesktopDiscoveryRoot[];
}> {
  return fetchJson<{ roots: DesktopDiscoveryRoot[] }>(
    `${apiBaseUrl}/api/v1/repositories/discovery/roots`,
  );
}

export async function addDesktopDiscoveryRootAction(path: string): Promise<DesktopDiscoveryRoot> {
  return fetchJson<DesktopDiscoveryRoot>(`${apiBaseUrl}/api/v1/repositories/discovery/roots`, {
    init: { method: "POST", body: JSON.stringify({ path }) },
  });
}

export async function reconnectDesktopDiscoveryRootAction(
  oldPath: string,
  newPath: string,
): Promise<DesktopDiscoveryRoot> {
  return fetchJson<DesktopDiscoveryRoot>(
    `${apiBaseUrl}/api/v1/repositories/discovery/roots/reconnect`,
    { init: { method: "POST", body: JSON.stringify({ path: oldPath, new_path: newPath }) } },
  );
}

export async function removeDesktopDiscoveryRootAction(path: string): Promise<void> {
  await fetchJson<void>(
    `${apiBaseUrl}/api/v1/repositories/discovery/roots?path=${encodeURIComponent(path)}`,
    { init: { method: "DELETE" } },
  );
}
