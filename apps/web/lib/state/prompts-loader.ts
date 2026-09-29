import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { listPrompts } from "@/lib/api";

type RequestState = { revision: number; pending?: Promise<void> };
const requests = new WeakMap<StoreApi<AppState>, RequestState>();

function requestState(store: StoreApi<AppState>) {
  let state = requests.get(store);
  if (!state) {
    state = { revision: 0 };
    requests.set(store, state);
  }
  return state;
}

export function invalidatePrompts(store: StoreApi<AppState>) {
  requestState(store).revision++;
  store.setState((state) => ({ prompts: { ...state.prompts, loaded: false } }));
}

export function refreshPrompts(store: StoreApi<AppState>) {
  invalidatePrompts(store);
  return loadPrompts(store);
}

export function loadPrompts(store: StoreApi<AppState>): Promise<void> {
  const state = requestState(store);
  if (state.pending) return state.pending;
  if (store.getState().prompts.loaded) return Promise.resolve();
  // Deferring until the request is registered also deduplicates reentrant consumers.
  const pending = Promise.resolve()
    .then(async () => {
      for (;;) {
        const revision = state.revision;
        try {
          const response = await listPrompts({ cache: "no-store" });
          if (revision !== state.revision) continue;
          store.getState().setPrompts(response.prompts ?? []);
        } catch {
          if (revision !== state.revision) continue;
          store.getState().setPrompts(store.getState().prompts.items);
        }
        break;
      }
    })
    .finally(() => {
      state.pending = undefined;
      store.getState().setPromptsLoading(false);
    });
  state.pending = pending;
  store.getState().setPromptsLoading(true);
  return pending;
}
