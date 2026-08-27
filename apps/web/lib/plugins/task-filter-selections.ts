/** Selected option values, keyed by owning plugin plus `TaskFilterRegistration.id`. */
export type PluginTaskFilterSelections = Record<string, string[]>;

/**
 * Singleton, in-memory selection store for plugin-registered task filters
 * (`registerTaskFilter`) — shared across every `usePluginTaskFilters()` call
 * site in the app (the kanban display dropdown and the board's filtering
 * pipeline are separate components, so a plain `useState` local to one of
 * them would not be visible to the other) and `host.taskFilters`, which lets
 * a plugin's own UI (e.g. a top-bar filter dropdown) drive the same
 * selection. Never persisted to backend user settings, unlike the
 * Workflow/Repository filters.
 *
 * Extracted from `hooks/use-plugin-task-filters.ts` so `host-api.ts` can
 * import it directly without a `hooks/ -> lib/plugins/ -> hooks/` cycle; the
 * hook re-exports this module's surface, so its own public API is unchanged.
 */
export class PluginTaskFilterStore {
  private selections: PluginTaskFilterSelections = {};
  private listeners = new Set<() => void>();

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };

  getSelections = (): PluginTaskFilterSelections => this.selections;

  /** The current selection for one filter key (`pluginId:id`), or `[]` if unset. */
  getSelection(filterKey: string): string[] {
    return this.selections[filterKey] ?? [];
  }

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

export const pluginTaskFilterStore = new PluginTaskFilterStore();

/** Test-only: clears all selection state so specs don't leak into each other. */
export function resetPluginTaskFilterSelectionsForTests(): void {
  for (const filterId of Object.keys(pluginTaskFilterStore.getSelections())) {
    pluginTaskFilterStore.setFilterSelection(filterId, []);
  }
}
