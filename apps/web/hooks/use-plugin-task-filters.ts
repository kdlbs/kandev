import { useCallback, useMemo, useSyncExternalStore } from "react";
import { pluginRegistry, pluginTaskFilterRegistrationKey } from "@/lib/plugins/registry";
import {
  pluginTaskFilterStore,
  resetPluginTaskFilterSelectionsForTests,
  type PluginTaskFilterSelections,
} from "@/lib/plugins/task-filter-selections";
import type { PluginTaskFilterContext } from "@/lib/plugins/types";

// The store singleton and its selection type live in lib/plugins/ so
// host-api.ts (host.taskFilters) can import them without a
// hooks/ -> lib/plugins/ -> hooks/ cycle; re-exported here so this hook's
// public surface is unchanged for existing callers.
export type { PluginTaskFilterSelections };
export { resetPluginTaskFilterSelectionsForTests };

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
  // `hidden` filters (a plugin driving its own dedicated UI, e.g. a top-bar
  // dropdown, instead of the built-in display/filter section) stay in
  // `filters` for gating purposes — taskMatchesPluginFilters below must keep
  // evaluating every registered filter — but drop out of what the built-in
  // KanbanDisplayDropdown renders.
  const visibleFilters = useMemo(() => filters.filter((filter) => !filter.hidden), [filters]);

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

  return { filters, visibleFilters, selections, setFilterSelection, taskMatchesPluginFilters };
}
