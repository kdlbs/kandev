import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { ThemeDescriptor } from "@/lib/theme/theme-types";
import { listThemes, getThemeDefinition } from "@/lib/api/domains/themes-api";
import { t } from "@/lib/i18n";

type Requests = { owner: string; inventory: number; content: number };
const requests = new WeakMap<StoreApi<AppState>, Requests>();

export function themeOwner(store: StoreApi<AppState>): string {
  const auth = store.getState().auth;
  return `${auth.mode}:${auth.authenticated}:${auth.user?.id ?? ""}`;
}
function stateFor(store: StoreApi<AppState>): Requests {
  const owner = themeOwner(store);
  let state = requests.get(store);
  if (!state || state.owner !== owner) {
    if (state) store.getState().resetThemes();
    state = { owner, inventory: 0, content: 0 };
    requests.set(store, state);
  }
  return state;
}
export function invalidateThemeRequests(store: StoreApi<AppState>): void {
  const state = stateFor(store);
  state.inventory++;
  state.content++;
}
export async function refreshThemeInventory(store: StoreApi<AppState>): Promise<void> {
  const state = stateFor(store);
  if (!store.getState().auth.authenticated) return;
  const request = ++state.inventory;
  const current = () =>
    state.owner === themeOwner(store) &&
    state.inventory === request &&
    requests.get(store) === state;
  try {
    const inventory = await listThemes();
    if (current()) store.getState().setThemeInventory(inventory);
  } catch (error) {
    if (current())
      store
        .getState()
        .setThemeError(error instanceof Error ? error.message : t("plugins:failedToLoadPlugins"));
  }
}
export async function loadThemeContent(
  store: StoreApi<AppState>,
  descriptor: ThemeDescriptor,
): Promise<void> {
  const state = stateFor(store);
  const request = ++state.content;
  const inventory = store.getState().themeInventory;
  const current = () =>
    state.owner === themeOwner(store) &&
    state.content === request &&
    store.getState().themeInventory === inventory;
  if (store.getState().themeDefinitions[descriptor.digest]) return;
  try {
    const definition = await getThemeDefinition(descriptor);
    if (current()) store.getState().setThemeDefinition(descriptor.digest, definition);
  } catch (error) {
    if (current())
      store
        .getState()
        .setThemeError(error instanceof Error ? error.message : t("plugins:failedToLoadPlugins"));
  }
}
