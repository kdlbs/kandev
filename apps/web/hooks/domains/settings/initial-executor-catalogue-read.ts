import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { listExecutors } from "@/lib/api/domains/settings-api";

export async function loadInitialExecutors(store: StoreApi<AppState>) {
  const state = store.getState();
  if (state.settingsData.executorsLoaded) return;
  if (state.executors.items.length > 0) {
    state.setSettingsData({ executorsLoaded: true });
    return;
  }

  let changed = false;
  const unsubscribe = store.subscribe((current) => {
    if (current.executors.items.length > 0) changed = true;
  });
  try {
    const response = await listExecutors({ cache: "no-store" });
    if (!changed && store.getState().executors.items.length === 0) {
      store.getState().setExecutors(response.executors);
    }
  } catch {
    // Failed initial reads retain the current catalogue.
  } finally {
    unsubscribe();
    store.getState().setSettingsData({ executorsLoaded: true });
  }
}
