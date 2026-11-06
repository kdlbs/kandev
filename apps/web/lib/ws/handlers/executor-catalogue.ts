import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { listExecutors } from "@/lib/api/domains/settings-api";
import { executorCatalogueEpoch, invalidateExecutorCatalogue } from "@/lib/api/executor-catalogue";

export function refreshExecutorCatalogue(store: StoreApi<AppState>) {
  const generation = invalidateExecutorCatalogue();
  const userId = store.getState().auth.user?.id;
  store.setState((state) => ({
    executors: { items: [] },
    settingsData: { ...state.settingsData, executorsLoaded: false },
  }));
  void listExecutors({ cache: "no-store" })
    .then((response) => {
      if (generation !== executorCatalogueEpoch() || userId !== store.getState().auth.user?.id)
        return;
      store.getState().setExecutors(response.executors);
      store.getState().setSettingsData({ executorsLoaded: true });
    })
    .catch(() => {
      // A failed or obsolete read cannot restore revoked configuration.
    });
}
