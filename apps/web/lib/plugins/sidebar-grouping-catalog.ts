import type { StoreApi } from "zustand";
import type { SidebarGroupingProvider } from "@kdlbs/plugin-sdk";
import type { AppState } from "@/lib/state/store";
import { listSidebarGroupingProviders } from "@/lib/api/domains/plugins-api";
import { ApiError } from "@/lib/api/client";
import { isTemporaryReadError, readRetryAfterMs } from "@/lib/api/temporary-read-error";
import { pluginRegistry } from "./registry";
import { sidebarTaskPageScope } from "@/lib/sidebar/sidebar-task-page-cache";

let catalogGeneration = 0;
let catalogOwner: StoreApi<AppState> | undefined;
let catalogScope: string | undefined;
const CATALOG_RETRY_DELAYS_MS = [1_000, 2_000] as const;
const CATALOG_RECOVERY_MS = 5_000;

export function seedSidebarGroupingCatalog(
  store: StoreApi<AppState>,
  providers: readonly SidebarGroupingProvider[],
): void {
  catalogGeneration++;
  if (catalogOwner !== store || catalogScope !== sidebarTaskPageScope(store.getState()))
    pluginRegistry.invalidateSidebarGroupings();
  catalogOwner = store;
  catalogScope = sidebarTaskPageScope(store.getState());
  pluginRegistry.setSidebarGroupings(providers);
}

export function reconcileSidebarGroupingCatalogScope(store: StoreApi<AppState>): string {
  const scope = sidebarTaskPageScope(store.getState());
  if (catalogOwner !== store || catalogScope !== scope) seedSidebarGroupingCatalog(store, []);
  return scope;
}

function isCurrentCatalog(store: StoreApi<AppState>, generation: number, scope: string): boolean {
  return (
    catalogOwner === store &&
    generation === catalogGeneration &&
    scope === sidebarTaskPageScope(store.getState())
  );
}

export async function refreshSidebarGroupingCatalog(store: StoreApi<AppState>): Promise<void> {
  const scope = reconcileSidebarGroupingCatalogScope(store);
  const generation = ++catalogGeneration;
  const started = Date.now();
  for (let attempt = 0; attempt <= CATALOG_RETRY_DELAYS_MS.length; attempt++) {
    if (!isCurrentCatalog(store, generation, scope)) return;
    try {
      const providers = await listSidebarGroupingProviders();
      if (isCurrentCatalog(store, generation, scope)) pluginRegistry.setSidebarGroupings(providers);
      return;
    } catch (error) {
      if (!isCurrentCatalog(store, generation, scope)) return;
      if (error instanceof ApiError && (error.status === 401 || error.status === 403)) {
        pluginRegistry.invalidateSidebarGroupings();
        pluginRegistry.setSidebarGroupings([]);
        return;
      }
      const delay = CATALOG_RETRY_DELAYS_MS[attempt];
      if (delay === undefined || !isTemporaryReadError(error)) return;
      const retryDelay = Math.max(delay, readRetryAfterMs(error));
      if (Date.now() - started + retryDelay > CATALOG_RECOVERY_MS) return;
      await new Promise((resolve) => setTimeout(resolve, retryDelay));
    }
  }
}
