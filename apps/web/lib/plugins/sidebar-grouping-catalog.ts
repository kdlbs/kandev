import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { listSidebarGroupingProviders } from "@/lib/api/domains/plugins-api";
import { pluginRegistry } from "./registry";
import { sidebarTaskPageScope } from "@/lib/sidebar/sidebar-task-page-cache";

let catalogGeneration = 0;
let catalogOwner: StoreApi<AppState> | undefined;

export async function refreshSidebarGroupingCatalog(store: StoreApi<AppState>): Promise<void> {
  const generation = ++catalogGeneration;
  catalogOwner = store;
  const scope = sidebarTaskPageScope(store.getState());
  const providers = await listSidebarGroupingProviders().catch(() => []);
  if (
    catalogOwner !== store ||
    generation !== catalogGeneration ||
    scope !== sidebarTaskPageScope(store.getState())
  )
    return;
  pluginRegistry.setSidebarGroupings(providers);
}
