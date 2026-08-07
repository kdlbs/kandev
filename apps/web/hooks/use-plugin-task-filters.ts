import { useCallback, useMemo, useSyncExternalStore } from "react";
import { pluginRegistry, pluginTaskFilterRegistrationKey } from "@/lib/plugins/registry";
import type { PluginTaskFilterContext } from "@/lib/plugins/types";

/** Selected option values, keyed by owning plugin plus `TaskFilterRegistration.id`. */
export type PluginTaskFilterSelections = Record<string, string[]>;

/**
 * Singleton, in-memory selection store for plugin-registered task filters
 * (`registerTaskFilter`) — shared across every `usePluginTaskFilters()` call
 * site in the app (the kanban display dropdown and the board's filtering
 * pipeline are separate components, so a plain `useState` local to one of
 * them would not be visible to the other). Never persisted to backend user
 * settings, unlike the Workflow/Repository filters.
 */
class PluginTaskFilterStore {
  private selections: PluginTaskFilterSelections = {};
  private listeners = new Set<() => void>();

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };

  getSelections = (): PluginTaskFilterSelections => this.selections;

  setFilterSelection(filterKey: string, values: string[]): void {
    if (values.length === 0) {
      if (!(filterKey in this.selections)) return;
      const next = { ...this.selections };
      delete next[filterKey];
      this.selections = next;
    } else {
      this.selections = { ...this.selections, [filterKey]: values };
    }
    this.notify();
  }

  private notify(): void {
    this.listeners.forEach((listener) => listener());
  }
}

const pluginTaskFilterStore = new PluginTaskFilterStore();

/** Test-only: clears all selection state so specs don't leak into each other. */
export function resetPluginTaskFilterSelectionsForTests(): void {
  for (const filterId of Object.keys(pluginTaskFilterStore.getSelections())) {
    pluginTaskFilterStore.setFilterSelection(filterId, []);
  }
}

/**
 * Reactive hook over the shared `pluginTaskFilterStore` plus the currently
 * registered `registerTaskFilter` filters. Re-renders whenever a plugin
 * registers/unregisters a filter (enable/disable/uninstall), or when any
 * component updates a selection via `setFilterSelection`.
 */
export function usePluginTaskFilters() {
  const registryVersion = useSyncExternalStore(
    pluginRegistry.subscribe,
    pluginRegistry.getVersion,
    pluginRegistry.getVersion,
  );
  const selections = useSyncExternalStore(
    pluginTaskFilterStore.subscribe,
    pluginTaskFilterStore.getSelections,
    pluginTaskFilterStore.getSelections,
  );

  const filters = useMemo(() => pluginRegistry.getTaskFilters(), [registryVersion]);

  const setFilterSelection = useCallback((filterKey: string, values: string[]) => {
    pluginTaskFilterStore.setFilterSelection(filterKey, values);
  }, []);

  /**
   * A task matches when every active plugin filter's `matches` returns true.
   * A filter with no selection (empty/absent array) is "All" — it does not
   * gate the task and its `matches` is not even called.
   */
  const taskMatchesPluginFilters = useCallback(
    (context: PluginTaskFilterContext): boolean => {
      return filters.every((filter) => {
        const selected = selections[pluginTaskFilterRegistrationKey(filter)] ?? [];
        if (selected.length === 0) return true;
        try {
          return filter.matches(context, selected);
        } catch (error: unknown) {
          console.error(
            `[plugins] task filter "${filter.pluginId}:${filter.id}" matches() threw`,
            error,
          );
          return false;
        }
      });
    },
    [filters, selections],
  );

  return { filters, selections, setFilterSelection, taskMatchesPluginFilters };
}
